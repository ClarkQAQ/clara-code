package demo

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
)

// ResponsesUpstream returns a deterministic OpenResponses SSE upstream.
func ResponsesUpstream(text string) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			Tools []struct {
				Name string `json:"name"`
			} `json:"tools"`
		}
		_ = json.NewDecoder(r.Body).Decode(&request)
		w.Header().Set("Content-Type", "text/event-stream")
		output := fmt.Sprintf(`{"type":"message","id":"msg_demo","status":"completed","role":"assistant","content":[{"type":"output_text","text":%q,"annotations":[]}]}`, text)
		if len(request.Tools) > 0 {
			output = fmt.Sprintf(`{"type":"function_call","id":"fc_demo","call_id":"call_demo","name":%q,"arguments":"{\"location\":\"San Francisco, CA\"}","status":"completed"}`, request.Tools[0].Name)
		}
		resource := fmt.Sprintf(`{"id":"resp_demo","object":"response","created_at":1,"completed_at":2,"status":"completed","model":"demo","previous_response_id":null,"instructions":null,"output":[%s],"error":null,"usage":{"input_tokens":2,"output_tokens":2,"total_tokens":4,"input_tokens_details":{"cached_tokens":0},"output_tokens_details":{"reasoning_tokens":0}}}`, output)
		events := []string{`{"type":"response.created","sequence_number":0,"response":` + resource + `}`}
		if len(request.Tools) == 0 {
			events = append(events, `{"type":"response.output_item.added","sequence_number":1,"output_index":0,"item":{"type":"message","id":"msg_demo","status":"in_progress","role":"assistant","content":[]}}`,
				`{"type":"response.output_text.delta","sequence_number":2,"item_id":"msg_demo","output_index":0,"content_index":0,"delta":`+fmt.Sprintf("%q", text)+`}`)
		}
		events = append(events, `{"type":"response.completed","sequence_number":3,"response":`+resource+`}`)
		for _, item := range events {
			var event struct {
				Type string `json:"type"`
			}
			_ = json.Unmarshal([]byte(item), &event)
			_, _ = io.WriteString(w, "event: "+event.Type+"\ndata: "+item+"\n\n")
		}
	}))
}
