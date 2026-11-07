package model

import (
	"encoding/json"
)

// ReasoningEffort represents the reasoning effort level for OpenAI models.
type ReasoningEffort string

const (
	// ReasoningEffortNone disables reasoning.
	ReasoningEffortNone ReasoningEffort = "none"
	// ReasoningEffortMinimal represents minimal reasoning effort.
	ReasoningEffortMinimal ReasoningEffort = "minimal"
	// ReasoningEffortLow represents low reasoning effort.
	ReasoningEffortLow ReasoningEffort = "low"
	// ReasoningEffortMedium represents medium reasoning effort.
	ReasoningEffortMedium ReasoningEffort = "medium"
	// ReasoningEffortHigh represents high reasoning effort.
	ReasoningEffortHigh ReasoningEffort = "high"
	// ReasoningEffortXHigh represents extra-high reasoning effort.
	ReasoningEffortXHigh ReasoningEffort = "xhigh"
	// ReasoningEffortMax represents maximum reasoning effort.
	ReasoningEffortMax ReasoningEffort = "max"
)

// IncludeType represents the type of content to include in a Responses API
// request.
type IncludeType string

const (
	// IncludeReasoningEncryptedContent includes encrypted reasoning content.
	IncludeReasoningEncryptedContent IncludeType = "reasoning.encrypted_content"
	// IncludeFileSearchCallResults includes file search call results.
	IncludeFileSearchCallResults IncludeType = "file_search_call.results"
	// IncludeMessageOutputTextLogprobs includes message output text log probabilities.
	IncludeMessageOutputTextLogprobs IncludeType = "message.output_text.logprobs"
)

// ServiceTier represents the OpenAI service tier.
type ServiceTier string

const (
	// ServiceTierAuto represents the auto service tier.
	ServiceTierAuto ServiceTier = "auto"
	// ServiceTierFlex represents the flex service tier.
	ServiceTierFlex ServiceTier = "flex"
	// ServiceTierPriority represents the priority service tier.
	ServiceTierPriority ServiceTier = "priority"
)

// TextVerbosity represents the text verbosity level.
type TextVerbosity string

const (
	// TextVerbosityLow represents low text verbosity.
	TextVerbosityLow TextVerbosity = "low"
	// TextVerbosityMedium represents medium text verbosity.
	TextVerbosityMedium TextVerbosity = "medium"
	// TextVerbosityHigh represents high text verbosity.
	TextVerbosityHigh TextVerbosity = "high"
)

// ProviderOptions carries OpenAI request options. Fields that only apply to
// one API shape are ignored (with a warning) when the provider was
// configured with the other one.
//
// ProviderOptions is also used for content-part level settings, which is what
// ExtraFields and ImageDetail are for.
type ProviderOptions struct {
	// Shared by both APIs.
	User              *string          `json:"user,omitempty"`
	ReasoningEffort   *ReasoningEffort `json:"reasoning_effort,omitempty"`
	ParallelToolCalls *bool            `json:"parallel_tool_calls,omitempty"`
	Store             *bool            `json:"store,omitempty"`
	Metadata          map[string]any   `json:"metadata,omitempty"`
	PromptCacheKey    *string          `json:"prompt_cache_key,omitempty"`
	SafetyIdentifier  *string          `json:"safety_identifier,omitempty"`
	ServiceTier       *ServiceTier     `json:"service_tier,omitempty"`
	TextVerbosity     *TextVerbosity   `json:"text_verbosity,omitempty"`

	// Chat Completions only.
	LogitBias           map[string]int64 `json:"logit_bias,omitempty"`
	LogProbs            *bool            `json:"log_probs,omitempty"`
	TopLogProbs         *int64           `json:"top_log_probs,omitempty"`
	MaxCompletionTokens *int64           `json:"max_completion_tokens,omitempty"`
	Prediction          map[string]any   `json:"prediction,omitempty"`
	StructuredOutputs   *bool            `json:"structured_outputs,omitempty"`

	// Responses only.
	Include            []IncludeType `json:"include,omitempty"`
	Instructions       *string       `json:"instructions,omitempty"`
	MaxToolCalls       *int64        `json:"max_tool_calls,omitempty"`
	PreviousResponseID *string       `json:"previous_response_id,omitempty"`
	ReasoningSummary   *string       `json:"reasoning_summary,omitempty"`
	StrictJSONSchema   *bool         `json:"strict_json_schema,omitempty"`

	// ExtraBody injects non-standard fields into the request root. It exists
	// for OpenAI-compatible endpoints that support extensions the OpenAI
	// request format does not model.
	ExtraBody map[string]any `json:"extra_body,omitempty"`

	// ExtraFields injects non-standard fields onto an individual message
	// content block. Content carrying extra fields is serialized in array
	// form with the fields merged onto the block. The primary use case is
	// explicit prompt caching (e.g. Alibaba Cloud Qwen's cache_control).
	ExtraFields map[string]any `json:"extra_fields,omitempty"`

	// ImageDetail sets the detail level for image file parts.
	ImageDetail string `json:"image_detail,omitempty"`
}

// WebSearchCallMetadata stores structured data from a web_search_call output
// item for round-tripping through multi-turn conversations. The ItemID is used
// with item_reference for efficient round-tripping when response storage is
// enabled.
type WebSearchCallMetadata struct {
	// ItemID is the server-side ID of the web_search_call output item.
	ItemID string `json:"item_id"`
	// Action contains the structured action data from the search.
	Action *WebSearchAction `json:"action,omitempty"`
}

// WebSearchSource represents a single source from a web search action.
type WebSearchSource struct {
	Type string `json:"type"`
	URL  string `json:"url"`
}

// WebSearchAction represents the action taken during a web search call.
type WebSearchAction struct {
	// Type is the kind of action: "search", "open_page", or "find".
	Type string `json:"type"`
	// Query is the search query (present when Type is "search").
	Query string `json:"query,omitempty"`
	// Sources are the results returned by the search.
	Sources []WebSearchSource `json:"sources,omitempty"`
}

// ProviderMetadata carries OpenAI response metadata. Depending on the
// content it is attached to, only a subset of the fields is populated.
type ProviderMetadata struct {
	// Chat Completions.
	Logprobs                 []ChatTokenLogprob `json:"logprobs,omitempty"`
	AcceptedPredictionTokens int64              `json:"accepted_prediction_tokens,omitempty"`
	RejectedPredictionTokens int64              `json:"rejected_prediction_tokens,omitempty"`

	// Responses.
	ResponseID string `json:"response_id,omitempty"`
	// ReasoningItemID and EncryptedReasoning identify a reasoning item so it
	// can be round-tripped when response storage is enabled.
	ReasoningItemID    string                 `json:"reasoning_item_id,omitempty"`
	EncryptedReasoning *string                `json:"encrypted_reasoning,omitempty"`
	ReasoningSummary   []string               `json:"reasoning_summary,omitempty"`
	WebSearch          *WebSearchCallMetadata `json:"web_search,omitempty"`

	// ExtraFields holds non-standard fields from the response, including any
	// captured via [LanguageModelHeaderFunc].
	ExtraFields map[string]json.RawMessage `json:"extra_fields,omitempty"`
}

// ChatTokenLogprob describes the log probability of a single token of a chat
// completion.
type ChatTokenLogprob struct {
	Token       string                `json:"token"`
	Bytes       []int64               `json:"bytes"`
	Logprob     float64               `json:"logprob"`
	TopLogprobs []ChatTokenTopLogprob `json:"top_logprobs"`
}

// ChatTokenTopLogprob describes one of the most likely alternatives for a
// token of a chat completion.
type ChatTokenTopLogprob struct {
	Token   string  `json:"token"`
	Bytes   []int64 `json:"bytes"`
	Logprob float64 `json:"logprob"`
}

// ExtraField parses an extra field into the provided target. Returns false if
// the field is not present or cannot be parsed.
func (m *ProviderMetadata) ExtraField(key string, target any) bool {
	if m == nil || m.ExtraFields == nil {
		return false
	}
	raw, ok := m.ExtraFields[key]
	if !ok {
		return false
	}
	return json.Unmarshal(raw, target) == nil
}

// IsZero reports whether the metadata carries no values.
func (m ProviderMetadata) IsZero() bool {
	return m.ResponseID == "" &&
		m.ReasoningItemID == "" &&
		m.EncryptedReasoning == nil &&
		len(m.ReasoningSummary) == 0 &&
		m.WebSearch == nil &&
		len(m.Logprobs) == 0 &&
		m.AcceptedPredictionTokens == 0 &&
		m.RejectedPredictionTokens == 0 &&
		len(m.ExtraFields) == 0
}

// mergeProviderOptions returns the agent-level options with every field the
// call-level options set overriding them.
func MergeProviderOptions(base, override ProviderOptions) ProviderOptions {
	if override.User != nil {
		base.User = override.User
	}
	if override.ReasoningEffort != nil {
		base.ReasoningEffort = override.ReasoningEffort
	}
	if override.ParallelToolCalls != nil {
		base.ParallelToolCalls = override.ParallelToolCalls
	}
	if override.Store != nil {
		base.Store = override.Store
	}
	if override.Metadata != nil {
		base.Metadata = override.Metadata
	}
	if override.PromptCacheKey != nil {
		base.PromptCacheKey = override.PromptCacheKey
	}
	if override.SafetyIdentifier != nil {
		base.SafetyIdentifier = override.SafetyIdentifier
	}
	if override.ServiceTier != nil {
		base.ServiceTier = override.ServiceTier
	}
	if override.TextVerbosity != nil {
		base.TextVerbosity = override.TextVerbosity
	}
	if override.LogitBias != nil {
		base.LogitBias = override.LogitBias
	}
	if override.LogProbs != nil {
		base.LogProbs = override.LogProbs
	}
	if override.TopLogProbs != nil {
		base.TopLogProbs = override.TopLogProbs
	}
	if override.MaxCompletionTokens != nil {
		base.MaxCompletionTokens = override.MaxCompletionTokens
	}
	if override.Prediction != nil {
		base.Prediction = override.Prediction
	}
	if override.StructuredOutputs != nil {
		base.StructuredOutputs = override.StructuredOutputs
	}
	if override.Include != nil {
		base.Include = override.Include
	}
	if override.Instructions != nil {
		base.Instructions = override.Instructions
	}
	if override.MaxToolCalls != nil {
		base.MaxToolCalls = override.MaxToolCalls
	}
	if override.PreviousResponseID != nil {
		base.PreviousResponseID = override.PreviousResponseID
	}
	if override.ReasoningSummary != nil {
		base.ReasoningSummary = override.ReasoningSummary
	}
	if override.StrictJSONSchema != nil {
		base.StrictJSONSchema = override.StrictJSONSchema
	}
	if override.ExtraBody != nil {
		base.ExtraBody = override.ExtraBody
	}
	if override.ExtraFields != nil {
		base.ExtraFields = override.ExtraFields
	}
	if override.ImageDetail != "" {
		base.ImageDetail = override.ImageDetail
	}
	return base
}

// ReasoningEffortOption creates a pointer to a ReasoningEffort value.
//
//go:fix inline
func ReasoningEffortOption(e ReasoningEffort) *ReasoningEffort {
	return new(e)
}
