// Package models defines the provider-independent language model contract.
package models

import (
	"context"
	"encoding/json"
	"errors"
	"io"

	"clara-code/pkg/models/relay/internal/message"
)

// ToolDefinition is one function exposed to a model.
type ToolDefinition struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters"`
	Strict      bool            `json:"strict,omitempty"`
}

// ToolChoice controls tool selection.
type ToolChoice struct {
	Mode string `json:"mode,omitempty"`
	Name string `json:"name,omitempty"`
}

// RequestOptions configures inference for an Agent and its sessions.
type RequestOptions struct {
	ReasoningEffort  string
	ReasoningSummary string
	MaxOutputTokens  *int
	OutputFormat     json.RawMessage
}

// Apply adds the configured inference options to a request.
func (o RequestOptions) Apply(r *Request) {
	r.ReasoningEffort = o.ReasoningEffort
	r.ReasoningSummary = o.ReasoningSummary
	r.MaxOutputTokens = o.MaxOutputTokens
	r.OutputFormat = o.OutputFormat
}

// Request is one model inference request.
type Request struct {
	Model              string
	Instructions       string
	Messages           []message.Message
	Tools              []ToolDefinition
	ToolChoice         *ToolChoice
	Temperature        *float64
	TopP               *float64
	MaxOutputTokens    *int
	ParallelToolCalls  *bool
	Include            []string
	ReasoningEffort    string
	ReasoningSummary   string
	TextVerbosity      string
	OutputFormat       json.RawMessage
	PromptCacheKey     string
	PreviousResponseID string
	Store              *bool
	Metadata           map[string]string
	Extra              map[string]json.RawMessage
}

// Capabilities describes the features of a model.
type Capabilities struct {
	Tools            bool
	Images           bool
	Files            bool
	Audio            bool
	Reasoning        bool
	PromptCaching    bool
	RemoteCompaction bool
	WebSocket        bool
}

// Info identifies a model and its runtime constraints.
type Info struct {
	Provider      string
	ID            string
	ContextWindow int
	MaxOutput     int
	Capabilities  Capabilities
}

// Usage reports token and cost accounting.
type Usage struct {
	InputTokens      int64   `json:"input_tokens"`
	OutputTokens     int64   `json:"output_tokens"`
	TotalTokens      int64   `json:"total_tokens"`
	ReasoningTokens  int64   `json:"reasoning_tokens"`
	CacheReadTokens  int64   `json:"cache_read_tokens"`
	CacheWriteTokens int64   `json:"cache_write_tokens"`
	CostUSD          float64 `json:"cost_usd,omitempty"`
}

// FinishReason describes why generation stopped.
type FinishReason string

const (
	FinishStop          FinishReason = "stop"
	FinishLength        FinishReason = "length"
	FinishToolCalls     FinishReason = "tool_calls"
	FinishContentFilter FinishReason = "content_filter"
	FinishError         FinishReason = "error"
	FinishOther         FinishReason = "other"
)

// Response is a complete model response.
type Response struct {
	ID           string
	Model        string
	Message      message.Message
	FinishReason FinishReason
	Usage        Usage
	ProviderData json.RawMessage
}

// StreamEventType identifies a normalized stream event.
type StreamEventType string

const (
	StreamCreated        StreamEventType = "created"
	StreamTextDelta      StreamEventType = "text_delta"
	StreamTextDone       StreamEventType = "text_done"
	StreamRefusalDelta   StreamEventType = "refusal_delta"
	StreamReasoningDelta StreamEventType = "reasoning_delta"
	StreamReasoningDone  StreamEventType = "reasoning_done"
	StreamToolCallStart  StreamEventType = "tool_call_start"
	StreamToolCallDelta  StreamEventType = "tool_call_delta"
	StreamToolCallDone   StreamEventType = "tool_call_done"
	StreamUsage          StreamEventType = "usage"
	StreamCompleted      StreamEventType = "completed"
	StreamFailed         StreamEventType = "failed"
	StreamProviderEvent  StreamEventType = "provider_event"
)

// StreamEvent is one normalized provider stream event.
type StreamEvent struct {
	Type         StreamEventType
	Index        int
	ItemID       string
	CallID       string
	ToolName     string
	Delta        string
	Arguments    string
	Usage        *Usage
	Response     *Response
	ProviderType string
	ProviderData json.RawMessage
	Err          error
}

// Stream is a pull-based response stream.
type Stream interface {
	Next(context.Context) (StreamEvent, error)
	Close() error
}

// Model generates model responses.
type Model interface {
	Info() Info
	Stream(context.Context, Request) (Stream, error)
}

// Generate drains a stream and returns its terminal response.
func Generate(ctx context.Context, model Model, request Request) (*Response, error) {
	return Consume(ctx, model, request, nil)
}

// Consume observes the stream and returns its final response.
func Consume(ctx context.Context, model Model, request Request, observe func(StreamEvent) error) (response *Response, err error) {
	if model == nil {
		return nil, errors.New("models: model is nil")
	}
	stream, err := model.Stream(ctx, request)
	if err != nil {
		return nil, err
	}
	defer func() { err = errors.Join(err, stream.Close()) }()
	var usage Usage
	for {
		event, err := stream.Next(ctx)
		if errors.Is(err, io.EOF) {
			return nil, errors.New("models: stream ended without a response")
		}
		if err != nil {
			return nil, err
		}
		if observe != nil {
			if err := observe(event); err != nil {
				return nil, err
			}
		}
		if event.Usage != nil {
			usage = *event.Usage
		}
		if event.Type == StreamFailed {
			if event.Err != nil {
				return nil, event.Err
			}
			return nil, errors.New("models: stream failed")
		}
		if event.Type == StreamCompleted {
			if event.Response == nil {
				return nil, errors.New("models: completed stream has no response")
			}
			result := *event.Response
			if result.Usage == (Usage{}) {
				result.Usage = usage
			}
			return &result, nil
		}
	}
}

// Add accumulates the usage of a completed model request.
func (u *Usage) Add(next Usage) {
	u.InputTokens += next.InputTokens
	u.OutputTokens += next.OutputTokens
	u.TotalTokens += next.TotalTokens
	u.ReasoningTokens += next.ReasoningTokens
	u.CacheReadTokens += next.CacheReadTokens
	u.CacheWriteTokens += next.CacheWriteTokens
	u.CostUSD += next.CostUSD
}
