package provider

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"net/http"
	"slices"
	"strings"
	"uuid"

	yuzu "clara-code/pkg/model"
	openai "clara-code/pkg/model/internal/wire"
	"clara-code/pkg/model/schema"
)

type languageModel struct {
	provider                   string
	modelID                    string
	client                     openai.Client
	objectMode                 yuzu.ObjectMode
	prepareCallFunc            LanguageModelPrepareCallFunc
	mapFinishReasonFunc        LanguageModelMapFinishReasonFunc
	extraContentFunc           LanguageModelExtraContentFunc
	usageFunc                  LanguageModelUsageFunc
	streamUsageFunc            LanguageModelStreamUsageFunc
	streamExtraFunc            LanguageModelStreamExtraFunc
	streamProviderMetadataFunc LanguageModelStreamProviderMetadataFunc
	headerFunc                 LanguageModelHeaderFunc
	toPromptFunc               LanguageModelToPromptFunc
}

// LanguageModelOption is a function that configures a languageModel.
type LanguageModelOption = func(*languageModel)

// WithLanguageModelPrepareCallFunc sets the prepare call function for the language model.
func WithLanguageModelPrepareCallFunc(fn LanguageModelPrepareCallFunc) LanguageModelOption {
	return func(l *languageModel) {
		l.prepareCallFunc = fn
	}
}

// WithLanguageModelMapFinishReasonFunc sets the map finish reason function for the language model.
func WithLanguageModelMapFinishReasonFunc(fn LanguageModelMapFinishReasonFunc) LanguageModelOption {
	return func(l *languageModel) {
		l.mapFinishReasonFunc = fn
	}
}

// WithLanguageModelExtraContentFunc sets the extra content function for the language model.
func WithLanguageModelExtraContentFunc(fn LanguageModelExtraContentFunc) LanguageModelOption {
	return func(l *languageModel) {
		l.extraContentFunc = fn
	}
}

// WithLanguageModelStreamExtraFunc sets the stream extra function for the language model.
func WithLanguageModelStreamExtraFunc(fn LanguageModelStreamExtraFunc) LanguageModelOption {
	return func(l *languageModel) {
		l.streamExtraFunc = fn
	}
}

// WithLanguageModelUsageFunc sets the usage function for the language model.
func WithLanguageModelUsageFunc(fn LanguageModelUsageFunc) LanguageModelOption {
	return func(l *languageModel) {
		l.usageFunc = fn
	}
}

// WithLanguageModelStreamUsageFunc sets the stream usage function for the language model.
func WithLanguageModelStreamUsageFunc(fn LanguageModelStreamUsageFunc) LanguageModelOption {
	return func(l *languageModel) {
		l.streamUsageFunc = fn
	}
}

// WithLanguageModelHeaderFunc sets the response header function for the
// language model. When set, the HTTP response headers of each call are
// captured and passed to the function alongside the provider metadata,
// which it may mutate (e.g. copying headers of interest into ExtraFields).
// When unset, response headers are not captured at all.
func WithLanguageModelHeaderFunc(fn LanguageModelHeaderFunc) LanguageModelOption {
	return func(l *languageModel) {
		l.headerFunc = fn
	}
}

// WithLanguageModelToPromptFunc sets the to prompt function for the language model.
func WithLanguageModelToPromptFunc(fn LanguageModelToPromptFunc) LanguageModelOption {
	return func(l *languageModel) {
		l.toPromptFunc = fn
	}
}

// WithLanguageModelObjectMode sets the object generation mode.
func WithLanguageModelObjectMode(om yuzu.ObjectMode) LanguageModelOption {
	return func(l *languageModel) {
		// not supported
		if om == yuzu.ObjectModeJSON {
			om = yuzu.ObjectModeAuto
		}
		l.objectMode = om
	}
}

func newLanguageModel(modelID string, provider string, client openai.Client, opts ...LanguageModelOption) languageModel {
	model := languageModel{
		modelID:                    modelID,
		provider:                   provider,
		client:                     client,
		objectMode:                 yuzu.ObjectModeAuto,
		prepareCallFunc:            DefaultPrepareCallFunc,
		mapFinishReasonFunc:        DefaultMapFinishReasonFunc,
		extraContentFunc:           DefaultExtraContentFunc,
		usageFunc:                  DefaultUsageFunc,
		streamUsageFunc:            DefaultStreamUsageFunc,
		streamExtraFunc:            DefaultStreamExtraFunc,
		streamProviderMetadataFunc: DefaultStreamProviderMetadataFunc,
		toPromptFunc:               DefaultToPrompt,
	}

	for _, o := range opts {
		o(&model)
	}
	return model
}

type streamToolCall struct {
	id          string
	name        string
	arguments   string
	hasFinished bool
}

// responseCapture holds the raw HTTP response of a call so response headers
// can be surfaced through provider metadata. Capturing is only enabled when
// a header func is configured on the language model.
type responseCapture struct {
	response *http.Response
}

// requestOptions returns the given per-call request options with the raw
// HTTP response capture appended when the given header func is configured.
func (c *responseCapture) requestOptions(headerFunc LanguageModelHeaderFunc, opts []openai.RequestOption) []openai.RequestOption {
	if headerFunc == nil {
		return opts
	}
	return append(
		opts,
		openai.WithResponseInto(&c.response),
		openai.WithMiddleware(drainOnCloseMiddleware),
	)
}

// drainOnCloseMiddleware wraps the response body so that a close before
// EOF — which the SSE stream does at its [DONE] sentinel — drains the
// remaining bytes first. net/http parses HTTP trailers only once the body
// has been read that far, so without the drain a trailer arriving after
// the stream's terminal event would be discarded.
func drainOnCloseMiddleware(req *http.Request, next openai.MiddlewareNext) (*http.Response, error) {
	res, err := next(req)
	if err != nil || res == nil || res.Body == nil {
		return res, err
	}
	res.Body = &drainOnCloseBody{ReadCloser: res.Body}
	return res, err
}

// drainOnCloseBody reads the wrapped body to EOF before closing it.
type drainOnCloseBody struct {
	io.ReadCloser
}

func (b *drainOnCloseBody) Close() error {
	_, _ = io.Copy(io.Discard, b.ReadCloser)
	return b.ReadCloser.Close()
}

// header returns the captured response headers, with any HTTP trailers
// merged in (trailer keys win on collision), or nil when no response was
// captured. The header func runs after the response body has been fully
// consumed, so trailers — which net/http populates only then — are visible.
func (c *responseCapture) header() http.Header {
	if c.response == nil {
		return nil
	}
	if len(c.response.Trailer) == 0 {
		return c.response.Header
	}
	merged := c.response.Header.Clone()
	maps.Copy(merged, c.response.Trailer)
	return merged
}

// languageModelHeaderFunc returns the header func configured through the
// given language model options, if any. It is the only language model
// option also honored by the responses language model.
func languageModelHeaderFunc(opts []LanguageModelOption) LanguageModelHeaderFunc {
	var lm languageModel
	for _, opt := range opts {
		opt(&lm)
	}
	return lm.headerFunc
}

// applyHeaders invokes the configured header func against the provider
// metadata, creating it when no call populated any. It is a no-op when no
// header func is configured or no response was captured.
func (o languageModel) applyHeaders(header http.Header, metadata yuzu.ProviderMetadata) yuzu.ProviderMetadata {
	if o.headerFunc == nil || header == nil {
		return metadata
	}
	o.headerFunc(header, &metadata)
	return metadata
}

// Model implements LanguageModel.
func (o languageModel) Model() string {
	return o.modelID
}

// Provider implements LanguageModel.
func (o languageModel) Provider() string {
	return o.provider
}

func (o languageModel) prepareParams(call yuzu.Call) (*openai.ChatCompletionNewParams, []yuzu.CallWarning, error) {
	if err := validatePromptRoles(call.Prompt); err != nil {
		return nil, nil, err
	}
	params := &openai.ChatCompletionNewParams{}
	messages, warnings := o.toPromptFunc(call.Prompt, o.provider, o.modelID)
	if call.TopK != nil {
		warnings = append(warnings, yuzu.CallWarning{
			Type:    yuzu.CallWarningTypeUnsupportedSetting,
			Setting: "top_k",
		})
	}

	if call.MaxOutputTokens != nil {
		params.MaxTokens = openai.NewOpt(*call.MaxOutputTokens)
	}
	if call.Temperature != nil {
		params.Temperature = openai.NewOpt(*call.Temperature)
	}
	if call.TopP != nil {
		params.TopP = openai.NewOpt(*call.TopP)
	}
	if call.FrequencyPenalty != nil {
		params.FrequencyPenalty = openai.NewOpt(*call.FrequencyPenalty)
	}
	if call.PresencePenalty != nil {
		params.PresencePenalty = openai.NewOpt(*call.PresencePenalty)
	}

	if isReasoningModel(o.modelID) {
		// remove unsupported settings for reasoning models
		// see https://platform.openai.com/docs/guides/reasoning#limitations
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
				Setting: "TopP",
				Details: "TopP is not supported for reasoning models",
			})
		}
		if call.FrequencyPenalty != nil {
			params.FrequencyPenalty = openai.Opt[float64]{}
			warnings = append(warnings, yuzu.CallWarning{
				Type:    yuzu.CallWarningTypeUnsupportedSetting,
				Setting: "FrequencyPenalty",
				Details: "FrequencyPenalty is not supported for reasoning models",
			})
		}
		if call.PresencePenalty != nil {
			params.PresencePenalty = openai.Opt[float64]{}
			warnings = append(warnings, yuzu.CallWarning{
				Type:    yuzu.CallWarningTypeUnsupportedSetting,
				Setting: "PresencePenalty",
				Details: "PresencePenalty is not supported for reasoning models",
			})
		}

		// reasoning models use max_completion_tokens instead of max_tokens
		if call.MaxOutputTokens != nil {
			if !params.MaxCompletionTokens.Valid() {
				params.MaxCompletionTokens = openai.NewOpt(*call.MaxOutputTokens)
			}
			params.MaxTokens = openai.Opt[int64]{}
		}
	}

	// Handle search preview models
	if isSearchPreviewModel(o.modelID) {
		if call.Temperature != nil {
			params.Temperature = openai.Opt[float64]{}
			warnings = append(warnings, yuzu.CallWarning{
				Type:    yuzu.CallWarningTypeUnsupportedSetting,
				Setting: "temperature",
				Details: "temperature is not supported for the search preview models and has been removed.",
			})
		}
	}

	optionsWarnings, err := o.prepareCallFunc(o, params, call)
	if err != nil {
		return nil, nil, err
	}

	if len(optionsWarnings) > 0 {
		warnings = append(warnings, optionsWarnings...)
	}

	params.Messages = messages
	params.Model = o.modelID

	if len(call.Tools) > 0 {
		tools, toolChoice, toolWarnings := toOpenAiTools(call.Tools, call.ToolChoice)
		params.Tools = tools
		if toolChoice != nil {
			params.ToolChoice = *toolChoice
		}
		warnings = append(warnings, toolWarnings...)
	}
	return params, warnings, nil
}

// Generate implements LanguageModel.
func (o languageModel) Generate(ctx context.Context, call yuzu.Call) (*yuzu.Response, error) {
	capture := responseCapture{}
	params, warnings, err := o.prepareParams(call)
	if err != nil {
		return nil, err
	}
	response, err := o.client.ChatCompletions(ctx, *params, capture.requestOptions(o.headerFunc, append(callUARequestOptions(call), callHeadersRequestOptions(call)...))...)
	if err != nil {
		return nil, toProviderErr(err)
	}
	if response == nil {
		return nil, &yuzu.Error{Title: "no response", Message: "provider returned nil response"}
	}

	if len(response.Choices) == 0 {
		return nil, &yuzu.Error{Title: "no response", Message: "no response generated"}
	}
	choice := response.Choices[0]
	content := make([]yuzu.Content, 0, 1+len(choice.Message.ToolCalls)+len(choice.Message.Annotations))
	text := choice.Message.Content
	if text != "" {
		content = append(content, yuzu.TextContent{
			Text: text,
		})
	}
	if o.extraContentFunc != nil {
		extraContent := o.extraContentFunc(choice)
		content = append(content, extraContent...)
	}

	usage, providerMetadata := o.usageFunc(*response)
	providerMetadata = o.applyHeaders(capture.header(), providerMetadata)

	mappedFinishReason := o.mapFinishReasonFunc(choice.FinishReason)
	// Terminal reasons that can cut output mid-call — length,
	// content_filter, provider errors — must not be rewritten into a
	// tool-call turn: dispatching their partial calls executes truncated
	// input (CHARM-2020).
	suppressedToolCalls := len(choice.Message.ToolCalls) > 0 &&
		(mappedFinishReason == yuzu.FinishReasonLength ||
			mappedFinishReason == yuzu.FinishReasonContentFilter ||
			mappedFinishReason == yuzu.FinishReasonError)
	if len(choice.Message.ToolCalls) > 0 && !suppressedToolCalls {
		mappedFinishReason = yuzu.FinishReasonToolCalls
	}
	if suppressedToolCalls {
		warnings = append(warnings, yuzu.CallWarning{
			Type:    yuzu.CallWarningTypeOther,
			Message: "tool calls were returned but the turn ended abnormally (token limit, content filter, or provider error); arguments may be truncated",
		})
	}

	// Suppress truncated tool-call content so agents don't dispatch
	// calls with incomplete arguments.
	if !suppressedToolCalls {
		for _, tc := range choice.Message.ToolCalls {
			toolCallID := tc.ID
			content = append(content, yuzu.ToolCallContent{
				ProviderExecuted: false,
				ToolCallID:       toolCallID,
				ToolName:         tc.Function.Name,
				Input:            tc.Function.Arguments,
			})
		}
	}
	for _, annotation := range choice.Message.Annotations {
		if annotation.Type == "url_citation" {
			content = append(content, yuzu.SourceContent{
				SourceType: yuzu.SourceTypeURL,
				ID:         uuid.New().String(),
				URL:        annotation.URLCitation.URL,
				Title:      annotation.URLCitation.Title,
			})
		}
	}

	return &yuzu.Response{
		Content:          content,
		Usage:            usage,
		FinishReason:     mappedFinishReason,
		ProviderMetadata: providerMetadata,
		Warnings:         warnings,
	}, nil
}

// Stream implements LanguageModel.
func (o languageModel) Stream(ctx context.Context, call yuzu.Call) (yuzu.StreamResponse, error) {
	capture := responseCapture{}
	params, warnings, err := o.prepareParams(call)
	if err != nil {
		return nil, err
	}

	params.StreamOptions = openai.ChatCompletionStreamOptionsParam{
		IncludeUsage: openai.Bool(true),
	}

	stream := o.client.ChatCompletionsStream(ctx, *params, capture.requestOptions(o.headerFunc, append(callUARequestOptions(call), callHeadersRequestOptions(call)...))...)
	isActiveText := false
	toolCalls := make(map[int64]streamToolCall)

	var providerMetadata yuzu.ProviderMetadata
	acc := openai.ChatCompletionAccumulator{}
	extraContext := make(map[string]any)
	var usage yuzu.Usage
	var finishReason string
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
			chunk := stream.Current()
			acc.AddChunk(chunk)
			usage, providerMetadata = o.streamUsageFunc(chunk, extraContext, providerMetadata)
			if len(chunk.Choices) == 0 {
				continue
			}
			for _, choice := range chunk.Choices {
				if choice.FinishReason != "" {
					finishReason = choice.FinishReason
				}
				if choice.Delta.Content != "" {
					if !isActiveText {
						isActiveText = true
						if !yield(yuzu.StreamPart{
							Type: yuzu.StreamPartTypeTextStart,
							ID:   "0",
						}) {
							return
						}
					}
					if !yield(yuzu.StreamPart{
						Type:  yuzu.StreamPartTypeTextDelta,
						ID:    "0",
						Delta: choice.Delta.Content,
					}) {
						return
					}
				}
				if len(choice.Delta.ToolCalls) > 0 {
					if isActiveText {
						isActiveText = false
						if !yield(yuzu.StreamPart{
							Type: yuzu.StreamPartTypeTextEnd,
							ID:   "0",
						}) {
							return
						}
					}

					for _, toolCallDelta := range choice.Delta.ToolCalls {
						if existingToolCall, ok := toolCalls[toolCallDelta.Index]; ok {
							if toolCallDelta.Function.Arguments != "" {
								existingToolCall.arguments += toolCallDelta.Function.Arguments
								if !yield(yuzu.StreamPart{
									Type:  yuzu.StreamPartTypeToolInputDelta,
									ID:    existingToolCall.id,
									Delta: toolCallDelta.Function.Arguments,
								}) {
									return
								}
							}
							toolCalls[toolCallDelta.Index] = existingToolCall
						} else {
							// Some provider like Ollama may send empty tool calls or miss some fields.
							// We'll skip when we don't have enough info and also assume sane defaults.
							if toolCallDelta.Function.Name == "" && toolCallDelta.Function.Arguments == "" {
								continue
							}
							toolCallDelta.Type = cmp.Or(toolCallDelta.Type, "function")
							toolCallDelta.ID = cmp.Or(toolCallDelta.ID, fmt.Sprintf("tool-call-%d", toolCallDelta.Index))

							if toolCallDelta.Type != "function" {
								yield(yuzu.StreamPart{
									Type:  yuzu.StreamPartTypeError,
									Error: &yuzu.Error{Title: "invalid provider response", Message: "expected 'function' type."},
								})
								return
							}

							if !yield(yuzu.StreamPart{
								Type:         yuzu.StreamPartTypeToolInputStart,
								ID:           toolCallDelta.ID,
								ToolCallName: toolCallDelta.Function.Name,
							}) {
								return
							}
							toolCalls[toolCallDelta.Index] = streamToolCall{
								id:        toolCallDelta.ID,
								name:      toolCallDelta.Function.Name,
								arguments: toolCallDelta.Function.Arguments,
							}

							if toolCallDelta.Function.Arguments != "" {
								if !yield(yuzu.StreamPart{
									Type:  yuzu.StreamPartTypeToolInputDelta,
									ID:    toolCallDelta.ID,
									Delta: toolCallDelta.Function.Arguments,
								}) {
									return
								}
							}
							continue
						}
					}
				}
			}

			// The extra hook receives the whole chunk and iterates choices
			// itself; calling it per choice would duplicate its events once
			// per additional choice.
			if o.streamExtraFunc != nil {
				updatedContext, shouldContinue := o.streamExtraFunc(chunk, yield, extraContext)
				if !shouldContinue {
					return
				}
				extraContext = updatedContext
			}
			for _, choice := range chunk.Choices {
				if annotations := parseAnnotationsFromDelta(choice.Delta); len(annotations) > 0 {
					for _, annotation := range annotations {
						if annotation.Type == "url_citation" {
							if !yield(yuzu.StreamPart{
								Type:       yuzu.StreamPartTypeSource,
								ID:         uuid.New().String(),
								SourceType: yuzu.SourceTypeURL,
								URL:        annotation.URLCitation.URL,
								Title:      annotation.URLCitation.Title,
							}) {
								return
							}
						}
					}
				}
			}
		}
		err := stream.Err()
		if err == nil || errors.Is(err, io.EOF) {
			if isActiveText {
				isActiveText = false
				if !yield(yuzu.StreamPart{
					Type: yuzu.StreamPartTypeTextEnd,
					ID:   "0",
				}) {
					return
				}
			}

			// Evaluate the finish reason before emitting tool calls so we can
			// suppress them when the response was truncated (finish_reason=length).
			// Emitting partial tool calls causes agents to dispatch them with
			// invalid arguments before seeing the terminal reason.
			mappedFinishReason := o.mapFinishReasonFunc(finishReason)

			// "Tool calls were seen" is not proof of a complete turn. Infer a
			// tool-call turn only when the upstream said tool_calls/function_call
			// explicitly (kept verbatim by the mapper) or sent no finish reason
			// at all and every accumulated call's arguments parse as complete
			// JSON. Terminal reasons that can cut output mid-call — length,
			// content_filter, provider errors — must never be rewritten into a
			// tool-call turn: dispatching their partial calls executes truncated
			// input (CHARM-2020).
			var missingFinishWithBadArgs bool
			var missingFinish bool
			if finishReason == "" && len(toolCalls) > 0 {
				missingFinish = true
				for _, tc := range toolCalls {
					// A call with no arguments was cut before any argument
					// arrived; filling in "{}" would invent arguments the model
					// never sent.
					if tc.arguments == "" || !json.Valid([]byte(tc.arguments)) {
						missingFinishWithBadArgs = true
						break
					}
				}
			}

			if finishReason == "" && len(acc.Choices) > 0 && !missingFinishWithBadArgs {
				if len(acc.Choices[0].Message.ToolCalls) > 0 {
					mappedFinishReason = yuzu.FinishReasonToolCalls
				}
			}
			suppressedWithToolCalls := (mappedFinishReason == yuzu.FinishReasonLength ||
				mappedFinishReason == yuzu.FinishReasonError ||
				mappedFinishReason == yuzu.FinishReasonContentFilter ||
				missingFinishWithBadArgs) && len(toolCalls) > 0

			// A cut stream with unusable partial calls errors out before the
			// finalizer runs: emitting ToolInputEnd after backfilling "{}" would
			// present fabricated completed input to consumers (CHARM-2020).
			if missingFinishWithBadArgs {
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

			// Finalize tool calls in index order after the stream completes.
			// When truncated, skip ToolCall parts to prevent agents from
			// dispatching calls with incomplete arguments.
			indices := make([]int64, 0, len(toolCalls))
			for idx := range toolCalls {
				indices = append(indices, idx)
			}
			slices.Sort(indices)
			for _, idx := range indices {
				tc := toolCalls[idx]
				if !tc.hasFinished {
					if tc.arguments == "" {
						tc.arguments = "{}"
						toolCalls[idx] = tc
					}
					if !yield(yuzu.StreamPart{Type: yuzu.StreamPartTypeToolInputEnd, ID: tc.id}) {
						return
					}
				}
				if !suppressedWithToolCalls {
					if !yield(yuzu.StreamPart{Type: yuzu.StreamPartTypeToolCall, ID: tc.id, ToolCallName: tc.name, ToolCallInput: tc.arguments}) {
						return
					}
				}
				tc.hasFinished = true
				toolCalls[idx] = tc
			}

			if len(acc.Choices) > 0 {
				choice := acc.Choices[0]
				providerMetadata = o.streamProviderMetadataFunc(choice, providerMetadata)

				for _, annotation := range choice.Message.Annotations {
					if annotation.Type == "url_citation" {
						if !yield(yuzu.StreamPart{
							Type:       yuzu.StreamPartTypeSource,
							ID:         acc.ID,
							SourceType: yuzu.SourceTypeURL,
							URL:        annotation.URLCitation.URL,
							Title:      annotation.URLCitation.Title,
						}) {
							return
						}
					}
				}
			}
			// Truncated stream: upstream closed without finish_reason and we
			// can't infer a tool-call turn. Surface as a retryable error so
			// the retry middleware re-runs the step.
			if finishReason == "" && mappedFinishReason != yuzu.FinishReasonToolCalls {
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
			if missingFinish && !missingFinishWithBadArgs {
				yield(yuzu.StreamPart{
					Type: yuzu.StreamPartTypeWarnings,
					Warnings: []yuzu.CallWarning{{
						Type:    yuzu.CallWarningTypeOther,
						Message: "stream ended without finish_reason; assuming tool-call turn",
					}},
				})
			}
			if suppressedWithToolCalls && !missingFinishWithBadArgs {
				yield(yuzu.StreamPart{
					Type: yuzu.StreamPartTypeWarnings,
					Warnings: []yuzu.CallWarning{{
						Type:    yuzu.CallWarningTypeOther,
						Message: "tool calls were returned but the turn ended abnormally (token limit, content filter, or provider error); arguments may be truncated",
					}},
				})
			}
			providerMetadata = o.applyHeaders(capture.header(), providerMetadata)
			yield(yuzu.StreamPart{
				Type:             yuzu.StreamPartTypeFinish,
				Usage:            usage,
				FinishReason:     mappedFinishReason,
				ProviderMetadata: providerMetadata,
			})
			return
		} else { //nolint: revive
			yield(yuzu.StreamPart{
				Type:  yuzu.StreamPartTypeError,
				Error: toProviderErr(err),
			})
			return
		}
	}, nil
}

func isReasoningModel(modelID string) bool {
	return strings.HasPrefix(modelID, "o1") || strings.Contains(modelID, "-o1") ||
		strings.HasPrefix(modelID, "o3") || strings.Contains(modelID, "-o3") ||
		strings.HasPrefix(modelID, "o4") || strings.Contains(modelID, "-o4") ||
		strings.HasPrefix(modelID, "oss") || strings.Contains(modelID, "-oss") ||
		strings.Contains(strings.ToLower(modelID), "gpt-5")
}

func isSearchPreviewModel(modelID string) bool {
	return strings.Contains(modelID, "search-preview")
}

func supportsFlexProcessing(modelID string) bool {
	return strings.HasPrefix(modelID, "o3") || strings.Contains(modelID, "-o3") ||
		strings.Contains(modelID, "o4-mini") ||
		strings.Contains(strings.ToLower(modelID), "gpt-5")
}

func supportsPriorityProcessing(modelID string) bool {
	return strings.Contains(strings.ToLower(modelID), "gpt-4") ||
		strings.Contains(strings.ToLower(modelID), "gpt-5") ||
		strings.HasPrefix(modelID, "o3") ||
		strings.Contains(modelID, "-o3") ||
		strings.Contains(modelID, "o4-mini")
}

func toOpenAiTools(tools []yuzu.Tool, toolChoice *yuzu.ToolChoice) (openAiTools []openai.ChatCompletionToolUnionParam, openAiToolChoice *openai.ChatCompletionToolChoiceOptionUnionParam, warnings []yuzu.CallWarning) {
	for _, tool := range tools {
		if tool.GetType() == yuzu.ToolTypeFunction {
			ft, ok := tool.(yuzu.FunctionTool)
			if !ok {
				continue
			}
			openAiTools = append(openAiTools, openai.ChatCompletionToolUnionParam{
				OfFunction: &openai.ChatCompletionFunctionToolParam{
					Function: openai.FunctionDefinitionParam{
						Name:        ft.Name,
						Description: openai.NewOpt(ft.Description),
						Parameters:  openai.FunctionParameters(ft.InputSchema),
						Strict:      openai.NewOpt(false),
					},
					Type: "function",
				},
			})
			continue
		}

		warnings = append(warnings, yuzu.CallWarning{
			Type:    yuzu.CallWarningTypeUnsupportedTool,
			Tool:    tool,
			Message: "tool is not supported",
		})
	}
	if toolChoice == nil {
		return openAiTools, openAiToolChoice, warnings
	}

	switch *toolChoice {
	case yuzu.ToolChoiceAuto:
		openAiToolChoice = &openai.ChatCompletionToolChoiceOptionUnionParam{
			OfAuto: openai.NewOpt("auto"),
		}
	case yuzu.ToolChoiceNone:
		openAiToolChoice = &openai.ChatCompletionToolChoiceOptionUnionParam{
			OfAuto: openai.NewOpt("none"),
		}
	case yuzu.ToolChoiceRequired:
		openAiToolChoice = &openai.ChatCompletionToolChoiceOptionUnionParam{
			OfAuto: openai.NewOpt("required"),
		}
	default:
		openAiToolChoice = &openai.ChatCompletionToolChoiceOptionUnionParam{
			OfFunctionToolChoice: &openai.ChatCompletionNamedToolChoiceParam{
				Type: "function",
				Function: openai.ChatCompletionNamedToolChoiceFunctionParam{
					Name: string(*toolChoice),
				},
			},
		}
	}
	return openAiTools, openAiToolChoice, warnings
}

// parseAnnotationsFromDelta parses annotations from the raw JSON of a delta.
func parseAnnotationsFromDelta(delta openai.ChatCompletionChunkChoiceDelta) []openai.ChatCompletionMessageAnnotation {
	var annotations []openai.ChatCompletionMessageAnnotation

	// Parse the raw JSON to extract annotations
	var deltaData map[string]any
	if err := json.Unmarshal([]byte(delta.RawJSON()), &deltaData); err != nil {
		return annotations
	}

	// Check if annotations exist in the delta
	if annotationsData, ok := deltaData["annotations"].([]any); ok {
		for _, annotationData := range annotationsData {
			if annotationMap, ok := annotationData.(map[string]any); ok {
				if annotationType, ok := annotationMap["type"].(string); ok && annotationType == "url_citation" {
					if urlCitationData, ok := annotationMap["url_citation"].(map[string]any); ok {
						url, urlOk := urlCitationData["url"].(string)
						title, titleOk := urlCitationData["title"].(string)
						if urlOk && titleOk {
							annotation := openai.ChatCompletionMessageAnnotation{
								Type: "url_citation",
								URLCitation: openai.ChatCompletionMessageAnnotationURLCitation{
									URL:   url,
									Title: title,
								},
							}
							annotations = append(annotations, annotation)
						}
					}
				}
			}
		}
	}

	return annotations
}

// GenerateObject implements LanguageModel.
func (o languageModel) GenerateObject(ctx context.Context, call yuzu.ObjectCall) (*yuzu.ObjectResponse, error) {
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
func (o languageModel) StreamObject(ctx context.Context, call yuzu.ObjectCall) (yuzu.ObjectStreamResponse, error) {
	switch o.objectMode {
	case yuzu.ObjectModeTool:
		return streamObjectWithTool(ctx, o, call)
	case yuzu.ObjectModeText:
		return streamObjectWithText(ctx, o, call)
	default:
		return o.streamObjectWithJSONMode(ctx, call)
	}
}

func (o languageModel) generateObjectWithJSONMode(ctx context.Context, call yuzu.ObjectCall) (*yuzu.ObjectResponse, error) {
	jsonSchemaMap := schema.ToMap(call.Schema)

	addAdditionalPropertiesFalse(jsonSchemaMap)

	schemaName := call.SchemaName
	if schemaName == "" {
		schemaName = "response"
	}

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

	params.ResponseFormat = openai.ChatCompletionNewParamsResponseFormatUnion{
		OfJSONSchema: &openai.ResponseFormatJSONSchemaParam{
			JSONSchema: openai.ResponseFormatJSONSchemaJSONSchemaParam{
				Name:        schemaName,
				Description: openai.NewOpt(call.SchemaDescription),
				Schema:      jsonSchemaMap,
				Strict:      openai.NewOpt(true),
			},
		},
	}

	response, err := o.client.ChatCompletions(ctx, *params, append(objectCallUARequestOptions(call), objectCallHeadersRequestOptions(call)...)...)
	if err != nil {
		return nil, toProviderErr(err)
	}
	if len(response.Choices) == 0 {
		usage, _ := o.usageFunc(*response)
		return nil, &yuzu.NoObjectGeneratedError{
			RawText:      "",
			ParseError:   fmt.Errorf("no choices in response"),
			Usage:        usage,
			FinishReason: yuzu.FinishReasonUnknown,
		}
	}

	choice := response.Choices[0]
	jsonText := choice.Message.Content

	var obj any
	if call.RepairText != nil {
		obj, err = schema.ParseAndValidateWithRepair(ctx, jsonText, call.Schema, call.RepairText)
	} else {
		obj, err = schema.ParseAndValidate(jsonText, call.Schema)
	}

	usage, _ := o.usageFunc(*response)
	finishReason := o.mapFinishReasonFunc(choice.FinishReason)

	if err != nil {
		if nogErr, ok := errors.AsType[*yuzu.NoObjectGeneratedError](err); ok {
			nogErr.Usage = usage
			nogErr.FinishReason = finishReason
		}
		return nil, err
	}

	return &yuzu.ObjectResponse{
		Object:       obj,
		RawText:      jsonText,
		Usage:        usage,
		FinishReason: finishReason,
		Warnings:     warnings,
	}, nil
}

func (o languageModel) streamObjectWithJSONMode(ctx context.Context, call yuzu.ObjectCall) (yuzu.ObjectStreamResponse, error) {
	jsonSchemaMap := schema.ToMap(call.Schema)

	addAdditionalPropertiesFalse(jsonSchemaMap)

	schemaName := call.SchemaName
	if schemaName == "" {
		schemaName = "response"
	}

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

	params.ResponseFormat = openai.ChatCompletionNewParamsResponseFormatUnion{
		OfJSONSchema: &openai.ResponseFormatJSONSchemaParam{
			JSONSchema: openai.ResponseFormatJSONSchemaJSONSchemaParam{
				Name:        schemaName,
				Description: openai.NewOpt(call.SchemaDescription),
				Schema:      jsonSchemaMap,
				Strict:      openai.NewOpt(true),
			},
		},
	}

	params.StreamOptions = openai.ChatCompletionStreamOptionsParam{
		IncludeUsage: openai.Bool(true),
	}

	stream := o.client.ChatCompletionsStream(ctx, *params, append(objectCallUARequestOptions(call), objectCallHeadersRequestOptions(call)...)...)

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
		var sawFinishReason bool
		var providerMetadata yuzu.ProviderMetadata

		for stream.Next() {
			chunk := stream.Current()

			// Update usage
			usage, providerMetadata = o.streamUsageFunc(chunk, make(map[string]any), providerMetadata)

			if len(chunk.Choices) == 0 {
				continue
			}

			choice := chunk.Choices[0]
			if choice.FinishReason != "" {
				finishReason = o.mapFinishReasonFunc(choice.FinishReason)
				sawFinishReason = true
			}

			if delta := choice.Delta.Content; delta != "" && !object.append(delta, call, yield) {
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

		object.finish(ctx, call, sawFinishReason, yuzu.ObjectStreamPart{
			Usage: usage, FinishReason: finishReason, ProviderMetadata: providerMetadata,
		}, yield)
	}, nil
}

// addAdditionalPropertiesFalse recursively adds "additionalProperties": false to all object schemas.
// This is required by OpenAI's strict mode for structured outputs.
func addAdditionalPropertiesFalse(schema map[string]any) {
	if schema["type"] == "object" {
		if _, hasAdditional := schema["additionalProperties"]; !hasAdditional {
			schema["additionalProperties"] = false
		}

		// Recursively process nested properties
		if properties, ok := schema["properties"].(map[string]any); ok {
			for _, propValue := range properties {
				if propSchema, ok := propValue.(map[string]any); ok {
					addAdditionalPropertiesFalse(propSchema)
				}
			}
		}
	}

	// Handle array items
	if items, ok := schema["items"].(map[string]any); ok {
		addAdditionalPropertiesFalse(items)
	}
}
