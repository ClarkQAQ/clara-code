// Package responses defines the OpenResponses wire model used by Clara Core.
package responses

import "encoding/json"

// CreateRequest is the OpenResponses request body.
type CreateRequest struct {
	Model              string                     `json:"model"`
	Input              any                        `json:"input,omitempty"`
	PreviousResponseID *string                    `json:"previous_response_id,omitempty"`
	Instructions       *string                    `json:"instructions,omitempty"`
	Tools              []Tool                     `json:"tools,omitempty"`
	ToolChoice         any                        `json:"tool_choice,omitempty"`
	Metadata           map[string]string          `json:"metadata,omitempty"`
	Text               any                        `json:"text,omitempty"`
	Temperature        *float64                   `json:"temperature,omitempty"`
	TopP               *float64                   `json:"top_p,omitempty"`
	ParallelToolCalls  *bool                      `json:"parallel_tool_calls,omitempty"`
	Stream             bool                       `json:"stream,omitempty"`
	MaxOutputTokens    *int                       `json:"max_output_tokens,omitempty"`
	Reasoning          *Reasoning                 `json:"reasoning,omitempty"`
	PromptCacheKey     string                     `json:"prompt_cache_key,omitempty"`
	Store              *bool                      `json:"store,omitempty"`
	Include            []string                   `json:"include,omitempty"`
	Extra              map[string]json.RawMessage `json:"-"`
}

// MarshalJSON merges provider extension fields into the request.
func (r CreateRequest) MarshalJSON() ([]byte, error) {
	type alias CreateRequest
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

// Tool is one function tool.
type Tool struct {
	Type        string          `json:"type"`
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Parameters  json.RawMessage `json:"parameters,omitempty"`
	Strict      bool            `json:"strict,omitempty"`
}

// Reasoning controls reasoning output.
type Reasoning struct {
	Effort  string `json:"effort,omitempty"`
	Summary string `json:"summary,omitempty"`
}

// InputMessage is a message item in a request or response.
type InputMessage struct {
	Type    string `json:"type"`
	ID      string `json:"id,omitempty"`
	Role    string `json:"role,omitempty"`
	Status  string `json:"status,omitempty"`
	Phase   string `json:"phase,omitempty"`
	Content any    `json:"content,omitempty"`
}

// InputText is one text content part.
type InputText struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// OutputText is one assistant output text part.
type OutputText struct {
	Type        string            `json:"type"`
	Text        string            `json:"text"`
	Annotations []json.RawMessage `json:"annotations,omitempty"`
}

// ImageInput is one image content part.
type ImageInput struct {
	Type     string `json:"type"`
	ImageURL string `json:"image_url"`
	Detail   string `json:"detail,omitempty"`
}

// FileInput is one file content part.
type FileInput struct {
	Type     string `json:"type"`
	Filename string `json:"filename,omitempty"`
	FileData string `json:"file_data,omitempty"`
	FileURL  string `json:"file_url,omitempty"`
}

// Refusal is one refusal content part.
type Refusal struct {
	Type    string `json:"type"`
	Refusal string `json:"refusal"`
}

// FunctionCall is a model tool call.
type FunctionCall struct {
	Type      string `json:"type"`
	ID        string `json:"id,omitempty"`
	CallID    string `json:"call_id"`
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
	Status    string `json:"status,omitempty"`
}

// FunctionCallOutput returns a tool result.
type FunctionCallOutput struct {
	Type   string `json:"type"`
	ID     string `json:"id,omitempty"`
	CallID string `json:"call_id"`
	Output any    `json:"output"`
	Status string `json:"status,omitempty"`
}

// ReasoningItem is a model reasoning item.
type ReasoningItem struct {
	Type             string    `json:"type"`
	ID               string    `json:"id,omitempty"`
	Summary          []Summary `json:"summary"`
	Content          any       `json:"content,omitempty"`
	EncryptedContent string    `json:"encrypted_content,omitempty"`
	Status           string    `json:"status,omitempty"`
}

// Summary is reasoning summary text.
type Summary struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// ResponseResource is an OpenResponses response object.
type ResponseResource struct {
	ID                string `json:"id"`
	Object            string `json:"object"`
	CreatedAt         int64  `json:"created_at"`
	CompletedAt       *int64 `json:"completed_at"`
	Status            string `json:"status"`
	IncompleteDetails *struct {
		Reason string `json:"reason"`
	} `json:"incomplete_details"`
	Model              string            `json:"model"`
	PreviousResponseID *string           `json:"previous_response_id"`
	Instructions       *string           `json:"instructions"`
	Output             []json.RawMessage `json:"output"`
	Error              *APIError         `json:"error"`
	Usage              *Usage            `json:"usage"`
	Metadata           map[string]any    `json:"metadata,omitempty"`
	PromptCacheKey     *string           `json:"prompt_cache_key,omitempty"`
}

// Usage is OpenResponses token accounting.
type Usage struct {
	InputTokens        int64 `json:"input_tokens"`
	OutputTokens       int64 `json:"output_tokens"`
	TotalTokens        int64 `json:"total_tokens"`
	InputTokensDetails struct {
		CachedTokens int64 `json:"cached_tokens"`
	} `json:"input_tokens_details"`
	OutputTokensDetails struct {
		ReasoningTokens int64 `json:"reasoning_tokens"`
	} `json:"output_tokens_details"`
}

// APIError is the wire error shape.
type APIError struct {
	Type    string `json:"type,omitempty"`
	Code    string `json:"code,omitempty"`
	Message string `json:"message"`
	Param   string `json:"param,omitempty"`
}

// ErrorEnvelope wraps non-streaming errors.
type ErrorEnvelope struct {
	Error APIError `json:"error"`
}

// StreamEvent is one streaming event.
type StreamEvent struct {
	Type           string          `json:"type"`
	SequenceNumber int64           `json:"sequence_number,omitempty"`
	Response       json.RawMessage `json:"response,omitempty"`
	Item           json.RawMessage `json:"item,omitempty"`
	ItemID         string          `json:"item_id,omitempty"`
	OutputIndex    int             `json:"output_index,omitempty"`
	ContentIndex   int             `json:"content_index,omitempty"`
	SummaryIndex   int             `json:"summary_index,omitempty"`
	Delta          string          `json:"delta,omitempty"`
	Text           string          `json:"text,omitempty"`
	Arguments      string          `json:"arguments,omitempty"`
	Part           json.RawMessage `json:"part,omitempty"`
	Error          *APIError       `json:"error,omitempty"`
}

// SSEEvent is one parsed server-sent event.
type SSEEvent struct {
	Type string
	Data []byte
}
