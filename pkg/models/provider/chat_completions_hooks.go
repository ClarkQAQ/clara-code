package provider

import (
	yuzu "clara-code/pkg/models"
	openai "clara-code/pkg/models/internal/wire"
	"fmt"
	"net/http"
	"strings"
)

// LanguageModelPrepareCallFunc is a function that prepares the call for the language model.
type LanguageModelPrepareCallFunc = func(model yuzu.LanguageModel, params *openai.ChatCompletionNewParams, call yuzu.Call) ([]yuzu.CallWarning, error)

// LanguageModelMapFinishReasonFunc is a function that maps the finish reason for the language model.
type LanguageModelMapFinishReasonFunc = func(finishReason string) yuzu.FinishReason

// LanguageModelUsageFunc is a function that calculates usage for the language model.
type LanguageModelUsageFunc = func(choice openai.ChatCompletion) (yuzu.Usage, yuzu.ProviderMetadata)

// LanguageModelExtraContentFunc is a function that adds extra content for the language model.
type LanguageModelExtraContentFunc = func(choice openai.ChatCompletionChoice) []yuzu.Content

// LanguageModelStreamExtraFunc is a function that handles stream extra functionality for the language model.
type LanguageModelStreamExtraFunc = func(chunk openai.ChatCompletionChunk, yield func(yuzu.StreamPart) bool, ctx map[string]any) (map[string]any, bool)

// LanguageModelStreamUsageFunc is a function that calculates stream usage for the language model.
type LanguageModelStreamUsageFunc = func(chunk openai.ChatCompletionChunk, ctx map[string]any, metadata yuzu.ProviderMetadata) (yuzu.Usage, yuzu.ProviderMetadata)

// LanguageModelStreamProviderMetadataFunc is a function that handles stream provider metadata for the language model.
type LanguageModelStreamProviderMetadataFunc = func(choice openai.ChatCompletionChoice, metadata yuzu.ProviderMetadata) yuzu.ProviderMetadata

// LanguageModelHeaderFunc is a function that receives the HTTP response
// headers of a completed call. It may copy headers of interest into the
// provider metadata, so they are surfaced to callers through
// [yuzu.Response] and the terminal [yuzu.StreamPart].
type LanguageModelHeaderFunc = func(header http.Header, metadata *yuzu.ProviderMetadata)

// LanguageModelToPromptFunc converts prompts to OpenAI SDK messages.
type LanguageModelToPromptFunc = func(prompt yuzu.Prompt, provider, model string) ([]openai.ChatCompletionMessageParamUnion, []yuzu.CallWarning)

// DefaultPrepareCallFunc is the default implementation for preparing a call to the language model.
func DefaultPrepareCallFunc(model yuzu.LanguageModel, params *openai.ChatCompletionNewParams, call yuzu.Call) ([]yuzu.CallWarning, error) {
	providerOptions := call.ProviderOptions
	warnings := settingsWarnings(unsupportedChatCompletionsSettings(providerOptions))

	if len(providerOptions.ExtraBody) > 0 {
		params.SetExtraFields(providerOptions.ExtraBody)
	}

	if providerOptions.LogitBias != nil {
		params.LogitBias = providerOptions.LogitBias
	}
	if providerOptions.LogProbs != nil && providerOptions.TopLogProbs != nil {
		providerOptions.LogProbs = nil
	}
	if providerOptions.LogProbs != nil {
		params.Logprobs = openai.NewOpt(*providerOptions.LogProbs)
	}
	if providerOptions.TopLogProbs != nil {
		params.TopLogprobs = openai.NewOpt(*providerOptions.TopLogProbs)
	}
	if providerOptions.User != nil {
		params.User = openai.NewOpt(*providerOptions.User)
	}
	if providerOptions.ParallelToolCalls != nil {
		params.ParallelToolCalls = openai.NewOpt(*providerOptions.ParallelToolCalls)
	}
	if providerOptions.MaxCompletionTokens != nil {
		params.MaxCompletionTokens = openai.NewOpt(*providerOptions.MaxCompletionTokens)
	}

	if providerOptions.TextVerbosity != nil {
		params.Verbosity = openai.ChatCompletionNewParamsVerbosity(*providerOptions.TextVerbosity)
	}
	if providerOptions.Prediction != nil {
		// Convert map[string]any to ChatCompletionPredictionContentParam
		if content, ok := providerOptions.Prediction["content"]; ok {
			if contentStr, ok := content.(string); ok {
				params.Prediction = openai.ChatCompletionPredictionContentParam{
					Content: openai.ChatCompletionPredictionContentContentUnionParam{
						OfString: openai.NewOpt(contentStr),
					},
				}
			}
		}
	}
	if providerOptions.Store != nil {
		params.Store = openai.NewOpt(*providerOptions.Store)
	}
	if providerOptions.Metadata != nil {
		// Convert map[string]any to map[string]string
		metadata := make(map[string]string)
		for k, v := range providerOptions.Metadata {
			if str, ok := v.(string); ok {
				metadata[k] = str
			}
		}
		params.Metadata = metadata
	}
	if providerOptions.PromptCacheKey != nil {
		params.PromptCacheKey = openai.NewOpt(*providerOptions.PromptCacheKey)
	}
	if providerOptions.SafetyIdentifier != nil {
		params.SafetyIdentifier = openai.NewOpt(*providerOptions.SafetyIdentifier)
	}
	if providerOptions.ServiceTier != nil {
		params.ServiceTier = openai.ChatCompletionNewParamsServiceTier(*providerOptions.ServiceTier)
	}

	if providerOptions.ReasoningEffort != nil {
		switch *providerOptions.ReasoningEffort {
		case yuzu.ReasoningEffortNone:
			params.ReasoningEffort = openai.ReasoningEffortNone
		case yuzu.ReasoningEffortMinimal:
			params.ReasoningEffort = openai.ReasoningEffortMinimal
		case yuzu.ReasoningEffortLow:
			params.ReasoningEffort = openai.ReasoningEffortLow
		case yuzu.ReasoningEffortMedium:
			params.ReasoningEffort = openai.ReasoningEffortMedium
		case yuzu.ReasoningEffortHigh:
			params.ReasoningEffort = openai.ReasoningEffortHigh
		case yuzu.ReasoningEffortXHigh:
			params.ReasoningEffort = openai.ReasoningEffortXhigh
		case yuzu.ReasoningEffortMax:
			params.ReasoningEffort = openai.ReasoningEffortMax
		default:
			return nil, fmt.Errorf("reasoning model `%s` not supported", *providerOptions.ReasoningEffort)
		}
	}

	if isReasoningModel(model.Model()) {
		if providerOptions.LogitBias != nil {
			params.LogitBias = nil
			warnings = append(warnings, yuzu.CallWarning{
				Type:    yuzu.CallWarningTypeUnsupportedSetting,
				Setting: "LogitBias",
				Message: "LogitBias is not supported for reasoning models",
			})
		}
		if providerOptions.LogProbs != nil {
			params.Logprobs = openai.Opt[bool]{}
			warnings = append(warnings, yuzu.CallWarning{
				Type:    yuzu.CallWarningTypeUnsupportedSetting,
				Setting: "Logprobs",
				Message: "Logprobs is not supported for reasoning models",
			})
		}
		if providerOptions.TopLogProbs != nil {
			params.TopLogprobs = openai.Opt[int64]{}
			warnings = append(warnings, yuzu.CallWarning{
				Type:    yuzu.CallWarningTypeUnsupportedSetting,
				Setting: "TopLogprobs",
				Message: "TopLogprobs is not supported for reasoning models",
			})
		}
	}

	// Handle service tier validation
	if providerOptions.ServiceTier != nil {
		serviceTier := *providerOptions.ServiceTier
		if serviceTier == yuzu.ServiceTierFlex && !supportsFlexProcessing(model.Model()) {
			params.ServiceTier = ""
			warnings = append(warnings, yuzu.CallWarning{
				Type:    yuzu.CallWarningTypeUnsupportedSetting,
				Setting: "ServiceTier",
				Details: "flex processing is only available for o3, o4-mini, and gpt-5 models",
			})
		} else if serviceTier == yuzu.ServiceTierPriority && !supportsPriorityProcessing(model.Model()) {
			params.ServiceTier = ""
			warnings = append(warnings, yuzu.CallWarning{
				Type:    yuzu.CallWarningTypeUnsupportedSetting,
				Setting: "ServiceTier",
				Details: "priority processing is only available for supported models (gpt-4, gpt-5, gpt-5-mini, o3, o4-mini) and requires Enterprise access. gpt-5-nano is not supported",
			})
		}
	}
	return warnings, nil
}

// DefaultMapFinishReasonFunc is the default implementation for mapping finish reasons.
func DefaultMapFinishReasonFunc(finishReason string) yuzu.FinishReason {
	switch finishReason {
	case "stop":
		return yuzu.FinishReasonStop
	case "length":
		return yuzu.FinishReasonLength
	case "content_filter":
		return yuzu.FinishReasonContentFilter
	case "function_call", "tool_calls":
		return yuzu.FinishReasonToolCalls
	case "insufficient_system_resource":
		// DeepSeek-family upstreams report this when the request could not
		// be served; the partial output (including open tool calls) must be
		// treated like a truncation, never a completed turn.
		return yuzu.FinishReasonError
	default:
		return yuzu.FinishReasonUnknown
	}
}

// DefaultUsageFunc is the default implementation for calculating usage.
func DefaultUsageFunc(response openai.ChatCompletion) (yuzu.Usage, yuzu.ProviderMetadata) {
	completionTokenDetails := response.Usage.CompletionTokensDetails
	promptTokenDetails := response.Usage.PromptTokensDetails

	// Build provider metadata
	var providerMetadata yuzu.ProviderMetadata

	// Add logprobs if available
	if len(response.Choices) > 0 && len(response.Choices[0].Logprobs.Content) > 0 {
		providerMetadata.Logprobs = toChatTokenLogprobs(response.Choices[0].Logprobs.Content)
	}

	// Add prediction tokens if available
	if completionTokenDetails.AcceptedPredictionTokens > 0 || completionTokenDetails.RejectedPredictionTokens > 0 {
		if completionTokenDetails.AcceptedPredictionTokens > 0 {
			providerMetadata.AcceptedPredictionTokens = completionTokenDetails.AcceptedPredictionTokens
		}
		if completionTokenDetails.RejectedPredictionTokens > 0 {
			providerMetadata.RejectedPredictionTokens = completionTokenDetails.RejectedPredictionTokens
		}
	}
	// OpenAI reports prompt_tokens INCLUDING cached tokens. Subtract to avoid double-counting.
	inputTokens := max(response.Usage.PromptTokens-promptTokenDetails.CachedTokens, 0)
	providerMetadata.ExtraFields = ExtractExtraFields(response.Usage.ExtraFields)
	return yuzu.Usage{
		InputTokens:     inputTokens,
		OutputTokens:    response.Usage.CompletionTokens,
		TotalTokens:     response.Usage.TotalTokens,
		ReasoningTokens: completionTokenDetails.ReasoningTokens,
		CacheReadTokens: promptTokenDetails.CachedTokens,
	}, providerMetadata
}

// DefaultStreamUsageFunc is the default implementation for calculating stream usage.
func DefaultStreamUsageFunc(chunk openai.ChatCompletionChunk, _ map[string]any, metadata yuzu.ProviderMetadata) (yuzu.Usage, yuzu.ProviderMetadata) {
	if chunk.Usage.TotalTokens == 0 {
		return yuzu.Usage{}, metadata
	}
	streamProviderMetadata := metadata
	// we do this here because the acc does not add prompt details
	completionTokenDetails := chunk.Usage.CompletionTokensDetails
	promptTokenDetails := chunk.Usage.PromptTokensDetails
	// OpenAI reports prompt_tokens INCLUDING cached tokens. Subtract to avoid double-counting.
	inputTokens := max(chunk.Usage.PromptTokens-promptTokenDetails.CachedTokens, 0)
	usage := yuzu.Usage{
		InputTokens:     inputTokens,
		OutputTokens:    chunk.Usage.CompletionTokens,
		TotalTokens:     chunk.Usage.TotalTokens,
		ReasoningTokens: completionTokenDetails.ReasoningTokens,
		CacheReadTokens: promptTokenDetails.CachedTokens,
	}

	// Add prediction tokens if available
	if completionTokenDetails.AcceptedPredictionTokens > 0 || completionTokenDetails.RejectedPredictionTokens > 0 {
		if completionTokenDetails.AcceptedPredictionTokens > 0 {
			streamProviderMetadata.AcceptedPredictionTokens = completionTokenDetails.AcceptedPredictionTokens
		}
		if completionTokenDetails.RejectedPredictionTokens > 0 {
			streamProviderMetadata.RejectedPredictionTokens = completionTokenDetails.RejectedPredictionTokens
		}
	}

	streamProviderMetadata.ExtraFields = ExtractExtraFields(chunk.Usage.ExtraFields)

	return usage, streamProviderMetadata
}

// DefaultStreamProviderMetadataFunc is the default implementation for handling stream provider metadata.
func DefaultStreamProviderMetadataFunc(choice openai.ChatCompletionChoice, metadata yuzu.ProviderMetadata) yuzu.ProviderMetadata {
	metadata.Logprobs = toChatTokenLogprobs(choice.Logprobs.Content)
	return metadata
}

// toChatTokenLogprobs converts the wire logprobs of the provider into the
// yuzu representation.
func toChatTokenLogprobs(logprobs []openai.ChatCompletionTokenLogprob) []yuzu.ChatTokenLogprob {
	if len(logprobs) == 0 {
		return nil
	}
	out := make([]yuzu.ChatTokenLogprob, len(logprobs))
	for i, logprob := range logprobs {
		out[i] = yuzu.ChatTokenLogprob{
			Token:   logprob.Token,
			Bytes:   logprob.Bytes,
			Logprob: logprob.Logprob,
		}
		for _, top := range logprob.TopLogprobs {
			out[i].TopLogprobs = append(out[i].TopLogprobs, yuzu.ChatTokenTopLogprob{
				Token:   top.Token,
				Bytes:   top.Bytes,
				Logprob: top.Logprob,
			})
		}
	}
	return out
}

// ToolResultMediaMessages maps a tool-result media output to the chat
// completions messages that convey it. OpenAI tool messages can only carry
// text, so this emits a text tool message (using any accompanying text, or a
// placeholder describing the media) to keep the tool_call/tool_result pairing
// valid, followed by a synthetic user message holding the actual image or
// audio content part so vision- and audio-capable models can see it.
//
// Unsupported media types produce only the text tool message plus a warning.
// This is shared with OpenAI-compatible providers, which face the same
// constraint.
func ToolResultMediaMessages(output yuzu.ToolResultOutputContentMedia, toolCallID string) ([]openai.ChatCompletionMessageParamUnion, []yuzu.CallWarning) {
	messages := []openai.ChatCompletionMessageParamUnion{toolResultMediaPlaceholderMessage(output, toolCallID)}

	userMessage, warning, emit := toolResultMediaUserMessage(output)
	if warning != nil {
		return messages, []yuzu.CallWarning{*warning}
	}
	if emit {
		messages = append(messages, userMessage)
	}
	return messages, nil
}

// toolResultMediaPlaceholderMessage is the text tool message that answers a
// tool call whose output is media. Chat completions tool messages cannot carry
// image or audio content, so the media itself travels in the user message
// built by toolResultMediaUserMessage.
func toolResultMediaPlaceholderMessage(output yuzu.ToolResultOutputContentMedia, toolCallID string) openai.ChatCompletionMessageParamUnion {
	placeholder := output.Text
	if placeholder == "" {
		placeholder = fmt.Sprintf("The tool returned %s content; see the following user message.", output.MediaType)
	}
	return openai.ToolMessage(placeholder, toolCallID)
}

// toolResultMediaUserMessage wraps the media content part of a tool result in
// the synthetic user message a vision- or audio-capable model can read. It
// returns the message, an optional warning, and whether the caller should emit
// the returned message.
func toolResultMediaUserMessage(output yuzu.ToolResultOutputContentMedia) (openai.ChatCompletionMessageParamUnion, *yuzu.CallWarning, bool) {
	mediaPart, warning, emit := toolResultMediaUserPart(output)
	if warning != nil || !emit {
		return openai.ChatCompletionMessageParamUnion{}, warning, false
	}
	return openai.UserMessage([]openai.ChatCompletionContentPartUnionParam{mediaPart}), nil, true
}

// toolResultMediaUserPart maps a tool-result media output to an OpenAI chat
// completions user content part. It returns the content part, an optional
// warning, and whether the caller should emit the returned part.
func toolResultMediaUserPart(output yuzu.ToolResultOutputContentMedia) (openai.ChatCompletionContentPartUnionParam, *yuzu.CallWarning, bool) {
	switch {
	case strings.HasPrefix(output.MediaType, "image/"):
		data := "data:" + output.MediaType + ";base64," + output.Data
		imageBlock := openai.ChatCompletionContentPartImageParam{
			ImageURL: openai.ChatCompletionContentPartImageImageURLParam{URL: data},
		}
		return openai.ChatCompletionContentPartUnionParam{OfImageURL: &imageBlock}, nil, true
	case output.MediaType == "audio/wav", output.MediaType == "audio/mpeg", output.MediaType == "audio/mp3":
		format := "wav"
		if output.MediaType != "audio/wav" {
			format = "mp3"
		}
		audioBlock := openai.ChatCompletionContentPartInputAudioParam{
			InputAudio: openai.ChatCompletionContentPartInputAudioInputAudioParam{
				Data:   output.Data,
				Format: format,
			},
		}
		return openai.ChatCompletionContentPartUnionParam{OfInputAudio: &audioBlock}, nil, true
	default:
		return openai.ChatCompletionContentPartUnionParam{}, &yuzu.CallWarning{
			Type:    yuzu.CallWarningTypeOther,
			Message: fmt.Sprintf("tool result media type %s not supported, sending text placeholder only", output.MediaType),
		}, false
	}
}
