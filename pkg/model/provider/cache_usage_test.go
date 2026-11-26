package provider

import (
	"clara-code/pkg/model"
	"encoding/json"
	"fmt"
	"testing"

	openai "clara-code/pkg/model/internal/wire"
)

func TestCacheReportingDistinguishesMissingAndZero(t *testing.T) {
	for _, tc := range []struct {
		name, details string
		reported      bool
		cached        int64
	}{
		{"missing", `{}`, false, 0},
		{"null", `{"cached_tokens":null}`, false, 0},
		{"miss", `{"cached_tokens":0}`, true, 0},
		{"hit", `{"cached_tokens":40}`, true, 40},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var response openai.Response
			if err := json.Unmarshal([]byte(fmt.Sprintf(`{"usage":{"input_tokens":100,"output_tokens":10,"input_tokens_details":%s}}`, tc.details)), &response); err != nil {
				t.Fatal(err)
			}
			usage := responsesUsage(response)
			if usage.CacheReadReported != tc.reported || usage.CacheReadTokens != tc.cached || usage.InputTokens != 100-tc.cached {
				t.Fatal("Responses cache accounting", usage)
			}
			var chat openai.ChatCompletion
			if err := json.Unmarshal([]byte(fmt.Sprintf(`{"usage":{"prompt_tokens":100,"completion_tokens":10,"total_tokens":110,"prompt_tokens_details":%s}}`, tc.details)), &chat); err != nil {
				t.Fatal(err)
			}
			usage, _ = DefaultUsageFunc(chat)
			if usage.CacheReadReported != tc.reported || usage.CacheReadTokens != tc.cached || usage.InputTokens != 100-tc.cached {
				t.Fatal("Completions cache accounting", usage)
			}
			var chunk openai.ChatCompletionChunk
			if err := json.Unmarshal([]byte(fmt.Sprintf(`{"usage":{"prompt_tokens":100,"completion_tokens":10,"total_tokens":110,"prompt_tokens_details":%s}}`, tc.details)), &chunk); err != nil {
				t.Fatal(err)
			}
			usage, _ = DefaultStreamUsageFunc(chunk, nil, model.ProviderMetadata{})
			if usage.CacheReadReported != tc.reported || usage.CacheReadTokens != tc.cached {
				t.Fatal("stream cache accounting", usage)
			}
		})
	}
}

func TestChatStreamsRetainLastUsageWithoutTotal(t *testing.T) {
	for _, name := range []string{"text", "object"} {
		t.Run(name, func(t *testing.T) {
			server := newStreamingMockServer()
			defer server.close()
			server.chunks = []string{
				"data: " + `{"id":"a","choices":[{"index":0,"delta":{"role":"assistant","content":"{}"}}]}` + "\n\n",
				"data: " + `{"id":"a","choices":[],"usage":{"prompt_tokens":100,"completion_tokens":12,"prompt_tokens_details":{"cached_tokens":40},"completion_tokens_details":{"reasoning_tokens":8,"accepted_prediction_tokens":4},"gateway":"local"}}` + "\n\n",
				"data: " + `{"id":"a","choices":[{"index":0,"delta":{},"finish_reason":"stop"}],"usage":null}` + "\n\n",
				"data: [DONE]\n\n",
			}
			hook := func(chunk openai.ChatCompletionChunk, ctx map[string]any, metadata model.ProviderMetadata) (model.Usage, model.ProviderMetadata) {
				count, _ := ctx["chunks"].(int)
				ctx["chunks"] = count + 1
				usage, metadata := DefaultStreamUsageFunc(chunk, ctx, metadata)
				metadata.ResponseID = fmt.Sprint(count + 1)
				return usage, metadata
			}
			client, err := New(WithAPI(APIChatCompletions), WithBaseURL(server.server.URL), WithLanguageModelOptions(WithLanguageModelStreamUsageFunc(hook)))
			if err != nil {
				t.Fatal(err)
			}
			m, err := client.LanguageModel(t.Context(), "test-model")
			if err != nil {
				t.Fatal(err)
			}
			var usage model.Usage
			var metadata model.ProviderMetadata
			if name == "text" {
				stream, err := m.Stream(t.Context(), model.Call{Prompt: testPrompt})
				if err != nil {
					t.Fatal(err)
				}
				for part := range stream {
					if part.Error != nil {
						t.Fatal(part.Error)
					}
					if part.Type == model.StreamPartTypeFinish {
						usage, metadata = part.Usage, part.ProviderMetadata
					}
				}
			} else {
				stream, err := m.StreamObject(t.Context(), model.ObjectCall{Prompt: testPrompt, Schema: model.Schema{Type: "object"}})
				if err != nil {
					t.Fatal(err)
				}
				for part := range stream {
					if part.Error != nil {
						t.Fatal(part.Error)
					}
					if part.Type == model.ObjectStreamPartTypeFinish {
						usage, metadata = part.Usage, part.ProviderMetadata
					}
				}
			}
			want := model.Usage{InputTokens: 60, OutputTokens: 12, ReasoningTokens: 8, CacheReadTokens: 40, CacheReadReported: true}
			if usage != want || metadata.ResponseID != "3" || metadata.AcceptedPredictionTokens != 4 || string(metadata.ExtraFields["gateway"]) != `"local"` {
				t.Fatalf("finish usage or metadata lost: %+v %+v", usage, metadata)
			}
		})
	}
}
