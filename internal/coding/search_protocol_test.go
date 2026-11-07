package coding

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"

	"clara-core/agent/prompt"
)

func TestSearchContractAndCorrectionThroughProtocols(t *testing.T) {
	for _, protocol := range []string{"completions", "responses"} {
		t.Run(protocol, func(t *testing.T) {
			var requests atomic.Int32
			c, _ := remoteFixture(t, func(w http.ResponseWriter, r *http.Request) {
				body, err := io.ReadAll(r.Body)
				if err != nil {
					t.Error(err)
					w.WriteHeader(500)
					return
				}
				step := requests.Add(1)
				var payload struct {
					Tools []json.RawMessage `json:"tools"`
				}
				if err := json.Unmarshal(body, &payload); err != nil {
					t.Error(err)
				}
				found := false
				for _, raw := range payload.Tools {
					var tool struct {
						Name       string
						Parameters json.RawMessage
						Function   struct {
							Name       string
							Parameters json.RawMessage
						}
					}
					if err := json.Unmarshal(raw, &tool); err != nil {
						t.Error(err)
					}
					name, parameters := tool.Name, tool.Parameters
					if name == "" {
						name, parameters = tool.Function.Name, tool.Function.Parameters
					}
					if name == "grep" {
						found = true
						for _, field := range []string{"query", "include", "include_ignored"} {
							if !strings.Contains(string(parameters), field) {
								t.Errorf("wire schema lacks %s: %s", field, parameters)
							}
						}
					}
				}
				if !found || !strings.Contains(string(body), "File search") || !strings.Contains(string(body), "yield_seconds") || !strings.Contains(string(body), "global instructions") {
					t.Errorf("wire contract or prompt missing: found grep=%v", found)
				}
				if step == 2 && (!strings.Contains(string(body), "grep: invalid arguments") || !strings.Contains(string(body), "Allowed fields:")) {
					t.Errorf("failed call did not return actionable error")
				}
				if step == 3 && !strings.Contains(string(body), "/internal/layout.go:1:") {
					t.Errorf("corrected search did not return real file location")
				}
				if step > 3 {
					t.Errorf("unexpected repeated request %d", step)
				}
				if step >= 3 {
					if protocol == "completions" {
						sendTextSSE(w, "Search corrected and verified.", true)
					} else {
						sendResponseText(w)
					}
					return
				}
				args := `{"pattern":"cursorPosition"}`
				if step == 2 {
					args = `{"path":"internal","query":"cursorPosition","include":["*.go"]}`
				}
				sendSearchCall(w, protocol, args, step)
			})
			writeTestFile(t, c.Workspace.Cwd, "internal/layout.go", "type cursorPosition struct{}\n")
			if protocol == "responses" {
				c.Host.Config.Protocol = protocol
				factory, err := c.Host.Config.Factory()
				if err != nil {
					t.Fatal(err)
				}
				c.Host.Models = factory
				if err := c.ChangeModel(t.Context(), "fixture", ""); err != nil {
					t.Fatal(err)
				}
			}
			result := finish(t, c, prompt.Execute, "Search for cursorPosition in Go files.")
			if requests.Load() != 3 || !strings.Contains(result.Response.Content.Text(), "Search corrected") {
				t.Fatalf("requests %d result %+v", requests.Load(), result)
			}
		})
	}
}

func responseEvent(w http.ResponseWriter, name string, value map[string]any) {
	value["type"] = name
	body, _ := json.Marshal(value)
	fmt.Fprintf(w, "event: %s\ndata: %s\n\n", name, body)
}

func sendSearchCall(w http.ResponseWriter, protocol, args string, step int32) {
	w.Header().Set("Content-Type", "text/event-stream")
	id := fmt.Sprintf("search_%d", step)
	if protocol == "completions" {
		body, _ := json.Marshal(map[string]any{"id": id, "choices": []any{map[string]any{"index": 0, "delta": map[string]any{"tool_calls": []any{map[string]any{"index": 0, "id": id, "type": "function", "function": map[string]any{"name": "grep", "arguments": args}}}}, "finish_reason": "tool_calls"}}})
		fmt.Fprintf(w, "data: %s\n\ndata: [DONE]\n\n", body)
		return
	}
	item := map[string]any{"id": "fc_" + id, "call_id": id, "type": "function_call", "name": "grep", "arguments": "", "status": "in_progress"}
	responseEvent(w, "response.created", map[string]any{"response": map[string]any{"id": id, "status": "in_progress", "output": []any{}}})
	responseEvent(w, "response.output_item.added", map[string]any{"output_index": 0, "item": item})
	responseEvent(w, "response.function_call_arguments.delta", map[string]any{"output_index": 0, "delta": args})
	item["arguments"], item["status"] = args, "completed"
	responseEvent(w, "response.output_item.done", map[string]any{"output_index": 0, "item": item})
	responseEvent(w, "response.completed", map[string]any{"response": map[string]any{"id": id, "status": "completed", "output": []any{item}, "usage": map[string]int{"input_tokens": 10, "output_tokens": 10, "total_tokens": 20}}})
}

func sendResponseText(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/event-stream")
	item := map[string]any{"id": "message", "type": "message", "role": "assistant", "status": "in_progress", "content": []any{}}
	responseEvent(w, "response.created", map[string]any{"response": map[string]any{"id": "final", "status": "in_progress", "output": []any{}}})
	responseEvent(w, "response.output_item.added", map[string]any{"output_index": 0, "item": item})
	responseEvent(w, "response.output_text.delta", map[string]any{"item_id": "message", "output_index": 0, "content_index": 0, "delta": "Search corrected and verified."})
	item["status"] = "completed"
	item["content"] = []any{map[string]any{"type": "output_text", "text": "Search corrected and verified.", "annotations": []any{}}}
	responseEvent(w, "response.output_item.done", map[string]any{"output_index": 0, "item": item})
	responseEvent(w, "response.completed", map[string]any{"response": map[string]any{"id": "final", "status": "completed", "output": []any{item}, "usage": map[string]int{"input_tokens": 10, "output_tokens": 10, "total_tokens": 20}}})
}
