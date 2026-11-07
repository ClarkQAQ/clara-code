package completions_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"clara-code/pkg/models/relay/internal/message"
	"clara-code/pkg/models/relay/internal/models"
	"clara-code/pkg/models/relay/internal/models/completions"
)

func TestCompletionsStreamsTextAndToolCall(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Errorf("path = %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		chunks := []string{
			`{"id":"cmpl_1","model":"test","choices":[{"index":0,"delta":{"role":"assistant","reasoning_content":"think"}}]}`,
			`{"id":"cmpl_1","model":"test","choices":[{"index":0,"delta":{"content":"Hel"}}]}`,
			`{"id":"cmpl_1","model":"test","choices":[{"index":0,"delta":{"content":"lo"}}]}`,
			`{"id":"cmpl_1","model":"test","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"call_1","type":"function","function":{"name":"lookup","arguments":"{\"q\":"}}]}}]}`,
			`{"id":"cmpl_1","model":"test","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"\"x\"}"}}]}}]}`,
			`{"id":"cmpl_1","model":"test","choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}],"usage":{"prompt_tokens":5,"completion_tokens":3,"total_tokens":8,"prompt_tokens_details":{"cached_tokens":2},"completion_tokens_details":{"reasoning_tokens":1}}}`,
		}
		for _, chunk := range chunks {
			_, _ = io.WriteString(w, "data: "+chunk+"\n\n")
		}
		_, _ = io.WriteString(w, "data: [DONE]\n\n")
	}))
	defer server.Close()

	model, err := completions.New(completions.Config{
		BaseURL: server.URL + "/v1", Model: "test",
	})
	if err != nil {
		t.Fatal(err)
	}
	response, err := models.Generate(context.Background(), model, models.Request{
		Messages: []message.Message{message.UserText("hi")},
	})
	if err != nil {
		t.Fatal(err)
	}
	if response.Message.Text() != "Hello" || response.Message.ReasoningText() != "think" {
		t.Fatalf("message = %+v", response.Message)
	}
	calls := response.Message.ToolCalls()
	if len(calls) != 1 || calls[0].Arguments != `{"q":"x"}` {
		t.Fatalf("calls = %+v", calls)
	}
	if response.Usage.TotalTokens != 8 || response.Usage.CacheReadTokens != 2 || response.Usage.ReasoningTokens != 1 {
		t.Fatalf("usage = %+v", response.Usage)
	}
}
