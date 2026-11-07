// Package responses implements the OpenResponses language model adapter.
package responses

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
	"strconv"
	"strings"
	"sync"
	"time"

	"clara-code/pkg/model/relay/internal/message"
	"clara-code/pkg/model/relay/internal/models"
	wire "clara-code/pkg/model/relay/internal/models/protocol/responses"
)

const defaultUserAgent = "clara-core/dev"

// HTTPDoer is the HTTP client contract used by the adapter.
type HTTPDoer interface {
	Do(*http.Request) (*http.Response, error)
}

// Config configures one OpenResponses endpoint.
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

// Client talks to one OpenResponses-compatible endpoint.
type Client struct {
	config Config
	info   models.Info
}

// New creates a Responses model. BaseURL is required.
func New(config Config) (*Client, error) {
	config.BaseURL = strings.TrimRight(strings.TrimSpace(config.BaseURL), "/")
	if config.BaseURL == "" {
		return nil, errors.New("responses: base URL is required")
	}
	parsed, err := url.Parse(config.BaseURL)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return nil, fmt.Errorf("responses: invalid base URL %q", config.BaseURL)
	}
	if strings.TrimSpace(config.Model) == "" {
		return nil, errors.New("responses: model is required")
	}
	config.Headers = config.Headers.Clone()
	if config.HTTPClient == nil {
		config.HTTPClient = &http.Client{Timeout: 10 * time.Minute}
	}
	config.MaxRetries = min(5, max(0, config.MaxRetries))
	if config.UserAgent == "" {
		config.UserAgent = defaultUserAgent
	}
	if config.Provider == "" {
		config.Provider = "responses"
	}
	return &Client{
		config: config,
		info: models.Info{
			Provider: config.Provider, ID: config.Model,
			ContextWindow: config.ContextWindow, MaxOutput: config.MaxOutput,
			Capabilities: models.Capabilities{
				Tools: true, Images: true, Files: true, Reasoning: true,
				PromptCaching: true,
			},
		},
	}, nil
}

func (c *Client) Info() models.Info { return c.info }

// Stream sends one request and returns a normalized stream.
func (c *Client) Stream(ctx context.Context, request models.Request) (models.Stream, error) {
	body, err := c.requestBody(request)
	if err != nil {
		return nil, err
	}
	response, err := c.post(ctx, body) //nolint:bodyclose // stream.Close owns the body.
	if err != nil {
		return nil, err
	}
	decoder := wire.NewSSEDecoder(response.Body)
	return &stream{
		decoder:   decoder,
		response:  &models.Response{Model: c.config.Model},
		itemCalls: map[string]wire.FunctionCall{},
	}, nil
}

func (c *Client) requestBody(request models.Request) ([]byte, error) {
	if request.Model == "" {
		request.Model = c.config.Model
	}
	input, include, err := toInput(request.Messages)
	if err != nil {
		return nil, err
	}
	if len(request.Include) > 0 {
		seen := map[string]bool{}
		for _, field := range append(include, request.Include...) {
			seen[field] = true
		}
		include = include[:0]
		for field := range seen {
			include = append(include, field)
		}
		sort.Strings(include)
	}
	tools := make([]wire.Tool, 0, len(request.Tools))
	for _, tool := range request.Tools {
		parameters := tool.Parameters
		if len(parameters) == 0 {
			parameters = json.RawMessage(`{"type":"object","properties":{}}`)
		}
		tools = append(tools, wire.Tool{
			Type: "function", Name: tool.Name, Description: tool.Description,
			Parameters: parameters, Strict: tool.Strict,
		})
	}
	var instructions *string
	if request.Instructions != "" {
		instructions = &request.Instructions
	}
	var previous *string
	if request.PreviousResponseID != "" {
		previous = &request.PreviousResponseID
	}
	var toolChoice any
	if request.ToolChoice != nil {
		switch request.ToolChoice.Mode {
		case "specific":
			toolChoice = map[string]string{"type": "function", "name": request.ToolChoice.Name}
		default:
			toolChoice = request.ToolChoice.Mode
		}
	}
	var reasoning *wire.Reasoning
	if request.ReasoningEffort != "" || request.ReasoningSummary != "" {
		reasoning = &wire.Reasoning{
			Effort: request.ReasoningEffort, Summary: request.ReasoningSummary,
		}
	}
	var text any
	if len(request.OutputFormat) > 0 || request.TextVerbosity != "" {
		text = map[string]any{}
		if len(request.OutputFormat) > 0 {
			format, err := models.ResponsesFormat(request.OutputFormat)
			if err != nil {
				return nil, err
			}
			text.(map[string]any)["format"] = format
		}
		if request.TextVerbosity != "" {
			text.(map[string]any)["verbosity"] = request.TextVerbosity
		}
	}
	payload := wire.CreateRequest{
		Model: request.Model, Input: input, PreviousResponseID: previous,
		Instructions: instructions, Tools: tools, ToolChoice: toolChoice,
		Metadata: request.Metadata, Text: text, Temperature: request.Temperature,
		TopP: request.TopP, ParallelToolCalls: request.ParallelToolCalls,
		Stream: true, MaxOutputTokens: request.MaxOutputTokens,
		Reasoning: reasoning, PromptCacheKey: request.PromptCacheKey,
		Store: request.Store, Include: include, Extra: request.Extra,
	}
	return json.Marshal(payload)
}

func toInput(messages []message.Message) ([]any, []string, error) {
	var items []any
	var include []string
	for _, msg := range messages {
		switch msg.Role {
		case message.RoleSystem, message.RoleDeveloper, message.RoleUser:
			content := make([]any, 0, len(msg.Parts))
			for _, part := range msg.Parts {
				switch part.Type {
				case message.PartText:
					content = append(content, wire.InputText{Type: "input_text", Text: part.Text})
				case message.PartImage:
					if part.Image == nil {
						continue
					}
					value := part.Image.URL
					if value == "" && len(part.Image.Data) > 0 {
						mime := part.Image.MIME
						if mime == "" {
							mime = "image/png"
						}
						value = message.DataURL(part.Image.Data, mime)
					}
					content = append(content, wire.ImageInput{
						Type: "input_image", ImageURL: value, Detail: part.Image.Detail,
					})
				case message.PartFile:
					if part.File == nil {
						continue
					}
					item := wire.FileInput{Type: "input_file", Filename: part.File.Filename, FileURL: part.File.URL}
					if len(part.File.Data) > 0 {
						item.FileData = message.DataURL(part.File.Data, part.File.MIME)
					}
					content = append(content, item)
				}
			}
			items = append(items, wire.InputMessage{
				Type: "message", ID: msg.ID, Role: string(msg.Role),
				Status: string(msg.Status), Content: content,
			})
		case message.RoleAssistant:
			content := make([]any, 0, len(msg.Parts))
			for _, part := range msg.Parts {
				switch part.Type {
				case message.PartText:
					content = append(content, wire.OutputText{Type: "output_text", Text: part.Text, Annotations: []json.RawMessage{}})
				case message.PartRefusal:
					content = append(content, wire.Refusal{Type: "refusal", Refusal: part.Refusal})
				case message.PartToolCall:
					if part.ToolCall != nil {
						items = append(items, wire.FunctionCall{
							Type: "function_call", ID: part.ToolCall.ID,
							CallID: part.ToolCall.ID, Name: part.ToolCall.Name,
							Arguments: part.ToolCall.Arguments, Status: "completed",
						})
					}
				case message.PartReasoning:
					if part.Reasoning != nil {
						summary := []wire.Summary{}
						if part.Reasoning.Text != "" {
							summary = append(summary, wire.Summary{Type: "summary_text", Text: part.Reasoning.Text})
						}
						items = append(items, wire.ReasoningItem{
							Type: "reasoning", Summary: summary,
							EncryptedContent: part.Reasoning.EncryptedContent,
						})
						if part.Reasoning.EncryptedContent != "" {
							include = appendUnique(include, "reasoning.encrypted_content")
						}
					}
				}
			}
			if len(content) > 0 {
				items = append(items, wire.InputMessage{
					Type: "message", ID: msg.ID, Role: "assistant",
					Status: string(msg.Status), Phase: msg.Phase, Content: content,
				})
			}
		case message.RoleTool:
			for _, part := range msg.Parts {
				if part.Type != message.PartToolResult || part.ToolResult == nil {
					continue
				}
				result := part.ToolResult
				output := toolOutput(result)
				items = append(items, wire.FunctionCallOutput{
					Type: "function_call_output", CallID: result.CallID,
					Output: output, Status: "completed",
				})
			}
		default:
			return nil, nil, fmt.Errorf("responses: unsupported message role %q", msg.Role)
		}
	}
	return items, include, nil
}

func toolOutput(result *message.ToolResult) any {
	content := make([]any, 0, len(result.Content)+1)
	if result.Output != "" {
		content = append(content, wire.InputText{Type: "input_text", Text: result.Output})
	}
	for _, nested := range result.Content {
		switch nested.Type {
		case message.PartText:
			content = append(content, wire.InputText{Type: "input_text", Text: nested.Text})
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
			content = append(content, wire.ImageInput{
				Type: "input_image", ImageURL: value, Detail: nested.Image.Detail,
			})
		case message.PartFile:
			if nested.File == nil {
				continue
			}
			item := wire.FileInput{
				Type: "input_file", Filename: nested.File.Filename, FileURL: nested.File.URL,
			}
			if len(nested.File.Data) > 0 {
				item.FileData = message.DataURL(nested.File.Data, nested.File.MIME)
			}
			content = append(content, item)
		}
	}
	if len(content) == 1 {
		if text, ok := content[0].(wire.InputText); ok {
			return text.Text
		}
	}
	return content
}

func appendUnique(values []string, value string) []string {
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	return append(values, value)
}

func (c *Client) post(ctx context.Context, body []byte) (*http.Response, error) {
	client := c.config.HTTPClient
	endpoint := c.config.BaseURL + "/responses"
	var lastErr error
	for attempt := 0; attempt <= c.config.MaxRetries; attempt++ {
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
		if err == nil && response.StatusCode >= 200 && response.StatusCode < 300 {
			return response, nil
		}
		if err == nil {
			lastErr = readHTTPError(response)
			if attempt < c.config.MaxRetries && retryable(response.StatusCode) {
				delay := retryDelay(response.Header.Get("Retry-After"), attempt)
				_ = response.Body.Close()
				if err := wait(ctx, delay); err != nil {
					return nil, err
				}
				continue
			}
			_ = response.Body.Close()
			return nil, lastErr
		}
		lastErr = err
		if attempt < c.config.MaxRetries {
			if err := wait(ctx, time.Duration(attempt+1)*200*time.Millisecond); err != nil {
				return nil, err
			}
			continue
		}
	}
	if lastErr == nil {
		lastErr = errors.New("responses: request failed")
	}
	return nil, lastErr
}

func readHTTPError(response *http.Response) error {
	payload, _ := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	var envelope wire.ErrorEnvelope
	if json.Unmarshal(payload, &envelope) == nil && envelope.Error.Message != "" {
		return fmt.Errorf("responses: HTTP %d: %s", response.StatusCode, envelope.Error.Message)
	}
	text := strings.TrimSpace(string(payload))
	if text == "" {
		text = response.Status
	}
	return fmt.Errorf("responses: HTTP %d: %s", response.StatusCode, text)
}

func retryable(status int) bool {
	return status == http.StatusRequestTimeout || status == http.StatusConflict ||
		status == http.StatusTooManyRequests || status >= http.StatusInternalServerError
}

func retryDelay(value string, attempt int) time.Duration {
	if seconds, err := strconv.Atoi(strings.TrimSpace(value)); err == nil && seconds >= 0 {
		return min(time.Duration(seconds)*time.Second, 30*time.Second)
	}
	return time.Duration(attempt+1) * 500 * time.Millisecond
}

func wait(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

type stream struct {
	decoder  *wire.SSEDecoder
	response *models.Response

	mu         sync.Mutex
	closed     bool
	terminal   bool
	pendingErr error
	bytes      int
	itemCalls  map[string]wire.FunctionCall
}

func (s *stream) Next(ctx context.Context) (models.StreamEvent, error) {
	if err := ctx.Err(); err != nil {
		return models.StreamEvent{}, err
	}
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return models.StreamEvent{}, io.EOF
	}
	if s.terminal {
		s.mu.Unlock()
		return models.StreamEvent{}, io.EOF
	}
	if s.pendingErr != nil {
		err := s.pendingErr
		s.pendingErr = nil
		s.terminal = true
		s.mu.Unlock()
		return models.StreamEvent{Type: models.StreamFailed, Err: err}, nil
	}
	s.mu.Unlock()

	stop := context.AfterFunc(ctx, func() { _ = s.Close() })
	defer stop()
	for s.decoder.Next() {
		if err := ctx.Err(); err != nil {
			return models.StreamEvent{}, err
		}
		raw := s.decoder.Event().Data
		s.bytes += len(raw)
		if s.bytes > 64<<20 {
			return s.complete(errs.New(errs.CodeLimit, "model stream exceeds the size limit"))
		}
		if bytes.Equal(raw, []byte("[DONE]")) {
			return s.complete(errs.New(errs.CodeInternal, "Responses stream ended before a terminal event"))
		}
		var event wire.StreamEvent
		if err := json.Unmarshal(raw, &event); err != nil {
			s.pendingErr = fmt.Errorf("responses: decode stream event: %w", err)
			return s.Next(ctx)
		}
		if event.Type == "" {
			event.Type = s.decoder.Event().Type
		}
		switch event.Type {
		case "response.created", "response.in_progress", "response.queued":
			return models.StreamEvent{
				Type: models.StreamCreated, ProviderType: event.Type, ProviderData: raw,
			}, nil
		case "response.output_item.added":
			var item struct {
				Type   string `json:"type"`
				ID     string `json:"id"`
				CallID string `json:"call_id"`
				Name   string `json:"name"`
			}
			if err := json.Unmarshal(event.Item, &item); err != nil {
				return s.complete(err)
			}
			if item.Type == "function_call" {
				if len(s.itemCalls) >= 128 && s.itemCalls[item.ID].ID == "" {
					return s.complete(errs.New(errs.CodeLimit, "too many function calls"))
				}
				s.itemCalls[item.ID] = wire.FunctionCall{
					Type: "function_call", ID: item.ID, CallID: item.CallID, Name: item.Name,
				}
				return models.StreamEvent{
					Type: models.StreamToolCallStart, Index: event.OutputIndex,
					ItemID: item.ID, CallID: item.CallID, ToolName: item.Name,
					ProviderType: event.Type, ProviderData: raw,
				}, nil
			}
			return models.StreamEvent{
				Type: models.StreamCreated, Index: event.OutputIndex, ItemID: item.ID,
				ProviderType: event.Type, ProviderData: raw,
			}, nil
		case "response.output_text.delta":
			return models.StreamEvent{
				Type: models.StreamTextDelta, Index: event.OutputIndex,
				ItemID: event.ItemID, Delta: event.Delta,
				ProviderType: event.Type, ProviderData: raw,
			}, nil
		case "response.refusal.delta":
			return models.StreamEvent{Type: models.StreamRefusalDelta, Index: event.OutputIndex, ItemID: event.ItemID, Delta: event.Delta}, nil
		case "response.output_text.done":
			return models.StreamEvent{
				Type: models.StreamTextDone, Index: event.OutputIndex,
				ItemID: event.ItemID, Delta: event.Text,
				ProviderType: event.Type, ProviderData: raw,
			}, nil
		case "response.reasoning.delta", "response.reasoning_summary_text.delta":
			return models.StreamEvent{
				Type: models.StreamReasoningDelta, Index: event.OutputIndex,
				ItemID: event.ItemID, Delta: event.Delta,
				ProviderType: event.Type, ProviderData: raw,
			}, nil
		case "response.reasoning.done", "response.reasoning_summary_text.done":
			return models.StreamEvent{
				Type: models.StreamReasoningDone, Index: event.OutputIndex,
				ItemID: event.ItemID, Delta: event.Text,
				ProviderType: event.Type, ProviderData: raw,
			}, nil
		case "response.function_call_arguments.delta":
			call := s.itemCalls[event.ItemID]
			return models.StreamEvent{
				Type: models.StreamToolCallDelta, Index: event.OutputIndex,
				ItemID: event.ItemID, CallID: call.CallID, ToolName: call.Name, Delta: event.Delta,
				ProviderType: event.Type, ProviderData: raw,
			}, nil
		case "response.function_call_arguments.done":
			call := s.itemCalls[event.ItemID]
			call.Arguments = event.Arguments
			s.itemCalls[event.ItemID] = call
			return models.StreamEvent{
				Type: models.StreamToolCallDone, Index: event.OutputIndex,
				ItemID: event.ItemID, CallID: call.CallID, ToolName: call.Name,
				Arguments: event.Arguments, ProviderType: event.Type, ProviderData: raw,
			}, nil
		case "response.output_item.done":
			var item struct {
				Type      string `json:"type"`
				ID        string `json:"id"`
				CallID    string `json:"call_id"`
				Name      string `json:"name"`
				Arguments string `json:"arguments"`
			}
			if err := json.Unmarshal(event.Item, &item); err != nil {
				return s.complete(err)
			}
			if item.Type == "function_call" {
				call := wire.FunctionCall{
					Type: "function_call", ID: item.ID, CallID: item.CallID,
					Name: item.Name, Arguments: item.Arguments,
				}
				s.itemCalls[item.ID] = call
				return models.StreamEvent{
					Type: models.StreamToolCallDone, Index: event.OutputIndex,
					ItemID: item.ID, CallID: call.CallID, ToolName: call.Name,
					Arguments: call.Arguments, ProviderType: event.Type, ProviderData: raw,
				}, nil
			}
			return models.StreamEvent{
				Type: models.StreamProviderEvent, Index: event.OutputIndex,
				ItemID: item.ID, ProviderType: event.Type, ProviderData: raw,
			}, nil
		case "response.completed", "response.incomplete", "response.failed":
			var resource wire.ResponseResource
			if len(event.Response) == 0 {
				return s.complete(errs.New(errs.CodeInternal, "terminal event has no response"))
			}
			if len(event.Response) > 0 {
				if err := json.Unmarshal(event.Response, &resource); err != nil {
					s.pendingErr = fmt.Errorf("responses: decode terminal response: %w", err)
					return s.Next(ctx)
				}
			}
			if event.Type == "response.incomplete" {
				resource.Status = "incomplete"
			}
			rendered, err := fromResponse(resource)
			if err != nil {
				s.pendingErr = err
				return s.Next(ctx)
			}
			s.response = rendered
			s.mu.Lock()
			s.terminal = true
			s.mu.Unlock()
			if event.Type == "response.failed" {
				message := "response failed"
				if resource.Error != nil && resource.Error.Message != "" {
					message = resource.Error.Message
				}
				return models.StreamEvent{
					Type: models.StreamFailed, Err: errors.New(message),
					Response: rendered, ProviderType: event.Type, ProviderData: raw,
				}, nil
			}
			return models.StreamEvent{
				Type: models.StreamCompleted, Response: rendered,
				ProviderType: event.Type, ProviderData: raw,
			}, nil
		case "error":
			text := "responses stream error"
			if event.Error != nil && event.Error.Message != "" {
				text = event.Error.Message
			}
			s.pendingErr = errors.New(text)
			return models.StreamEvent{
				Type: models.StreamProviderEvent, ProviderType: event.Type, ProviderData: raw,
			}, nil
		default:
			return models.StreamEvent{
				Type: models.StreamProviderEvent, ProviderType: event.Type, ProviderData: raw,
			}, nil
		}
	}
	if ctx.Err() != nil {
		return models.StreamEvent{}, ctx.Err()
	}
	err := s.decoder.Err()
	if errors.Is(err, io.EOF) {
		return s.complete(errors.New("responses: stream ended without a terminal event"))
	}
	if err == nil {
		err = errors.New("responses: stream closed")
	}
	return s.complete(err)
}

func (s *stream) complete(err error) (models.StreamEvent, error) {
	s.mu.Lock()
	s.terminal = true
	s.mu.Unlock()
	if err != nil {
		return models.StreamEvent{Type: models.StreamFailed, Err: err}, nil
	}
	return models.StreamEvent{Type: models.StreamCompleted, Response: s.response}, nil
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

func fromResponse(resource wire.ResponseResource) (*models.Response, error) {
	result := &models.Response{
		ID: resource.ID, Model: resource.Model,
		FinishReason: models.FinishStop,
	}
	if resource.Usage != nil {
		result.Usage = models.Usage{
			InputTokens:     resource.Usage.InputTokens,
			OutputTokens:    resource.Usage.OutputTokens,
			TotalTokens:     resource.Usage.TotalTokens,
			ReasoningTokens: resource.Usage.OutputTokensDetails.ReasoningTokens,
			CacheReadTokens: resource.Usage.InputTokensDetails.CachedTokens,
		}
	}
	assistant := message.Message{Role: message.RoleAssistant, Status: message.StatusCompleted}
	for _, raw := range resource.Output {
		var head struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal(raw, &head); err != nil {
			continue
		}
		switch head.Type {
		case "message":
			var item wire.InputMessage
			if err := json.Unmarshal(raw, &item); err != nil {
				return nil, err
			}
			assistant.ID = item.ID
			assistant.Phase = item.Phase
			parts, err := decodeContent(item.Content)
			if err != nil {
				return nil, err
			}
			assistant.Parts = append(assistant.Parts, parts...)
		case "function_call":
			var call wire.FunctionCall
			if err := json.Unmarshal(raw, &call); err != nil {
				return nil, err
			}
			callID := call.CallID
			if callID == "" {
				callID = call.ID
			}
			assistant.Parts = append(assistant.Parts, message.ToolCallPart(message.ToolCall{
				ID: callID, Name: call.Name, Arguments: call.Arguments,
			}))
			result.FinishReason = models.FinishToolCalls
		case "reasoning":
			var item wire.ReasoningItem
			if err := json.Unmarshal(raw, &item); err != nil {
				return nil, err
			}
			var text strings.Builder
			for _, summary := range item.Summary {
				text.WriteString(summary.Text)
			}
			assistant.Parts = append(assistant.Parts, message.ReasoningPart(message.Reasoning{
				Text: text.String(), EncryptedContent: item.EncryptedContent,
			}))
		}
	}
	if resource.Status == "incomplete" {
		assistant.Status = message.StatusIncomplete
		result.FinishReason = models.FinishLength
		if resource.IncompleteDetails != nil && resource.IncompleteDetails.Reason == "content_filter" {
			result.FinishReason = models.FinishContentFilter
		}
	}
	result.Message = assistant
	result.ProviderData, _ = json.Marshal(resource)
	return result, nil
}

func decodeContent(content any) ([]message.Part, error) {
	raw, err := json.Marshal(content)
	if err != nil {
		return nil, err
	}
	var parts []json.RawMessage
	if err := json.Unmarshal(raw, &parts); err != nil {
		return nil, err
	}
	out := make([]message.Part, 0, len(parts))
	for _, rawPart := range parts {
		var head struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal(rawPart, &head); err != nil {
			return nil, err
		}
		switch head.Type {
		case "output_text", "text", "input_text":
			var part wire.OutputText
			if err := json.Unmarshal(rawPart, &part); err != nil {
				return nil, err
			}
			out = append(out, message.Text(part.Text))
		case "refusal":
			var part wire.Refusal
			if err := json.Unmarshal(rawPart, &part); err != nil {
				return nil, err
			}
			out = append(out, message.Refusal(part.Refusal))
		}
	}
	return out, nil
}
