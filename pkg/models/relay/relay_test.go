package relay_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	relayconnector "clara-code/pkg/models/relay"
	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
)

func TestRelayResponsesAndCompletions(t *testing.T) {
	upstream := fakeResponsesUpstream(t, "hello from upstream")
	handler, err := relayconnector.New(relayconnector.Config{Upstreams: map[string]relayconnector.Upstream{
		"default": {BaseURL: upstream.URL + "/v1", Protocol: relayconnector.ProtocolResponses, Model: "upstream"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(handler)
	defer server.Close()

	response := postJSON(t, server.URL+"/v1/responses", map[string]any{
		"model": "test", "input": "hi",
	})
	var resource map[string]any
	if err := json.Unmarshal(response, &resource); err != nil {
		t.Fatal(err)
	}
	if resource["object"] != "response" {
		t.Fatalf("resource = %s", response)
	}
	if !strings.Contains(string(response), "hello from upstream") {
		t.Fatalf("response = %s", response)
	}

	chat := postJSON(t, server.URL+"/v1/chat/completions", map[string]any{
		"model": "test", "messages": []any{map[string]any{"role": "user", "content": "hi"}},
	})
	if !strings.Contains(string(chat), "hello from upstream") {
		t.Fatalf("chat = %s", chat)
	}

	streamRequest := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/v1/responses", bytes.NewBufferString(
		`{"model":"test","input":"hi","stream":true}`,
	))
	streamRequest.Header.Set("Content-Type", "application/json")
	streamRecorder := httptest.NewRecorder()
	handler.ServeHTTP(streamRecorder, streamRequest)
	if !strings.Contains(streamRecorder.Body.String(), "event: response.completed") {
		t.Fatalf("stream = %s", streamRecorder.Body.String())
	}
}

func TestRelayWebSocket(t *testing.T) {
	upstream := fakeResponsesUpstream(t, "socket text")
	handler, err := relayconnector.New(relayconnector.Config{Upstreams: map[string]relayconnector.Upstream{
		"default": {BaseURL: upstream.URL + "/v1", Protocol: relayconnector.ProtocolResponses},
	}})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(handler)
	defer server.Close()
	ctx := context.Background()
	connection, _, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(server.URL, "http")+"/v1/responses", nil) //nolint:bodyclose // websocket connection is closed below.
	if err != nil {
		t.Fatal(err)
	}
	defer connection.CloseNow()
	if err := wsjson.Write(ctx, connection, map[string]any{
		"type": "response.create", "model": "test", "input": "hi", "store": false,
	}); err != nil {
		t.Fatal(err)
	}
	terminal := false
	for range 20 {
		var event map[string]any
		if err := wsjson.Read(ctx, connection, &event); err != nil {
			t.Fatal(err)
		}
		if event["type"] == "response.completed" {
			terminal = true
			break
		}
	}
	if !terminal {
		t.Fatal("no terminal WebSocket event")
	}
}

func fakeResponsesUpstream(t *testing.T, text string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "text/event-stream")
		resource := `{"id":"resp_up","object":"response","created_at":1,"completed_at":2,"status":"completed","model":"upstream","previous_response_id":null,"instructions":null,"output":[{"type":"message","id":"msg_up","status":"completed","role":"assistant","content":[{"type":"output_text","text":"` + text + `","annotations":[]}]}],"error":null,"usage":{"input_tokens":2,"output_tokens":2,"total_tokens":4,"input_tokens_details":{"cached_tokens":0},"output_tokens_details":{"reasoning_tokens":0}}}`
		events := []string{
			`{"type":"response.created","sequence_number":0,"response":` + resource + `}`,
			`{"type":"response.output_item.added","sequence_number":1,"output_index":0,"item":{"type":"message","id":"msg_up","status":"in_progress","role":"assistant","content":[]}}`,
			`{"type":"response.output_text.delta","sequence_number":2,"item_id":"msg_up","output_index":0,"content_index":0,"delta":"` + text + `"}`,
			`{"type":"response.completed","sequence_number":3,"response":` + resource + `}`,
		}
		for _, event := range events {
			_, _ = io.WriteString(w, "event: event\ndata: "+event+"\n\n")
		}
	}))
}

func postJSON(t *testing.T, target string, body any) []byte {
	t.Helper()
	raw, _ := json.Marshal(body)
	request, err := http.NewRequestWithContext(context.Background(), http.MethodPost, target, bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	data, _ := io.ReadAll(response.Body)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("status = %d: %s", response.StatusCode, data)
	}
	return data
}

func TestResponsesRelayPreservesChatToolCallsAndContinuation(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Model    string `json:"model"`
			Messages []struct {
				Role       string            `json:"role"`
				ToolCalls  []json.RawMessage `json:"tool_calls"`
				ToolCallID string            `json:"tool_call_id"`
			} `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
			w.WriteHeader(400)
			return
		}
		if body.Model != "chat-upstream" {
			t.Errorf("unexpected model %q", body.Model)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		send := func(data string) {
			if _, err := io.WriteString(w, "data: "+data+"\n\n"); err != nil {
				t.Error(err)
			}
		}
		if len(body.Messages) == 1 {
			send(`{"choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"call_a","type":"function","function":{"name":"lookup","arguments":"{}"}},{"index":1,"id":"call_b","type":"function","function":{"name":"lookup","arguments":"{}"}}]},"finish_reason":null}]}`)
			send(`{"choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}]}`)
		} else {
			if len(body.Messages) != 4 || body.Messages[1].Role != "assistant" || len(body.Messages[1].ToolCalls) != 2 ||
				body.Messages[2].ToolCallID != "call_a" || body.Messages[3].ToolCallID != "call_b" {
				t.Errorf("continuation messages: %+v", body.Messages)
			}
			send(`{"choices":[{"index":0,"delta":{"content":"both tool results received"},"finish_reason":"stop"}]}`)
		}
		send(`{"choices":[],"usage":{"prompt_tokens":3,"completion_tokens":2,"total_tokens":5}}`)
		send("[DONE]")
	}))
	defer upstream.Close()
	handler, err := relayconnector.New(relayconnector.Config{Upstreams: map[string]relayconnector.Upstream{
		"public": {BaseURL: upstream.URL, Model: "chat-upstream", Protocol: relayconnector.ProtocolCompletions},
	}})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(handler)
	defer server.Close()
	raw := postJSON(t, server.URL+"/v1/responses", map[string]any{
		"model": "public", "input": "lookup twice", "tools": []any{map[string]any{"type": "function", "name": "lookup"}},
	})
	var first struct {
		ID     string
		Output []json.RawMessage
		Usage  struct {
			Total int `json:"total_tokens"`
		}
	}
	if err := json.Unmarshal(raw, &first); err != nil {
		t.Fatal(err)
	}
	if first.ID == "" || len(first.Output) != 2 || first.Usage.Total != 5 {
		t.Fatalf("response: %s", raw)
	}
	raw = postJSON(t, server.URL+"/v1/responses", map[string]any{
		"model": "public", "previous_response_id": first.ID,
		"input": []any{
			map[string]string{"type": "function_call_output", "call_id": "call_a", "output": "a"},
			map[string]string{"type": "function_call_output", "call_id": "call_b", "output": "b"},
		},
	})
	if !strings.Contains(string(raw), "both tool results received") {
		t.Fatalf("continuation: %s", raw)
	}
}
