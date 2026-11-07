package provider

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"net/http"
	"strings"
	"uuid"

	yuzu "clara-code/pkg/models"
	openai "clara-code/pkg/models/internal/wire"
	"clara-code/pkg/models/schema"
)

const topLogprobsMax = 20

type responsesLanguageModel struct {
	provider   string
	modelID    string
	client     openai.Client
	objectMode yuzu.ObjectMode
	// replayReasoning emits assistant reasoning items, which strict
	// thinking-mode providers (DeepSeek) require on tool continuations.
	replayReasoning bool
	// reasoningModel marks a model this deployment declared as one that takes a
	// reasoning setting, whatever its name looks like.
	reasoningModel bool
	headerFunc     LanguageModelHeaderFunc
}

// newResponsesLanguageModel implements a responses api model.
func newResponsesLanguageModel(modelID string, provider string, client openai.Client, objectMode yuzu.ObjectMode, replayReasoning bool, declaredReasoningModel bool, headerFunc LanguageModelHeaderFunc) responsesLanguageModel {
	return responsesLanguageModel{
		modelID:         modelID,
		provider:        provider,
		client:          client,
		objectMode:      objectMode,
		replayReasoning: replayReasoning,
		reasoningModel:  declaredReasoningModel,
		headerFunc:      headerFunc,
	}
}

// applyHeaders invokes the configured header func against a metadata sink and
// copies the resulting extra fields into the responses provider metadata. It
// is a no-op when no header func is configured or no response was captured.
func (o responsesLanguageModel) applyHeaders(header http.Header, metadata yuzu.ProviderMetadata) yuzu.ProviderMetadata {
	if o.headerFunc == nil || header == nil {
		return metadata
	}
	sink := yuzu.ProviderMetadata{}
	o.headerFunc(header, &sink)
	if len(sink.ExtraFields) == 0 {
		return metadata
	}
	if metadata.ExtraFields == nil {
		metadata.ExtraFields = make(map[string]json.RawMessage)
	}
	maps.Copy(metadata.ExtraFields, sink.ExtraFields)
	return metadata
}

func (o responsesLanguageModel) Model() string {
	return o.modelID
}

func (o responsesLanguageModel) Provider() string {
	return o.provider
}

type responsesModelConfig struct {
	isReasoningModel           bool
	systemMessageMode          string
	requiredAutoTruncation     bool
	supportsFlexProcessing     bool
	supportsPriorityProcessing bool
}

func getResponsesModelConfig(modelID string) responsesModelConfig {
	supportsFlexProcessing := strings.HasPrefix(modelID, "o3") ||
		strings.Contains(modelID, "-o3") || strings.Contains(modelID, "o4-mini") ||
		(strings.Contains(modelID, "gpt-5") && !strings.Contains(modelID, "gpt-5-chat"))

	supportsPriorityProcessing := strings.Contains(strings.ToLower(modelID), "gpt-4") ||
		strings.Contains(strings.ToLower(modelID), "gpt-5-mini") ||
		(strings.Contains(strings.ToLower(modelID), "gpt-5") &&
			!strings.Contains(strings.ToLower(modelID), "gpt-5-nano") &&
			!strings.Contains(strings.ToLower(modelID), "gpt-5-chat")) ||
		strings.HasPrefix(modelID, "o3") ||
		strings.Contains(modelID, "-o3") ||
		strings.Contains(modelID, "o4-mini")

	defaults := responsesModelConfig{
		requiredAutoTruncation:     false,
		systemMessageMode:          "system",
		supportsFlexProcessing:     supportsFlexProcessing,
		supportsPriorityProcessing: supportsPriorityProcessing,
	}

	if strings.Contains(strings.ToLower(modelID), "gpt-5-chat") {
		return responsesModelConfig{
			isReasoningModel:           false,
			systemMessageMode:          defaults.systemMessageMode,
			requiredAutoTruncation:     defaults.requiredAutoTruncation,
			supportsFlexProcessing:     defaults.supportsFlexProcessing,
			supportsPriorityProcessing: defaults.supportsPriorityProcessing,
		}
	}

	if strings.HasPrefix(modelID, "o1") || strings.Contains(modelID, "-o1") ||
		strings.HasPrefix(modelID, "o3") || strings.Contains(modelID, "-o3") ||
		strings.HasPrefix(modelID, "o4") || strings.Contains(modelID, "-o4") ||
		strings.HasPrefix(modelID, "oss") || strings.Contains(modelID, "-oss") ||
		strings.Contains(strings.ToLower(modelID), "gpt-5") ||
		strings.Contains(modelID, "codex-") || strings.Contains(modelID, "computer-use") {
		if strings.Contains(modelID, "o1-mini") || strings.Contains(modelID, "o1-preview") {
			return responsesModelConfig{
				isReasoningModel:           true,
				systemMessageMode:          "remove",
				requiredAutoTruncation:     defaults.requiredAutoTruncation,
				supportsFlexProcessing:     defaults.supportsFlexProcessing,
				supportsPriorityProcessing: defaults.supportsPriorityProcessing,
			}
		}

		return responsesModelConfig{
			isReasoningModel:           true,
			systemMessageMode:          "developer",
			requiredAutoTruncation:     defaults.requiredAutoTruncation,
			supportsFlexProcessing:     defaults.supportsFlexProcessing,
			supportsPriorityProcessing: defaults.supportsPriorityProcessing,
		}
	}

	return responsesModelConfig{
		isReasoningModel:           false,
		systemMessageMode:          defaults.systemMessageMode,
		requiredAutoTruncation:     defaults.requiredAutoTruncation,
		supportsFlexProcessing:     defaults.supportsFlexProcessing,
		supportsPriorityProcessing: defaults.supportsPriorityProcessing,
	}
}

const (
	previousResponseIDHistoryError = "cannot combine previous_response_id with replayed conversation history; use either previous_response_id (server-side chaining) or explicit message replay, not both"
	previousResponseIDStoreError   = "previous_response_id requires store to be true; the current response will not be stored and cannot be used for further chaining"
)

func (o responsesLanguageModel) prepareParams(call yuzu.Call) (*openai.ResponseNewParams, []yuzu.CallWarning, error) {
	var warnings []yuzu.CallWarning
	params := &openai.ResponseNewParams{}

	modelConfig := getResponsesModelConfig(o.modelID)

	if call.TopK != nil {
		warnings = append(warnings, yuzu.CallWarning{
			Type:    yuzu.CallWarningTypeUnsupportedSetting,
			Setting: "topK",
		})
	}

	if call.PresencePenalty != nil {
		warnings = append(warnings, yuzu.CallWarning{
			Type:    yuzu.CallWarningTypeUnsupportedSetting,
			Setting: "presencePenalty",
		})
	}

	if call.FrequencyPenalty != nil {
		warnings = append(warnings, yuzu.CallWarning{
			Type:    yuzu.CallWarningTypeUnsupportedSetting,
			Setting: "frequencyPenalty",
		})
	}

	providerOptions := call.ProviderOptions
	warnings = append(warnings, settingsWarnings(unsupportedResponsesSettings(providerOptions))...)

	if providerOptions.Store != nil {
		params.Store = openai.NewOpt(*providerOptions.Store)
	} else {
		params.Store = openai.NewOpt(false)
	}

	if providerOptions.PreviousResponseID != nil && *providerOptions.PreviousResponseID != "" {
		if err := validatePreviousResponseIDPrompt(call.Prompt); err != nil {
			return nil, warnings, err
		}
		if providerOptions.Store == nil || !*providerOptions.Store {
			return nil, warnings, errors.New(previousResponseIDStoreError)
		}
		params.PreviousResponseID = openai.NewOpt(*providerOptions.PreviousResponseID)
	}

	storeEnabled := providerOptions.Store != nil && *providerOptions.Store
	input, inputWarnings := toResponsesPrompt(call.Prompt, responsesPromptOptions{
		systemMessageMode: modelConfig.systemMessageMode,
		store:             storeEnabled,
		replayReasoning:   o.replayReasoning,
	})
	warnings = append(warnings, inputWarnings...)

	var include []yuzu.IncludeType

	addInclude := func(key yuzu.IncludeType) {
		include = append(include, key)
	}

	topLogprobs := 0
	if providerOptions.TopLogProbs != nil {
		topLogprobs = int(*providerOptions.TopLogProbs)
	} else if providerOptions.LogProbs != nil && *providerOptions.LogProbs {
		topLogprobs = topLogprobsMax
	}

	if topLogprobs > 0 {
		addInclude(yuzu.IncludeMessageOutputTextLogprobs)
	}

	params.Model = o.modelID
	params.Input = openai.ResponseNewParamsInputUnion{
		OfInputItemList: input,
	}

	if call.Temperature != nil {
		params.Temperature = openai.NewOpt(*call.Temperature)
	}
	if call.TopP != nil {
		params.TopP = openai.NewOpt(*call.TopP)
	}
	if call.MaxOutputTokens != nil {
		params.MaxOutputTokens = openai.NewOpt(*call.MaxOutputTokens)
	}

	if providerOptions.MaxToolCalls != nil {
		params.MaxToolCalls = openai.NewOpt(*providerOptions.MaxToolCalls)
	}
	if providerOptions.Metadata != nil {
		metadata := make(openai.Metadata)
		for k, v := range providerOptions.Metadata {
			if str, ok := v.(string); ok {
				metadata[k] = str
			}
		}
		params.Metadata = metadata
	}
	if providerOptions.ParallelToolCalls != nil {
		params.ParallelToolCalls = openai.NewOpt(*providerOptions.ParallelToolCalls)
	}
	if providerOptions.User != nil {
		params.User = openai.NewOpt(*providerOptions.User)
	}
	if providerOptions.Instructions != nil {
		params.Instructions = openai.NewOpt(*providerOptions.Instructions)
	}
	if providerOptions.ServiceTier != nil {
		params.ServiceTier = openai.ResponseNewParamsServiceTier(*providerOptions.ServiceTier)
	}
	if providerOptions.PromptCacheKey != nil {
		params.PromptCacheKey = openai.NewOpt(*providerOptions.PromptCacheKey)
	}
	if providerOptions.SafetyIdentifier != nil {
		params.SafetyIdentifier = openai.NewOpt(*providerOptions.SafetyIdentifier)
	}
	if topLogprobs > 0 {
		params.TopLogprobs = openai.NewOpt(int64(topLogprobs))
	}

	if len(providerOptions.Include) > 0 {
		include = append(include, providerOptions.Include...)
	}

	if (modelConfig.isReasoningModel || o.reasoningModel) &&
		(providerOptions.ReasoningEffort != nil || providerOptions.ReasoningSummary != nil) {
		reasoning := openai.ReasoningParam{}
		if providerOptions.ReasoningEffort != nil {
			reasoning.Effort = openai.ReasoningEffort(*providerOptions.ReasoningEffort)
		}
		if providerOptions.ReasoningSummary != nil {
			reasoning.Summary = openai.ReasoningSummary(*providerOptions.ReasoningSummary)
		}
		params.Reasoning = reasoning
	}

	if modelConfig.requiredAutoTruncation {
		params.Truncation = openai.ResponseNewParamsTruncationAuto
	}

	if len(include) > 0 {
		includeParams := make([]openai.ResponseIncludable, len(include))
		for i, inc := range include {
			includeParams[i] = openai.ResponseIncludable(string(inc))
		}
		params.Include = includeParams
	}

	if modelConfig.isReasoningModel {
		if call.Temperature != nil {
			params.Temperature = openai.Opt[float64]{}
			warnings = append(warnings, yuzu.CallWarning{
				Type:    yuzu.CallWarningTypeUnsupportedSetting,
				Setting: "temperature",
				Details: "temperature is not supported for reasoning models",
			})
		}

		if call.TopP != nil {
			params.TopP = openai.Opt[float64]{}
			warnings = append(warnings, yuzu.CallWarning{
				Type:    yuzu.CallWarningTypeUnsupportedSetting,
				Setting: "topP",
				Details: "topP is not supported for reasoning models",
			})
		}
	} else {
		if providerOptions.ReasoningEffort != nil {
			warnings = append(warnings, yuzu.CallWarning{
				Type:    yuzu.CallWarningTypeUnsupportedSetting,
				Setting: "reasoningEffort",
				Details: "reasoningEffort is not supported for non-reasoning models",
			})
		}

		if providerOptions.ReasoningSummary != nil {
			warnings = append(warnings, yuzu.CallWarning{
				Type:    yuzu.CallWarningTypeUnsupportedSetting,
				Setting: "reasoningSummary",
				Details: "reasoningSummary is not supported for non-reasoning models",
			})
		}
	}

	if providerOptions.ServiceTier != nil {
		if *providerOptions.ServiceTier == yuzu.ServiceTierFlex && !modelConfig.supportsFlexProcessing {
			warnings = append(warnings, yuzu.CallWarning{
				Type:    yuzu.CallWarningTypeUnsupportedSetting,
				Setting: "serviceTier",
				Details: "flex processing is only available for o3, o4-mini, and gpt-5 models",
			})
			params.ServiceTier = ""
		}

		if *providerOptions.ServiceTier == yuzu.ServiceTierPriority && !modelConfig.supportsPriorityProcessing {
			warnings = append(warnings, yuzu.CallWarning{
				Type:    yuzu.CallWarningTypeUnsupportedSetting,
				Setting: "serviceTier",
				Details: "priority processing is only available for supported models (gpt-4, gpt-5, gpt-5-mini, o3, o4-mini) and requires Enterprise access. gpt-5-nano is not supported",
			})
			params.ServiceTier = ""
		}
	}

	tools, toolChoice, toolWarnings := toResponsesTools(call.Tools, call.ToolChoice, providerOptions)
	warnings = append(warnings, toolWarnings...)

	if len(tools) > 0 {
		params.Tools = tools
		params.ToolChoice = toolChoice
	}

	return params, warnings, nil
}

func validatePreviousResponseIDPrompt(prompt yuzu.Prompt) error {
	for _, msg := range prompt {
		switch msg.Role {
		case yuzu.MessageRoleSystem, yuzu.MessageRoleUser:
			continue
		default:
			return errors.New(previousResponseIDHistoryError)
		}
	}
	return nil
}

func responsesProviderMetadata(responseID string) yuzu.ProviderMetadata {
	return yuzu.ProviderMetadata{ResponseID: responseID}
}

func responsesUsage(resp openai.Response) yuzu.Usage {
	// OpenAI reports input_tokens INCLUDING cached tokens. Subtract to avoid double-counting.
	inputTokens := max(resp.Usage.InputTokens-resp.Usage.InputTokensDetails.CachedTokens, 0)
	usage := yuzu.Usage{
		InputTokens:  inputTokens,
		OutputTokens: resp.Usage.OutputTokens,
		TotalTokens:  resp.Usage.InputTokens + resp.Usage.OutputTokens,
	}
	if resp.Usage.OutputTokensDetails.ReasoningTokens != 0 {
		usage.ReasoningTokens = resp.Usage.OutputTokensDetails.ReasoningTokens
	}
	if resp.Usage.InputTokensDetails.CachedTokens != 0 {
		usage.CacheReadTokens = resp.Usage.InputTokensDetails.CachedTokens
	}
	return usage
}

// responsesPromptOptions configures how a prompt is rendered as Responses API
// input items.
type responsesPromptOptions struct {
	// systemMessageMode is "system" or "developer", or "remove" to drop the
	// system message for models that reject it.
	systemMessageMode string
	// store mirrors the request's store flag: with it the API owns the items
	// of the conversation, without it the client replays them.
	store bool
	// replayReasoning emits assistant reasoning items so providers that
	// require the chain of thought back accept tool continuations.
	replayReasoning bool
}

func toResponsesPrompt(prompt yuzu.Prompt, options responsesPromptOptions) (openai.ResponseInputParam, []yuzu.CallWarning) {
	var input openai.ResponseInputParam
	var warnings []yuzu.CallWarning
	systemMessageMode := options.systemMessageMode

	for _, msg := range prompt {
		switch msg.Role {
		case yuzu.MessageRoleSystem:
			var systemText string
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
				if strings.TrimSpace(textPart.Text) != "" {
					systemText += textPart.Text
				}
			}

			if systemText == "" {
				warnings = append(warnings, yuzu.CallWarning{
					Type:    yuzu.CallWarningTypeOther,
					Message: "system prompt has no text parts",
				})
				continue
			}

			switch systemMessageMode {
			case "system":
				input = append(input, openai.ResponseInputItemParamOfMessage(systemText, openai.EasyInputMessageRoleSystem))
			case "developer":
				input = append(input, openai.ResponseInputItemParamOfMessage(systemText, openai.EasyInputMessageRoleDeveloper))
			case "remove":
				warnings = append(warnings, yuzu.CallWarning{
					Type:    yuzu.CallWarningTypeOther,
					Message: "system messages are removed for this model",
				})
			}

		case yuzu.MessageRoleUser:
			var contentParts openai.ResponseInputMessageContentListParam
			for i, c := range msg.Content {
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
					contentParts = append(contentParts, openai.ResponseInputContentUnionParam{
						OfInputText: &openai.ResponseInputTextParam{
							Type: "input_text",
							Text: textPart.Text,
						},
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

					if strings.HasPrefix(filePart.MediaType, "image/") {
						base64Encoded := base64.StdEncoding.EncodeToString(filePart.Data)
						imageURL := fmt.Sprintf("data:%s;base64,%s", filePart.MediaType, base64Encoded)
						contentParts = append(contentParts, openai.ResponseInputContentUnionParam{
							OfInputImage: &openai.ResponseInputImageParam{
								Type:     "input_image",
								ImageURL: openai.NewOpt(imageURL),
							},
						})
					} else if filePart.MediaType == "application/pdf" {
						base64Encoded := base64.StdEncoding.EncodeToString(filePart.Data)
						fileData := fmt.Sprintf("data:application/pdf;base64,%s", base64Encoded)
						filename := filePart.Filename
						if filename == "" {
							filename = fmt.Sprintf("part-%d.pdf", i)
						}
						contentParts = append(contentParts, openai.ResponseInputContentUnionParam{
							OfInputFile: &openai.ResponseInputFileParam{
								Type:     "input_file",
								Filename: openai.NewOpt(filename),
								FileData: openai.NewOpt(fileData),
							},
						})
					} else {
						warnings = append(warnings, yuzu.CallWarning{
							Type:    yuzu.CallWarningTypeOther,
							Message: fmt.Sprintf("file part media type %s not supported", filePart.MediaType),
						})
					}
				}
			}

			if !hasVisibleResponsesUserContent(contentParts) {
				warnings = append(warnings, yuzu.CallWarning{
					Type:    yuzu.CallWarningTypeOther,
					Message: "dropping empty user message (contains neither user-facing content nor tool results)",
				})
				continue
			}

			input = append(input, openai.ResponseInputItemParamOfMessage(contentParts, openai.EasyInputMessageRoleUser))

		case yuzu.MessageRoleAssistant:
			startIdx := len(input)
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
					input = append(input, openai.ResponseInputItemParamOfMessage(textPart.Text, openai.EasyInputMessageRoleAssistant))

				case yuzu.ContentTypeToolCall:
					toolCallPart, ok := yuzu.AsContentType[yuzu.ToolCallPart](c)
					if !ok {
						warnings = append(warnings, yuzu.CallWarning{
							Type:    yuzu.CallWarningTypeOther,
							Message: "assistant message tool call part does not have the right type",
						})
						continue
					}

					if toolCallPart.ProviderExecuted {
						if options.store {
							// Round-trip provider-executed tools via
							// item_reference, letting the API resolve
							// the stored output item by ID.
							input = append(input, openai.ResponseInputItemParamOfItemReference(toolCallPart.ToolCallID))
						}
						// When store is disabled, server-side items are
						// ephemeral and cannot be referenced. Skip the
						// tool call; results are already omitted for
						// provider-executed tools.
						continue
					}

					input = append(input, openai.ResponseInputItemParamOfFunctionCall(toolCallPart.Input, toolCallPart.ToolCallID, toolCallPart.ToolName))
				case yuzu.ContentTypeSource:
					// Source citations from web search are not a
					// recognised Responses API input type; skip.
					continue
				case yuzu.ContentTypeReasoning:
					// Reasoning items are skipped by default: when store is
					// enabled the API already has them persisted server-side,
					// and when store is disabled the item IDs are ephemeral, so
					// referencing them causes "Item not found" errors.
					// Providers that reject tool continuations without the
					// chain of thought (DeepSeek thinking mode) opt in through
					// WithReplayReasoning.
					reasoningPart, ok := yuzu.AsContentType[yuzu.ReasoningPart](c)
					if !ok {
						warnings = append(warnings, yuzu.CallWarning{
							Type:    yuzu.CallWarningTypeOther,
							Message: "assistant message reasoning part does not have the right type",
						})
						continue
					}

					if reasoningPart.Encrypted != nil && reasoningPart.ItemID != "" {
						input = append(input, openai.ResponseInputItemUnionParam{
							OfReasoning: &openai.ResponseReasoningItemParam{
								ID: reasoningPart.ItemID, Summary: []openai.ResponseReasoningItemSummaryParam{},
								EncryptedContent: openai.String(*reasoningPart.Encrypted),
							},
						})
						continue
					}
					if !options.replayReasoning || reasoningPart.Text == "" {
						continue
					}

					input = append(input, openai.ResponseInputItemUnionParam{
						OfReasoning: &openai.ResponseReasoningItemParam{
							Summary: []openai.ResponseReasoningItemSummaryParam{},
							Content: []openai.ResponseReasoningItemContentParam{
								{Text: reasoningPart.Text},
							},
						},
					})
				}
			}

			if !hasVisibleResponsesAssistantContent(input, startIdx) {
				warnings = append(warnings, yuzu.CallWarning{
					Type:    yuzu.CallWarningTypeOther,
					Message: "dropping empty assistant message (contains neither user-facing content nor tool calls)",
				})
				// Remove any items that were added during this iteration
				input = input[:startIdx]
				continue
			}

		case yuzu.MessageRoleTool:
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

				// Provider-executed tool results (e.g. web search)
				// are already round-tripped via the tool call; skip.
				if toolResultPart.ProviderExecuted {
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
					input = append(input, functionCallOutputItem(toolResultPart.ToolCallID, output.Text))
				case yuzu.ToolResultContentTypeError:
					output, ok := yuzu.AsToolResultOutputType[yuzu.ToolResultOutputContentError](toolResultPart.Output)
					if !ok {
						warnings = append(warnings, yuzu.CallWarning{
							Type:    yuzu.CallWarningTypeOther,
							Message: "tool result output does not have the right type",
						})
						continue
					}
					input = append(input, functionCallOutputItem(toolResultPart.ToolCallID, output.Error.Error()))
				case yuzu.ToolResultContentTypeMedia:
					output, ok := yuzu.AsToolResultOutputType[yuzu.ToolResultOutputContentMedia](toolResultPart.Output)
					if !ok {
						warnings = append(warnings, yuzu.CallWarning{
							Type:    yuzu.CallWarningTypeOther,
							Message: "tool result output does not have the right type",
						})
						continue
					}
					contentItems, mediaWarning := responsesToolResultContentItems(output)
					if mediaWarning != nil {
						warnings = append(warnings, *mediaWarning)
					}
					// The media travels inside the tool result. Emitting it as
					// a separate user message instead would sit between a call
					// and its output whenever a step returns more than one
					// media result, which strict providers (DeepSeek) reject
					// with "No tool output found for tool call ...".
					input = append(input, functionCallOutputContentItems(toolResultPart.ToolCallID, contentItems))
				default:
					warnings = append(warnings, yuzu.CallWarning{
						Type:    yuzu.CallWarningTypeOther,
						Message: fmt.Sprintf("tool result output type %q not supported", toolResultPart.Output.GetType()),
					})
					continue
				}
			}
		}
	}

	return input, warnings
}

// functionCallOutputItem answers a tool call with plain text.
func functionCallOutputItem(callID string, text string) openai.ResponseInputItemUnionParam {
	item := openai.ResponseInputItemParamOfFunctionCallOutput(text)
	item.OfFunctionCallOutput.CallID = openai.NewOpt(callID)
	return item
}

// functionCallOutputContentItems answers a tool call with structured content,
// which is how media reaches a vision-capable model.
func functionCallOutputContentItems(callID string, content openai.ResponseFunctionCallOutputItemListParam) openai.ResponseInputItemUnionParam {
	item := openai.ResponseInputItemParamOfFunctionCallOutput(content)
	item.OfFunctionCallOutput.CallID = openai.NewOpt(callID)
	return item
}

// responsesToolResultContentItems maps a media tool result to the content items
// of its function_call_output: the accompanying text when the tool reported
// any, followed by the media itself. Images become input_image items; other
// media types are not part of the API and degrade to text with a warning.
func responsesToolResultContentItems(output yuzu.ToolResultOutputContentMedia) (openai.ResponseFunctionCallOutputItemListParam, *yuzu.CallWarning) {
	items := make(openai.ResponseFunctionCallOutputItemListParam, 0, 2)
	if output.Text != "" {
		items = append(items, inputTextContentItem(output.Text))
	}

	if strings.HasPrefix(output.MediaType, "image/") {
		items = append(items, openai.ResponseFunctionCallOutputItemUnionParam{
			OfInputImage: &openai.ResponseInputImageContentParam{
				Type:     "input_image",
				ImageURL: openai.NewOpt(fmt.Sprintf("data:%s;base64,%s", output.MediaType, output.Data)),
			},
		})
		return items, nil
	}

	warning := yuzu.CallWarning{
		Type:    yuzu.CallWarningTypeOther,
		Message: fmt.Sprintf("tool result media type %s not supported, sending text placeholder only", output.MediaType),
	}
	if len(items) == 0 {
		items = append(items, inputTextContentItem(fmt.Sprintf("The tool returned %s content.", output.MediaType)))
	}
	return items, &warning
}

func inputTextContentItem(text string) openai.ResponseFunctionCallOutputItemUnionParam {
	return openai.ResponseFunctionCallOutputItemUnionParam{
		OfInputText: &openai.ResponseInputTextContentParam{
			Type: "input_text",
			Text: text,
		},
	}
}

func hasVisibleResponsesUserContent(content openai.ResponseInputMessageContentListParam) bool {
	return len(content) > 0
}

func hasVisibleResponsesAssistantContent(items []openai.ResponseInputItemUnionParam, startIdx int) bool {
	// Check if we added any assistant content parts from this message
	for i := startIdx; i < len(items); i++ {
		if items[i].OfMessage != nil || items[i].OfFunctionCall != nil || items[i].OfItemReference != nil {
			return true
		}
		if item := items[i].OfReasoning; item != nil && item.ID != "" {
			return true
		}
	}
	return false
}

func toResponsesTools(tools []yuzu.Tool, toolChoice *yuzu.ToolChoice, options yuzu.ProviderOptions) ([]openai.ToolUnionParam, openai.ResponseNewParamsToolChoiceUnion, []yuzu.CallWarning) {
	warnings := make([]yuzu.CallWarning, 0)
	var openaiTools []openai.ToolUnionParam

	if len(tools) == 0 {
		return nil, openai.ResponseNewParamsToolChoiceUnion{}, nil
	}

	strictJSONSchema := false
	if options.StrictJSONSchema != nil {
		strictJSONSchema = *options.StrictJSONSchema
	}

	for _, tool := range tools {
		if tool.GetType() == yuzu.ToolTypeFunction {
			ft, ok := tool.(yuzu.FunctionTool)
			if !ok {
				continue
			}
			openaiTools = append(openaiTools, openai.ToolUnionParam{
				OfFunction: &openai.FunctionToolParam{
					Name:        ft.Name,
					Description: openai.NewOpt(ft.Description),
					Parameters:  ft.InputSchema,
					Strict:      openai.NewOpt(strictJSONSchema),
					Type:        "function",
				},
			})
			continue
		}
		if tool.GetType() == yuzu.ToolTypeProviderDefined {
			pt, ok := tool.(yuzu.ProviderDefinedTool)
			if !ok {
				continue
			}
			switch pt.ID {
			case "web_search":
				openaiTools = append(openaiTools, toWebSearchToolParam(pt))
				continue
			}
		}

		warnings = append(warnings, yuzu.CallWarning{
			Type:    yuzu.CallWarningTypeUnsupportedTool,
			Tool:    tool,
			Message: "tool is not supported",
		})
	}

	if toolChoice == nil {
		return openaiTools, openai.ResponseNewParamsToolChoiceUnion{}, warnings
	}

	var openaiToolChoice openai.ResponseNewParamsToolChoiceUnion

	switch *toolChoice {
	case yuzu.ToolChoiceAuto:
		openaiToolChoice = openai.ResponseNewParamsToolChoiceUnion{
			OfToolChoiceMode: openai.NewOpt(openai.ToolChoiceOptionsAuto),
		}
	case yuzu.ToolChoiceNone:
		openaiToolChoice = openai.ResponseNewParamsToolChoiceUnion{
			OfToolChoiceMode: openai.NewOpt(openai.ToolChoiceOptionsNone),
		}
	case yuzu.ToolChoiceRequired:
		openaiToolChoice = openai.ResponseNewParamsToolChoiceUnion{
			OfToolChoiceMode: openai.NewOpt(openai.ToolChoiceOptionsRequired),
		}
	default:
		openaiToolChoice = openai.ResponseNewParamsToolChoiceUnion{
			OfFunctionTool: &openai.ToolChoiceFunctionParam{
				Type: "function",
				Name: string(*toolChoice),
			},
		}
	}

	return openaiTools, openaiToolChoice, warnings
}

func (o responsesLanguageModel) Generate(ctx context.Context, call yuzu.Call) (*yuzu.Response, error) {
	capture := responseCapture{}
	params, warnings, err := o.prepareParams(call)
	if err != nil {
		return nil, err
	}

	response, err := o.client.Responses(ctx, *params, capture.requestOptions(o.headerFunc, append(callUARequestOptions(call), callHeadersRequestOptions(call)...))...)
	if err != nil {
		return nil, toProviderErr(err)
	}

	if response == nil {
		return nil, &yuzu.Error{Title: "no response", Message: "provider returned nil response"}
	}

	if response.Error.Message != "" {
		return nil, &yuzu.Error{
			Title:   "provider error",
			Message: fmt.Sprintf("%s (code: %s)", response.Error.Message, response.Error.Code),
		}
	}

	var content []yuzu.Content
	hasFunctionCall := false
	var pendingFunctionCalls []yuzu.ToolCallContent

	for _, outputItem := range response.Output {
		switch outputItem.Type {
		case "message":
			for _, contentPart := range outputItem.Content {
				if contentPart.Type == "output_text" {
					content = append(content, yuzu.TextContent{
						Text: contentPart.Text,
					})

					for _, annotation := range contentPart.Annotations {
						switch annotation.Type {
						case "url_citation":
							content = append(content, yuzu.SourceContent{
								SourceType: yuzu.SourceTypeURL,
								ID:         uuid.New().String(),
								URL:        annotation.URL,
								Title:      annotation.Title,
							})
						case "file_citation":
							title := "Document"
							if annotation.Filename != "" {
								title = annotation.Filename
							}
							filename := annotation.Filename
							if filename == "" {
								filename = annotation.FileID
							}
							content = append(content, yuzu.SourceContent{
								SourceType: yuzu.SourceTypeDocument,
								ID:         uuid.New().String(),
								MediaType:  "text/plain",
								Title:      title,
								Filename:   filename,
							})
						}
					}
				}
			}

		case "function_call":
			hasFunctionCall = true
			pendingFunctionCalls = append(pendingFunctionCalls, yuzu.ToolCallContent{
				ProviderExecuted: false,
				ToolCallID:       outputItem.CallID,
				ToolName:         outputItem.Name,
				Input:            outputItem.Arguments.OfString,
			})

		case "web_search_call":
			// Provider-executed web search tool call. Emit both a
			// ToolCallContent and ToolResultContent as a pair so the
			// agent loop can round-trip the provider tool.
			//
			// Note: source citations come from url_citation annotations
			// on the message text (handled in the "message" case above),
			// not from the web_search_call action.
			wsMeta := webSearchCallToMetadata(outputItem.ID, outputItem.Action)
			content = append(content, yuzu.ToolCallContent{
				ProviderExecuted: true,
				ToolCallID:       outputItem.ID,
				ToolName:         "web_search",
			})
			content = append(content, yuzu.ToolResultContent{
				ProviderExecuted: true,
				ToolCallID:       outputItem.ID,
				ToolName:         "web_search",
				ProviderMetadata: yuzu.ProviderMetadata{WebSearch: wsMeta},
			})
		case "reasoning":
			metadata := yuzu.ProviderMetadata{
				ReasoningItemID: outputItem.ID,
			}
			if outputItem.EncryptedContent != "" {
				metadata.EncryptedReasoning = &outputItem.EncryptedContent
			}

			for _, s := range outputItem.Summary {
				metadata.ReasoningSummary = append(metadata.ReasoningSummary, s.Text)
			}

			// Models that report reasoning as inline content parts instead
			// of a summary keep the text there.
			if len(metadata.ReasoningSummary) == 0 {
				for _, c := range outputItem.Content {
					metadata.ReasoningSummary = append(metadata.ReasoningSummary, c.Text)
				}
			}

			if len(metadata.ReasoningSummary) == 0 && metadata.EncryptedReasoning == nil {
				continue
			}

			content = append(content, yuzu.ReasoningContent{
				Text:             strings.Join(metadata.ReasoningSummary, "\n"),
				ProviderMetadata: metadata,
			})
		}
	}

	usage := responsesUsage(*response)
	finishReason := mapResponsesFinishReason(response.IncompleteDetails.Reason, hasFunctionCall)
	truncatedWithToolCalls := hasFunctionCall && finishReason == yuzu.FinishReasonLength
	if truncatedWithToolCalls {
		warnings = append(warnings, yuzu.CallWarning{
			Type:    yuzu.CallWarningTypeOther,
			Message: "tool calls were returned but the model hit the token limit; arguments may be truncated",
		})
	} else {
		for _, tc := range pendingFunctionCalls {
			content = append(content, tc)
		}
	}

	metadata := responsesProviderMetadata(response.ID)
	metadata = o.applyHeaders(capture.header(), metadata)

	return &yuzu.Response{
		Content:          content,
		Usage:            usage,
		FinishReason:     finishReason,
		ProviderMetadata: metadata,
		Warnings:         warnings,
	}, nil
}

func mapResponsesFinishReason(reason string, hasFunctionCall bool) yuzu.FinishReason {
	if hasFunctionCall && reason != "max_tokens" && reason != "max_output_tokens" {
		return yuzu.FinishReasonToolCalls
	}

	switch reason {
	case "":
		if hasFunctionCall {
			return yuzu.FinishReasonToolCalls
		}
		return yuzu.FinishReasonStop
	case "max_tokens", "max_output_tokens":
		return yuzu.FinishReasonLength
	case "content_filter":
		return yuzu.FinishReasonContentFilter
	default:
		return yuzu.FinishReasonOther
	}
}

func (o responsesLanguageModel) Stream(ctx context.Context, call yuzu.Call) (yuzu.StreamResponse, error) {
	capture := responseCapture{}
	params, warnings, err := o.prepareParams(call)
	if err != nil {
		return nil, err
	}

	stream := o.client.ResponsesStream(ctx, *params, capture.requestOptions(o.headerFunc, append(callUARequestOptions(call), callHeadersRequestOptions(call)...))...)

	finishReason := yuzu.FinishReasonUnknown
	var usage yuzu.Usage
	// responseID tracks the server-assigned response ID. It's first set from the
	// response.created event and may be overwritten by response.completed or
	// response.incomplete events. Per the OpenAI API contract, these IDs are
	// identical; the overwrites ensure we have the final value even if an event
	// is missed.
	responseID := ""
	sawTerminalEvent := false
	ongoingToolCalls := make(map[int64]*ongoingToolCall)
	hasFunctionCall := false
	activeReasoning := make(map[string]*reasoningState)

	return func(yield func(yuzu.StreamPart) bool) {
		if len(warnings) > 0 {
			if !yield(yuzu.StreamPart{
				Type:     yuzu.StreamPartTypeWarnings,
				Warnings: warnings,
			}) {
				return
			}
		}

		for stream.Next() {
			event := stream.Current()

			switch event.Type {
			case "response.created":
				created := event.AsResponseCreated()
				responseID = created.Response.ID

			case "response.output_item.added":
				added := event.AsResponseOutputItemAdded()
				switch added.Item.Type {
				case "function_call":
					ongoingToolCalls[added.OutputIndex] = &ongoingToolCall{
						toolName:   added.Item.Name,
						toolCallID: added.Item.CallID,
					}
					if !yield(yuzu.StreamPart{
						Type:         yuzu.StreamPartTypeToolInputStart,
						ID:           added.Item.CallID,
						ToolCallName: added.Item.Name,
					}) {
						return
					}

				case "web_search_call":
					// Provider-executed web search; emit start.
					if !yield(yuzu.StreamPart{
						Type:             yuzu.StreamPartTypeToolInputStart,
						ID:               added.Item.ID,
						ToolCallName:     "web_search",
						ProviderExecuted: true,
					}) {
						return
					}

				case "message":
					if !yield(yuzu.StreamPart{
						Type: yuzu.StreamPartTypeTextStart,
						ID:   added.Item.ID,
					}) {
						return
					}

				case "reasoning":
					metadata := yuzu.ProviderMetadata{
						ReasoningItemID:  added.Item.ID,
						ReasoningSummary: []string{},
					}
					if added.Item.EncryptedContent != "" {
						metadata.EncryptedReasoning = &added.Item.EncryptedContent
					}

					activeReasoning[added.Item.ID] = &reasoningState{
						metadata: metadata,
					}
					if !yield(yuzu.StreamPart{
						Type:             yuzu.StreamPartTypeReasoningStart,
						ID:               added.Item.ID,
						ProviderMetadata: metadata,
					}) {
						return
					}
				}

			case "response.output_item.done":
				done := event.AsResponseOutputItemDone()
				switch done.Item.Type {
				case "function_call":
					tc := ongoingToolCalls[done.OutputIndex]
					if tc != nil {
						delete(ongoingToolCalls, done.OutputIndex)
						hasFunctionCall = true

						if !yield(yuzu.StreamPart{
							Type: yuzu.StreamPartTypeToolInputEnd,
							ID:   done.Item.CallID,
						}) {
							return
						}
						if !yield(yuzu.StreamPart{
							Type:          yuzu.StreamPartTypeToolCall,
							ID:            done.Item.CallID,
							ToolCallName:  done.Item.Name,
							ToolCallInput: done.Item.Arguments.OfString,
						}) {
							return
						}
					}

				case "web_search_call":
					// Provider-executed web search completed.
					// Source citations come from url_citation annotations
					// on the streamed message text, not from the action.
					if !yield(yuzu.StreamPart{
						Type: yuzu.StreamPartTypeToolInputEnd,
						ID:   done.Item.ID,
					}) {
						return
					}
					if !yield(yuzu.StreamPart{
						Type:             yuzu.StreamPartTypeToolCall,
						ID:               done.Item.ID,
						ToolCallName:     "web_search",
						ProviderExecuted: true,
					}) {
						return
					}
					// Emit a ToolResult so the agent framework
					// includes it in round-trip messages.
					if !yield(yuzu.StreamPart{
						Type:             yuzu.StreamPartTypeToolResult,
						ID:               done.Item.ID,
						ToolCallName:     "web_search",
						ProviderExecuted: true,
						ProviderMetadata: yuzu.ProviderMetadata{WebSearch: webSearchCallToMetadata(done.Item.ID, done.Item.Action)},
					}) {
						return
					}
				case "message":
					if !yield(yuzu.StreamPart{
						Type: yuzu.StreamPartTypeTextEnd,
						ID:   done.Item.ID,
					}) {
						return
					}

				case "reasoning":
					state := activeReasoning[done.Item.ID]
					if state != nil {
						if !yield(yuzu.StreamPart{
							Type:             yuzu.StreamPartTypeReasoningEnd,
							ID:               done.Item.ID,
							ProviderMetadata: state.metadata,
						}) {
							return
						}
						delete(activeReasoning, done.Item.ID)
					}
				}

			case "response.function_call_arguments.delta":
				delta := event.AsResponseFunctionCallArgumentsDelta()
				tc := ongoingToolCalls[delta.OutputIndex]
				if tc != nil {
					if !yield(yuzu.StreamPart{
						Type:  yuzu.StreamPartTypeToolInputDelta,
						ID:    tc.toolCallID,
						Delta: delta.Delta,
					}) {
						return
					}
				}

			case "response.output_text.delta":
				textDelta := event.AsResponseOutputTextDelta()
				if !yield(yuzu.StreamPart{
					Type:  yuzu.StreamPartTypeTextDelta,
					ID:    textDelta.ItemID,
					Delta: textDelta.Delta,
				}) {
					return
				}

			case "response.output_text.annotation.added":
				added := event.AsResponseOutputTextAnnotationAdded()
				annotation := added.Annotation
				switch annotation.Type {
				case "url_citation":
					if !yield(yuzu.StreamPart{
						Type:       yuzu.StreamPartTypeSource,
						ID:         uuid.New().String(),
						SourceType: yuzu.SourceTypeURL,
						URL:        annotation.URL,
						Title:      annotation.Title,
					}) {
						return
					}
				case "file_citation":
					title := "Document"
					if annotation.Filename != "" {
						title = annotation.Filename
					}
					if !yield(yuzu.StreamPart{
						Type:       yuzu.StreamPartTypeSource,
						ID:         uuid.New().String(),
						SourceType: yuzu.SourceTypeDocument,
						Title:      title,
					}) {
						return
					}
				}

			case "response.reasoning_summary_part.added":
				added := event.AsResponseReasoningSummaryPartAdded()
				state := activeReasoning[added.ItemID]
				if state != nil {
					state.metadata.ReasoningSummary = append(state.metadata.ReasoningSummary, "")
					activeReasoning[added.ItemID] = state
					if !yield(yuzu.StreamPart{
						Type:             yuzu.StreamPartTypeReasoningDelta,
						ID:               added.ItemID,
						Delta:            "\n",
						ProviderMetadata: state.metadata,
					}) {
						return
					}
				}

			case "response.reasoning_summary_text.delta":
				textDelta := event.AsResponseReasoningSummaryTextDelta()
				state := activeReasoning[textDelta.ItemID]
				if state != nil {
					if len(state.metadata.ReasoningSummary)-1 >= int(textDelta.SummaryIndex) {
						state.metadata.ReasoningSummary[textDelta.SummaryIndex] += textDelta.Delta
					}
					activeReasoning[textDelta.ItemID] = state
					if !yield(yuzu.StreamPart{
						Type:             yuzu.StreamPartTypeReasoningDelta,
						ID:               textDelta.ItemID,
						Delta:            textDelta.Delta,
						ProviderMetadata: state.metadata,
					}) {
						return
					}
				}

			case "response.reasoning_text.delta":
				// Models that report reasoning as inline content parts
				// instead of a summary stream these events.
				textDelta := event.AsResponseReasoningTextDelta()
				state := activeReasoning[textDelta.ItemID]
				if state != nil {
					if !yield(yuzu.StreamPart{
						Type:             yuzu.StreamPartTypeReasoningDelta,
						ID:               textDelta.ItemID,
						Delta:            textDelta.Delta,
						ProviderMetadata: state.metadata,
					}) {
						return
					}
				}

			case "response.completed":
				sawTerminalEvent = true
				completed := event.AsResponseCompleted()
				responseID = completed.Response.ID
				finishReason = mapResponsesFinishReason(completed.Response.IncompleteDetails.Reason, hasFunctionCall)
				usage = responsesUsage(completed.Response)

			case "response.incomplete":
				sawTerminalEvent = true
				incomplete := event.AsResponseIncomplete()
				responseID = incomplete.Response.ID
				finishReason = mapResponsesFinishReason(incomplete.Response.IncompleteDetails.Reason, hasFunctionCall)
				usage = responsesUsage(incomplete.Response)

			case "response.failed":
				failed := event.AsResponseFailed()
				if !yield(yuzu.StreamPart{
					Type:  yuzu.StreamPartTypeError,
					Error: responsesFailedStreamError(failed.Response.Error.Message, string(failed.Response.Error.Code)),
				}) {
					return
				}
				return

			case "error":
				errorEvent := event.AsError()
				if !yield(yuzu.StreamPart{
					Type:  yuzu.StreamPartTypeError,
					Error: responsesErrorStreamError(errorEvent.Message, errorEvent.Code),
				}) {
					return
				}
				return
			}
		}

		err := stream.Err()
		if err != nil && !errors.Is(err, io.EOF) {
			yield(yuzu.StreamPart{
				Type:  yuzu.StreamPartTypeError,
				Error: toProviderErr(err),
			})
			return
		}

		if !sawTerminalEvent {
			err := ctx.Err()
			if err == nil {
				err = yuzu.NewIncompleteStreamError()
			}
			yield(yuzu.StreamPart{
				Type:  yuzu.StreamPartTypeError,
				Error: err,
			})
			return
		}

		if hasFunctionCall && finishReason == yuzu.FinishReasonLength {
			yield(yuzu.StreamPart{
				Type: yuzu.StreamPartTypeWarnings,
				Warnings: []yuzu.CallWarning{{
					Type:    yuzu.CallWarningTypeOther,
					Message: "tool calls were returned but the model hit the token limit; arguments may be truncated",
				}},
			})
		}

		metadata := responsesProviderMetadata(responseID)
		metadata = o.applyHeaders(capture.header(), metadata)
		yield(yuzu.StreamPart{
			Type:             yuzu.StreamPartTypeFinish,
			Usage:            usage,
			FinishReason:     finishReason,
			ProviderMetadata: metadata,
		})
	}, nil
}

// responsesFailedStreamError intentionally returns a provider-declared failure
// instead of a retryable transport error. Only synthetic stream truncation
// errors are wrapped with io.ErrUnexpectedEOF.
func responsesFailedStreamError(message, code string) error {
	return responsesStreamFailureError("response failed", message, code)
}

func responsesErrorStreamError(message, code string) error {
	return responsesStreamFailureError("response error", message, code)
}

func responsesStreamFailureError(title, message, code string) error {
	if code != "" {
		message = fmt.Sprintf("%s (code: %s)", message, code)
	}
	return &yuzu.Error{Title: title, Message: message}
}

// toWebSearchToolParam converts a ProviderDefinedTool with ID
// "web_search" into the OpenAI SDK's WebSearchToolParam.
func toWebSearchToolParam(pt yuzu.ProviderDefinedTool) openai.ToolUnionParam {
	wst := openai.WebSearchToolParam{
		Type: openai.WebSearchToolTypeWebSearch,
	}
	if pt.Args != nil {
		if size, ok := pt.Args["search_context_size"].(SearchContextSize); ok && size != "" {
			wst.SearchContextSize = openai.WebSearchToolSearchContextSize(size)
		}
		// Also accept plain string for search_context_size.
		if size, ok := pt.Args["search_context_size"].(string); ok && size != "" {
			wst.SearchContextSize = openai.WebSearchToolSearchContextSize(size)
		}
		if domains, ok := pt.Args["allowed_domains"].([]string); ok && len(domains) > 0 {
			wst.Filters.AllowedDomains = domains
		}
		if loc, ok := pt.Args["user_location"].(*WebSearchUserLocation); ok && loc != nil {
			if loc.City != "" {
				wst.UserLocation.City = openai.NewOpt(loc.City)
			}
			if loc.Region != "" {
				wst.UserLocation.Region = openai.NewOpt(loc.Region)
			}
			if loc.Country != "" {
				wst.UserLocation.Country = openai.NewOpt(loc.Country)
			}
			if loc.Timezone != "" {
				wst.UserLocation.Timezone = openai.NewOpt(loc.Timezone)
			}
		}
	}
	return openai.ToolUnionParam{
		OfWebSearch: &wst,
	}
}

// webSearchCallToMetadata converts an OpenAI web search call output
// into our structured metadata for round-tripping.
func webSearchCallToMetadata(itemID string, action openai.ResponseOutputItemUnionAction) *yuzu.WebSearchCallMetadata {
	meta := &yuzu.WebSearchCallMetadata{ItemID: itemID}
	if action.Type != "" {
		a := &yuzu.WebSearchAction{
			Type:  action.Type,
			Query: action.Query,
		}
		for _, src := range action.Sources {
			a.Sources = append(a.Sources, yuzu.WebSearchSource{
				Type: src.Type,
				URL:  src.URL,
			})
		}
		meta.Action = a
	}
	return meta
}

type ongoingToolCall struct {
	toolName   string
	toolCallID string
}

type reasoningState struct {
	metadata yuzu.ProviderMetadata
}

// GenerateObject implements LanguageModel.
func (o responsesLanguageModel) GenerateObject(ctx context.Context, call yuzu.ObjectCall) (*yuzu.ObjectResponse, error) {
	switch o.objectMode {
	case yuzu.ObjectModeText:
		return generateObjectWithText(ctx, o, call)
	case yuzu.ObjectModeTool:
		return generateObjectWithTool(ctx, o, call)
	default:
		return o.generateObjectWithJSONMode(ctx, call)
	}
}

// StreamObject implements LanguageModel.
func (o responsesLanguageModel) StreamObject(ctx context.Context, call yuzu.ObjectCall) (yuzu.ObjectStreamResponse, error) {
	switch o.objectMode {
	case yuzu.ObjectModeTool:
		return streamObjectWithTool(ctx, o, call)
	case yuzu.ObjectModeText:
		return streamObjectWithText(ctx, o, call)
	default:
		return o.streamObjectWithJSONMode(ctx, call)
	}
}

func (o responsesLanguageModel) generateObjectWithJSONMode(ctx context.Context, call yuzu.ObjectCall) (*yuzu.ObjectResponse, error) {
	// Convert our Schema to OpenAI's JSON Schema format
	jsonSchemaMap := schema.ToMap(call.Schema)

	// Add additionalProperties: false recursively for strict mode (OpenAI requirement)
	addAdditionalPropertiesFalse(jsonSchemaMap)

	schemaName := call.SchemaName
	if schemaName == "" {
		schemaName = "response"
	}

	// Build request using prepareParams
	modelCall := yuzu.Call{
		Prompt:           call.Prompt,
		MaxOutputTokens:  call.MaxOutputTokens,
		Temperature:      call.Temperature,
		TopP:             call.TopP,
		PresencePenalty:  call.PresencePenalty,
		FrequencyPenalty: call.FrequencyPenalty,
		ProviderOptions:  call.ProviderOptions,
	}

	params, warnings, err := o.prepareParams(modelCall)
	if err != nil {
		return nil, err
	}

	// Add structured output via Text.Format field
	params.Text = openai.ResponseTextConfigParam{
		Format: openai.ResponseFormatTextConfigParamOfJSONSchema(schemaName, jsonSchemaMap),
	}

	// Make request
	capture := responseCapture{}
	response, err := o.client.Responses(ctx, *params, capture.requestOptions(o.headerFunc, append(objectCallUARequestOptions(call), objectCallHeadersRequestOptions(call)...))...)
	if err != nil {
		return nil, toProviderErr(err)
	}

	if response.Error.Message != "" {
		return nil, &yuzu.Error{
			Title:   "provider error",
			Message: fmt.Sprintf("%s (code: %s)", response.Error.Message, response.Error.Code),
		}
	}

	// Extract JSON text from response
	var jsonText string
	for _, outputItem := range response.Output {
		if outputItem.Type == "message" {
			for _, contentPart := range outputItem.Content {
				if contentPart.Type == "output_text" {
					jsonText = contentPart.Text
					break
				}
			}
		}
	}

	if jsonText == "" {
		usage := yuzu.Usage{
			InputTokens:  response.Usage.InputTokens,
			OutputTokens: response.Usage.OutputTokens,
			TotalTokens:  response.Usage.InputTokens + response.Usage.OutputTokens,
		}
		finishReason := mapResponsesFinishReason(response.IncompleteDetails.Reason, false)
		return nil, &yuzu.NoObjectGeneratedError{
			RawText:      "",
			ParseError:   fmt.Errorf("no text content in response"),
			Usage:        usage,
			FinishReason: finishReason,
		}
	}

	// Parse and validate
	var obj any
	if call.RepairText != nil {
		obj, err = schema.ParseAndValidateWithRepair(ctx, jsonText, call.Schema, call.RepairText)
	} else {
		obj, err = schema.ParseAndValidate(jsonText, call.Schema)
	}

	usage := responsesUsage(*response)
	finishReason := mapResponsesFinishReason(response.IncompleteDetails.Reason, false)

	if err != nil {
		// Add usage info to error
		if nogErr, ok := errors.AsType[*yuzu.NoObjectGeneratedError](err); ok {
			nogErr.Usage = usage
			nogErr.FinishReason = finishReason
		}
		return nil, err
	}

	metadata := responsesProviderMetadata(response.ID)
	metadata = o.applyHeaders(capture.header(), metadata)

	return &yuzu.ObjectResponse{
		Object:           obj,
		RawText:          jsonText,
		Usage:            usage,
		FinishReason:     finishReason,
		Warnings:         warnings,
		ProviderMetadata: metadata,
	}, nil
}

func (o responsesLanguageModel) streamObjectWithJSONMode(ctx context.Context, call yuzu.ObjectCall) (yuzu.ObjectStreamResponse, error) {
	// Convert our Schema to OpenAI's JSON Schema format
	jsonSchemaMap := schema.ToMap(call.Schema)

	// Add additionalProperties: false recursively for strict mode (OpenAI requirement)
	addAdditionalPropertiesFalse(jsonSchemaMap)

	schemaName := call.SchemaName
	if schemaName == "" {
		schemaName = "response"
	}

	// Build request using prepareParams
	modelCall := yuzu.Call{
		Prompt:           call.Prompt,
		MaxOutputTokens:  call.MaxOutputTokens,
		Temperature:      call.Temperature,
		TopP:             call.TopP,
		PresencePenalty:  call.PresencePenalty,
		FrequencyPenalty: call.FrequencyPenalty,
		ProviderOptions:  call.ProviderOptions,
	}

	params, warnings, err := o.prepareParams(modelCall)
	if err != nil {
		return nil, err
	}

	// Add structured output via Text.Format field
	params.Text = openai.ResponseTextConfigParam{
		Format: openai.ResponseFormatTextConfigParamOfJSONSchema(schemaName, jsonSchemaMap),
	}

	capture := responseCapture{}
	stream := o.client.ResponsesStream(ctx, *params, capture.requestOptions(o.headerFunc, append(objectCallUARequestOptions(call), objectCallHeadersRequestOptions(call)...))...)

	return func(yield func(yuzu.ObjectStreamPart) bool) {
		defer stream.Close()
		if len(warnings) > 0 {
			if !yield(yuzu.ObjectStreamPart{
				Type:     yuzu.ObjectStreamPartTypeObject,
				Warnings: warnings,
			}) {
				return
			}
		}

		var object objectStream
		var usage yuzu.Usage
		var finishReason yuzu.FinishReason
		// responseID tracks the server-assigned response ID. It's first set from the
		// response.created event and may be overwritten by response.completed or
		// response.incomplete events. Per the OpenAI API contract, these IDs are
		// identical; the overwrites ensure we have the final value even if an event
		// is missed.
		var responseID string
		var sawTerminalEvent bool

		for stream.Next() {
			event := stream.Current()

			switch event.Type {
			case "response.created":
				created := event.AsResponseCreated()
				responseID = created.Response.ID

			case "response.output_text.delta":
				if !object.append(event.AsResponseOutputTextDelta().Delta, call, yield) {
					return
				}

			case "response.completed":
				sawTerminalEvent = true
				completed := event.AsResponseCompleted()
				responseID = completed.Response.ID
				finishReason = mapResponsesFinishReason(completed.Response.IncompleteDetails.Reason, false)
				var finalText strings.Builder
				for _, item := range completed.Response.Output {
					if item.Type == "message" {
						for _, content := range item.Content {
							if content.Type == "output_text" {
								finalText.WriteString(content.Text)
							}
						}
					}
				}
				if finalText.Len() > 0 {
					object.replace(finalText.String())
				}
				usage = responsesUsage(completed.Response)

			case "response.incomplete":
				sawTerminalEvent = true
				incomplete := event.AsResponseIncomplete()
				responseID = incomplete.Response.ID
				finishReason = mapResponsesFinishReason(incomplete.Response.IncompleteDetails.Reason, false)
				if finishReason == yuzu.FinishReasonStop {
					finishReason = yuzu.FinishReasonOther
				}
				usage = responsesUsage(incomplete.Response)

			case "response.failed":
				failed := event.AsResponseFailed()
				if !yield(yuzu.ObjectStreamPart{
					Type:  yuzu.ObjectStreamPartTypeError,
					Error: responsesFailedStreamError(failed.Response.Error.Message, string(failed.Response.Error.Code)),
				}) {
					return
				}
				return

			case "error":
				errorEvent := event.AsError()
				if !yield(yuzu.ObjectStreamPart{
					Type:  yuzu.ObjectStreamPartTypeError,
					Error: responsesErrorStreamError(errorEvent.Message, errorEvent.Code),
				}) {
					return
				}
				return
			}
		}

		err := stream.Err()
		if err != nil && !errors.Is(err, io.EOF) {
			yield(yuzu.ObjectStreamPart{
				Type:  yuzu.ObjectStreamPartTypeError,
				Error: toProviderErr(err),
			})
			return
		}

		metadata := o.applyHeaders(capture.header(), responsesProviderMetadata(responseID))
		object.finish(ctx, call, sawTerminalEvent, yuzu.ObjectStreamPart{
			Usage: usage, FinishReason: finishReason, ProviderMetadata: metadata,
		}, yield)
	}, nil
}
