// Code generated from the openai-go v3.54.0 wire definitions. DO NOT EDIT.

package openai

import (
	"encoding/json"
	"time"
)

type AnnotationUnion struct {
	EndIndex     int64                              `json:"end_index"`
	FileCitation FileCitationAnnotationFileCitation `json:"file_citation"`
	StartIndex   int64                              `json:"start_index"`
	Text         string                             `json:"text"`
	Type         string                             `json:"type"`
	FilePath     FilePathAnnotationFilePath         `json:"file_path"`
}

type ChatCompletion struct {
	ID                string                    `json:"id"`
	Choices           []ChatCompletionChoice    `json:"choices"`
	Created           int64                     `json:"created"`
	Model             string                    `json:"model"`
	Object            string                    `json:"object"`
	Metadata          Metadata                  `json:"metadata"`
	Moderation        ChatCompletionModeration  `json:"moderation"`
	ServiceTier       ChatCompletionServiceTier `json:"service_tier"`
	SystemFingerprint string                    `json:"system_fingerprint"`
	Usage             CompletionUsage           `json:"usage"`
}

type ChatCompletionAllowedToolChoiceParam struct {
	AllowedTools ChatCompletionAllowedToolsParam `json:"allowed_tools,omitzero"`
	Type         string                          `json:"type"`
	paramObj
}

func (p ChatCompletionAllowedToolChoiceParam) MarshalJSON() ([]byte, error) {
	type raw ChatCompletionAllowedToolChoiceParam
	return marshalObject(raw(p), p.ExtraFields(), map[string]any{"type": "allowed_tools"})
}

type ChatCompletionAllowedToolsMode string

type ChatCompletionAllowedToolsParam struct {
	Mode  ChatCompletionAllowedToolsMode `json:"mode,omitzero"`
	Tools []map[string]any               `json:"tools,omitzero"`
	paramObj
}

func (p ChatCompletionAllowedToolsParam) MarshalJSON() ([]byte, error) {
	type raw ChatCompletionAllowedToolsParam
	return marshalObject(raw(p), p.ExtraFields(), nil)
}

type ChatCompletionAssistantMessageParam struct {
	Refusal      Opt[string]                                     `json:"refusal,omitzero"`
	Name         Opt[string]                                     `json:"name,omitzero"`
	Audio        ChatCompletionAssistantMessageParamAudio        `json:"audio,omitzero"`
	Content      ChatCompletionAssistantMessageParamContentUnion `json:"content,omitzero"`
	FunctionCall ChatCompletionAssistantMessageParamFunctionCall `json:"function_call,omitzero"`
	ToolCalls    []ChatCompletionMessageToolCallUnionParam       `json:"tool_calls,omitzero"`
	Role         string                                          `json:"role"`
	paramObj
}

func (p ChatCompletionAssistantMessageParam) MarshalJSON() ([]byte, error) {
	type raw ChatCompletionAssistantMessageParam
	return marshalObject(raw(p), p.ExtraFields(), map[string]any{"role": "assistant"})
}

type ChatCompletionAssistantMessageParamAudio struct {
	ID string `json:"id"`
	paramObj
}

func (p ChatCompletionAssistantMessageParamAudio) MarshalJSON() ([]byte, error) {
	type raw ChatCompletionAssistantMessageParamAudio
	return marshalObject(raw(p), p.ExtraFields(), nil)
}

type ChatCompletionAssistantMessageParamContentArrayOfContentPartUnion struct {
	OfText    *ChatCompletionContentPartTextParam    `json:",omitzero,inline"`
	OfRefusal *ChatCompletionContentPartRefusalParam `json:",omitzero,inline"`
}

func (u ChatCompletionAssistantMessageParamContentArrayOfContentPartUnion) MarshalJSON() ([]byte, error) {
	return marshalUnion(u.OfText, u.OfRefusal)
}

func (u *ChatCompletionAssistantMessageParamContentArrayOfContentPartUnion) UnmarshalJSON(data []byte) error {
	switch unionDiscriminant(data) {
	case "text":
		return json.Unmarshal(data, &u.OfText)
	case "refusal":
		return json.Unmarshal(data, &u.OfRefusal)
	}
	return tryUnmarshal(data)
}

type ChatCompletionAssistantMessageParamContentUnion struct {
	OfString              Opt[string]                                                         `json:",omitzero,inline"`
	OfArrayOfContentParts []ChatCompletionAssistantMessageParamContentArrayOfContentPartUnion `json:",omitzero,inline"`
}

func (u ChatCompletionAssistantMessageParamContentUnion) MarshalJSON() ([]byte, error) {
	return marshalUnion(u.OfString, u.OfArrayOfContentParts)
}

func (u *ChatCompletionAssistantMessageParamContentUnion) UnmarshalJSON(data []byte) error {
	return tryUnmarshal(data, &u.OfString, &u.OfArrayOfContentParts)
}

type ChatCompletionAssistantMessageParamFunctionCall struct {
	Arguments string `json:"arguments"`
	Name      string `json:"name"`
	paramObj
}

func (p ChatCompletionAssistantMessageParamFunctionCall) MarshalJSON() ([]byte, error) {
	type raw ChatCompletionAssistantMessageParamFunctionCall
	return marshalObject(raw(p), p.ExtraFields(), nil)
}

type ChatCompletionAudio struct {
	ID         string `json:"id"`
	Data       string `json:"data"`
	ExpiresAt  int64  `json:"expires_at"`
	Transcript string `json:"transcript"`
}

type ChatCompletionAudioParam struct {
	Format ChatCompletionAudioParamFormat     `json:"format,omitzero"`
	Voice  ChatCompletionAudioParamVoiceUnion `json:"voice,omitzero"`
	paramObj
}

func (p ChatCompletionAudioParam) MarshalJSON() ([]byte, error) {
	type raw ChatCompletionAudioParam
	return marshalObject(raw(p), p.ExtraFields(), nil)
}

type ChatCompletionAudioParamFormat string

type ChatCompletionAudioParamVoiceID struct {
	ID string `json:"id"`
	paramObj
}

func (p ChatCompletionAudioParamVoiceID) MarshalJSON() ([]byte, error) {
	type raw ChatCompletionAudioParamVoiceID
	return marshalObject(raw(p), p.ExtraFields(), nil)
}

type ChatCompletionAudioParamVoiceUnion struct {
	OfString                          Opt[string]                      `json:",omitzero,inline"`
	OfChatCompletionAudioVoiceString2 Opt[string]                      `json:",omitzero,inline"`
	OfChatCompletionAudioVoiceID      *ChatCompletionAudioParamVoiceID `json:",omitzero,inline"`
}

func (u ChatCompletionAudioParamVoiceUnion) MarshalJSON() ([]byte, error) {
	return marshalUnion(u.OfString, u.OfChatCompletionAudioVoiceString2, u.OfChatCompletionAudioVoiceID)
}

func (u *ChatCompletionAudioParamVoiceUnion) UnmarshalJSON(data []byte) error {
	return tryUnmarshal(data, &u.OfString, &u.OfChatCompletionAudioVoiceString2, &u.OfChatCompletionAudioVoiceID)
}

type ChatCompletionChoice struct {
	FinishReason string                       `json:"finish_reason"`
	Index        int64                        `json:"index"`
	Logprobs     ChatCompletionChoiceLogprobs `json:"logprobs"`
	Message      ChatCompletionMessage        `json:"message"`
}

type ChatCompletionChoiceLogprobs struct {
	Content []ChatCompletionTokenLogprob `json:"content"`
	Refusal []ChatCompletionTokenLogprob `json:"refusal"`
}

type ChatCompletionChunk struct {
	ID                string                         `json:"id"`
	Choices           []ChatCompletionChunkChoice    `json:"choices"`
	Created           int64                          `json:"created"`
	Model             string                         `json:"model"`
	Object            string                         `json:"object"`
	Moderation        ChatCompletionChunkModeration  `json:"moderation"`
	Obfuscation       string                         `json:"obfuscation"`
	ServiceTier       ChatCompletionChunkServiceTier `json:"service_tier"`
	SystemFingerprint string                         `json:"system_fingerprint"`
	Usage             CompletionUsage                `json:"usage"`
}

type ChatCompletionChunkChoice struct {
	Delta        ChatCompletionChunkChoiceDelta    `json:"delta"`
	FinishReason string                            `json:"finish_reason"`
	Index        int64                             `json:"index"`
	Logprobs     ChatCompletionChunkChoiceLogprobs `json:"logprobs"`
}

type ChatCompletionChunkChoiceDelta struct {
	Content      string                                     `json:"content"`
	FunctionCall ChatCompletionChunkChoiceDeltaFunctionCall `json:"function_call"`
	Refusal      string                                     `json:"refusal"`
	Role         string                                     `json:"role"`
	ToolCalls    []ChatCompletionChunkChoiceDeltaToolCall   `json:"tool_calls"`
	// Raw is the unmodified JSON payload of the object.
	Raw json.RawMessage `json:"-"`
	// ExtraFields holds non-standard fields returned by the endpoint.
	ExtraFields map[string]json.RawMessage `json:"-"`
}

// RawJSON returns the unmodified JSON payload.
func (r ChatCompletionChunkChoiceDelta) RawJSON() string { return string(r.Raw) }

func (r *ChatCompletionChunkChoiceDelta) UnmarshalJSON(data []byte) error {
	type raw ChatCompletionChunkChoiceDelta
	var v raw
	if err := json.Unmarshal(data, &v); err != nil {
		return err
	}
	*r = ChatCompletionChunkChoiceDelta(v)
	r.Raw = append(json.RawMessage(nil), data...)
	extras, err := decodeExtras(data, &v)
	if err != nil {
		return err
	}
	r.ExtraFields = extras
	return nil
}

type ChatCompletionChunkChoiceDeltaFunctionCall struct {
	Arguments string `json:"arguments"`
	Name      string `json:"name"`
}

type ChatCompletionChunkChoiceDeltaToolCall struct {
	Index    int64                                          `json:"index"`
	ID       string                                         `json:"id"`
	Function ChatCompletionChunkChoiceDeltaToolCallFunction `json:"function"`
	Type     string                                         `json:"type"`
}

type ChatCompletionChunkChoiceDeltaToolCallFunction struct {
	Arguments string `json:"arguments"`
	Name      string `json:"name"`
}

type ChatCompletionChunkChoiceLogprobs struct {
	Content []ChatCompletionTokenLogprob `json:"content"`
	Refusal []ChatCompletionTokenLogprob `json:"refusal"`
}

type ChatCompletionChunkModeration struct {
	Input  ChatCompletionChunkModerationInputUnion  `json:"input"`
	Output ChatCompletionChunkModerationOutputUnion `json:"output"`
}

type ChatCompletionChunkModerationInputModerationResultsResult struct {
	Categories                map[string]bool     `json:"categories"`
	CategoryAppliedInputTypes map[string][]string `json:"category_applied_input_types"`
	CategoryScores            map[string]float64  `json:"category_scores"`
	Flagged                   bool                `json:"flagged"`
	Model                     string              `json:"model"`
	Type                      string              `json:"type"`
}

type ChatCompletionChunkModerationInputUnion struct {
	Model   string                                                      `json:"model"`
	Results []ChatCompletionChunkModerationInputModerationResultsResult `json:"results"`
	Type    string                                                      `json:"type"`
	Code    string                                                      `json:"code"`
	Message string                                                      `json:"message"`
}

type ChatCompletionChunkModerationOutputModerationResultsResult struct {
	Categories                map[string]bool     `json:"categories"`
	CategoryAppliedInputTypes map[string][]string `json:"category_applied_input_types"`
	CategoryScores            map[string]float64  `json:"category_scores"`
	Flagged                   bool                `json:"flagged"`
	Model                     string              `json:"model"`
	Type                      string              `json:"type"`
}

type ChatCompletionChunkModerationOutputUnion struct {
	Model   string                                                       `json:"model"`
	Results []ChatCompletionChunkModerationOutputModerationResultsResult `json:"results"`
	Type    string                                                       `json:"type"`
	Code    string                                                       `json:"code"`
	Message string                                                       `json:"message"`
}

type ChatCompletionChunkServiceTier string

type ChatCompletionContentPartFileFileParam struct {
	FileData Opt[string] `json:"file_data,omitzero"`
	FileID   Opt[string] `json:"file_id,omitzero"`
	Filename Opt[string] `json:"filename,omitzero"`
	paramObj
}

func (p ChatCompletionContentPartFileFileParam) MarshalJSON() ([]byte, error) {
	type raw ChatCompletionContentPartFileFileParam
	return marshalObject(raw(p), p.ExtraFields(), nil)
}

type ChatCompletionContentPartFileParam struct {
	File                  ChatCompletionContentPartFileFileParam                  `json:"file,omitzero"`
	PromptCacheBreakpoint ChatCompletionContentPartFilePromptCacheBreakpointParam `json:"prompt_cache_breakpoint,omitzero"`
	Type                  string                                                  `json:"type"`
	paramObj
}

func (p ChatCompletionContentPartFileParam) MarshalJSON() ([]byte, error) {
	type raw ChatCompletionContentPartFileParam
	return marshalObject(raw(p), p.ExtraFields(), map[string]any{"type": "file"})
}

type ChatCompletionContentPartFilePromptCacheBreakpointParam struct {
	Mode string `json:"mode"`
	paramObj
}

func (p ChatCompletionContentPartFilePromptCacheBreakpointParam) MarshalJSON() ([]byte, error) {
	type raw ChatCompletionContentPartFilePromptCacheBreakpointParam
	return marshalObject(raw(p), p.ExtraFields(), map[string]any{"mode": "explicit"})
}

type ChatCompletionContentPartImageImageURLParam struct {
	URL    string `json:"url"`
	Detail string `json:"detail,omitzero"`
	paramObj
}

func (p ChatCompletionContentPartImageImageURLParam) MarshalJSON() ([]byte, error) {
	type raw ChatCompletionContentPartImageImageURLParam
	return marshalObject(raw(p), p.ExtraFields(), nil)
}

type ChatCompletionContentPartImageParam struct {
	ImageURL              ChatCompletionContentPartImageImageURLParam              `json:"image_url,omitzero"`
	PromptCacheBreakpoint ChatCompletionContentPartImagePromptCacheBreakpointParam `json:"prompt_cache_breakpoint,omitzero"`
	Type                  string                                                   `json:"type"`
	paramObj
}

func (p ChatCompletionContentPartImageParam) MarshalJSON() ([]byte, error) {
	type raw ChatCompletionContentPartImageParam
	return marshalObject(raw(p), p.ExtraFields(), map[string]any{"type": "image_url"})
}

type ChatCompletionContentPartImagePromptCacheBreakpointParam struct {
	Mode string `json:"mode"`
	paramObj
}

func (p ChatCompletionContentPartImagePromptCacheBreakpointParam) MarshalJSON() ([]byte, error) {
	type raw ChatCompletionContentPartImagePromptCacheBreakpointParam
	return marshalObject(raw(p), p.ExtraFields(), map[string]any{"mode": "explicit"})
}

type ChatCompletionContentPartInputAudioInputAudioParam struct {
	Data   string `json:"data"`
	Format string `json:"format,omitzero"`
	paramObj
}

func (p ChatCompletionContentPartInputAudioInputAudioParam) MarshalJSON() ([]byte, error) {
	type raw ChatCompletionContentPartInputAudioInputAudioParam
	return marshalObject(raw(p), p.ExtraFields(), nil)
}

type ChatCompletionContentPartInputAudioParam struct {
	InputAudio            ChatCompletionContentPartInputAudioInputAudioParam            `json:"input_audio,omitzero"`
	PromptCacheBreakpoint ChatCompletionContentPartInputAudioPromptCacheBreakpointParam `json:"prompt_cache_breakpoint,omitzero"`
	Type                  string                                                        `json:"type"`
	paramObj
}

func (p ChatCompletionContentPartInputAudioParam) MarshalJSON() ([]byte, error) {
	type raw ChatCompletionContentPartInputAudioParam
	return marshalObject(raw(p), p.ExtraFields(), map[string]any{"type": "input_audio"})
}

type ChatCompletionContentPartInputAudioPromptCacheBreakpointParam struct {
	Mode string `json:"mode"`
	paramObj
}

func (p ChatCompletionContentPartInputAudioPromptCacheBreakpointParam) MarshalJSON() ([]byte, error) {
	type raw ChatCompletionContentPartInputAudioPromptCacheBreakpointParam
	return marshalObject(raw(p), p.ExtraFields(), map[string]any{"mode": "explicit"})
}

type ChatCompletionContentPartRefusalParam struct {
	Refusal string `json:"refusal"`
	Type    string `json:"type"`
	paramObj
}

func (p ChatCompletionContentPartRefusalParam) MarshalJSON() ([]byte, error) {
	type raw ChatCompletionContentPartRefusalParam
	return marshalObject(raw(p), p.ExtraFields(), map[string]any{"type": "refusal"})
}

type ChatCompletionContentPartTextParam struct {
	Text                  string                                                  `json:"text"`
	PromptCacheBreakpoint ChatCompletionContentPartTextPromptCacheBreakpointParam `json:"prompt_cache_breakpoint,omitzero"`
	Type                  string                                                  `json:"type"`
	paramObj
}

func (p ChatCompletionContentPartTextParam) MarshalJSON() ([]byte, error) {
	type raw ChatCompletionContentPartTextParam
	return marshalObject(raw(p), p.ExtraFields(), map[string]any{"type": "text"})
}

type ChatCompletionContentPartTextPromptCacheBreakpointParam struct {
	Mode string `json:"mode"`
	paramObj
}

func (p ChatCompletionContentPartTextPromptCacheBreakpointParam) MarshalJSON() ([]byte, error) {
	type raw ChatCompletionContentPartTextPromptCacheBreakpointParam
	return marshalObject(raw(p), p.ExtraFields(), map[string]any{"mode": "explicit"})
}

type ChatCompletionContentPartUnionParam struct {
	OfText       *ChatCompletionContentPartTextParam       `json:",omitzero,inline"`
	OfImageURL   *ChatCompletionContentPartImageParam      `json:",omitzero,inline"`
	OfInputAudio *ChatCompletionContentPartInputAudioParam `json:",omitzero,inline"`
	OfFile       *ChatCompletionContentPartFileParam       `json:",omitzero,inline"`
}

func (u ChatCompletionContentPartUnionParam) MarshalJSON() ([]byte, error) {
	return marshalUnion(u.OfText, u.OfImageURL, u.OfInputAudio, u.OfFile)
}

func (u *ChatCompletionContentPartUnionParam) UnmarshalJSON(data []byte) error {
	switch unionDiscriminant(data) {
	case "text":
		return json.Unmarshal(data, &u.OfText)
	case "image_url":
		return json.Unmarshal(data, &u.OfImageURL)
	case "input_audio":
		return json.Unmarshal(data, &u.OfInputAudio)
	case "file":
		return json.Unmarshal(data, &u.OfFile)
	}
	return tryUnmarshal(data)
}

type ChatCompletionCustomToolCustomFormatGrammarGrammarParam struct {
	Definition string `json:"definition"`
	Syntax     string `json:"syntax,omitzero"`
	paramObj
}

func (p ChatCompletionCustomToolCustomFormatGrammarGrammarParam) MarshalJSON() ([]byte, error) {
	type raw ChatCompletionCustomToolCustomFormatGrammarGrammarParam
	return marshalObject(raw(p), p.ExtraFields(), nil)
}

type ChatCompletionCustomToolCustomFormatGrammarParam struct {
	Grammar ChatCompletionCustomToolCustomFormatGrammarGrammarParam `json:"grammar,omitzero"`
	Type    string                                                  `json:"type"`
	paramObj
}

func (p ChatCompletionCustomToolCustomFormatGrammarParam) MarshalJSON() ([]byte, error) {
	type raw ChatCompletionCustomToolCustomFormatGrammarParam
	return marshalObject(raw(p), p.ExtraFields(), map[string]any{"type": "grammar"})
}

type ChatCompletionCustomToolCustomFormatTextParam struct {
	Type string `json:"type"`
	paramObj
}

func (p ChatCompletionCustomToolCustomFormatTextParam) MarshalJSON() ([]byte, error) {
	type raw ChatCompletionCustomToolCustomFormatTextParam
	return marshalObject(raw(p), p.ExtraFields(), map[string]any{"type": "text"})
}

type ChatCompletionCustomToolCustomFormatUnionParam struct {
	OfText    *ChatCompletionCustomToolCustomFormatTextParam    `json:",omitzero,inline"`
	OfGrammar *ChatCompletionCustomToolCustomFormatGrammarParam `json:",omitzero,inline"`
}

func (u ChatCompletionCustomToolCustomFormatUnionParam) MarshalJSON() ([]byte, error) {
	return marshalUnion(u.OfText, u.OfGrammar)
}

func (u *ChatCompletionCustomToolCustomFormatUnionParam) UnmarshalJSON(data []byte) error {
	switch unionDiscriminant(data) {
	case "text":
		return json.Unmarshal(data, &u.OfText)
	case "grammar":
		return json.Unmarshal(data, &u.OfGrammar)
	}
	return tryUnmarshal(data)
}

type ChatCompletionCustomToolCustomParam struct {
	Name        string                                         `json:"name"`
	Description Opt[string]                                    `json:"description,omitzero"`
	Format      ChatCompletionCustomToolCustomFormatUnionParam `json:"format,omitzero"`
	paramObj
}

func (p ChatCompletionCustomToolCustomParam) MarshalJSON() ([]byte, error) {
	type raw ChatCompletionCustomToolCustomParam
	return marshalObject(raw(p), p.ExtraFields(), nil)
}

type ChatCompletionCustomToolParam struct {
	Custom ChatCompletionCustomToolCustomParam `json:"custom,omitzero"`
	Type   string                              `json:"type"`
	paramObj
}

func (p ChatCompletionCustomToolParam) MarshalJSON() ([]byte, error) {
	type raw ChatCompletionCustomToolParam
	return marshalObject(raw(p), p.ExtraFields(), map[string]any{"type": "custom"})
}

type ChatCompletionDeveloperMessageParam struct {
	Content ChatCompletionDeveloperMessageParamContentUnion `json:"content,omitzero"`
	Name    Opt[string]                                     `json:"name,omitzero"`
	Role    string                                          `json:"role"`
	paramObj
}

func (p ChatCompletionDeveloperMessageParam) MarshalJSON() ([]byte, error) {
	type raw ChatCompletionDeveloperMessageParam
	return marshalObject(raw(p), p.ExtraFields(), map[string]any{"role": "developer"})
}

type ChatCompletionDeveloperMessageParamContentUnion struct {
	OfString              Opt[string]                          `json:",omitzero,inline"`
	OfArrayOfContentParts []ChatCompletionContentPartTextParam `json:",omitzero,inline"`
}

func (u ChatCompletionDeveloperMessageParamContentUnion) MarshalJSON() ([]byte, error) {
	return marshalUnion(u.OfString, u.OfArrayOfContentParts)
}

func (u *ChatCompletionDeveloperMessageParamContentUnion) UnmarshalJSON(data []byte) error {
	return tryUnmarshal(data, &u.OfString, &u.OfArrayOfContentParts)
}

type ChatCompletionFunctionCallOptionParam struct {
	Name string `json:"name"`
	paramObj
}

func (p ChatCompletionFunctionCallOptionParam) MarshalJSON() ([]byte, error) {
	type raw ChatCompletionFunctionCallOptionParam
	return marshalObject(raw(p), p.ExtraFields(), nil)
}

type ChatCompletionFunctionMessageParam struct {
	Content Opt[string] `json:"content,omitzero"`
	Name    string      `json:"name"`
	Role    string      `json:"role"`
	paramObj
}

func (p ChatCompletionFunctionMessageParam) MarshalJSON() ([]byte, error) {
	type raw ChatCompletionFunctionMessageParam
	return marshalObject(raw(p), p.ExtraFields(), map[string]any{"role": "function"})
}

type ChatCompletionFunctionToolParam struct {
	Function FunctionDefinitionParam `json:"function,omitzero"`
	Type     string                  `json:"type"`
	paramObj
}

func (p ChatCompletionFunctionToolParam) MarshalJSON() ([]byte, error) {
	type raw ChatCompletionFunctionToolParam
	return marshalObject(raw(p), p.ExtraFields(), map[string]any{"type": "function"})
}

type ChatCompletionMessage struct {
	Content      string                               `json:"content"`
	Refusal      string                               `json:"refusal"`
	Role         string                               `json:"role"`
	Annotations  []ChatCompletionMessageAnnotation    `json:"annotations"`
	Audio        ChatCompletionAudio                  `json:"audio"`
	FunctionCall ChatCompletionMessageFunctionCall    `json:"function_call"`
	ToolCalls    []ChatCompletionMessageToolCallUnion `json:"tool_calls"`
	// Raw is the unmodified JSON payload of the object.
	Raw json.RawMessage `json:"-"`
	// ExtraFields holds non-standard fields returned by the endpoint.
	ExtraFields map[string]json.RawMessage `json:"-"`
}

// RawJSON returns the unmodified JSON payload.
func (r ChatCompletionMessage) RawJSON() string { return string(r.Raw) }

func (r *ChatCompletionMessage) UnmarshalJSON(data []byte) error {
	type raw ChatCompletionMessage
	var v raw
	if err := json.Unmarshal(data, &v); err != nil {
		return err
	}
	*r = ChatCompletionMessage(v)
	r.Raw = append(json.RawMessage(nil), data...)
	extras, err := decodeExtras(data, &v)
	if err != nil {
		return err
	}
	r.ExtraFields = extras
	return nil
}

type ChatCompletionMessageAnnotation struct {
	Type        string                                     `json:"type"`
	URLCitation ChatCompletionMessageAnnotationURLCitation `json:"url_citation"`
}

type ChatCompletionMessageAnnotationURLCitation struct {
	EndIndex   int64  `json:"end_index"`
	StartIndex int64  `json:"start_index"`
	Title      string `json:"title"`
	URL        string `json:"url"`
}

type ChatCompletionMessageCustomToolCallCustom struct {
	Input string `json:"input"`
	Name  string `json:"name"`
}

type ChatCompletionMessageCustomToolCallCustomParam struct {
	Input string `json:"input"`
	Name  string `json:"name"`
	paramObj
}

func (p ChatCompletionMessageCustomToolCallCustomParam) MarshalJSON() ([]byte, error) {
	type raw ChatCompletionMessageCustomToolCallCustomParam
	return marshalObject(raw(p), p.ExtraFields(), nil)
}

type ChatCompletionMessageCustomToolCallParam struct {
	ID     string                                         `json:"id"`
	Custom ChatCompletionMessageCustomToolCallCustomParam `json:"custom,omitzero"`
	Type   string                                         `json:"type"`
	paramObj
}

func (p ChatCompletionMessageCustomToolCallParam) MarshalJSON() ([]byte, error) {
	type raw ChatCompletionMessageCustomToolCallParam
	return marshalObject(raw(p), p.ExtraFields(), map[string]any{"type": "custom"})
}

type ChatCompletionMessageFunctionCall struct {
	Arguments string `json:"arguments"`
	Name      string `json:"name"`
}

type ChatCompletionMessageFunctionToolCallFunction struct {
	Arguments string `json:"arguments"`
	Name      string `json:"name"`
}

type ChatCompletionMessageFunctionToolCallFunctionParam struct {
	Arguments string `json:"arguments"`
	Name      string `json:"name"`
	paramObj
}

func (p ChatCompletionMessageFunctionToolCallFunctionParam) MarshalJSON() ([]byte, error) {
	type raw ChatCompletionMessageFunctionToolCallFunctionParam
	return marshalObject(raw(p), p.ExtraFields(), nil)
}

type ChatCompletionMessageFunctionToolCallParam struct {
	ID       string                                             `json:"id"`
	Function ChatCompletionMessageFunctionToolCallFunctionParam `json:"function,omitzero"`
	Type     string                                             `json:"type"`
	paramObj
}

func (p ChatCompletionMessageFunctionToolCallParam) MarshalJSON() ([]byte, error) {
	type raw ChatCompletionMessageFunctionToolCallParam
	return marshalObject(raw(p), p.ExtraFields(), map[string]any{"type": "function"})
}

type ChatCompletionMessageParamUnion struct {
	OfDeveloper *ChatCompletionDeveloperMessageParam `json:",omitzero,inline"`
	OfSystem    *ChatCompletionSystemMessageParam    `json:",omitzero,inline"`
	OfUser      *ChatCompletionUserMessageParam      `json:",omitzero,inline"`
	OfAssistant *ChatCompletionAssistantMessageParam `json:",omitzero,inline"`
	OfTool      *ChatCompletionToolMessageParam      `json:",omitzero,inline"`
	OfFunction  *ChatCompletionFunctionMessageParam  `json:",omitzero,inline"`
}

func (u ChatCompletionMessageParamUnion) MarshalJSON() ([]byte, error) {
	return marshalUnion(u.OfDeveloper, u.OfSystem, u.OfUser, u.OfAssistant, u.OfTool, u.OfFunction)
}

func (u *ChatCompletionMessageParamUnion) UnmarshalJSON(data []byte) error {
	switch unionDiscriminant(data) {
	case "developer":
		return json.Unmarshal(data, &u.OfDeveloper)
	case "system":
		return json.Unmarshal(data, &u.OfSystem)
	case "user":
		return json.Unmarshal(data, &u.OfUser)
	case "assistant":
		return json.Unmarshal(data, &u.OfAssistant)
	case "tool":
		return json.Unmarshal(data, &u.OfTool)
	case "function":
		return json.Unmarshal(data, &u.OfFunction)
	}
	return tryUnmarshal(data)
}

type ChatCompletionMessageToolCallUnion struct {
	ID       string                                        `json:"id"`
	Function ChatCompletionMessageFunctionToolCallFunction `json:"function"`
	Type     string                                        `json:"type"`
	Custom   ChatCompletionMessageCustomToolCallCustom     `json:"custom"`
}

type ChatCompletionMessageToolCallUnionParam struct {
	OfFunction *ChatCompletionMessageFunctionToolCallParam `json:",omitzero,inline"`
	OfCustom   *ChatCompletionMessageCustomToolCallParam   `json:",omitzero,inline"`
}

func (u ChatCompletionMessageToolCallUnionParam) MarshalJSON() ([]byte, error) {
	return marshalUnion(u.OfFunction, u.OfCustom)
}

func (u *ChatCompletionMessageToolCallUnionParam) UnmarshalJSON(data []byte) error {
	switch unionDiscriminant(data) {
	case "function":
		return json.Unmarshal(data, &u.OfFunction)
	case "custom":
		return json.Unmarshal(data, &u.OfCustom)
	}
	return tryUnmarshal(data)
}

type ChatCompletionModeration struct {
	Input  ChatCompletionModerationInputUnion  `json:"input"`
	Output ChatCompletionModerationOutputUnion `json:"output"`
}

type ChatCompletionModerationInputModerationResultsResult struct {
	Categories                map[string]bool     `json:"categories"`
	CategoryAppliedInputTypes map[string][]string `json:"category_applied_input_types"`
	CategoryScores            map[string]float64  `json:"category_scores"`
	Flagged                   bool                `json:"flagged"`
	Model                     string              `json:"model"`
	Type                      string              `json:"type"`
}

type ChatCompletionModerationInputUnion struct {
	Model   string                                                 `json:"model"`
	Results []ChatCompletionModerationInputModerationResultsResult `json:"results"`
	Type    string                                                 `json:"type"`
	Code    string                                                 `json:"code"`
	Message string                                                 `json:"message"`
}

type ChatCompletionModerationOutputModerationResultsResult struct {
	Categories                map[string]bool     `json:"categories"`
	CategoryAppliedInputTypes map[string][]string `json:"category_applied_input_types"`
	CategoryScores            map[string]float64  `json:"category_scores"`
	Flagged                   bool                `json:"flagged"`
	Model                     string              `json:"model"`
	Type                      string              `json:"type"`
}

type ChatCompletionModerationOutputUnion struct {
	Model   string                                                  `json:"model"`
	Results []ChatCompletionModerationOutputModerationResultsResult `json:"results"`
	Type    string                                                  `json:"type"`
	Code    string                                                  `json:"code"`
	Message string                                                  `json:"message"`
}

type ChatCompletionNamedToolChoiceCustomCustomParam struct {
	Name string `json:"name"`
	paramObj
}

func (p ChatCompletionNamedToolChoiceCustomCustomParam) MarshalJSON() ([]byte, error) {
	type raw ChatCompletionNamedToolChoiceCustomCustomParam
	return marshalObject(raw(p), p.ExtraFields(), nil)
}

type ChatCompletionNamedToolChoiceCustomParam struct {
	Custom ChatCompletionNamedToolChoiceCustomCustomParam `json:"custom,omitzero"`
	Type   string                                         `json:"type"`
	paramObj
}

func (p ChatCompletionNamedToolChoiceCustomParam) MarshalJSON() ([]byte, error) {
	type raw ChatCompletionNamedToolChoiceCustomParam
	return marshalObject(raw(p), p.ExtraFields(), map[string]any{"type": "custom"})
}

type ChatCompletionNamedToolChoiceFunctionParam struct {
	Name string `json:"name"`
	paramObj
}

func (p ChatCompletionNamedToolChoiceFunctionParam) MarshalJSON() ([]byte, error) {
	type raw ChatCompletionNamedToolChoiceFunctionParam
	return marshalObject(raw(p), p.ExtraFields(), nil)
}

type ChatCompletionNamedToolChoiceParam struct {
	Function ChatCompletionNamedToolChoiceFunctionParam `json:"function,omitzero"`
	Type     string                                     `json:"type"`
	paramObj
}

func (p ChatCompletionNamedToolChoiceParam) MarshalJSON() ([]byte, error) {
	type raw ChatCompletionNamedToolChoiceParam
	return marshalObject(raw(p), p.ExtraFields(), map[string]any{"type": "function"})
}

type ChatCompletionNewParams struct {
	Messages             []ChatCompletionMessageParamUnion           `json:"messages,omitzero"`
	Model                ChatModel                                   `json:"model,omitzero"`
	FrequencyPenalty     Opt[float64]                                `json:"frequency_penalty,omitzero"`
	Logprobs             Opt[bool]                                   `json:"logprobs,omitzero"`
	MaxCompletionTokens  Opt[int64]                                  `json:"max_completion_tokens,omitzero"`
	MaxTokens            Opt[int64]                                  `json:"max_tokens,omitzero"`
	N                    Opt[int64]                                  `json:"n,omitzero"`
	PresencePenalty      Opt[float64]                                `json:"presence_penalty,omitzero"`
	PromptCacheKey       Opt[string]                                 `json:"prompt_cache_key,omitzero"`
	SafetyIdentifier     Opt[string]                                 `json:"safety_identifier,omitzero"`
	Seed                 Opt[int64]                                  `json:"seed,omitzero"`
	Store                Opt[bool]                                   `json:"store,omitzero"`
	Temperature          Opt[float64]                                `json:"temperature,omitzero"`
	TopLogprobs          Opt[int64]                                  `json:"top_logprobs,omitzero"`
	TopP                 Opt[float64]                                `json:"top_p,omitzero"`
	ParallelToolCalls    Opt[bool]                                   `json:"parallel_tool_calls,omitzero"`
	User                 Opt[string]                                 `json:"user,omitzero"`
	Audio                ChatCompletionAudioParam                    `json:"audio,omitzero"`
	LogitBias            map[string]int64                            `json:"logit_bias,omitzero"`
	Metadata             Metadata                                    `json:"metadata,omitzero"`
	Modalities           []string                                    `json:"modalities,omitzero"`
	Moderation           ChatCompletionNewParamsModeration           `json:"moderation,omitzero"`
	PromptCacheRetention ChatCompletionNewParamsPromptCacheRetention `json:"prompt_cache_retention,omitzero"`
	ReasoningEffort      ReasoningEffort                             `json:"reasoning_effort,omitzero"`
	ServiceTier          ChatCompletionNewParamsServiceTier          `json:"service_tier,omitzero"`
	Stop                 ChatCompletionNewParamsStopUnion            `json:"stop,omitzero"`
	StreamOptions        ChatCompletionStreamOptionsParam            `json:"stream_options,omitzero"`
	Verbosity            ChatCompletionNewParamsVerbosity            `json:"verbosity,omitzero"`
	FunctionCall         ChatCompletionNewParamsFunctionCallUnion    `json:"function_call,omitzero"`
	Functions            []ChatCompletionNewParamsFunction           `json:"functions,omitzero"`
	Prediction           ChatCompletionPredictionContentParam        `json:"prediction,omitzero"`
	PromptCacheOptions   ChatCompletionNewParamsPromptCacheOptions   `json:"prompt_cache_options,omitzero"`
	ResponseFormat       ChatCompletionNewParamsResponseFormatUnion  `json:"response_format,omitzero"`
	ToolChoice           ChatCompletionToolChoiceOptionUnionParam    `json:"tool_choice,omitzero"`
	Tools                []ChatCompletionToolUnionParam              `json:"tools,omitzero"`
	WebSearchOptions     ChatCompletionNewParamsWebSearchOptions     `json:"web_search_options,omitzero"`
	paramObj
}

func (p ChatCompletionNewParams) MarshalJSON() ([]byte, error) {
	type raw ChatCompletionNewParams
	return marshalObject(raw(p), p.ExtraFields(), nil)
}

type ChatCompletionNewParamsFunction struct {
	Name        string             `json:"name"`
	Description Opt[string]        `json:"description,omitzero"`
	Parameters  FunctionParameters `json:"parameters,omitzero"`
	paramObj
}

func (p ChatCompletionNewParamsFunction) MarshalJSON() ([]byte, error) {
	type raw ChatCompletionNewParamsFunction
	return marshalObject(raw(p), p.ExtraFields(), nil)
}

type ChatCompletionNewParamsFunctionCallUnion struct {
	OfFunctionCallMode   Opt[string]                            `json:",omitzero,inline"`
	OfFunctionCallOption *ChatCompletionFunctionCallOptionParam `json:",omitzero,inline"`
}

func (u ChatCompletionNewParamsFunctionCallUnion) MarshalJSON() ([]byte, error) {
	return marshalUnion(u.OfFunctionCallMode, u.OfFunctionCallOption)
}

func (u *ChatCompletionNewParamsFunctionCallUnion) UnmarshalJSON(data []byte) error {
	return tryUnmarshal(data, &u.OfFunctionCallMode, &u.OfFunctionCallOption)
}

type ChatCompletionNewParamsModeration struct {
	Model  string                                  `json:"model"`
	Policy ChatCompletionNewParamsModerationPolicy `json:"policy,omitzero"`
	paramObj
}

func (p ChatCompletionNewParamsModeration) MarshalJSON() ([]byte, error) {
	type raw ChatCompletionNewParamsModeration
	return marshalObject(raw(p), p.ExtraFields(), nil)
}

type ChatCompletionNewParamsModerationPolicy struct {
	Input  ChatCompletionNewParamsModerationPolicyInput  `json:"input,omitzero"`
	Output ChatCompletionNewParamsModerationPolicyOutput `json:"output,omitzero"`
	paramObj
}

func (p ChatCompletionNewParamsModerationPolicy) MarshalJSON() ([]byte, error) {
	type raw ChatCompletionNewParamsModerationPolicy
	return marshalObject(raw(p), p.ExtraFields(), nil)
}

type ChatCompletionNewParamsModerationPolicyInput struct {
	Mode string `json:"mode,omitzero"`
	paramObj
}

func (p ChatCompletionNewParamsModerationPolicyInput) MarshalJSON() ([]byte, error) {
	type raw ChatCompletionNewParamsModerationPolicyInput
	return marshalObject(raw(p), p.ExtraFields(), nil)
}

type ChatCompletionNewParamsModerationPolicyOutput struct {
	Mode string `json:"mode,omitzero"`
	paramObj
}

func (p ChatCompletionNewParamsModerationPolicyOutput) MarshalJSON() ([]byte, error) {
	type raw ChatCompletionNewParamsModerationPolicyOutput
	return marshalObject(raw(p), p.ExtraFields(), nil)
}

type ChatCompletionNewParamsPromptCacheOptions struct {
	Mode string `json:"mode,omitzero"`
	Ttl  string `json:"ttl,omitzero"`
	paramObj
}

func (p ChatCompletionNewParamsPromptCacheOptions) MarshalJSON() ([]byte, error) {
	type raw ChatCompletionNewParamsPromptCacheOptions
	return marshalObject(raw(p), p.ExtraFields(), nil)
}

type ChatCompletionNewParamsPromptCacheRetention string

type ChatCompletionNewParamsResponseFormatUnion struct {
	OfText       *ResponseFormatTextParam       `json:",omitzero,inline"`
	OfJSONSchema *ResponseFormatJSONSchemaParam `json:",omitzero,inline"`
	OfJSONObject *ResponseFormatJSONObjectParam `json:",omitzero,inline"`
}

func (u ChatCompletionNewParamsResponseFormatUnion) MarshalJSON() ([]byte, error) {
	return marshalUnion(u.OfText, u.OfJSONSchema, u.OfJSONObject)
}

func (u *ChatCompletionNewParamsResponseFormatUnion) UnmarshalJSON(data []byte) error {
	return tryUnmarshal(data, &u.OfText, &u.OfJSONSchema, &u.OfJSONObject)
}

type ChatCompletionNewParamsServiceTier string

type ChatCompletionNewParamsStopUnion struct {
	OfString      Opt[string] `json:",omitzero,inline"`
	OfStringArray []string    `json:",omitzero,inline"`
}

func (u ChatCompletionNewParamsStopUnion) MarshalJSON() ([]byte, error) {
	return marshalUnion(u.OfString, u.OfStringArray)
}

func (u *ChatCompletionNewParamsStopUnion) UnmarshalJSON(data []byte) error {
	return tryUnmarshal(data, &u.OfString, &u.OfStringArray)
}

type ChatCompletionNewParamsVerbosity string

type ChatCompletionNewParamsWebSearchOptions struct {
	UserLocation      ChatCompletionNewParamsWebSearchOptionsUserLocation `json:"user_location,omitzero"`
	SearchContextSize string                                              `json:"search_context_size,omitzero"`
	paramObj
}

func (p ChatCompletionNewParamsWebSearchOptions) MarshalJSON() ([]byte, error) {
	type raw ChatCompletionNewParamsWebSearchOptions
	return marshalObject(raw(p), p.ExtraFields(), nil)
}

type ChatCompletionNewParamsWebSearchOptionsUserLocation struct {
	Approximate ChatCompletionNewParamsWebSearchOptionsUserLocationApproximate `json:"approximate,omitzero"`
	Type        string                                                         `json:"type"`
	paramObj
}

func (p ChatCompletionNewParamsWebSearchOptionsUserLocation) MarshalJSON() ([]byte, error) {
	type raw ChatCompletionNewParamsWebSearchOptionsUserLocation
	return marshalObject(raw(p), p.ExtraFields(), map[string]any{"type": "approximate"})
}

type ChatCompletionNewParamsWebSearchOptionsUserLocationApproximate struct {
	City     Opt[string] `json:"city,omitzero"`
	Country  Opt[string] `json:"country,omitzero"`
	Region   Opt[string] `json:"region,omitzero"`
	Timezone Opt[string] `json:"timezone,omitzero"`
	paramObj
}

func (p ChatCompletionNewParamsWebSearchOptionsUserLocationApproximate) MarshalJSON() ([]byte, error) {
	type raw ChatCompletionNewParamsWebSearchOptionsUserLocationApproximate
	return marshalObject(raw(p), p.ExtraFields(), nil)
}

type ChatCompletionPredictionContentContentUnionParam struct {
	OfString              Opt[string]                          `json:",omitzero,inline"`
	OfArrayOfContentParts []ChatCompletionContentPartTextParam `json:",omitzero,inline"`
}

func (u ChatCompletionPredictionContentContentUnionParam) MarshalJSON() ([]byte, error) {
	return marshalUnion(u.OfString, u.OfArrayOfContentParts)
}

func (u *ChatCompletionPredictionContentContentUnionParam) UnmarshalJSON(data []byte) error {
	return tryUnmarshal(data, &u.OfString, &u.OfArrayOfContentParts)
}

type ChatCompletionPredictionContentParam struct {
	Content ChatCompletionPredictionContentContentUnionParam `json:"content,omitzero"`
	Type    string                                           `json:"type"`
	paramObj
}

func (p ChatCompletionPredictionContentParam) MarshalJSON() ([]byte, error) {
	type raw ChatCompletionPredictionContentParam
	return marshalObject(raw(p), p.ExtraFields(), map[string]any{"type": "content"})
}

type ChatCompletionServiceTier string

type ChatCompletionStreamOptionsParam struct {
	IncludeObfuscation Opt[bool] `json:"include_obfuscation,omitzero"`
	IncludeUsage       Opt[bool] `json:"include_usage,omitzero"`
	paramObj
}

func (p ChatCompletionStreamOptionsParam) MarshalJSON() ([]byte, error) {
	type raw ChatCompletionStreamOptionsParam
	return marshalObject(raw(p), p.ExtraFields(), nil)
}

type ChatCompletionSystemMessageParam struct {
	Content ChatCompletionSystemMessageParamContentUnion `json:"content,omitzero"`
	Name    Opt[string]                                  `json:"name,omitzero"`
	Role    string                                       `json:"role"`
	paramObj
}

func (p ChatCompletionSystemMessageParam) MarshalJSON() ([]byte, error) {
	type raw ChatCompletionSystemMessageParam
	return marshalObject(raw(p), p.ExtraFields(), map[string]any{"role": "system"})
}

type ChatCompletionSystemMessageParamContentUnion struct {
	OfString              Opt[string]                          `json:",omitzero,inline"`
	OfArrayOfContentParts []ChatCompletionContentPartTextParam `json:",omitzero,inline"`
}

func (u ChatCompletionSystemMessageParamContentUnion) MarshalJSON() ([]byte, error) {
	return marshalUnion(u.OfString, u.OfArrayOfContentParts)
}

func (u *ChatCompletionSystemMessageParamContentUnion) UnmarshalJSON(data []byte) error {
	return tryUnmarshal(data, &u.OfString, &u.OfArrayOfContentParts)
}

type ChatCompletionTokenLogprob struct {
	Token       string                                 `json:"token"`
	Bytes       []int64                                `json:"bytes"`
	Logprob     float64                                `json:"logprob"`
	TopLogprobs []ChatCompletionTokenLogprobTopLogprob `json:"top_logprobs"`
}

type ChatCompletionTokenLogprobTopLogprob struct {
	Token   string  `json:"token"`
	Bytes   []int64 `json:"bytes"`
	Logprob float64 `json:"logprob"`
}

type ChatCompletionToolChoiceOptionUnionParam struct {
	OfAuto               Opt[string]                               `json:",omitzero,inline"`
	OfAllowedTools       *ChatCompletionAllowedToolChoiceParam     `json:",omitzero,inline"`
	OfFunctionToolChoice *ChatCompletionNamedToolChoiceParam       `json:",omitzero,inline"`
	OfCustomToolChoice   *ChatCompletionNamedToolChoiceCustomParam `json:",omitzero,inline"`
}

func (u ChatCompletionToolChoiceOptionUnionParam) MarshalJSON() ([]byte, error) {
	return marshalUnion(u.OfAuto, u.OfAllowedTools, u.OfFunctionToolChoice, u.OfCustomToolChoice)
}

func (u *ChatCompletionToolChoiceOptionUnionParam) UnmarshalJSON(data []byte) error {
	switch unionDiscriminant(data) {
	case "allowed_tools":
		return json.Unmarshal(data, &u.OfAllowedTools)
	case "function":
		return json.Unmarshal(data, &u.OfFunctionToolChoice)
	case "custom":
		return json.Unmarshal(data, &u.OfCustomToolChoice)
	}
	return tryUnmarshal(data, &u.OfAuto)
}

type ChatCompletionToolMessageParam struct {
	Content    ChatCompletionToolMessageParamContentUnion `json:"content,omitzero"`
	ToolCallID string                                     `json:"tool_call_id"`
	Role       string                                     `json:"role"`
	paramObj
}

func (p ChatCompletionToolMessageParam) MarshalJSON() ([]byte, error) {
	type raw ChatCompletionToolMessageParam
	return marshalObject(raw(p), p.ExtraFields(), map[string]any{"role": "tool"})
}

type ChatCompletionToolMessageParamContentUnion struct {
	OfString              Opt[string]                          `json:",omitzero,inline"`
	OfArrayOfContentParts []ChatCompletionContentPartTextParam `json:",omitzero,inline"`
}

func (u ChatCompletionToolMessageParamContentUnion) MarshalJSON() ([]byte, error) {
	return marshalUnion(u.OfString, u.OfArrayOfContentParts)
}

func (u *ChatCompletionToolMessageParamContentUnion) UnmarshalJSON(data []byte) error {
	return tryUnmarshal(data, &u.OfString, &u.OfArrayOfContentParts)
}

type ChatCompletionToolUnionParam struct {
	OfFunction *ChatCompletionFunctionToolParam `json:",omitzero,inline"`
	OfCustom   *ChatCompletionCustomToolParam   `json:",omitzero,inline"`
}

func (u ChatCompletionToolUnionParam) MarshalJSON() ([]byte, error) {
	return marshalUnion(u.OfFunction, u.OfCustom)
}

func (u *ChatCompletionToolUnionParam) UnmarshalJSON(data []byte) error {
	switch unionDiscriminant(data) {
	case "function":
		return json.Unmarshal(data, &u.OfFunction)
	case "custom":
		return json.Unmarshal(data, &u.OfCustom)
	}
	return tryUnmarshal(data)
}

type ChatCompletionUserMessageParam struct {
	Content ChatCompletionUserMessageParamContentUnion `json:"content,omitzero"`
	Name    Opt[string]                                `json:"name,omitzero"`
	Role    string                                     `json:"role"`
	paramObj
}

func (p ChatCompletionUserMessageParam) MarshalJSON() ([]byte, error) {
	type raw ChatCompletionUserMessageParam
	return marshalObject(raw(p), p.ExtraFields(), map[string]any{"role": "user"})
}

type ChatCompletionUserMessageParamContentUnion struct {
	OfString              Opt[string]                           `json:",omitzero,inline"`
	OfArrayOfContentParts []ChatCompletionContentPartUnionParam `json:",omitzero,inline"`
}

func (u ChatCompletionUserMessageParamContentUnion) MarshalJSON() ([]byte, error) {
	return marshalUnion(u.OfString, u.OfArrayOfContentParts)
}

func (u *ChatCompletionUserMessageParamContentUnion) UnmarshalJSON(data []byte) error {
	return tryUnmarshal(data, &u.OfString, &u.OfArrayOfContentParts)
}

type CompletionUsage struct {
	CompletionTokens        int64                                  `json:"completion_tokens"`
	PromptTokens            int64                                  `json:"prompt_tokens"`
	TotalTokens             int64                                  `json:"total_tokens"`
	CompletionTokensDetails CompletionUsageCompletionTokensDetails `json:"completion_tokens_details"`
	PromptTokensDetails     CompletionUsagePromptTokensDetails     `json:"prompt_tokens_details"`
	// Raw is the unmodified JSON payload of the object.
	Raw json.RawMessage `json:"-"`
	// ExtraFields holds non-standard fields returned by the endpoint.
	ExtraFields map[string]json.RawMessage `json:"-"`
}

// RawJSON returns the unmodified JSON payload.
func (r CompletionUsage) RawJSON() string { return string(r.Raw) }

func (r *CompletionUsage) UnmarshalJSON(data []byte) error {
	type raw CompletionUsage
	var v raw
	if err := json.Unmarshal(data, &v); err != nil {
		return err
	}
	*r = CompletionUsage(v)
	r.Raw = append(json.RawMessage(nil), data...)
	extras, err := decodeExtras(data, &v)
	if err != nil {
		return err
	}
	r.ExtraFields = extras
	return nil
}

type CompletionUsageCompletionTokensDetails struct {
	AcceptedPredictionTokens int64 `json:"accepted_prediction_tokens"`
	AudioTokens              int64 `json:"audio_tokens"`
	ReasoningTokens          int64 `json:"reasoning_tokens"`
	RejectedPredictionTokens int64 `json:"rejected_prediction_tokens"`
	TextTokens               int64 `json:"text_tokens"`
}

type CompletionUsagePromptTokensDetails struct {
	AudioTokens      int64 `json:"audio_tokens"`
	CacheWriteTokens int64 `json:"cache_write_tokens"`
	CachedTokens     int64 `json:"cached_tokens"`
	ImageTokens      int64 `json:"image_tokens"`
	TextTokens       int64 `json:"text_tokens"`
}

type FileCitationAnnotationFileCitation struct {
	FileID string `json:"file_id"`
}

type FilePathAnnotationFilePath struct {
	FileID string `json:"file_id"`
}

type ImageFile struct {
	FileID string          `json:"file_id"`
	Detail ImageFileDetail `json:"detail"`
}

type ImageFileDetail string

type ImageURL struct {
	URL    string         `json:"url"`
	Detail ImageURLDetail `json:"detail"`
}

type ImageURLDetail string

type Message struct {
	ID                string                   `json:"id"`
	AssistantID       string                   `json:"assistant_id"`
	Attachments       []MessageAttachment      `json:"attachments"`
	CompletedAt       int64                    `json:"completed_at"`
	Content           []MessageContentUnion    `json:"content"`
	CreatedAt         int64                    `json:"created_at"`
	IncompleteAt      int64                    `json:"incomplete_at"`
	IncompleteDetails MessageIncompleteDetails `json:"incomplete_details"`
	Metadata          Metadata                 `json:"metadata"`
	Object            string                   `json:"object"`
	Role              MessageRole              `json:"role"`
	RunID             string                   `json:"run_id"`
	Status            MessageStatus            `json:"status"`
	ThreadID          string                   `json:"thread_id"`
}

type MessageAttachment struct {
	FileID string                       `json:"file_id"`
	Tools  []MessageAttachmentToolUnion `json:"tools"`
}

type MessageAttachmentToolUnion struct {
	Type string `json:"type"`
}

type MessageContentUnion struct {
	ImageFile ImageFile `json:"image_file"`
	Type      string    `json:"type"`
	ImageURL  ImageURL  `json:"image_url"`
	Text      Text      `json:"text"`
	Refusal   string    `json:"refusal"`
}

type MessageIncompleteDetails struct {
	Reason string `json:"reason"`
}

type MessageRole string

type MessageStatus string

type Model struct {
	ID           string    `json:"id"`
	Created      int64     `json:"created"`
	Object       string    `json:"object"`
	OwnedBy      string    `json:"owned_by"`
	ShutdownDate time.Time `json:"shutdown_date"`
}

type Moderation struct {
	Categories                ModerationCategories                `json:"categories"`
	CategoryAppliedInputTypes ModerationCategoryAppliedInputTypes `json:"category_applied_input_types"`
	CategoryScores            ModerationCategoryScores            `json:"category_scores"`
	Flagged                   bool                                `json:"flagged"`
}

type ModerationCategories struct {
	Harassment            bool `json:"harassment"`
	HarassmentThreatening bool `json:"harassment/threatening"`
	Hate                  bool `json:"hate"`
	HateThreatening       bool `json:"hate/threatening"`
	Illicit               bool `json:"illicit"`
	IllicitViolent        bool `json:"illicit/violent"`
	SelfHarm              bool `json:"self-harm"`
	SelfHarmInstructions  bool `json:"self-harm/instructions"`
	SelfHarmIntent        bool `json:"self-harm/intent"`
	Sexual                bool `json:"sexual"`
	SexualMinors          bool `json:"sexual/minors"`
	Violence              bool `json:"violence"`
	ViolenceGraphic       bool `json:"violence/graphic"`
}

type ModerationCategoryAppliedInputTypes struct {
	Harassment            []string `json:"harassment"`
	HarassmentThreatening []string `json:"harassment/threatening"`
	Hate                  []string `json:"hate"`
	HateThreatening       []string `json:"hate/threatening"`
	Illicit               []string `json:"illicit"`
	IllicitViolent        []string `json:"illicit/violent"`
	SelfHarm              []string `json:"self-harm"`
	SelfHarmInstructions  []string `json:"self-harm/instructions"`
	SelfHarmIntent        []string `json:"self-harm/intent"`
	Sexual                []string `json:"sexual"`
	SexualMinors          []string `json:"sexual/minors"`
	Violence              []string `json:"violence"`
	ViolenceGraphic       []string `json:"violence/graphic"`
}

type ModerationCategoryScores struct {
	Harassment            float64 `json:"harassment"`
	HarassmentThreatening float64 `json:"harassment/threatening"`
	Hate                  float64 `json:"hate"`
	HateThreatening       float64 `json:"hate/threatening"`
	Illicit               float64 `json:"illicit"`
	IllicitViolent        float64 `json:"illicit/violent"`
	SelfHarm              float64 `json:"self-harm"`
	SelfHarmInstructions  float64 `json:"self-harm/instructions"`
	SelfHarmIntent        float64 `json:"self-harm/intent"`
	Sexual                float64 `json:"sexual"`
	SexualMinors          float64 `json:"sexual/minors"`
	Violence              float64 `json:"violence"`
	ViolenceGraphic       float64 `json:"violence/graphic"`
}

type Role struct {
	ID             string   `json:"id"`
	Description    string   `json:"description"`
	Name           string   `json:"name"`
	Object         string   `json:"object"`
	Permissions    []string `json:"permissions"`
	PredefinedRole bool     `json:"predefined_role"`
	ResourceType   string   `json:"resource_type"`
}

type Text struct {
	Annotations []AnnotationUnion `json:"annotations"`
	Value       string            `json:"value"`
}
