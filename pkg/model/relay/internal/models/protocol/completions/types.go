// Package completions defines the Chat Completions wire model used by Clara Core.
package completions

import "encoding/json"

// Request is a Chat Completions request.
type Request struct {
	Model               string                     `json:"model"`
	Messages            []Message                  `json:"messages"`
	Tools               []Tool                     `json:"tools,omitempty"`
	ToolChoice          any                        `json:"tool_choice,omitempty"`
	Temperature         *float64                   `json:"temperature,omitempty"`
	TopP                *float64                   `json:"top_p,omitempty"`
	MaxTokens           *int                       `json:"max_tokens,omitempty"`
	ParallelToolCalls   *bool                      `json:"parallel_tool_calls,omitempty"`
	Stream              bool                       `json:"stream,omitempty"`
	ReasoningEffort     string                     `json:"reasoning_effort,omitempty"`
	PromptCacheKey      string                     `json:"prompt_cache_key,omitempty"`
	MaxCompletionTokens *int                       `json:"max_completion_tokens,omitempty"`
	StreamOptions       *StreamOptions             `json:"stream_options,omitempty"`
	ResponseFormat      any                        `json:"response_format,omitempty"`
	Extra               map[string]json.RawMessage `json:"-"`
}

// MarshalJSON merges provider extension fields into the request.
func (r Request) MarshalJSON() ([]byte, error) {
	type alias Request
	raw, err := json.Marshal(alias(r))
	if err != nil {
		return nil, err
	}
	if len(r.Extra) == 0 {
		return raw, nil
	}
	fields := map[string]json.RawMessage{}
	if err := json.Unmarshal(raw, &fields); err != nil {
		return nil, err
	}
	for key, value := range r.Extra {
		if _, exists := fields[key]; !exists {
			fields[key] = value
		}
	}
	return json.Marshal(fields)
}

// StreamOptions requests usage in the final streamed chunk.
type StreamOptions struct {
	IncludeUsage bool `json:"include_usage"`
}

type Message struct {
	Role             string     `json:"role"`
	Content          any        `json:"content,omitempty"`
	Name             string     `json:"name,omitempty"`
	ToolCallID       string     `json:"tool_call_id,omitempty"`
	ToolCalls        []ToolCall `json:"tool_calls,omitempty"`
	ReasoningContent string     `json:"reasoning_content,omitempty"`
}

type ContentPart struct {
	Type     string    `json:"type"`
	Text     string    `json:"text,omitempty"`
	ImageURL *ImageURL `json:"image_url,omitempty"`
}

type ImageURL struct {
	URL    string `json:"url"`
	Detail string `json:"detail,omitempty"`
}

type Tool struct {
	Type     string       `json:"type"`
	Function ToolFunction `json:"function"`
}

type ToolFunction struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Parameters  json.RawMessage `json:"parameters,omitempty"`
	Strict      bool            `json:"strict,omitempty"`
}

type ToolCall struct {
	ID       string       `json:"id"`
	Type     string       `json:"type"`
	Function FunctionCall `json:"function"`
}

type FunctionCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type Response struct {
	ID      string   `json:"id"`
	Object  string   `json:"object"`
	Model   string   `json:"model"`
	Choices []Choice `json:"choices"`
	Usage   *Usage   `json:"usage"`
}

type Choice struct {
	Index        int     `json:"index"`
	Message      Message `json:"message"`
	FinishReason string  `json:"finish_reason"`
}

type Usage struct {
	PromptTokens        int64 `json:"prompt_tokens"`
	CompletionTokens    int64 `json:"completion_tokens"`
	TotalTokens         int64 `json:"total_tokens"`
	PromptTokensDetails struct {
		CachedTokens int64 `json:"cached_tokens"`
	} `json:"prompt_tokens_details"`
	CompletionTokensDetails struct {
		ReasoningTokens int64 `json:"reasoning_tokens"`
	} `json:"completion_tokens_details"`
}

type StreamChunk struct {
	ID      string         `json:"id"`
	Object  string         `json:"object"`
	Model   string         `json:"model"`
	Choices []StreamChoice `json:"choices"`
	Usage   *Usage         `json:"usage"`
	Error   *APIError      `json:"error,omitempty"`
}

type StreamChoice struct {
	Index        int         `json:"index"`
	Delta        StreamDelta `json:"delta"`
	FinishReason string      `json:"finish_reason"`
}

type StreamDelta struct {
	Role             string           `json:"role,omitempty"`
	Content          string           `json:"content,omitempty"`
	ReasoningContent string           `json:"reasoning_content,omitempty"`
	Reasoning        string           `json:"reasoning,omitempty"`
	Refusal          string           `json:"refusal,omitempty"`
	ToolCalls        []StreamToolCall `json:"tool_calls,omitempty"`
}

type StreamToolCall struct {
	Index    int                `json:"index"`
	ID       string             `json:"id,omitempty"`
	Type     string             `json:"type,omitempty"`
	Function StreamFunctionCall `json:"function"`
}

type StreamFunctionCall struct {
	Name      string `json:"name,omitempty"`
	Arguments string `json:"arguments,omitempty"`
}

type APIError struct {
	Type    string `json:"type,omitempty"`
	Code    any    `json:"code,omitempty"`
	Message string `json:"message"`
	Param   string `json:"param,omitempty"`
}

type ErrorEnvelope struct {
	Error APIError `json:"error"`
}
