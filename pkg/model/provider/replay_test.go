package provider

import (
	"clara-code/pkg/core"
	yuzu "clara-code/pkg/model"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// serveSSE returns an httptest server that answers streaming POSTs with the
// given SSE payloads in order (one per request), flushing after each event.
// It records every request body it receives. The special marker line
// "<connection closed>" hijacks and closes the connection without a [DONE].
// Non-streaming requests get a canned JSON chat.completion reply.
func serveSSE(t *testing.T, payloads ...string) (*httptest.Server, *[][]byte) {
	t.Helper()
	var bodies [][]byte
	var calls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		bodies = append(bodies, body)

		var reqShape struct {
			Stream bool `json:"stream"`
		}
		_ = json.Unmarshal(body, &reqShape)
		if !reqShape.Stream {
			w.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprint(w, `{"id":"y","created":1,"model":"deepseek-v4-flash","object":"chat.completion","choices":[{"index":0,"message":{"role":"assistant","content":"The date is 2026-08-28."},"finish_reason":"stop"}],"usage":{"prompt_tokens":200,"completion_tokens":10,"total_tokens":210}}`)
			return
		}

		payload := payloads[min(calls, len(payloads)-1)]
		calls++

		w.Header().Set("Content-Type", "text/event-stream")
		rc := http.NewResponseController(w)
		for event := range strings.Lines(payload) {
			if strings.TrimSpace(event) == "<connection closed>" {
				_ = rc.Flush()
				hj, ok := w.(http.Hijacker)
				if !ok {
					return
				}
				conn, _, err := hj.Hijack()
				if err != nil {
					return
				}
				_ = conn.Close()
				return
			}
			if strings.TrimSpace(event) == "" {
				continue
			}
			fmt.Fprintf(w, "data: %s\n\n", strings.TrimSpace(event))
			_ = rc.Flush()
		}
		_, _ = fmt.Fprint(w, "data: [DONE]\n\n")
		_ = rc.Flush()
	}))
	t.Cleanup(srv.Close)
	return srv, &bodies
}

func streamParts(t *testing.T, lm yuzu.LanguageModel, prompt yuzu.Prompt) []yuzu.StreamPart {
	t.Helper()
	stream, err := lm.Stream(context.Background(), yuzu.Call{Prompt: prompt})
	require.NoError(t, err)
	var parts []yuzu.StreamPart
	for part := range stream {
		parts = append(parts, part)
	}
	return parts
}

func partTypes(parts []yuzu.StreamPart) []yuzu.StreamPartType {
	types := make([]yuzu.StreamPartType, 0, len(parts))
	for _, p := range parts {
		types = append(types, p.Type)
	}
	return types
}

// countParts counts parts of a type; for reasoning deltas it also
// concatenates the deltas.
func reasoningText(parts []yuzu.StreamPart) string {
	var sb strings.Builder
	for _, p := range parts {
		if p.Type == yuzu.StreamPartTypeReasoningDelta {
			sb.WriteString(p.Delta)
		}
	}
	return sb.String()
}

func countType(parts []yuzu.StreamPart, typ yuzu.StreamPartType) int {
	n := 0
	for _, p := range parts {
		if p.Type == typ {
			n++
		}
	}
	return n
}

// A.1 from CHARM-2020: DeepSeek-shaped thinking + tool call. Every delta
// carries both keys; non-reasoning chunks have "reasoning_content": null.
const deepseekThinkingToolCallSSE = `{"id":"x","created":1,"model":"deepseek-v4-flash","choices":[{"index":0,"delta":{"role":"assistant","content":"","reasoning_content":null},"finish_reason":null}]}
{"id":"x","created":1,"model":"deepseek-v4-flash","choices":[{"index":0,"delta":{"content":null,"reasoning_content":"The user wants the date."},"finish_reason":null}]}
{"id":"x","created":1,"model":"deepseek-v4-flash","choices":[{"index":0,"delta":{"content":null,"reasoning_content":" I'll call get_date."},"finish_reason":null}]}
{"id":"x","created":1,"model":"deepseek-v4-flash","choices":[{"index":0,"delta":{"content":"Let me check.","reasoning_content":null},"finish_reason":null}]}
{"id":"x","created":1,"model":"deepseek-v4-flash","choices":[{"index":0,"delta":{"content":null,"reasoning_content":null,"tool_calls":[{"index":0,"id":"call_00_abc","type":"function","function":{"name":"get_date","arguments":""}}]},"finish_reason":null}]}
{"id":"x","created":1,"model":"deepseek-v4-flash","choices":[{"index":0,"delta":{"content":null,"reasoning_content":null,"tool_calls":[{"index":0,"function":{"arguments":"{}"}}]},"finish_reason":null}]}
{"id":"x","created":1,"model":"deepseek-v4-flash","choices":[{"index":0,"delta":{"content":"","reasoning_content":null},"finish_reason":"tool_calls"}]}
{"id":"x","created":1,"model":"deepseek-v4-flash","choices":[],"usage":{"prompt_tokens":120,"completion_tokens":40,"total_tokens":160}}`

const cannedStopReplySSE = `{"id":"y","created":1,"model":"deepseek-v4-flash","choices":[{"index":0,"delta":{"role":"assistant","content":"The date is 2026-08-28."},"finish_reason":null}]}
{"id":"y","created":1,"model":"deepseek-v4-flash","choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}
{"id":"y","created":1,"model":"deepseek-v4-flash","choices":[],"usage":{"prompt_tokens":200,"completion_tokens":10,"total_tokens":210}}`

// TestReplay_DeepSeekThinkingToolCall drives fixture A.1 through a real
// openaicompat model and asserts the reasoning block survives end-to-end:
// exactly one ReasoningStart, the concatenated deltas, exactly one
// ReasoningEnd before Finish, and no empty ReasoningDeltas.
func TestReplay_DeepSeekThinkingToolCall(t *testing.T) {
	srv, _ := serveSSE(t, deepseekThinkingToolCallSSE)
	provider, err := New(WithBaseURL(srv.URL), WithAPIKey("x"))
	require.NoError(t, err)
	lm, err := provider.LanguageModel(context.Background(), "deepseek-v4-flash")
	require.NoError(t, err)

	parts := streamParts(t, lm, yuzu.Prompt{
		{Role: yuzu.MessageRoleUser, Content: []yuzu.MessagePart{yuzu.TextPart{Text: "date?"}}},
	})

	require.Equal(t, 1, countType(parts, yuzu.StreamPartTypeReasoningStart), "types: %v", partTypes(parts))
	require.Equal(t, 1, countType(parts, yuzu.StreamPartTypeReasoningEnd), "types: %v", partTypes(parts))
	require.Equal(t, "The user wants the date. I'll call get_date.", reasoningText(parts))
	for _, p := range parts {
		if p.Type == yuzu.StreamPartTypeReasoningDelta {
			require.NotEmpty(t, p.Delta, "empty ReasoningDelta emitted")
		}
	}
	// ReasoningEnd must come before Finish.
	endIdx, finishIdx := -1, -1
	for i, p := range parts {
		if p.Type == yuzu.StreamPartTypeReasoningEnd {
			endIdx = i
		}
		if p.Type == yuzu.StreamPartTypeFinish {
			finishIdx = i
		}
	}
	require.Greater(t, endIdx, -1)
	require.Greater(t, finishIdx, endIdx, "ReasoningEnd must precede Finish: %v", partTypes(parts))
	require.Equal(t, 1, countType(parts, yuzu.StreamPartTypeToolCall), "types: %v", partTypes(parts))
}

// TestReplay_DeepSeekReasoningRoundTripsToNextRequest is the regression test
// for crush#2696 / CHARM-2020: the reasoning the model produced must be sent
// back, byte-equal, on the assistant message carrying tool_calls in the next
// request. We drive two steps manually (no agent) to keep the harness small.
// The Responses API has its own switch for the same requirement.
func TestToResponsesPrompt_ReasoningReplay(t *testing.T) {
	t.Parallel()

	prompt := yuzu.Prompt{
		{Role: yuzu.MessageRoleUser, Content: []yuzu.MessagePart{yuzu.TextPart{Text: "date?"}}},
		{Role: yuzu.MessageRoleAssistant, Content: []yuzu.MessagePart{
			yuzu.ReasoningPart{Text: "I should call the clock tool."},
			yuzu.ToolCallPart{ToolCallID: "call-1", ToolName: "clock", Input: "{}"},
		}},
		{Role: yuzu.MessageRoleTool, Content: []yuzu.MessagePart{
			yuzu.ToolResultPart{ToolCallID: "call-1", Output: yuzu.ToolResultOutputContentText{Text: "2026-09-17"}},
		}},
	}

	// Default: reasoning stays out of the request, which is what OpenAI
	// expects because it already owns the item server-side.
	dropped, warnings := toResponsesPrompt(prompt, responsesPromptOptions{systemMessageMode: "system"})
	require.Empty(t, warnings)
	require.Equal(t, []string{"message", "call=call-1", "output=call-1"}, responseItemSignatures(dropped))

	// DeepSeek thinking mode rejects a tool continuation without the chain of
	// thought, so the opt-in replays it as plaintext before the tool call.
	replayed, warnings := toResponsesPrompt(prompt, responsesPromptOptions{
		systemMessageMode: "system",
		replayReasoning:   true,
	})
	require.Empty(t, warnings)
	require.Equal(t, []string{"message", "reasoning", "call=call-1", "output=call-1"}, responseItemSignatures(replayed))

	reasoning := replayed[1].OfReasoning
	require.NotNil(t, reasoning)
	require.Len(t, reasoning.Content, 1)
	require.Equal(t, "I should call the clock tool.", reasoning.Content[0].Text)
}

func TestToResponsesPrompt_ReasoningReplaySkipsEmptyText(t *testing.T) {
	t.Parallel()

	prompt := yuzu.Prompt{
		{Role: yuzu.MessageRoleAssistant, Content: []yuzu.MessagePart{
			yuzu.ReasoningPart{},
			yuzu.TextPart{Text: "done"},
		}},
	}

	replayed, warnings := toResponsesPrompt(prompt, responsesPromptOptions{
		systemMessageMode: "system",
		replayReasoning:   true,
	})

	require.Empty(t, warnings)
	require.Equal(t, []string{"message"}, responseItemSignatures(replayed))
}

func TestReplay_DeepSeekReasoningRoundTripsToNextRequest(t *testing.T) {
	srv, bodies := serveSSE(t, deepseekThinkingToolCallSSE, cannedStopReplySSE)
	provider, err := New(WithBaseURL(srv.URL), WithAPIKey("x"))
	require.NoError(t, err)
	lm, err := provider.LanguageModel(context.Background(), "deepseek-v4-flash")
	require.NoError(t, err)

	// Step 1: collect reasoning + tool call from the stream.
	parts := streamParts(t, lm, yuzu.Prompt{
		{Role: yuzu.MessageRoleUser, Content: []yuzu.MessagePart{yuzu.TextPart{Text: "date?"}}},
	})
	reasoning := reasoningText(parts)
	require.NotEmpty(t, reasoning)
	var toolCall *yuzu.StreamPart
	for i, p := range parts {
		if p.Type == yuzu.StreamPartTypeToolCall {
			toolCall = &parts[i]
		}
	}
	require.NotNil(t, toolCall, "types: %v", partTypes(parts))

	// Step 2: replay the assistant message the way a faithful client would
	// and assert on the request body the provider sends upstream. Non-stream
	// so the server answers with JSON.
	resp, err := lm.Generate(context.Background(), yuzu.Call{Prompt: yuzu.Prompt{
		{Role: yuzu.MessageRoleUser, Content: []yuzu.MessagePart{yuzu.TextPart{Text: "date?"}}},
		{Role: yuzu.MessageRoleAssistant, Content: []yuzu.MessagePart{
			yuzu.ReasoningPart{Text: reasoning},
			yuzu.TextPart{Text: "Let me check."},
			yuzu.ToolCallPart{ToolCallID: toolCall.ID, ToolName: toolCall.ToolCallName, Input: toolCall.ToolCallInput},
		}},
		{Role: yuzu.MessageRoleTool, Content: []yuzu.MessagePart{
			yuzu.ToolResultPart{ToolCallID: toolCall.ID, Output: yuzu.ToolResultOutputContentText{Text: "2026-08-28"}},
		}},
	}})
	require.NoError(t, err)
	require.NotNil(t, resp)

	require.Len(t, *bodies, 2)
	var second struct {
		Messages []struct {
			Role             string  `json:"role"`
			ReasoningContent *string `json:"reasoning_content"`
			ToolCalls        []struct {
				ID       string `json:"id"`
				Function struct {
					Name      string `json:"name"`
					Arguments string `json:"arguments"`
				} `json:"function"`
			} `json:"tool_calls"`
		} `json:"messages"`
	}
	require.NoError(t, json.Unmarshal((*bodies)[1], &second))
	var assistant *struct {
		Role             string  `json:"role"`
		ReasoningContent *string `json:"reasoning_content"`
		ToolCalls        []struct {
			ID       string `json:"id"`
			Function struct {
				Name      string `json:"name"`
				Arguments string `json:"arguments"`
			} `json:"function"`
		} `json:"tool_calls"`
	}
	for i := range second.Messages {
		if second.Messages[i].Role == "assistant" {
			assistant = &second.Messages[i]
		}
	}
	require.NotNil(t, assistant)
	require.NotNil(t, assistant.ReasoningContent, "reasoning_content missing on replayed assistant message: %s", (*bodies)[1])
	require.Equal(t, reasoning, *assistant.ReasoningContent, "reasoning_content must round-trip byte-for-byte")
	require.Len(t, assistant.ToolCalls, 1)
	require.Equal(t, "call_00_abc", assistant.ToolCalls[0].ID)
	require.Equal(t, "get_date", assistant.ToolCalls[0].Function.Name)
	require.Equal(t, "{}", assistant.ToolCalls[0].Function.Arguments)
}

// A.2: reasoning-only response truncated by length. ReasoningEnd must still
// be emitted (on the finish chunk) and the reasoning kept.
const deepseekReasoningOnlyLengthSSE = `{"id":"x","created":1,"model":"deepseek-v4-flash","choices":[{"index":0,"delta":{"role":"assistant","content":"","reasoning_content":null},"finish_reason":null}]}
{"id":"x","created":1,"model":"deepseek-v4-flash","choices":[{"index":0,"delta":{"content":null,"reasoning_content":"Thinking about it"},"finish_reason":null}]}
{"id":"x","created":1,"model":"deepseek-v4-flash","choices":[{"index":0,"delta":{"content":null,"reasoning_content":" at length…"},"finish_reason":null}]}
{"id":"x","created":1,"model":"deepseek-v4-flash","choices":[{"index":0,"delta":{"content":"","reasoning_content":null},"finish_reason":"length"}]}`

func TestReplay_DeepSeekReasoningOnlyTruncated(t *testing.T) {
	srv, _ := serveSSE(t, deepseekReasoningOnlyLengthSSE)
	provider, err := New(WithBaseURL(srv.URL), WithAPIKey("x"))
	require.NoError(t, err)
	lm, err := provider.LanguageModel(context.Background(), "deepseek-v4-flash")
	require.NoError(t, err)

	parts := streamParts(t, lm, yuzu.Prompt{
		{Role: yuzu.MessageRoleUser, Content: []yuzu.MessagePart{yuzu.TextPart{Text: "hi"}}},
	})
	require.Equal(t, "Thinking about it at length…", reasoningText(parts))
	require.Equal(t, 1, countType(parts, yuzu.StreamPartTypeReasoningEnd), "types: %v", partTypes(parts))
	var finish *yuzu.StreamPart
	for i, p := range parts {
		if p.Type == yuzu.StreamPartTypeFinish {
			finish = &parts[i]
		}
	}
	require.NotNil(t, finish)
	require.Equal(t, yuzu.FinishReasonLength, finish.FinishReason)
}

// A.3: a batching host puts the reasoning tail and the whole tool call in
// one delta. The reasoning must be kept and the tool call intact.
const deepseekBatchedBoundarySSE = `{"id":"x","created":1,"model":"deepseek-v4-flash","choices":[{"index":0,"delta":{"content":null,"reasoning_content":"Need the date."},"finish_reason":null}]}
{"id":"x","created":1,"model":"deepseek-v4-flash","choices":[{"index":0,"delta":{"content":null,"reasoning_content":" Calling.","tool_calls":[{"index":0,"id":"call_00_x","type":"function","function":{"name":"get_date","arguments":"{}"}}]},"finish_reason":null}]}
{"id":"x","created":1,"model":"deepseek-v4-flash","choices":[{"index":0,"delta":{"content":"","reasoning_content":null},"finish_reason":"tool_calls"}]}`

func TestReplay_DeepSeekBatchedBoundaryChunk(t *testing.T) {
	srv, _ := serveSSE(t, deepseekBatchedBoundarySSE)
	provider, err := New(WithBaseURL(srv.URL), WithAPIKey("x"))
	require.NoError(t, err)
	lm, err := provider.LanguageModel(context.Background(), "deepseek-v4-flash")
	require.NoError(t, err)

	parts := streamParts(t, lm, yuzu.Prompt{
		{Role: yuzu.MessageRoleUser, Content: []yuzu.MessagePart{yuzu.TextPart{Text: "date?"}}},
	})
	require.Equal(t, "Need the date. Calling.", reasoningText(parts))
	require.Equal(t, 1, countType(parts, yuzu.StreamPartTypeToolCall), "types: %v", partTypes(parts))
}

// A.5: Kimi/Avian shape. Reasoning chunks carry reasoning_content, content
// chunks omit the key, the finish chunk has "reasoning_content": null —
// which must not reopen a reasoning block.
const kimiShapeSSE = `{"id":"x","created":1,"model":"kimi","choices":[{"index":0,"delta":{"role":"assistant","reasoning_content":"first"},"finish_reason":null}]}
{"id":"x","created":1,"model":"kimi","choices":[{"index":0,"delta":{"reasoning_content":" second"},"finish_reason":null}]}
{"id":"x","created":1,"model":"kimi","choices":[{"index":0,"delta":{"content":"answer"},"finish_reason":null}]}
{"id":"x","created":1,"model":"kimi","choices":[{"index":0,"delta":{"reasoning_content":null},"finish_reason":"stop"}]}`

func TestReplay_KimiNullFinishDoesNotReopen(t *testing.T) {
	srv, _ := serveSSE(t, kimiShapeSSE)
	provider, err := New(WithBaseURL(srv.URL), WithAPIKey("x"))
	require.NoError(t, err)
	lm, err := provider.LanguageModel(context.Background(), "kimi")
	require.NoError(t, err)

	parts := streamParts(t, lm, yuzu.Prompt{
		{Role: yuzu.MessageRoleUser, Content: []yuzu.MessagePart{yuzu.TextPart{Text: "hi"}}},
	})
	require.Equal(t, "first second", reasoningText(parts))
	require.Equal(t, 1, countType(parts, yuzu.StreamPartTypeReasoningStart), "types: %v", partTypes(parts))
	require.Equal(t, 1, countType(parts, yuzu.StreamPartTypeReasoningEnd), "types: %v", partTypes(parts))
	for _, p := range parts {
		if p.Type == yuzu.StreamPartTypeReasoningDelta {
			require.NotEmpty(t, p.Delta, "empty ReasoningDelta emitted (phantom block reopened)")
		}
	}
}

// Kimi present-but-empty: a chunk with "reasoning_content": "" before any
// content starts an (empty) reasoning block that closes on the first
// tool-call chunk and replays as "reasoning_content": "".
const kimiEmptyReasoningSSE = `{"id":"x","created":1,"model":"kimi","choices":[{"index":0,"delta":{"role":"assistant","content":"","reasoning_content":""},"finish_reason":null}]}
{"id":"x","created":1,"model":"kimi","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"call_1","type":"function","function":{"name":"f","arguments":"{}"}}]},"finish_reason":null}]}
{"id":"x","created":1,"model":"kimi","choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}]}`

func TestReplay_KimiPresentButEmpty(t *testing.T) {
	srv, _ := serveSSE(t, kimiEmptyReasoningSSE)
	provider, err := New(WithBaseURL(srv.URL), WithAPIKey("x"))
	require.NoError(t, err)
	lm, err := provider.LanguageModel(context.Background(), "kimi")
	require.NoError(t, err)

	parts := streamParts(t, lm, yuzu.Prompt{
		{Role: yuzu.MessageRoleUser, Content: []yuzu.MessagePart{yuzu.TextPart{Text: "hi"}}},
	})
	require.Equal(t, 1, countType(parts, yuzu.StreamPartTypeReasoningStart), "types: %v", partTypes(parts))
	require.Equal(t, 1, countType(parts, yuzu.StreamPartTypeReasoningEnd), "types: %v", partTypes(parts))
	for _, p := range parts {
		if p.Type == yuzu.StreamPartTypeReasoningDelta {
			require.NotEmpty(t, p.Delta, "empty ReasoningDelta emitted")
		}
	}
}

// A.4: a stream cut mid-arguments with no finish_reason and no [DONE] must
// not surface a complete ToolCall: truncated arguments must never be
// dispatched.
const deepseekCutMidArgumentsSSE = `{"id":"x","created":1,"model":"deepseek-v4-flash","choices":[{"index":0,"delta":{"content":null,"reasoning_content":"Write the file."},"finish_reason":null}]}
{"id":"x","created":1,"model":"deepseek-v4-flash","choices":[{"index":0,"delta":{"content":null,"reasoning_content":null,"tool_calls":[{"index":0,"id":"call_00_y","type":"function","function":{"name":"write","arguments":""}}]},"finish_reason":null}]}
{"id":"x","created":1,"model":"deepseek-v4-flash","choices":[{"index":0,"delta":{"content":null,"reasoning_content":null,"tool_calls":[{"index":0,"function":{"arguments":"{\"path\":\"main.go\",\"content\":\"package ma"}}]},"finish_reason":null}]}
<connection closed>`

func TestReplay_CutStreamMidArguments_NotDispatched(t *testing.T) {
	srv, _ := serveSSE(t, deepseekCutMidArgumentsSSE)
	provider, err := New(WithBaseURL(srv.URL), WithAPIKey("x"))
	require.NoError(t, err)
	lm, err := provider.LanguageModel(context.Background(), "deepseek-v4-flash")
	require.NoError(t, err)

	parts := streamParts(t, lm, yuzu.Prompt{
		{Role: yuzu.MessageRoleUser, Content: []yuzu.MessagePart{yuzu.TextPart{Text: "write main.go"}}},
	})
	require.Equal(t, 0, countType(parts, yuzu.StreamPartTypeToolCall),
		"truncated tool call must not be dispatched; types: %v", partTypes(parts))
	var sawIncomplete bool
	for _, p := range parts {
		if p.Type == yuzu.StreamPartTypeError {
			sawIncomplete = true
		}
	}
	require.True(t, sawIncomplete, "expected an error part (IncompleteStreamError), types: %v", partTypes(parts))
}

// TestReplay_AgentReasoningRoundTrip is the regression test for crush#2696
// and the client-side half of CHARM-2020: drive fixture A.1 through a real
// agent (the loop Crush runs), let it dispatch the tool, and assert on the
// SECOND request body — the assistant message carrying tool_calls must
// carry reasoning_content byte-equal to the concatenated reasoning deltas.
func TestReplay_AgentReasoningRoundTrip(t *testing.T) {
	srv, bodies := serveSSE(t, deepseekThinkingToolCallSSE, cannedStopReplySSE)
	provider, err := New(WithBaseURL(srv.URL), WithAPIKey("x"))
	require.NoError(t, err)
	lm, err := provider.LanguageModel(context.Background(), "deepseek-v4-flash")
	require.NoError(t, err)

	var invocations []string
	getDate := yuzu.NewAgentTool(
		"get_date",
		"Get the current date.",
		func(_ context.Context, _ struct{}, _ yuzu.ToolCall) (yuzu.ToolResponse, error) {
			invocations = append(invocations, "get_date")
			return yuzu.NewTextResponse("2026-08-28"), nil
		},
	)

	engine := core.NewAgent(lm, core.WithTools(getDate))
	result, err := engine.Stream(context.Background(), core.AgentStreamCall{Prompt: "what's the date?"})
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, []string{"get_date"}, invocations)

	require.Len(t, *bodies, 2)
	var second struct {
		Messages []struct {
			Role             string  `json:"role"`
			ReasoningContent *string `json:"reasoning_content"`
			ToolCalls        []struct {
				ID       string `json:"id"`
				Function struct {
					Name      string `json:"name"`
					Arguments string `json:"arguments"`
				} `json:"function"`
			} `json:"tool_calls"`
		} `json:"messages"`
	}
	require.NoError(t, json.Unmarshal((*bodies)[1], &second))
	var assistant *struct {
		Role             string  `json:"role"`
		ReasoningContent *string `json:"reasoning_content"`
		ToolCalls        []struct {
			ID       string `json:"id"`
			Function struct {
				Name      string `json:"name"`
				Arguments string `json:"arguments"`
			} `json:"function"`
		} `json:"tool_calls"`
	}
	for i := range second.Messages {
		if second.Messages[i].Role == "assistant" {
			assistant = &second.Messages[i]
		}
	}
	require.NotNil(t, assistant, "no assistant message in second request: %s", (*bodies)[1])
	require.NotNil(t, assistant.ReasoningContent,
		"reasoning_content missing on the agent's replayed assistant message — DeepSeek 400s this and other hosts loop: %s", (*bodies)[1])
	require.Equal(t, "The user wants the date. I'll call get_date.", *assistant.ReasoningContent)
	require.Len(t, assistant.ToolCalls, 1)
	require.Equal(t, "call_00_abc", assistant.ToolCalls[0].ID)
	require.Equal(t, "{}", assistant.ToolCalls[0].Function.Arguments)
}

// A.2 at agent level: a reasoning-only response truncated by length. The
// agent must surface the reasoning content in the step even though the
// provider can only close the block on the finish chunk.
func TestReplay_AgentReasoningOnlyTruncated(t *testing.T) {
	srv, _ := serveSSE(t, deepseekReasoningOnlyLengthSSE)
	provider, err := New(WithBaseURL(srv.URL), WithAPIKey("x"))
	require.NoError(t, err)
	lm, err := provider.LanguageModel(context.Background(), "deepseek-v4-flash")
	require.NoError(t, err)

	engine := core.NewAgent(lm)
	result, err := engine.Stream(context.Background(), core.AgentStreamCall{Prompt: "think"})
	require.NoError(t, err)
	require.NotNil(t, result)
	require.NotEmpty(t, result.Steps)
	var reasoning string
	for _, c := range result.Steps[0].Content {
		if rc, ok := yuzu.AsContentType[yuzu.ReasoningContent](c); ok {
			reasoning += rc.Text
		}
	}
	require.Equal(t, "Thinking about it at length…", reasoning)
	require.Equal(t, yuzu.FinishReasonLength, result.Steps[0].FinishReason)
}

// A chunk carrying two choices must not duplicate reasoning events:
// StreamExtraFunc is invoked per choice by the openai language model but
// iterates all choices itself, so without care each choice's reasoning is
// emitted once per choice.
const multiChoiceSSE = `{"id":"x","created":1,"model":"m","choices":[{"index":0,"delta":{"reasoning_content":"a"},"finish_reason":null},{"index":1,"delta":{"reasoning_content":"b"},"finish_reason":null}]}
{"id":"x","created":1,"model":"m","choices":[{"index":0,"delta":{"content":"A"},"finish_reason":null},{"index":1,"delta":{"content":"B"},"finish_reason":null}]}
{"id":"x","created":1,"model":"m","choices":[{"index":0,"delta":{},"finish_reason":"stop"},{"index":1,"delta":{},"finish_reason":"stop"}]}`

func TestReplay_MultiChoiceReasoningNotDuplicated(t *testing.T) {
	srv, _ := serveSSE(t, multiChoiceSSE)
	provider, err := New(WithBaseURL(srv.URL), WithAPIKey("x"))
	require.NoError(t, err)
	lm, err := provider.LanguageModel(context.Background(), "m")
	require.NoError(t, err)

	parts := streamParts(t, lm, yuzu.Prompt{
		{Role: yuzu.MessageRoleUser, Content: []yuzu.MessagePart{yuzu.TextPart{Text: "hi"}}},
	})
	deltasByID := map[string]string{}
	startsByID := map[string]int{}
	for _, p := range parts {
		switch p.Type {
		case yuzu.StreamPartTypeReasoningStart:
			startsByID[p.ID]++
		case yuzu.StreamPartTypeReasoningDelta:
			deltasByID[p.ID] += p.Delta
		}
	}
	require.Equal(t, map[string]int{"0": 1, "1": 1}, startsByID, "types: %v", partTypes(parts))
	require.Equal(t, map[string]string{"0": "a", "1": "b"}, deltasByID, "types: %v", partTypes(parts))
}

// A stream that ends cleanly ([DONE]) but never sent a finish_reason must
// not dispatch tool calls whose arguments are incomplete: "tool calls were
// seen" is not proof of a complete turn. Valid arguments keep today's
// inferred tool-call turn, with a warning.
const doneNoFinishTruncatedSSE = `{"id":"x","created":1,"model":"m","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"c1","type":"function","function":{"name":"f","arguments":"{\"pa"}}]},"finish_reason":null}]}`

const doneNoFinishValidSSE = `{"id":"x","created":1,"model":"m","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"c1","type":"function","function":{"name":"f","arguments":"{}"}}]},"finish_reason":null}]}`

func TestReplay_DoneWithoutFinishReason_TruncatedArgsSuppressed(t *testing.T) {
	srv, _ := serveSSE(t, doneNoFinishTruncatedSSE)
	provider, err := New(WithBaseURL(srv.URL), WithAPIKey("x"))
	require.NoError(t, err)
	lm, err := provider.LanguageModel(context.Background(), "m")
	require.NoError(t, err)

	parts := streamParts(t, lm, yuzu.Prompt{
		{Role: yuzu.MessageRoleUser, Content: []yuzu.MessagePart{yuzu.TextPart{Text: "x"}}},
	})
	require.Equal(t, 0, countType(parts, yuzu.StreamPartTypeToolCall),
		"truncated tool call must not be dispatched; types: %v", partTypes(parts))
	var sawError bool
	for _, p := range parts {
		if p.Type == yuzu.StreamPartTypeError {
			sawError = true
		}
	}
	require.True(t, sawError, "expected an IncompleteStreamError part, types: %v", partTypes(parts))
	require.Equal(t, 0, countType(parts, yuzu.StreamPartTypeFinish), "types: %v", partTypes(parts))
}

func TestReplay_DoneWithoutFinishReason_ValidArgsKeptWithWarning(t *testing.T) {
	srv, _ := serveSSE(t, doneNoFinishValidSSE)
	provider, err := New(WithBaseURL(srv.URL), WithAPIKey("x"))
	require.NoError(t, err)
	lm, err := provider.LanguageModel(context.Background(), "m")
	require.NoError(t, err)

	parts := streamParts(t, lm, yuzu.Prompt{
		{Role: yuzu.MessageRoleUser, Content: []yuzu.MessagePart{yuzu.TextPart{Text: "x"}}},
	})
	require.Equal(t, 1, countType(parts, yuzu.StreamPartTypeToolCall), "types: %v", partTypes(parts))
	var sawWarning bool
	for _, p := range parts {
		if p.Type == yuzu.StreamPartTypeWarnings {
			sawWarning = true
		}
	}
	require.True(t, sawWarning, "expected a CallWarning about the missing finish_reason, types: %v", partTypes(parts))
	var finish *yuzu.StreamPart
	for i := range parts {
		if parts[i].Type == yuzu.StreamPartTypeFinish {
			finish = &parts[i]
		}
	}
	require.NotNil(t, finish)
	require.Equal(t, yuzu.FinishReasonToolCalls, finish.FinishReason)
}

// insufficient_system_resource is a provider-side failure, not a completed
// turn: it must map to an error finish and suppress any open tool calls.
const insufficientResourceSSE = `{"id":"x","created":1,"model":"m","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"c1","type":"function","function":{"name":"f","arguments":"{}"}}]},"finish_reason":null}]}
{"id":"x","created":1,"model":"m","choices":[{"index":0,"delta":{},"finish_reason":"insufficient_system_resource"}]}`

func TestReplay_InsufficientSystemResource_SuppressesToolCalls(t *testing.T) {
	srv, _ := serveSSE(t, insufficientResourceSSE)
	provider, err := New(WithBaseURL(srv.URL), WithAPIKey("x"))
	require.NoError(t, err)
	lm, err := provider.LanguageModel(context.Background(), "m")
	require.NoError(t, err)

	parts := streamParts(t, lm, yuzu.Prompt{
		{Role: yuzu.MessageRoleUser, Content: []yuzu.MessagePart{yuzu.TextPart{Text: "x"}}},
	})
	require.Equal(t, 0, countType(parts, yuzu.StreamPartTypeToolCall),
		"tool calls from a failed upstream must not be dispatched; types: %v", partTypes(parts))
	var finish *yuzu.StreamPart
	for i := range parts {
		if parts[i].Type == yuzu.StreamPartTypeFinish {
			finish = &parts[i]
		}
	}
	require.NotNil(t, finish, "types: %v", partTypes(parts))
	require.NotEqual(t, yuzu.FinishReasonToolCalls, finish.FinishReason)
}

// Choices may not arrive in slice order; reasoning state and part IDs must
// follow choice.Index, not the slice position within a chunk.
const reorderedChoicesSSE = `{"id":"x","created":1,"model":"m","choices":[{"index":1,"delta":{"reasoning_content":"B1"},"finish_reason":null},{"index":0,"delta":{"reasoning_content":"A1"},"finish_reason":null}]}
{"id":"x","created":1,"model":"m","choices":[{"index":0,"delta":{"reasoning_content":"A2"},"finish_reason":null},{"index":1,"delta":{"reasoning_content":"B2"},"finish_reason":null}]}
{"id":"x","created":1,"model":"m","choices":[{"index":0,"delta":{"content":"a"},"finish_reason":null},{"index":1,"delta":{"content":"b"},"finish_reason":null}]}
{"id":"x","created":1,"model":"m","choices":[{"index":0,"delta":{},"finish_reason":"stop"},{"index":1,"delta":{},"finish_reason":"stop"}]}`

func TestReplay_ReorderedChoices(t *testing.T) {
	srv, _ := serveSSE(t, reorderedChoicesSSE)
	provider, err := New(WithBaseURL(srv.URL), WithAPIKey("x"))
	require.NoError(t, err)
	lm, err := provider.LanguageModel(context.Background(), "m")
	require.NoError(t, err)

	parts := streamParts(t, lm, yuzu.Prompt{
		{Role: yuzu.MessageRoleUser, Content: []yuzu.MessagePart{yuzu.TextPart{Text: "hi"}}},
	})
	deltasByID := map[string]string{}
	endsByID := map[string]int{}
	startsByID := map[string]int{}
	for _, p := range parts {
		switch p.Type {
		case yuzu.StreamPartTypeReasoningStart:
			startsByID[p.ID]++
		case yuzu.StreamPartTypeReasoningDelta:
			deltasByID[p.ID] += p.Delta
		case yuzu.StreamPartTypeReasoningEnd:
			endsByID[p.ID]++
		}
	}
	require.Equal(t, map[string]int{"0": 1, "1": 1}, startsByID, "types: %v", partTypes(parts))
	require.Equal(t, map[string]string{"0": "A1A2", "1": "B1B2"}, deltasByID, "types: %v", partTypes(parts))
	require.Equal(t, map[string]int{"0": 1, "1": 1}, endsByID, "types: %v", partTypes(parts))
}

// A content_filter finish can cut a tool call mid-arguments; like length,
// it must not be rewritten to tool_calls and its calls must not dispatch.
const contentFilterToolCallSSE = `{"id":"x","created":1,"model":"m","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"c1","type":"function","function":{"name":"f","arguments":"{}"}}]},"finish_reason":null}]}
{"id":"x","created":1,"model":"m","choices":[{"index":0,"delta":{},"finish_reason":"content_filter"}]}`

func TestReplay_ContentFilter_SuppressesToolCalls(t *testing.T) {
	srv, _ := serveSSE(t, contentFilterToolCallSSE)
	provider, err := New(WithBaseURL(srv.URL), WithAPIKey("x"))
	require.NoError(t, err)
	lm, err := provider.LanguageModel(context.Background(), "m")
	require.NoError(t, err)

	parts := streamParts(t, lm, yuzu.Prompt{
		{Role: yuzu.MessageRoleUser, Content: []yuzu.MessagePart{yuzu.TextPart{Text: "x"}}},
	})
	require.Equal(t, 0, countType(parts, yuzu.StreamPartTypeToolCall),
		"tool calls from a filtered response must not be dispatched; types: %v", partTypes(parts))
	var finish *yuzu.StreamPart
	for i := range parts {
		if parts[i].Type == yuzu.StreamPartTypeFinish {
			finish = &parts[i]
		}
	}
	require.NotNil(t, finish, "types: %v", partTypes(parts))
	require.Equal(t, yuzu.FinishReasonContentFilter, finish.FinishReason)
}

// A stream with no finish_reason whose only tool call is a bare
// declaration (no arguments at all) was cut before any argument arrived.
// Inferring a complete "{}" call invents arguments the model never sent.
const doneNoFinishNoArgsSSE = `{"id":"x","created":1,"model":"m","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"c1","type":"function","function":{"name":"f","arguments":""}}]},"finish_reason":null}]}`

func TestReplay_DoneWithoutFinishReason_NoArgsSuppressed(t *testing.T) {
	srv, _ := serveSSE(t, doneNoFinishNoArgsSSE)
	provider, err := New(WithBaseURL(srv.URL), WithAPIKey("x"))
	require.NoError(t, err)
	lm, err := provider.LanguageModel(context.Background(), "m")
	require.NoError(t, err)

	parts := streamParts(t, lm, yuzu.Prompt{
		{Role: yuzu.MessageRoleUser, Content: []yuzu.MessagePart{yuzu.TextPart{Text: "x"}}},
	})
	require.Equal(t, 0, countType(parts, yuzu.StreamPartTypeToolCall),
		"a bare tool declaration is not a complete call; types: %v", partTypes(parts))
	var sawError bool
	for _, p := range parts {
		if p.Type == yuzu.StreamPartTypeError {
			sawError = true
		}
	}
	require.True(t, sawError, "expected IncompleteStreamError, types: %v", partTypes(parts))
}

// Interleaved parallel tool calls: deltas for index 0 continue after index
// 1 has started. Arguments for both must accumulate fully — closing index 0
// when 1 first appears would drop its trailing deltas.
const interleavedParallelSSE = `{"id":"x","created":1,"model":"m","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"c0","type":"function","function":{"name":"grep","arguments":"{\"pat"}}]},"finish_reason":null}]}
{"id":"x","created":1,"model":"m","choices":[{"index":0,"delta":{"tool_calls":[{"index":1,"id":"c1","type":"function","function":{"name":"glob","arguments":"{\"pa"}}]},"finish_reason":null}]}
{"id":"x","created":1,"model":"m","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"tern\":\"foo\"}"}}]},"finish_reason":null}]}
{"id":"x","created":1,"model":"m","choices":[{"index":0,"delta":{"tool_calls":[{"index":1,"function":{"arguments":"th\":\"x\"}"}}]},"finish_reason":null}]}
{"id":"x","created":1,"model":"m","choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}]}`

func TestReplay_InterleavedParallelToolCalls(t *testing.T) {
	srv, _ := serveSSE(t, interleavedParallelSSE)
	provider, err := New(WithBaseURL(srv.URL), WithAPIKey("x"))
	require.NoError(t, err)
	lm, err := provider.LanguageModel(context.Background(), "m")
	require.NoError(t, err)

	parts := streamParts(t, lm, yuzu.Prompt{
		{Role: yuzu.MessageRoleUser, Content: []yuzu.MessagePart{yuzu.TextPart{Text: "x"}}},
	})
	calls := map[string]string{}
	for _, p := range parts {
		if p.Type == yuzu.StreamPartTypeToolCall {
			calls[p.ToolCallName] = p.ToolCallInput
		}
	}
	require.Equal(t, map[string]string{
		"grep": `{"pattern":"foo"}`,
		"glob": `{"path":"x"}`,
	}, calls)
}

// On a cut stream, consumers must not see a ToolInputEnd that presents
// fabricated "{}" input for a call that never sent arguments.
func TestReplay_DoneWithoutFinishReason_NoFabricatedEnd(t *testing.T) {
	srv, _ := serveSSE(t, doneNoFinishNoArgsSSE)
	provider, err := New(WithBaseURL(srv.URL), WithAPIKey("x"))
	require.NoError(t, err)
	lm, err := provider.LanguageModel(context.Background(), "m")
	require.NoError(t, err)

	parts := streamParts(t, lm, yuzu.Prompt{
		{Role: yuzu.MessageRoleUser, Content: []yuzu.MessagePart{yuzu.TextPart{Text: "x"}}},
	})
	for _, p := range parts {
		require.NotEqual(t, yuzu.StreamPartTypeToolInputEnd, p.Type,
			"no ToolInputEnd may be emitted on a cut stream; types: %v", partTypes(parts))
		require.NotEqual(t, yuzu.StreamPartTypeToolCall, p.Type, "types: %v", partTypes(parts))
	}
	var sawError bool
	for _, p := range parts {
		if p.Type == yuzu.StreamPartTypeError {
			sawError = true
		}
	}
	require.True(t, sawError, "types: %v", partTypes(parts))
}
