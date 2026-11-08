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
