package openai

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDecodeChatCompletion(t *testing.T) {
	t.Parallel()

	const payload = `{
		"id": "chatcmpl-1",
		"object": "chat.completion",
		"created": 1700000000,
		"model": "gpt-4o",
		"choices": [{
			"index": 0,
			"finish_reason": "tool_calls",
			"message": {
				"role": "assistant",
				"content": "calling a tool",
				"reasoning_content": "because",
				"tool_calls": [{
					"id": "call-1",
					"type": "function",
					"function": {"name": "add", "arguments": "{\"a\":1,\"b\":2}"}
				}],
				"annotations": [{
					"type": "url_citation",
					"url_citation": {"url": "https://example.com", "title": "Example"}
				}]
			},
			"logprobs": {
				"content": [{
					"token": "call",
					"bytes": [99],
					"logprob": -0.5,
					"top_logprobs": [{"token": "call", "bytes": [99], "logprob": -0.5}]
				}]
			}
		}],
		"usage": {"prompt_tokens": 10, "completion_tokens": 2, "total_tokens": 12, "x_vendor": "yes"}
	}`

	var completion ChatCompletion
	require.NoError(t, json.Unmarshal([]byte(payload), &completion))

	choice := completion.Choices[0]
	require.Equal(t, "calling a tool", choice.Message.Content)
	require.Equal(t, "tool_calls", choice.FinishReason)
	require.Len(t, choice.Message.ToolCalls, 1)
	require.Equal(t, "call-1", choice.Message.ToolCalls[0].ID)
	require.Equal(t, "function", choice.Message.ToolCalls[0].Type)
	require.Equal(t, "add", choice.Message.ToolCalls[0].Function.Name)
	require.Equal(t, `{"a":1,"b":2}`, choice.Message.ToolCalls[0].Function.Arguments)
	require.Len(t, choice.Message.Annotations, 1)
	require.Equal(t, "https://example.com", choice.Message.Annotations[0].URLCitation.URL)
	require.Len(t, choice.Logprobs.Content, 1)
	require.Equal(t, "call", choice.Logprobs.Content[0].Token)
	require.Len(t, choice.Logprobs.Content[0].TopLogprobs, 1)

	require.NotEmpty(t, choice.Message.RawJSON())
	require.Contains(t, choice.Message.RawJSON(), "reasoning_content")
	require.JSONEq(t, `"because"`, string(choice.Message.ExtraFields["reasoning_content"]))
	require.JSONEq(t, `"yes"`, string(completion.Usage.ExtraFields["x_vendor"]))
}

func TestChatCompletionAccumulator(t *testing.T) {
	t.Parallel()

	chunks := []string{
		`{"id":"chatcmpl-1","model":"gpt-4o","choices":[{"index":0,"delta":{"role":"assistant","content":"Hello"}}]}`,
		`{"id":"chatcmpl-1","choices":[{"index":0,"delta":{"content":" world"}}]}`,
		`{"id":"chatcmpl-1","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"call-1","type":"function","function":{"name":"add","arguments":"{\"a\":"}}]}}]}`,
		`{"id":"chatcmpl-1","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"1}"}}]},"finish_reason":"tool_calls"}]}`,
		`{"id":"chatcmpl-1","choices":[],"usage":{"prompt_tokens":3,"completion_tokens":4,"total_tokens":7}}`,
	}

	var accumulator ChatCompletionAccumulator
	for _, chunk := range chunks {
		var decoded ChatCompletionChunk
		require.NoError(t, json.Unmarshal([]byte(chunk), &decoded))
		accumulator.AddChunk(decoded)
	}

	require.Equal(t, "chatcmpl-1", accumulator.ID)
	require.Equal(t, "gpt-4o", accumulator.Model)
	require.Len(t, accumulator.Choices, 1)
	require.Equal(t, "Hello world", accumulator.Choices[0].Message.Content)
	require.Equal(t, "tool_calls", accumulator.Choices[0].FinishReason)
	require.Len(t, accumulator.Choices[0].Message.ToolCalls, 1)
	require.Equal(t, `{"a":1}`, accumulator.Choices[0].Message.ToolCalls[0].Function.Arguments)
	require.Equal(t, int64(7), accumulator.Usage.TotalTokens)
}

func TestDecodeResponseEvents(t *testing.T) {
	t.Parallel()

	t.Run("created carries the response", func(t *testing.T) {
		t.Parallel()

		var event StreamEvent
		require.NoError(t, json.Unmarshal([]byte(`{"type":"response.created","sequence_number":1,"response":{"id":"resp-1","model":"gpt-5"}}`), &event))
		require.Equal(t, "response.created", event.Type)
		require.Equal(t, int64(1), event.SequenceNumber)
		require.Equal(t, "resp-1", event.AsResponseCreated().Response.ID)
	})

	t.Run("output item added carries a typed item", func(t *testing.T) {
		t.Parallel()

		const payload = `{"type":"response.output_item.added","sequence_number":2,"output_index":0,"item":{"type":"function_call","call_id":"call-1","name":"add","arguments":"","id":"item-1"}}`
		var event StreamEvent
		require.NoError(t, json.Unmarshal([]byte(payload), &event))

		added := event.AsResponseOutputItemAdded()
		require.Equal(t, "function_call", added.Item.Type)
		require.Equal(t, "call-1", added.Item.CallID)
		require.Equal(t, "add", added.Item.Name)
		require.Equal(t, int64(0), added.OutputIndex)
	})

	t.Run("text delta", func(t *testing.T) {
		t.Parallel()

		var event StreamEvent
		require.NoError(t, json.Unmarshal([]byte(`{"type":"response.output_text.delta","item_id":"item-1","delta":"Hel"}`), &event))
		require.Equal(t, "Hel", event.AsResponseOutputTextDelta().Delta)
	})

	t.Run("error event", func(t *testing.T) {
		t.Parallel()

		var event StreamEvent
		require.NoError(t, json.Unmarshal([]byte(`{"type":"error","code":"server_error","message":"boom","param":null}`), &event))
		require.Equal(t, "boom", event.AsError().Message)
		require.Equal(t, "server_error", event.AsError().Code)
	})

	t.Run("unknown event keeps its payload", func(t *testing.T) {
		t.Parallel()

		const payload = `{"type":"response.future_thing","sequence_number":9,"payload":{"a":1}}`
		var event StreamEvent
		require.NoError(t, json.Unmarshal([]byte(payload), &event))
		require.Equal(t, "response.future_thing", event.Type)
		require.JSONEq(t, payload, event.RawJSON())
	})

	t.Run("typed events decode every documented type", func(t *testing.T) {
		t.Parallel()

		samples := []string{
			"response.queued", "response.in_progress", "response.completed", "response.failed",
			"response.incomplete", "response.output_item.done", "response.content_part.added",
			"response.content_part.done", "response.output_text.done", "response.refusal.delta",
			"response.refusal.done", "response.function_call_arguments.delta",
			"response.function_call_arguments.done", "response.reasoning_summary_part.added",
			"response.reasoning_summary_part.done", "response.reasoning_summary_text.delta",
			"response.reasoning_summary_text.done", "response.reasoning_text.delta",
			"response.reasoning_text.done", "response.output_text.annotation.added",
			"response.web_search_call.in_progress", "response.web_search_call.searching",
			"response.web_search_call.completed", "response.file_search_call.in_progress",
			"response.file_search_call.searching", "response.file_search_call.completed",
			"response.code_interpreter_call.in_progress", "response.code_interpreter_call.interpreting",
			"response.code_interpreter_call.completed", "response.code_interpreter_call_code.delta",
			"response.code_interpreter_call_code.done", "response.image_generation_call.in_progress",
			"response.image_generation_call.generating", "response.image_generation_call.partial_image",
			"response.image_generation_call.completed", "response.mcp_call.in_progress",
			"response.mcp_call.completed", "response.mcp_call.failed", "response.mcp_call_arguments.delta",
			"response.mcp_call_arguments.done", "response.mcp_list_tools.in_progress",
			"response.mcp_list_tools.completed", "response.mcp_list_tools.failed",
			"response.custom_tool_call_input.delta", "response.custom_tool_call_input.done",
			"response.audio.delta", "response.audio.done", "response.audio.transcript.delta",
			"response.audio.transcript.done", "response.shell_call_command.added",
			"response.shell_call_command.delta", "response.shell_call_command.done",
			"response.shell_call_output_content.delta", "response.shell_call_output_content.done",
		}
		for _, eventType := range samples {
			payload := `{"type":"` + eventType + `","sequence_number":1}`
			var event StreamEvent
			require.NoError(t, json.Unmarshal([]byte(payload), &event), eventType)
			require.Equal(t, eventType, event.Type)
			require.NotEmpty(t, event.Raw, eventType)
		}
	})
}

func TestMarshalResponseParams(t *testing.T) {
	t.Parallel()

	params := ResponseNewParams{
		Model:              "gpt-5",
		Instructions:       NewOpt("be brief"),
		MaxOutputTokens:    NewOpt(int64(1000)),
		Store:              NewOpt(false),
		PreviousResponseID: NewOpt("resp-0"),
		Include:            []ResponseIncludable{"reasoning.encrypted_content"},
		Reasoning:          ReasoningParam{Effort: ReasoningEffortHigh},
		Text: ResponseTextConfigParam{
			Format: ResponseFormatTextConfigParamOfJSONSchema("answer", map[string]any{"type": "object"}),
		},
		Input: ResponseNewParamsInputUnion{
			OfInputItemList: ResponseInputParam{
				ResponseInputItemParamOfMessage("hello", EasyInputMessageRoleUser),
				ResponseInputItemParamOfFunctionCallOutput("42"),
				ResponseInputItemParamOfItemReference("item-1"),
			},
		},
	}

	body, err := json.Marshal(params)
	require.NoError(t, err)

	var got map[string]any
	require.NoError(t, json.Unmarshal(body, &got))
	require.Equal(t, "gpt-5", got["model"])
	require.Equal(t, false, got["store"])
	require.Equal(t, "resp-0", got["previous_response_id"])
	require.Equal(t, "be brief", got["instructions"])
	require.Equal(t, []any{"reasoning.encrypted_content"}, got["include"])
	require.Equal(t, map[string]any{"effort": "high"}, got["reasoning"])

	format := got["text"].(map[string]any)["format"].(map[string]any)
	require.Equal(t, "json_schema", format["type"])
	require.Equal(t, "answer", format["name"])
	require.Equal(t, map[string]any{"type": "object"}, format["schema"])

	input := got["input"].([]any)
	require.Equal(t, map[string]any{"role": "user", "content": "hello"}, input[0])
	require.Equal(t, map[string]any{"output": "42", "type": "function_call_output"}, input[1])
	require.Equal(t, map[string]any{"id": "item-1"}, input[2])
}
