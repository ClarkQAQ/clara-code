package provider

import (
	"bytes"
	yuzu "clara-code/pkg/models"
	"encoding/json"
)

// unsupportedChatCompletionsSettings lists Responses-only settings that are
// ignored by the Chat Completions API.
func unsupportedChatCompletionsSettings(o yuzu.ProviderOptions) []string {
	var settings []string
	if o.Include != nil {
		settings = append(settings, "Include")
	}
	if o.Instructions != nil {
		settings = append(settings, "Instructions")
	}
	if o.MaxToolCalls != nil {
		settings = append(settings, "MaxToolCalls")
	}
	if o.PreviousResponseID != nil {
		settings = append(settings, "PreviousResponseID")
	}
	if o.ReasoningSummary != nil {
		settings = append(settings, "ReasoningSummary")
	}
	if o.StrictJSONSchema != nil {
		settings = append(settings, "StrictJSONSchema")
	}
	return settings
}

// unsupportedResponsesSettings lists Chat Completions-only settings that are
// ignored by the Responses API.
func unsupportedResponsesSettings(o yuzu.ProviderOptions) []string {
	var settings []string
	if o.LogitBias != nil {
		settings = append(settings, "LogitBias")
	}
	if o.LogProbs != nil {
		settings = append(settings, "LogProbs")
	}
	if o.TopLogProbs != nil {
		settings = append(settings, "TopLogProbs")
	}
	if o.MaxCompletionTokens != nil {
		settings = append(settings, "MaxCompletionTokens")
	}
	if o.Prediction != nil {
		settings = append(settings, "Prediction")
	}
	if o.StructuredOutputs != nil {
		settings = append(settings, "StructuredOutputs")
	}
	return settings
}

func settingsWarnings(settings []string) []yuzu.CallWarning {
	warnings := make([]yuzu.CallWarning, 0, len(settings))
	for _, setting := range settings {
		warnings = append(warnings, yuzu.CallWarning{
			Type:    yuzu.CallWarningTypeUnsupportedSetting,
			Setting: setting,
			Details: "not supported by the configured API",
		})
	}
	return warnings
}

// ExtractExtraFields reads non-standard fields from the SDK's ExtraFields map
// and returns them as a map of raw JSON values.
func ExtractExtraFields(extraFields map[string]json.RawMessage) map[string]json.RawMessage {
	if len(extraFields) == 0 {
		return nil
	}
	ext := make(map[string]json.RawMessage, len(extraFields))
	for k, raw := range extraFields {
		if len(raw) > 0 && string(raw) != "null" {
			ext[k] = raw
		}
	}
	if len(ext) == 0 {
		return nil
	}
	return ext
}

// ReasoningData represents reasoning data returned by OpenAI-compatible
// endpoints. Some use "reasoning_content" (e.g. Avian), others use "reasoning"
// (e.g. Moonshot AI/Kimi).
type ReasoningData struct {
	ReasoningContent string `json:"reasoning_content"`
	Reasoning        string `json:"reasoning"`
}

// GetReasoningContent returns the reasoning text from whichever field is populated.
func (r ReasoningData) GetReasoningContent() string {
	if r.ReasoningContent != "" {
		return r.ReasoningContent
	}
	return r.Reasoning
}

// hasReasoningField reports whether raw JSON contains a reasoning field
// ("reasoning_content" or "reasoning") whose value is a JSON string, including
// the empty string. A null value means "no reasoning in this delta": DeepSeek,
// vLLM and SGLang put "reasoning_content": null on every content/tool-call/finish
// chunk, and counting that as reasoning keeps the block open forever.
func hasReasoningField(rawJSON string) bool {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal([]byte(rawJSON), &fields); err != nil {
		return false
	}
	for _, key := range []string{"reasoning_content", "reasoning"} {
		if v, ok := fields[key]; ok && !bytes.Equal(bytes.TrimSpace(v), []byte("null")) {
			return true
		}
	}
	return false
}
