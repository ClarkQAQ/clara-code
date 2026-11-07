// Package completions implements the Chat Completions language model adapter.
package completions

import (
	"bytes"
	"clara-code/pkg/util/errs"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"

	"clara-code/pkg/models/relay/internal/message"
	"clara-code/pkg/models/relay/internal/models"
	wire "clara-code/pkg/models/relay/internal/models/protocol/completions"
)

const defaultUserAgent = "clara-core/dev"

// HTTPDoer is the HTTP client contract used by the adapter.
type HTTPDoer interface {
	Do(*http.Request) (*http.Response, error)
}

// Config configures one Chat Completions endpoint.
type Config struct {
	BaseURL       string
	APIKey        string
	Model         string
	Provider      string
	Headers       http.Header
	HTTPClient    HTTPDoer
	UserAgent     string
	ContextWindow int
	MaxOutput     int
	MaxRetries    int
}

// Client talks to one Chat Completions-compatible endpoint.
type Client struct {
	config Config
	info   models.Info
}

// New creates a Chat Completions model. BaseURL is required.
func New(config Config) (*Client, error) {
	config.BaseURL = strings.TrimRight(strings.TrimSpace(config.BaseURL), "/")
	if config.BaseURL == "" {
		return nil, errors.New("completions: base URL is required")
	}
	parsed, err := url.Parse(config.BaseURL)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return nil, fmt.Errorf("completions: invalid base URL %q", config.BaseURL)
	}
	if strings.TrimSpace(config.Model) == "" {
		return nil, errors.New("completions: model is required")
	}
	config.Headers = config.Headers.Clone()
	if config.HTTPClient == nil {
		config.HTTPClient = &http.Client{Timeout: 10 * time.Minute}
	}
	if config.UserAgent == "" {
		config.UserAgent = defaultUserAgent
	}
	if config.Provider == "" {
		config.Provider = "completions"
	}
	return &Client{
		config: config,
		info: models.Info{
			Provider: config.Provider, ID: config.Model,
			ContextWindow: config.ContextWindow, MaxOutput: config.MaxOutput,
			Capabilities: models.Capabilities{
				Tools: true, Images: true, Reasoning: true, PromptCaching: true,
			},
		},
	}, nil
}

func (c *Client) Info() models.Info { return c.info }

func (c *Client) Stream(ctx context.Context, request models.Request) (models.Stream, error) {
	body, err := c.requestBody(request)
	if err != nil {
		return nil, err
	}
	response, err := c.post(ctx, body) //nolint:bodyclose // stream.Close owns the body.
	if err != nil {
		return nil, err
	}
	return &stream{
		decoder:  wire.NewSSEDecoder(response.Body),
		response: &models.Response{Model: c.config.Model},
		calls:    map[int]*toolCallState{},
	}, nil
}

func (c *Client) requestBody(request models.Request) ([]byte, error) {
	if request.Model == "" {
		request.Model = c.config.Model
	}
	messages, err := toMessages(request)
	if err != nil {
		return nil, err
	}
	tools := make([]wire.Tool, 0, len(request.Tools))
	for _, tool := range request.Tools {
		parameters := tool.Parameters
		if len(parameters) == 0 {
			parameters = json.RawMessage(`{"type":"object","properties":{}}`)
		}
		tools = append(tools, wire.Tool{Type: "function", Function: wire.ToolFunction{
			Name: tool.Name, Description: tool.Description,
			Parameters: parameters, Strict: tool.Strict,
		}})
	}
	var toolChoice any
	if request.ToolChoice != nil {
		switch request.ToolChoice.Mode {
		case "specific":
			toolChoice = map[string]any{
				"type":     "function",
				"function": map[string]string{"name": request.ToolChoice.Name},
			}
		default:
			toolChoice = request.ToolChoice.Mode
		}
	}
	responseFormat, err := models.CompletionsFormat(request.OutputFormat)
	if err != nil {
		return nil, err
	}
	payload := wire.Request{
		Model: request.Model, Messages: messages, Tools: tools,
		ToolChoice: toolChoice, Temperature: request.Temperature, TopP: request.TopP,
		MaxTokens: request.MaxOutputTokens, ParallelToolCalls: request.ParallelToolCalls,
		Stream: true, StreamOptions: &wire.StreamOptions{IncludeUsage: true},
		ResponseFormat: responseFormat, Extra: request.Extra,
		ReasoningEffort: request.ReasoningEffort, PromptCacheKey: request.PromptCacheKey,
	}
	return json.Marshal(payload)
}

func toMessages(request models.Request) ([]wire.Message, error) {
	out := make([]wire.Message, 0, len(request.Messages)+1)
	if request.Instructions != "" {
		out = append(out, wire.Message{Role: "system", Content: request.Instructions})
	}
	for _, msg := range request.Messages {
		switch msg.Role {
		case message.RoleSystem, message.RoleDeveloper:
			text := msg.Text()
			if text != "" {
				out = append(out, wire.Message{Role: "system", Content: text})
			}
		case message.RoleUser:
			content, err := userContent(msg)
			if err != nil {
				return nil, err
			}
			out = append(out, wire.Message{Role: "user", Content: content})
		case message.RoleAssistant:
			text := msg.Text()
			var calls []wire.ToolCall
			for _, part := range msg.Parts {
				if part.Type == message.PartToolCall && part.ToolCall != nil {
					calls = append(calls, wire.ToolCall{
						ID: part.ToolCall.ID, Type: "function",
						Function: wire.FunctionCall{
							Name: part.ToolCall.Name, Arguments: part.ToolCall.Arguments,
						},
					})
				}
			}
			reasoning := msg.ReasoningText()
			if text != "" || len(calls) > 0 || reasoning != "" {
				out = append(out, wire.Message{
					Role: "assistant", Content: nullableText(text),
					ToolCalls: calls, ReasoningContent: reasoning,
				})
			}
		case message.RoleTool:
			for _, part := range msg.Parts {
				if part.Type != message.PartToolResult || part.ToolResult == nil {
					continue
				}
				out = append(out, wire.Message{
					Role: "tool", ToolCallID: part.ToolResult.CallID,
					Content: completionToolContent(part.ToolResult),
				})
			}
		default:
			return nil, fmt.Errorf("completions: unsupported message role %q", msg.Role)
		}
	}
	return out, nil
}

func completionToolContent(result *message.ToolResult) any {
	if len(result.Content) == 0 {
		return result.Output
	}
	parts := make([]wire.ContentPart, 0, len(result.Content)+1)
	if result.Output != "" {
		parts = append(parts, wire.ContentPart{Type: "text", Text: result.Output})
	}
	for _, nested := range result.Content {
		switch nested.Type {
		case message.PartText:
			parts = append(parts, wire.ContentPart{Type: "text", Text: nested.Text})
		case message.PartImage:
			if nested.Image == nil {
				continue
			}
			value := nested.Image.URL
			if value == "" && len(nested.Image.Data) > 0 {
				mime := nested.Image.MIME
				if mime == "" {
					mime = "image/png"
				}
				value = message.DataURL(nested.Image.Data, mime)
			}
			parts = append(parts, wire.ContentPart{
				Type: "image_url", ImageURL: &wire.ImageURL{URL: value, Detail: nested.Image.Detail},
			})
		}
	}
	return parts
}

func nullableText(text string) any {
	if text == "" {
		return nil
	}
	return text
}

func userContent(msg message.Message) (any, error) {
	parts := make([]wire.ContentPart, 0, len(msg.Parts))
	multimodal := false
	for _, part := range msg.Parts {
		switch part.Type {
		case message.PartText:
			parts = append(parts, wire.ContentPart{Type: "text", Text: part.Text})
		case message.PartImage:
			if part.Image == nil {
				continue
			}
			multimodal = true
			value := part.Image.URL
			if value == "" && len(part.Image.Data) > 0 {
				mime := part.Image.MIME
				if mime == "" {
					mime = "image/png"
				}
				value = message.DataURL(part.Image.Data, mime)
			}
			parts = append(parts, wire.ContentPart{
				Type: "image_url", ImageURL: &wire.ImageURL{URL: value, Detail: part.Image.Detail},
			})
		case message.PartFile:
			return nil, errors.New("completions: file inputs are not supported by this protocol")
		}
	}
	if !multimodal && len(parts) <= 1 {
		if len(parts) == 0 {
			return "", nil
		}
		return parts[0].Text, nil
	}
	return parts, nil
}

func (c *Client) post(ctx context.Context, body []byte) (*http.Response, error) {
	client := c.config.HTTPClient
	endpoint := c.config.BaseURL + "/chat/completions"
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "text/event-stream")
	request.Header.Set("User-Agent", c.config.UserAgent)
	for key, values := range c.config.Headers {
		request.Header[key] = append([]string(nil), values...)
	}
	if c.config.APIKey != "" {
		request.Header.Set("Authorization", "Bearer "+c.config.APIKey)
	}
	response, err := client.Do(request)
	if err != nil {
		return nil, err
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		defer response.Body.Close()
		payload, _ := io.ReadAll(io.LimitReader(response.Body, 1<<20))
		var envelope wire.ErrorEnvelope
		if json.Unmarshal(payload, &envelope) == nil && envelope.Error.Message != "" {
			return nil, fmt.Errorf("completions: HTTP %d: %s", response.StatusCode, envelope.Error.Message)
		}
		return nil, fmt.Errorf("completions: HTTP %d: %s", response.StatusCode, strings.TrimSpace(string(payload)))
	}
	return response, nil
}

type toolCallState struct {
	id        string
	started   bool
	name      string
	arguments strings.Builder
}

type stream struct {
	decoder                  *wire.SSEDecoder
	mu                       sync.Mutex
	closed, terminal         bool
	response                 *models.Response
	calls                    map[int]*toolCallState
	text, reasoning, refusal strings.Builder
	usage                    models.Usage
	finishReason             string
	pending                  []models.StreamEvent
	bytes                    int
}

func (s *stream) Next(ctx context.Context) (models.StreamEvent, error) {
	if err := ctx.Err(); err != nil {
		return models.StreamEvent{}, err
	}
	s.mu.Lock()
	done := s.closed || s.terminal
	s.mu.Unlock()
	if done {
		return models.StreamEvent{}, io.EOF
	}
	stop := context.AfterFunc(ctx, func() { _ = s.Close() })
	defer stop()
	for {
		if len(s.pending) > 0 {
			event := s.pending[0]
			s.pending = s.pending[1:]
			return event, nil
		}
		if !s.decoder.Next() {
			err := s.decoder.Err()
			if ctx.Err() != nil {
				return models.StreamEvent{}, ctx.Err()
			}
			if err != nil && !errors.Is(err, io.EOF) {
				return s.finish(err)
			}
			return s.finish(nil)
		}
		if s.decoder.Done() {
			return s.finish(nil)
		}
		raw := s.decoder.Data()
		s.bytes += len(raw)
		if s.bytes > 64<<20 {
			return s.finish(errs.New(errs.CodeLimit, "model stream exceeds the size limit"))
		}
		var chunk wire.StreamChunk
		if err := json.Unmarshal(raw, &chunk); err != nil {
			return s.finish(fmt.Errorf("completions: decode stream chunk: %w", err))
		}
		if chunk.Error != nil {
			return s.finish(errors.New(chunk.Error.Message))
		}
		if err := s.accept(chunk); err != nil {
			return s.finish(err)
		}
	}
}

func (s *stream) accept(chunk wire.StreamChunk) error {
	if chunk.ID != "" {
		s.response.ID = chunk.ID
	}
	if chunk.Model != "" {
		s.response.Model = chunk.Model
	}
	if chunk.Usage != nil {
		s.usage = fromUsage(*chunk.Usage)
		usage := s.usage
		s.pending = append(s.pending, models.StreamEvent{Type: models.StreamUsage, Usage: &usage})
	}
	for _, choice := range chunk.Choices {
		if choice.Index != 0 {
			continue
		}
		if choice.FinishReason != "" {
			s.finishReason = choice.FinishReason
		}
		delta := choice.Delta
		reasoning := delta.ReasoningContent
		if reasoning == "" {
			reasoning = delta.Reasoning
		}
		s.addDelta(&s.reasoning, models.StreamReasoningDelta, reasoning)
		s.addDelta(&s.text, models.StreamTextDelta, delta.Content)
		s.addDelta(&s.refusal, models.StreamRefusalDelta, delta.Refusal)
		for _, fragment := range delta.ToolCalls {
			if fragment.Index < 0 || fragment.Index >= 128 {
				return errs.New(errs.CodeLimit, "invalid tool call index")
			}
			call := s.calls[fragment.Index]
			if call == nil {
				call = &toolCallState{}
				s.calls[fragment.Index] = call
			}
			if fragment.ID != "" {
				call.id = fragment.ID
			}
			if fragment.Function.Name != "" {
				call.name += fragment.Function.Name
			}
			if !call.started && call.id != "" && call.name != "" {
				call.started = true
				s.pending = append(s.pending, models.StreamEvent{Type: models.StreamToolCallStart, Index: fragment.Index, CallID: call.id, ToolName: call.name})
			}
			if fragment.Function.Arguments != "" {
				call.arguments.WriteString(fragment.Function.Arguments)
				if call.arguments.Len() > 8<<20 {
					return errs.New(errs.CodeLimit, "tool arguments exceed the size limit")
				}
				s.pending = append(s.pending, models.StreamEvent{Type: models.StreamToolCallDelta, Index: fragment.Index, CallID: call.id, ToolName: call.name, Delta: fragment.Function.Arguments})
			}
		}
	}
	return nil
}

func (s *stream) addDelta(buf *strings.Builder, kind models.StreamEventType, delta string) {
	if delta == "" {
		return
	}
	buf.WriteString(delta)
	s.pending = append(s.pending, models.StreamEvent{Type: kind, Delta: delta})
}

func (s *stream) finish(err error) (models.StreamEvent, error) {
	s.mu.Lock()
	if s.terminal {
		s.mu.Unlock()
		return models.StreamEvent{}, io.EOF
	}
	s.terminal = true
	s.mu.Unlock()
	if err == nil && s.finishReason == "" {
		err = errs.New(errs.CodeInternal, "Completions stream ended before finish_reason")
	}
	if err != nil {
		return models.StreamEvent{Type: models.StreamFailed, Err: err}, nil
	}
	var parts []message.Part
	if s.reasoning.Len() > 0 {
		parts = append(parts, message.ReasoningPart(message.Reasoning{Text: s.reasoning.String()}))
	}
	if s.text.Len() > 0 {
		parts = append(parts, message.Text(s.text.String()))
	}
	if s.refusal.Len() > 0 {
		parts = append(parts, message.Refusal(s.refusal.String()))
	}
	indices := make([]int, 0, len(s.calls))
	for index := range s.calls {
		indices = append(indices, index)
	}
	sort.Ints(indices)
	for _, index := range indices {
		call := s.calls[index]
		parts = append(parts, message.ToolCallPart(message.ToolCall{ID: call.id, Name: call.name, Arguments: call.arguments.String()}))
	}
	reason := models.FinishReason(s.finishReason)
	switch reason {
	case models.FinishStop, models.FinishLength, models.FinishContentFilter, models.FinishToolCalls:
	default:
		reason = models.FinishOther
	}
	response := &models.Response{ID: s.response.ID, Model: s.response.Model, Message: message.Assistant(parts...), Usage: s.usage, FinishReason: reason}
	return models.StreamEvent{Type: models.StreamCompleted, Response: response}, nil
}

func (s *stream) Close() error {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil
	}
	s.closed = true
	s.terminal = true
	s.mu.Unlock()
	return s.decoder.Close()
}

func fromUsage(usage wire.Usage) models.Usage {
	return models.Usage{
		InputTokens: usage.PromptTokens, OutputTokens: usage.CompletionTokens,
		TotalTokens:     usage.TotalTokens,
		ReasoningTokens: usage.CompletionTokensDetails.ReasoningTokens,
		CacheReadTokens: usage.PromptTokensDetails.CachedTokens,
	}
}
