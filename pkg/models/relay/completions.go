package relay

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"clara-code/pkg/models/relay/internal/message"
	"clara-code/pkg/models/relay/internal/models"
	compwire "clara-code/pkg/models/relay/internal/models/protocol/completions"
)

func (h *Handler) handleCompletions(w http.ResponseWriter, r *http.Request) {
	var body compwire.Request
	if err := decodeBody(r, &body); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	if body.Model == "" {
		writeJSONError(w, http.StatusBadRequest, "model_required", "model is required")
		return
	}
	request, err := requestFromCompletions(body)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "conversion_error", err.Error())
		return
	}
	model, _, err := h.model(body.Model)
	if err != nil {
		writeJSONError(w, http.StatusBadGateway, "upstream_error", err.Error())
		return
	}
	request.Model = model.Info().ID
	id := "chatcmpl_" + newID()
	if body.Stream {
		includeUsage := body.StreamOptions != nil && body.StreamOptions.IncludeUsage
		streamChat(r.Context(), w, model, request, id, includeUsage)
		return
	}
	response, err := models.Generate(r.Context(), model, request)
	if err != nil {
		writeJSONError(w, http.StatusBadGateway, "upstream_error", err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(chatCompletion(id, response)); err != nil {
		return
	}
}

func streamChat(ctx context.Context, w http.ResponseWriter, model models.Model, request models.Request, id string, includeUsage bool) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	flusher, _ := w.(http.Flusher)
	created := timeNowUnix()
	var writeErr error
	write := func(value any) error {
		data, err := json.Marshal(value)
		if err != nil {
			return err
		}
		_, writeErr = fmt.Fprintf(w, "data: %s\n\n", data)
		if writeErr == nil && flusher != nil {
			flusher.Flush()
		}
		return writeErr
	}
	chunk := func(choices []any, usage any) map[string]any {
		value := map[string]any{"id": id, "object": "chat.completion.chunk", "created": created, "model": model.Info().ID, "choices": choices}
		if includeUsage {
			value["usage"] = usage
		}
		return value
	}
	delta := func(value map[string]any, finish any) error {
		return write(chunk([]any{map[string]any{"index": 0, "delta": value, "finish_reason": finish}}, nil))
	}
	if err := delta(map[string]any{"role": "assistant"}, nil); err != nil {
		return
	}
	type callState struct {
		index int
		args  string
	}
	calls := map[string]*callState{}
	content, reasoning, refusal := "", "", ""
	emitCall := func(callID, name, args string) error {
		state := calls[callID]
		function := map[string]any{}
		fragment := map[string]any{"function": function}
		if state == nil {
			state = &callState{index: len(calls)}
			calls[callID] = state
			fragment["id"], fragment["type"], function["name"] = callID, "function", name
		}
		state.args += args
		function["arguments"] = args
		fragment["index"] = state.index
		return delta(map[string]any{"tool_calls": []any{fragment}}, nil)
	}
	response, err := models.Consume(ctx, model, request, func(event models.StreamEvent) error {
		switch event.Type {
		case models.StreamTextDelta:
			content += event.Delta
			return delta(map[string]any{"content": event.Delta}, nil)
		case models.StreamReasoningDelta:
			reasoning += event.Delta
			return delta(map[string]any{"reasoning_content": event.Delta}, nil)
		case models.StreamRefusalDelta:
			refusal += event.Delta
			return delta(map[string]any{"refusal": event.Delta}, nil)
		case models.StreamToolCallStart, models.StreamToolCallDelta:
			if event.CallID != "" && event.ToolName != "" {
				return emitCall(event.CallID, event.ToolName, event.Delta)
			}
		}
		return nil
	})
	if writeErr != nil {
		return
	}
	if err != nil {
		_ = write(map[string]any{"error": map[string]any{"message": err.Error(), "type": "upstream_error", "code": "upstream_error"}})
		return
	}
	text, denial := answerParts(response.Message)
	for _, part := range []struct{ field, value, observed string }{
		{"reasoning_content", response.Message.ReasoningText(), reasoning}, {"content", text, content}, {"refusal", denial, refusal},
	} {
		if len(part.value) > len(part.observed) && strings.HasPrefix(part.value, part.observed) {
			if err := delta(map[string]any{part.field: part.value[len(part.observed):]}, nil); err != nil {
				return
			}
		}
	}
	for _, call := range response.Message.ToolCalls() {
		args := call.Arguments
		if state := calls[call.ID]; state != nil {
			args = strings.TrimPrefix(args, state.args)
			if args == "" {
				continue
			}
		}
		if err := emitCall(call.ID, call.Name, args); err != nil {
			return
		}
	}
	if err := delta(map[string]any{}, chatFinish(response)); err != nil {
		return
	}
	if includeUsage {
		if err := write(chunk([]any{}, completionUsage(response.Usage))); err != nil {
			return
		}
	}
	if _, err := fmt.Fprint(w, "data: [DONE]\n\n"); err == nil && flusher != nil {
		flusher.Flush()
	}
}

func answerParts(msg message.Message) (string, string) {
	var text, refusal strings.Builder
	for _, part := range msg.Parts {
		switch part.Type {
		case message.PartText:
			text.WriteString(part.Text)
		case message.PartRefusal:
			refusal.WriteString(part.Refusal)
		}
	}
	return text.String(), refusal.String()
}

func chatFinish(response *models.Response) string {
	switch response.FinishReason {
	case models.FinishLength, models.FinishContentFilter, models.FinishToolCalls:
		return string(response.FinishReason)
	default:
		if len(response.Message.ToolCalls()) > 0 {
			return string(models.FinishToolCalls)
		}
		return string(models.FinishStop)
	}
}

func chatCompletion(id string, response *models.Response) map[string]any {
	text, refusal := answerParts(response.Message)
	message := map[string]any{"role": "assistant", "content": text}
	if refusal != "" {
		message["refusal"] = refusal
	}
	if reasoning := response.Message.ReasoningText(); reasoning != "" {
		message["reasoning_content"] = reasoning
	}
	var calls []any
	for _, call := range response.Message.ToolCalls() {
		calls = append(calls, map[string]any{
			"id": call.ID, "type": "function",
			"function": map[string]any{"name": call.Name, "arguments": call.Arguments},
		})
	}
	if len(calls) > 0 {
		message["tool_calls"] = calls
		if text == "" {
			message["content"] = nil
		}
	}
	return map[string]any{
		"id": id, "object": "chat.completion", "created": timeNowUnix(),
		"model":   response.Model,
		"choices": []any{map[string]any{"index": 0, "message": message, "finish_reason": chatFinish(response)}},
		"usage":   completionUsage(response.Usage),
	}
}

func completionUsage(usage models.Usage) map[string]any {
	return map[string]any{
		"prompt_tokens": usage.InputTokens, "completion_tokens": usage.OutputTokens,
		"total_tokens":              usage.TotalTokens,
		"prompt_tokens_details":     map[string]any{"cached_tokens": usage.CacheReadTokens},
		"completion_tokens_details": map[string]any{"reasoning_tokens": usage.ReasoningTokens},
	}
}
