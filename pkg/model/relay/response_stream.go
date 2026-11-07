package relay

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"clara-code/pkg/model/relay/internal/models"
	wire "clara-code/pkg/model/relay/internal/models/protocol/responses"
)

type responseWriter struct {
	emit     func(map[string]any) error
	sequence int
	items    []*responseSlot
	byKey    map[string]*responseSlot
	writeErr error
}

type responseSlot struct {
	key, id string
	index   int
	value   strings.Builder
}

func streamResponses(ctx context.Context, model models.Model, request models.Request, body wire.CreateRequest, emit func(map[string]any) error) (*models.Response, error) {
	id := "resp_" + newID()
	writer := &responseWriter{emit: emit, byKey: map[string]*responseSlot{}}
	created := responseResource(&models.Response{ID: id, Model: model.Info().ID}, body)
	created["status"], created["output"], created["completed_at"], created["usage"] = "in_progress", []any{}, nil, nil
	if err := writer.send("response.created", map[string]any{"response": created}); err != nil {
		return nil, err
	}
	if err := writer.send("response.in_progress", map[string]any{"response": created}); err != nil {
		return nil, err
	}
	response, err := models.Consume(ctx, model, request, writer.observe)
	if writer.writeErr != nil {
		return nil, writer.writeErr
	}
	if err != nil {
		return nil, writer.fail(created, err)
	}
	response.ID = id
	resource := responseResource(response, body)
	resource["created_at"] = created["created_at"]
	if err := writer.complete(resource); err != nil {
		if writer.writeErr != nil {
			return nil, writer.writeErr
		}
		return nil, writer.fail(created, err)
	}
	return response, nil
}

func (w *responseWriter) send(kind string, data map[string]any) error {
	data["type"], data["sequence_number"] = kind, w.sequence
	w.sequence++
	w.writeErr = w.emit(data)
	return w.writeErr
}

func (w *responseWriter) fail(created map[string]any, err error) error {
	created["status"] = "failed"
	created["completed_at"] = timeNowUnix()
	created["error"] = map[string]any{"code": "server_error", "message": err.Error()}
	return w.send("response.failed", map[string]any{"response": created})
}

func (w *responseWriter) observe(event models.StreamEvent) error {
	var key string
	switch event.Type {
	case models.StreamTextDelta:
		key = "text"
	case models.StreamRefusalDelta:
		key = "refusal"
	case models.StreamReasoningDelta:
		key = "reasoning"
	case models.StreamToolCallStart, models.StreamToolCallDelta:
		if event.CallID == "" || event.ToolName == "" {
			return nil
		}
		key = "call:" + event.CallID
	default:
		return nil
	}
	slot := w.byKey[key]
	if slot == nil {
		var item map[string]any
		switch key {
		case "text", "refusal":
			item = assistantItem("", map[string]any{})
		case "reasoning":
			item = map[string]any{"type": "reasoning", "id": "rs_" + newID(), "summary": []any{}}
		default:
			item = map[string]any{"type": "function_call", "id": "fc_" + newID(), "name": event.ToolName, "call_id": event.CallID, "arguments": "", "status": "in_progress"}
		}
		var err error
		slot, err = w.open(key, item)
		if err != nil {
			return err
		}
	}
	return w.delta(slot, event.Delta)
}

func (w *responseWriter) open(key string, item map[string]any) (*responseSlot, error) {
	slot := &responseSlot{key: key, id: item["id"].(string), index: len(w.items)}
	w.items = append(w.items, slot)
	w.byKey[key] = slot
	start := map[string]any{}
	for name, value := range item {
		start[name] = value
	}
	switch key {
	case "text", "refusal":
		start["content"], start["status"] = []any{}, "in_progress"
	case "reasoning":
		start["summary"] = []any{}
	default:
		start["arguments"], start["status"] = "", "in_progress"
	}
	if err := w.send("response.output_item.added", map[string]any{"output_index": slot.index, "item": start}); err != nil {
		return nil, err
	}
	switch key {
	case "text", "refusal":
		part := map[string]any{"type": "output_text", "text": "", "annotations": []any{}, "logprobs": []any{}}
		if key == "refusal" {
			part = map[string]any{"type": "refusal", "refusal": ""}
		}
		return slot, w.send("response.content_part.added", map[string]any{"output_index": slot.index, "item_id": slot.id, "content_index": 0, "part": part})
	case "reasoning":
		return slot, w.send("response.reasoning_summary_part.added", map[string]any{"output_index": slot.index, "item_id": slot.id, "summary_index": 0, "part": map[string]any{"type": "summary_text", "text": ""}})
	}
	return slot, nil
}

func (w *responseWriter) delta(slot *responseSlot, text string) error {
	if text == "" {
		return nil
	}
	slot.value.WriteString(text)
	data := map[string]any{"output_index": slot.index, "item_id": slot.id, "delta": text}
	switch slot.key {
	case "text":
		data["content_index"] = 0
		data["logprobs"] = []any{}
		return w.send("response.output_text.delta", data)
	case "refusal":
		data["content_index"] = 0
		return w.send("response.refusal.delta", data)
	case "reasoning":
		data["summary_index"] = 0
		return w.send("response.reasoning_summary_text.delta", data)
	default:
		return w.send("response.function_call_arguments.delta", data)
	}
}

func (w *responseWriter) complete(resource map[string]any) error {
	final := map[string]map[string]any{}
	for _, value := range resource["output"].([]any) {
		item := value.(map[string]any)
		key, _ := slotValue(item)
		if final[key] != nil {
			return errors.New("upstream returned duplicate output items")
		}
		final[key] = item
		if w.byKey[key] == nil {
			if _, err := w.open(key, item); err != nil {
				return err
			}
		}
	}
	output := []any{}
	for _, slot := range w.items {
		item := final[slot.key]
		if item == nil {
			return fmt.Errorf("upstream terminal response omitted %s", slot.key)
		}
		_, value := slotValue(item)
		if !strings.HasPrefix(value, slot.value.String()) {
			return errors.New("upstream terminal response differs from its deltas")
		}
		item["id"] = slot.id
		if err := w.delta(slot, strings.TrimPrefix(value, slot.value.String())); err != nil {
			return err
		}
		if err := w.done(slot, item, value); err != nil {
			return err
		}
		output = append(output, item)
	}
	resource["output"] = output
	return w.send("response."+resource["status"].(string), map[string]any{"response": resource})
}

func slotValue(item map[string]any) (string, string) {
	switch item["type"] {
	case "reasoning":
		parts := item["summary"].([]any)
		if len(parts) > 0 {
			return "reasoning", parts[0].(map[string]any)["text"].(string)
		}
		return "reasoning", ""
	case "function_call":
		return "call:" + item["call_id"].(string), item["arguments"].(string)
	default:
		part := item["content"].([]any)[0].(map[string]any)
		if part["type"] == "refusal" {
			return "refusal", part["refusal"].(string)
		}
		return "text", part["text"].(string)
	}
}

func (w *responseWriter) done(slot *responseSlot, item map[string]any, text string) error {
	data := func() map[string]any { return map[string]any{"output_index": slot.index, "item_id": slot.id} }
	switch slot.key {
	case "text", "refusal":
		kind, field := "response.output_text.done", "text"
		if slot.key == "refusal" {
			kind, field = "response.refusal.done", "refusal"
		}
		content := data()
		content["content_index"], content[field] = 0, text
		if slot.key == "text" {
			content["logprobs"] = []any{}
		}
		if err := w.send(kind, content); err != nil {
			return err
		}
		part := data()
		part["content_index"], part["part"] = 0, item["content"].([]any)[0]
		if err := w.send("response.content_part.done", part); err != nil {
			return err
		}
	case "reasoning":
		summary := data()
		summary["summary_index"], summary["text"] = 0, text
		if err := w.send("response.reasoning_summary_text.done", summary); err != nil {
			return err
		}
		part := data()
		part["summary_index"], part["part"] = 0, map[string]any{"type": "summary_text", "text": text}
		if err := w.send("response.reasoning_summary_part.done", part); err != nil {
			return err
		}
	default:
		args := data()
		args["arguments"] = text
		if err := w.send("response.function_call_arguments.done", args); err != nil {
			return err
		}
	}
	return w.send("response.output_item.done", map[string]any{"output_index": slot.index, "item": item})
}
