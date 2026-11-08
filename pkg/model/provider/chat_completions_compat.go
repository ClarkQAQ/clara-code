package provider

import (
	yuzu "clara-code/pkg/model"
	openai "clara-code/pkg/model/internal/wire"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
)

const (
	reasoningStartedCtx = "reasoning_started"
	reasoningEndedCtx   = "reasoning_ended"
)

// buildTextBlock creates a text content block, applying any provider-specific extra
// fields (e.g. Qwen's cache_control) onto it. Part-level fields override message-level
// fields. Returns the block and whether any extra fields were applied; when they were,
// the message content must be serialized in array form.
func buildTextBlock(text string, partOpts, msgOpts yuzu.ProviderOptions) (*openai.ChatCompletionContentPartTextParam, bool) {
	block := &openai.ChatCompletionContentPartTextParam{Text: text}
	fields := partOpts.ExtraFields
	if len(fields) == 0 {
		fields = msgOpts.ExtraFields
	}
	if len(fields) > 0 {
		block.SetExtraFields(fields)
		return block, true
	}
	return block, false
}

// DefaultExtraContentFunc adds reasoning content reported through the
// non-standard "reasoning_content" or "reasoning" fields that some
// OpenAI-compatible endpoints return.
func DefaultExtraContentFunc(choice openai.ChatCompletionChoice) []yuzu.Content {
	var content []yuzu.Content
	reasoningData := ReasoningData{}
	err := json.Unmarshal([]byte(choice.Message.RawJSON()), &reasoningData)
	if err != nil {
		return content
	}
	if hasReasoningField(choice.Message.RawJSON()) {
		content = append(content, yuzu.ReasoningContent{
			Text: reasoningData.GetReasoningContent(),
		})
	}
	return content
}

func ctxBool(ctx map[string]any, key string) bool {
	v, ok := ctx[key]
	if !ok {
		return false
	}
	b, ok := v.(bool)
	if !ok {
		return false
	}
	return b
}

// DefaultStreamExtraFunc streams reasoning reported through the non-standard
// "reasoning_content" or "reasoning" fields that some OpenAI-compatible
// endpoints return.
func DefaultStreamExtraFunc(chunk openai.ChatCompletionChunk, yield func(yuzu.StreamPart) bool, ctx map[string]any) (map[string]any, bool) {
	if len(chunk.Choices) == 0 {
		return ctx, true
	}

	for _, choice := range chunk.Choices {
		// Reasoning state is tracked per choice: the openai language model
		// invokes this hook once per chunk, a chunk may carry several
		// choices, and providers may emit them in any slice order — key on
		// the choice's own Index, not its position in the chunk.
		inx := choice.Index
		startedKey := fmt.Sprintf("%s:%d", reasoningStartedCtx, inx)
		endedKey := fmt.Sprintf("%s:%d", reasoningEndedCtx, inx)
		reasoningStarted := ctxBool(ctx, startedKey)
		reasoningEnded := ctxBool(ctx, endedKey)

		reasoningData := ReasoningData{}
		err := json.Unmarshal([]byte(choice.Delta.RawJSON()), &reasoningData)
		if err != nil {
			yield(yuzu.StreamPart{
				Type:  yuzu.StreamPartTypeError,
				Error: &yuzu.Error{Title: "stream error", Message: "error unmarshalling delta", Cause: err},
			})
			return ctx, false
		}

		rc := reasoningData.GetReasoningContent()
		hasField := hasReasoningField(choice.Delta.RawJSON())
		boundary := choice.Delta.Content != "" || len(choice.Delta.ToolCalls) > 0 || choice.FinishReason != ""

		// A reasoning delta carries non-empty reasoning text, or a
		// present-but-empty field on a chunk that carries nothing else before
		// any block was closed (Kimi's "thinking on, nothing to think"
		// shape). A boundary chunk whose reasoning field is empty/null is
		// never reasoning.
		if rc != "" || (hasField && !boundary && !reasoningEnded) {
			if !reasoningStarted {
				reasoningStarted = true
				ctx[startedKey] = true
				if !yield(yuzu.StreamPart{
					Type: yuzu.StreamPartTypeReasoningStart,
					ID:   fmt.Sprintf("%d", inx),
				}) {
					return ctx, false
				}
			}
			// Skip empty deltas: a present-but-empty field opens the block so
			// it replays as reasoning_content: "", but there is nothing to
			// stream.
			if rc != "" {
				if !yield(yuzu.StreamPart{
					Type:  yuzu.StreamPartTypeReasoningDelta,
					ID:    fmt.Sprintf("%d", inx),
					Delta: rc,
				}) {
					return ctx, false
				}
			}
			// Fall through: a batching host may put the reasoning tail and the
			// first content/tool-call token in the same delta.
		}
		if reasoningStarted && boundary {
			ctx[startedKey] = false
			ctx[endedKey] = true
			// The openai main loop emits a chunk's text/tool-call parts before
			// this hook runs, so on a batched boundary chunk the part order is
			// ToolInputStart, ReasoningDelta(tail), ReasoningEnd. Parts are
			// keyed by id, so consumers can still attribute them correctly.
			if !yield(yuzu.StreamPart{
				Type: yuzu.StreamPartTypeReasoningEnd,
				ID:   fmt.Sprintf("%d", inx),
			}) {
				return ctx, false
			}
		}
	}
	return ctx, true
}

// DefaultToPrompt converts a prompt to OpenAI chat completions messages. It
// honors content-block extra fields and round-trips reasoning content through
// the non-standard reasoning_content field.
func DefaultToPrompt(prompt yuzu.Prompt, _, _ string) ([]openai.ChatCompletionMessageParamUnion, []yuzu.CallWarning) {
	var messages []openai.ChatCompletionMessageParamUnion
	var warnings []yuzu.CallWarning

	for _, msg := range prompt {
		switch msg.Role {
		case yuzu.MessageRoleSystem, yuzu.MessageRoleDeveloper:
			var blocks []openai.ChatCompletionContentPartTextParam
			useArrayForm := false

			for _, c := range msg.Content {
				if c.GetType() != yuzu.ContentTypeText {
					warnings = append(warnings, yuzu.CallWarning{
						Type:    yuzu.CallWarningTypeOther,
						Message: "system prompt can only have text content",
					})
					continue
				}
				textPart, ok := yuzu.AsContentType[yuzu.TextPart](c)
				if !ok {
					warnings = append(warnings, yuzu.CallWarning{
						Type:    yuzu.CallWarningTypeOther,
						Message: "system prompt text part does not have the right type",
					})
					continue
				}
				if strings.TrimSpace(textPart.Text) == "" {
					continue
				}

				block, hasExtra := buildTextBlock(textPart.Text, textPart.ProviderOptions, msg.ProviderOptions)
				useArrayForm = useArrayForm || hasExtra
				blocks = append(blocks, *block)
			}

			if len(blocks) == 0 {
				warnings = append(warnings, yuzu.CallWarning{
					Type:    yuzu.CallWarningTypeOther,
					Message: "system prompt has no text parts",
				})
				continue
			}

			if msg.Role == yuzu.MessageRoleDeveloper {
				if useArrayForm || len(blocks) > 1 {
					messages = append(messages, openai.DeveloperMessage(blocks))
				} else {
					texts := make([]string, len(blocks))
					for i, block := range blocks {
						texts[i] = block.Text
					}
					messages = append(messages, openai.DeveloperMessage(strings.Join(texts, "\n")))
				}
			} else if useArrayForm {
				messages = append(messages, openai.SystemMessage(blocks))
			} else {
				texts := make([]string, len(blocks))
				for i, b := range blocks {
					texts[i] = b.Text
				}
				messages = append(messages, openai.SystemMessage(strings.Join(texts, "\n")))
			}
		case yuzu.MessageRoleUser:
			// simple user message just text content. Messages carrying extra
			// content fields fall through to the array path below, which applies
			// them via buildTextBlock.
			if len(msg.Content) == 1 && msg.Content[0].GetType() == yuzu.ContentTypeText &&
				len(msg.Content[0].Options().ExtraFields) == 0 && len(msg.ProviderOptions.ExtraFields) == 0 {
				textPart, ok := yuzu.AsContentType[yuzu.TextPart](msg.Content[0])
				if !ok {
					warnings = append(warnings, yuzu.CallWarning{
						Type:    yuzu.CallWarningTypeOther,
						Message: "user message text part does not have the right type",
					})
					continue
				}
				messages = append(messages, openai.UserMessage(textPart.Text))
				continue
			}
			// text content and attachments
			var content []openai.ChatCompletionContentPartUnionParam
			for _, c := range msg.Content {
				switch c.GetType() {
				case yuzu.ContentTypeText:
					textPart, ok := yuzu.AsContentType[yuzu.TextPart](c)
					if !ok {
						warnings = append(warnings, yuzu.CallWarning{
							Type:    yuzu.CallWarningTypeOther,
							Message: "user message text part does not have the right type",
						})
						continue
					}
					textBlock, _ := buildTextBlock(textPart.Text, textPart.ProviderOptions, msg.ProviderOptions)
					content = append(content, openai.ChatCompletionContentPartUnionParam{
						OfText: textBlock,
					})
				case yuzu.ContentTypeFile:
					filePart, ok := yuzu.AsContentType[yuzu.FilePart](c)
					if !ok {
						warnings = append(warnings, yuzu.CallWarning{
							Type:    yuzu.CallWarningTypeOther,
							Message: "user message file part does not have the right type",
						})
						continue
					}

					switch {
					case strings.HasPrefix(filePart.MediaType, "text/"):
						base64Encoded := base64.StdEncoding.EncodeToString(filePart.Data)
						documentBlock := openai.ChatCompletionContentPartFileFileParam{
							FileData: openai.NewOpt(base64Encoded),
						}
						content = append(content, openai.FileContentPart(documentBlock))

					case strings.HasPrefix(filePart.MediaType, "image/"):
						// Handle image files
						base64Encoded := base64.StdEncoding.EncodeToString(filePart.Data)
						data := "data:" + filePart.MediaType + ";base64," + base64Encoded
						imageURL := openai.ChatCompletionContentPartImageImageURLParam{URL: data}
						imageURL.Detail = filePart.ProviderOptions.ImageDetail

						imageBlock := openai.ChatCompletionContentPartImageParam{ImageURL: imageURL}
						content = append(content, openai.ChatCompletionContentPartUnionParam{OfImageURL: &imageBlock})

					case filePart.MediaType == "audio/wav":
						// Handle WAV audio files
						base64Encoded := base64.StdEncoding.EncodeToString(filePart.Data)
						audioBlock := openai.ChatCompletionContentPartInputAudioParam{
							InputAudio: openai.ChatCompletionContentPartInputAudioInputAudioParam{
								Data:   base64Encoded,
								Format: "wav",
							},
						}
						content = append(content, openai.ChatCompletionContentPartUnionParam{OfInputAudio: &audioBlock})

					case filePart.MediaType == "audio/mpeg" || filePart.MediaType == "audio/mp3":
						// Handle MP3 audio files
						base64Encoded := base64.StdEncoding.EncodeToString(filePart.Data)
						audioBlock := openai.ChatCompletionContentPartInputAudioParam{
							InputAudio: openai.ChatCompletionContentPartInputAudioInputAudioParam{
								Data:   base64Encoded,
								Format: "mp3",
							},
						}
						content = append(content, openai.ChatCompletionContentPartUnionParam{OfInputAudio: &audioBlock})

					case filePart.MediaType == "application/pdf":
						// Handle PDF files
						dataStr := string(filePart.Data)

						// Check if data looks like a file ID (starts with "file-")
						if strings.HasPrefix(dataStr, "file-") {
							fileBlock := openai.ChatCompletionContentPartFileParam{
								File: openai.ChatCompletionContentPartFileFileParam{
									FileID: openai.NewOpt(dataStr),
								},
							}
							content = append(content, openai.ChatCompletionContentPartUnionParam{OfFile: &fileBlock})
						} else {
							// Handle as base64 data
							base64Encoded := base64.StdEncoding.EncodeToString(filePart.Data)
							data := "data:application/pdf;base64," + base64Encoded

							filename := filePart.Filename
							if filename == "" {
								// Generate default filename based on content index
								filename = fmt.Sprintf("part-%d.pdf", len(content))
							}

							fileBlock := openai.ChatCompletionContentPartFileParam{
								File: openai.ChatCompletionContentPartFileFileParam{
									Filename: openai.NewOpt(filename),
									FileData: openai.NewOpt(data),
								},
							}
							content = append(content, openai.ChatCompletionContentPartUnionParam{OfFile: &fileBlock})
						}

					default:
						warnings = append(warnings, yuzu.CallWarning{
							Type:    yuzu.CallWarningTypeOther,
							Message: fmt.Sprintf("file part media type %s not supported", filePart.MediaType),
						})
					}
				}
			}
			if !hasVisibleUserContent(content) {
				warnings = append(warnings, yuzu.CallWarning{
					Type:    yuzu.CallWarningTypeOther,
					Message: "dropping empty user message (contains neither user-facing content nor tool results)",
				})
				continue
			}
			messages = append(messages, openai.UserMessage(content))
		case yuzu.MessageRoleAssistant:
			// simple assistant message just text content
			if len(msg.Content) == 1 && msg.Content[0].GetType() == yuzu.ContentTypeText {
				textPart, ok := yuzu.AsContentType[yuzu.TextPart](msg.Content[0])
				if !ok {
					warnings = append(warnings, yuzu.CallWarning{
						Type:    yuzu.CallWarningTypeOther,
						Message: "assistant message text part does not have the right type",
					})
					continue
				}
				// Extra content fields (e.g. cache_control) require array form.
				textBlock, hasExtra := buildTextBlock(textPart.Text, textPart.ProviderOptions, msg.ProviderOptions)
				if hasExtra {
					messages = append(messages, openai.AssistantMessage([]openai.ChatCompletionAssistantMessageParamContentArrayOfContentPartUnion{
						{OfText: textBlock},
					}))
				} else {
					messages = append(messages, openai.AssistantMessage(textPart.Text))
				}
				continue
			}
			assistantMsg := openai.ChatCompletionAssistantMessageParam{
				Role: "assistant",
			}
			// A turn may carry several reasoning or text segments (a model that
			// reasons, speaks, then reasons again). Concatenate in order rather
			// than last-write-wins so no segment is silently dropped (F2,
			// CHARM-2020). Text segments are collected as ordered blocks; the
			// string form is used only for a single segment with no extra
			// fields, so mixed plain/extra turns keep their original order.
			var reasoningTexts []string
			reasoningPresent := false
			var textBlocks []openai.ChatCompletionAssistantMessageParamContentArrayOfContentPartUnion
			textHasExtra := false
			for _, c := range msg.Content {
				switch c.GetType() {
				case yuzu.ContentTypeText:
					textPart, ok := yuzu.AsContentType[yuzu.TextPart](c)
					if !ok {
						warnings = append(warnings, yuzu.CallWarning{
							Type:    yuzu.CallWarningTypeOther,
							Message: "assistant message text part does not have the right type",
						})
						continue
					}
					textBlock, hasExtra := buildTextBlock(textPart.Text, textPart.ProviderOptions, msg.ProviderOptions)
					textHasExtra = textHasExtra || hasExtra
					textBlocks = append(textBlocks, openai.ChatCompletionAssistantMessageParamContentArrayOfContentPartUnion{
						OfText: textBlock,
					})
				case yuzu.ContentTypeReasoning:
					reasoningPart, ok := yuzu.AsContentType[yuzu.ReasoningPart](c)
					if !ok {
						warnings = append(warnings, yuzu.CallWarning{
							Type:    yuzu.CallWarningTypeOther,
							Message: "assistant message reasoning part does not have the right type",
						})
						continue
					}
					reasoningTexts = append(reasoningTexts, reasoningPart.Text)
					reasoningPresent = true
				case yuzu.ContentTypeToolCall:
					toolCallPart, ok := yuzu.AsContentType[yuzu.ToolCallPart](c)
					if !ok {
						warnings = append(warnings, yuzu.CallWarning{
							Type:    yuzu.CallWarningTypeOther,
							Message: "assistant message tool part does not have the right type",
						})
						continue
					}
					assistantMsg.ToolCalls = append(assistantMsg.ToolCalls,
						openai.ChatCompletionMessageToolCallUnionParam{
							OfFunction: &openai.ChatCompletionMessageFunctionToolCallParam{
								ID:   toolCallPart.ToolCallID,
								Type: "function",
								Function: openai.ChatCompletionMessageFunctionToolCallFunctionParam{
									Name:      toolCallPart.ToolName,
									Arguments: toolCallPart.Input,
								},
							},
						})
				}
			}
			// Text: when no segment carries extra fields, join into the string
			// form; otherwise emit the ordered blocks one per segment so mixed
			// plain/extra turns keep their original order.
			if len(textBlocks) > 0 && !textHasExtra {
				texts := make([]string, len(textBlocks))
				for i, b := range textBlocks {
					texts[i] = b.OfText.Text
				}
				assistantMsg.Content = openai.ChatCompletionAssistantMessageParamContentUnion{
					OfString: openai.NewOpt(strings.Join(texts, "\n")),
				}
			} else if len(textBlocks) > 0 {
				assistantMsg.Content = openai.ChatCompletionAssistantMessageParamContentUnion{
					OfArrayOfContentParts: textBlocks,
				}
			}
			// Add reasoning_content field if the message carries a reasoning
			// part, even when its text is empty: presence must mirror what the
			// model emitted so resubmitted history stays byte-stable for prefix
			// caching. Providers like Kimi require the field on assistant
			// tool-call messages when thinking is enabled.
			if reasoningPresent {
				assistantMsg.SetExtraFields(map[string]any{
					"reasoning_content": strings.Join(reasoningTexts, "\n"),
				})
			}
			if !hasVisibleAssistantContent(&assistantMsg) {
				warnings = append(warnings, yuzu.CallWarning{
					Type:    yuzu.CallWarningTypeOther,
					Message: "dropping empty assistant message (contains neither user-facing content nor tool calls)",
				})
				continue
			}
			messages = append(messages, openai.ChatCompletionMessageParamUnion{
				OfAssistant: &assistantMsg,
			})
		case yuzu.MessageRoleTool:
			// Tool messages must stay contiguous right after the assistant turn
			// that requested them, so the media user messages of every result
			// are held back until the whole tool message has been emitted.
			var mediaMessages []openai.ChatCompletionMessageParamUnion
			for _, c := range msg.Content {
				if c.GetType() != yuzu.ContentTypeToolResult {
					warnings = append(warnings, yuzu.CallWarning{
						Type:    yuzu.CallWarningTypeOther,
						Message: "tool message can only have tool result content",
					})
					continue
				}

				toolResultPart, ok := yuzu.AsContentType[yuzu.ToolResultPart](c)
				if !ok {
					warnings = append(warnings, yuzu.CallWarning{
						Type:    yuzu.CallWarningTypeOther,
						Message: "tool message result part does not have the right type",
					})
					continue
				}

				switch toolResultPart.Output.GetType() {
				case yuzu.ToolResultContentTypeText:
					output, ok := yuzu.AsToolResultOutputType[yuzu.ToolResultOutputContentText](toolResultPart.Output)
					if !ok {
						warnings = append(warnings, yuzu.CallWarning{
							Type:    yuzu.CallWarningTypeOther,
							Message: "tool result output does not have the right type",
						})
						continue
					}
					messages = append(messages, openai.ToolMessage(output.Text, toolResultPart.ToolCallID))
				case yuzu.ToolResultContentTypeError:
					output, ok := yuzu.AsToolResultOutputType[yuzu.ToolResultOutputContentError](toolResultPart.Output)
					if !ok {
						warnings = append(warnings, yuzu.CallWarning{
							Type:    yuzu.CallWarningTypeOther,
							Message: "tool result output does not have the right type",
						})
						continue
					}
					messages = append(messages, openai.ToolMessage(output.Error.Error(), toolResultPart.ToolCallID))
				case yuzu.ToolResultContentTypeMedia:
					output, ok := yuzu.AsToolResultOutputType[yuzu.ToolResultOutputContentMedia](toolResultPart.Output)
					if !ok {
						warnings = append(warnings, yuzu.CallWarning{
							Type:    yuzu.CallWarningTypeOther,
							Message: "tool result output does not have the right type",
						})
						continue
					}
					// OpenAI-compatible chat completions tool messages cannot
					// carry image or audio content directly; the SDK's content
					// union only accepts text. Answer the call with a text tool
					// message and queue the media itself as a user message.
					messages = append(messages, toolResultMediaPlaceholderMessage(output, toolResultPart.ToolCallID))
					userMessage, mediaWarning, emit := toolResultMediaUserMessage(output)
					if mediaWarning != nil {
						warnings = append(warnings, *mediaWarning)
					}
					if emit {
						mediaMessages = append(mediaMessages, userMessage)
					}
				default:
					warnings = append(warnings, yuzu.CallWarning{
						Type:    yuzu.CallWarningTypeOther,
						Message: fmt.Sprintf("tool result output type %q not supported", toolResultPart.Output.GetType()),
					})
				}
			}
			messages = append(messages, mediaMessages...)
		}
	}
	return messages, warnings
}

func hasVisibleUserContent(content []openai.ChatCompletionContentPartUnionParam) bool {
	for _, part := range content {
		if part.OfText != nil || part.OfImageURL != nil || part.OfInputAudio != nil || part.OfFile != nil {
			return true
		}
	}
	return false
}

func hasVisibleAssistantContent(msg *openai.ChatCompletionAssistantMessageParam) bool {
	// Check if there's text content
	if !openai.IsOmitted(msg.Content.OfString) || len(msg.Content.OfArrayOfContentParts) > 0 {
		return true
	}
	// Check if there are tool calls
	if len(msg.ToolCalls) > 0 {
		return true
	}
	// A reasoning-only turn is visible: reasoning_content must round-trip
	// for DeepSeek-family replay, and dropping the turn breaks the
	// conversation's message ordering.
	if _, ok := msg.ExtraFields()["reasoning_content"]; ok {
		return true
	}
	return false
}
