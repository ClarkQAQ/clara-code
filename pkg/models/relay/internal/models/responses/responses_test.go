package responses_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"clara-code/pkg/models/relay/internal/message"
	"clara-code/pkg/models/relay/internal/models"
	"clara-code/pkg/models/relay/internal/models/responses"
)

func TestResponsesStreamsTextAndToolCall(t *testing.T) {
	var requestBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/responses" {
			t.Errorf("path = %s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Errorf("authorization = %q", got)
		}
		if err := json.NewDecoder(r.Body).Decode(&requestBody); err != nil {
			t.Error(err)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		events := []string{
			`{"type":"response.created","sequence_number":0,"response":{"id":"resp_1","object":"response","created_at":1,"completed_at":null,"status":"in_progress","model":"test-model","previous_response_id":null,"instructions":null,"output":[],"error":null,"usage":null}}`,
			`{"type":"response.output_item.added","sequence_number":1,"output_index":0,"item":{"type":"message","id":"msg_1","status":"in_progress","role":"assistant","content":[]}}`,
			`{"type":"response.content_part.added","sequence_number":2,"item_id":"msg_1","output_index":0,"content_index":0,"part":{"type":"output_text","text":"","annotations":[]}}`,
			`{"type":"response.output_text.delta","sequence_number":3,"item_id":"msg_1","output_index":0,"content_index":0,"delta":"Hello"}`,
			`{"type":"response.output_text.done","sequence_number":4,"item_id":"msg_1","output_index":0,"content_index":0,"text":"Hello"}`,
			`{"type":"response.output_item.added","sequence_number":5,"output_index":1,"item":{"type":"function_call","id":"fc_1","call_id":"call_1","name":"lookup","arguments":"","status":"in_progress"}}`,
			`{"type":"response.function_call_arguments.delta","sequence_number":6,"item_id":"fc_1","output_index":1,"delta":"{\"q\":\"x\"}"}`,
			`{"type":"response.function_call_arguments.done","sequence_number":7,"item_id":"fc_1","output_index":1,"arguments":"{\"q\":\"x\"}"}`,
			`{"type":"response.completed","sequence_number":8,"response":{"id":"resp_1","object":"response","created_at":1,"completed_at":2,"status":"completed","model":"test-model","previous_response_id":null,"instructions":null,"output":[{"type":"message","id":"msg_1","status":"completed","role":"assistant","content":[{"type":"output_text","text":"Hello","annotations":[]}]},{"type":"function_call","id":"fc_1","call_id":"call_1","name":"lookup","arguments":"{\"q\":\"x\"}"}],"error":null,"usage":{"input_tokens":5,"output_tokens":3,"total_tokens":8,"input_tokens_details":{"cached_tokens":2},"output_tokens_details":{"reasoning_tokens":0}}}}`,
		}
		for _, event := range events {
			_, _ = io.WriteString(w, "event: "+eventType(event)+"\ndata: "+event+"\n\n")
		}
	}))
	defer server.Close()

	model, err := responses.New(responses.Config{
		BaseURL: server.URL + "/v1", APIKey: "test-key", Model: "test-model",
	})
	if err != nil {
		t.Fatal(err)
	}
	response, err := models.Generate(context.Background(), model, models.Request{
		Messages: []message.Message{message.UserText("hi")},
		Tools: []models.ToolDefinition{{
			Name: "lookup", Description: "Look up a value",
			Parameters: json.RawMessage(`{"type":"object","properties":{"q":{"type":"string"}}}`),
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := response.Message.Text(); got != "Hello" {
		t.Fatalf("text = %q", got)
	}
	calls := response.Message.ToolCalls()
	if len(calls) != 1 || calls[0].ID != "call_1" || calls[0].Name != "lookup" || calls[0].Arguments != `{"q":"x"}` {
		t.Fatalf("calls = %+v", calls)
	}
	if response.Usage.TotalTokens != 8 || response.Usage.CacheReadTokens != 2 {
		t.Fatalf("usage = %+v", response.Usage)
	}
	if requestBody["stream"] != true {
		t.Fatalf("stream = %#v", requestBody["stream"])
	}
}

func eventType(raw string) string {
	var event struct {
		Type string `json:"type"`
	}
	_ = json.Unmarshal([]byte(raw), &event)
	return event.Type
}

func TestResponsesRequiresBaseURL(t *testing.T) {
	_, err := responses.New(responses.Config{Model: "x"})
	if err == nil || !strings.Contains(err.Error(), "base URL") {
		t.Fatalf("err = %v", err)
	}
}
