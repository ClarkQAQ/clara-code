package provider

import (
	modelapi "clara-code/pkg/model"
	"context"
	"encoding/json"
	"errors"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestObjectStreamsValidatePreviewAndFinalJSON(t *testing.T) {
	cases := []struct {
		name                         string
		deltas                       []string
		repair, truncate, wantFinish bool
		wantPreviews, wantRepairs    int
	}{
		{name: "split JSON", deltas: []string{`{"answer":"hel`, `lo"}`}, repair: true, wantFinish: true, wantPreviews: 1},
		{name: "trailing bytes invalidate preview", deltas: []string{`{"answer":"hello"}`, " extra"}, wantPreviews: 1},
		{name: "schema mismatch", deltas: []string{`{"answer":true}`}},
		{name: "explicit repair once at finish", deltas: []string{`{"answer":`, `"hello"`}, repair: true, wantFinish: true, wantPreviews: 1, wantRepairs: 1},
		{name: "transport interruption cannot be repaired", deltas: []string{`{"answer":"hello"`}, repair: true, truncate: true},
	}
	for _, api := range []API{APIChatCompletions, APIResponses} {
		for _, tt := range cases {
			t.Run(string(api)+"/"+tt.name, func(t *testing.T) {
				server := newStreamingMockServer()
				t.Cleanup(server.close)
				if api == APIResponses {
					for _, delta := range tt.deltas {
						data, err := json.Marshal(map[string]any{
							"type": "response.output_text.delta", "delta": delta,
							"item_id": "msg_test", "output_index": 0, "content_index": 0,
						})
						require.NoError(t, err)
						server.chunks = append(server.chunks, responsesSSEEvent("response.output_text.delta", string(data)))
					}
					if !tt.truncate {
						server.chunks = append(server.chunks, responsesSSEEvent("response.completed",
							`{"type":"response.completed","response":{"id":"resp_test","status":"completed","output":[]}}`))
					}
				} else {
					server.prepareStreamResponse(map[string]any{"content": tt.deltas})
					if tt.truncate {
						server.chunks = chatCompletionChunksBeforeFinishReason(server.chunks)
					}
				}

				client, err := New(WithBaseURL(server.server.URL), WithAPI(api))
				require.NoError(t, err)
				model, err := client.LanguageModel(t.Context(), "test-model")
				require.NoError(t, err)
				call := modelapi.ObjectCall{Prompt: testPrompt, Schema: modelapi.Schema{
					Type: "object", Required: []string{"answer"},
					Properties: map[string]*modelapi.Schema{"answer": {Type: "string"}},
				}}
				repairs := 0
				if tt.repair {
					call.RepairText = func(_ context.Context, raw string, err error) (string, error) {
						repairs++
						if raw != `{"answer":"hello"` || err == nil {
							return "", errors.New("unexpected repair request")
						}
						return `{"answer":"hello"}`, nil
					}
				}
				stream, err := model.StreamObject(t.Context(), call)
				require.NoError(t, err)
				objects, finishes, failures := 0, 0, 0
				for part := range stream {
					switch part.Type {
					case modelapi.ObjectStreamPartTypeObject:
						if part.Object != nil {
							objects++
							require.Equal(t, map[string]any{"answer": "hello"}, part.Object)
						}
					case modelapi.ObjectStreamPartTypeFinish:
						finishes++
					case modelapi.ObjectStreamPartTypeError:
						failures++
						require.Error(t, part.Error)
					}
				}
				require.Equal(t, tt.wantPreviews, objects)
				require.Equal(t, tt.wantRepairs, repairs)
				if tt.wantFinish {
					require.Equal(t, 1, finishes)
					require.Zero(t, failures)
				} else {
					require.Zero(t, finishes)
					require.Equal(t, 1, failures)
				}
			})
		}
	}
}
