package provider

import (
	yuzu "clara-code/pkg/models"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

const reasoningStreamResponse = `{"id":"resp_1","object":"response","created_at":1,"status":"completed","model":"gpt-5","output":[],"usage":{"input_tokens":1,"output_tokens":1,"output_tokens_details":{},"total_tokens":2}}`

// serveResponsesReasoning answers Responses API requests with a reasoning
// item that reports its text as inline content parts instead of a summary,
// which is what Codex-derived gateways stream.
func serveResponsesReasoning(t *testing.T, inlineText string) (*httptest.Server, *[]byte) {
	t.Helper()
	var requestBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body := make([]byte, r.ContentLength)
		_, _ = r.Body.Read(body)
		requestBody = body

		var reqShape struct {
			Stream bool `json:"stream"`
		}
		_ = json.Unmarshal(body, &reqShape)

		if reqShape.Stream {
			w.Header().Set("Content-Type", "text/event-stream")
			rc := http.NewResponseController(w)
			events := []string{
				`{"type":"response.created","response":` + reasoningStreamResponse + `}`,
				`{"type":"response.output_item.added","output_index":0,"item":{"id":"rs_1","type":"reasoning","summary":[]}}`,
				`{"type":"response.reasoning_text.delta","item_id":"rs_1","output_index":0,"content_index":0,"delta":"thinking"}`,
				`{"type":"response.reasoning_text.delta","item_id":"rs_1","output_index":0,"content_index":0,"delta":" hard"}`,
				`{"type":"response.output_item.done","output_index":0,"item":{"id":"rs_1","type":"reasoning","summary":[],"content":[{"type":"reasoning_text","text":"thinking hard"}]}}`,
				`{"type":"response.output_item.added","output_index":1,"item":{"id":"msg_1","type":"message","role":"assistant","status":"in_progress","content":[]}}`,
				`{"type":"response.output_text.delta","item_id":"msg_1","output_index":1,"content_index":0,"delta":"Hi"}`,
				`{"type":"response.output_item.done","output_index":1,"item":{"id":"msg_1","type":"message","role":"assistant","status":"completed","content":[{"type":"output_text","text":"Hi","annotations":[]}]}}`,
				`{"type":"response.completed","response":` + reasoningStreamResponse + `}`,
			}
			for _, event := range events {
				_, _ = fmt.Fprintf(w, "event: %s\ndata: %s\n\n", eventType(event), event)
				_ = rc.Flush()
			}
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"id":"resp_1","object":"response","created_at":1,"status":"completed","model":"gpt-5","output":[{"id":"rs_1","type":"reasoning","summary":[],"content":[{"type":"reasoning_text","text":%q}]},{"id":"msg_1","type":"message","role":"assistant","status":"completed","content":[{"type":"output_text","text":"Hi","annotations":[]}]}],"usage":{"input_tokens":1,"output_tokens":1,"output_tokens_details":{},"total_tokens":2}}`, inlineText)
	}))
	t.Cleanup(srv.Close)
	return srv, &requestBody
}

func eventType(event string) string {
	var shape struct {
		Type string `json:"type"`
	}
	_ = json.Unmarshal([]byte(event), &shape)
	return shape.Type
}

func reasoningProvider(t *testing.T, srv *httptest.Server) yuzu.LanguageModel {
	t.Helper()
	p, err := New(
		WithAPI(APIResponses),
		WithBaseURL(srv.URL),
		WithAPIKey("test"),
	)
	require.NoError(t, err)
	model, err := p.LanguageModel(context.Background(), "gpt-5")
	require.NoError(t, err)
	return model
}

func TestResponsesStreamInlineReasoningText(t *testing.T) {
	srv, _ := serveResponsesReasoning(t, "thinking hard")
	model := reasoningProvider(t, srv)

	stream, err := model.Stream(context.Background(), yuzu.Call{Prompt: prompt()})
	require.NoError(t, err)

	var reasoning string
	var text string
	for part := range stream {
		switch part.Type {
		case yuzu.StreamPartTypeReasoningDelta:
			reasoning += part.Delta
		case yuzu.StreamPartTypeTextDelta:
			text += part.Delta
		case yuzu.StreamPartTypeError:
			t.Fatalf("stream error: %v", part.Error)
		}
	}

	require.Equal(t, "thinking hard", reasoning)
	require.Equal(t, "Hi", text)
}

func TestResponsesGenerateInlineReasoningText(t *testing.T) {
	srv, _ := serveResponsesReasoning(t, "thinking hard")
	model := reasoningProvider(t, srv)

	response, err := model.Generate(context.Background(), yuzu.Call{Prompt: prompt()})
	require.NoError(t, err)

	var reasoning string
	for _, content := range response.Content {
		if rc, ok := yuzu.AsContentType[yuzu.ReasoningContent](content); ok {
			reasoning += rc.Text
		}
	}

	require.Equal(t, "thinking hard", reasoning)
}
