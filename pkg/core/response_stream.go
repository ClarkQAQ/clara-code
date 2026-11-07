package core

import "clara-code/pkg/models"

func responseStream(response *Response) StreamResponse {
	return func(yield func(StreamPart) bool) {
		if len(response.Warnings) > 0 && !yield(StreamPart{Type: StreamPartTypeWarnings, Warnings: response.Warnings}) {
			return
		}
		for _, content := range response.Content {
			part := StreamPart{Type: models.StreamPartTypeContent, Content: content}
			if call, ok := models.AsContentType[ToolCallContent](content); ok {
				part = StreamPart{Type: StreamPartTypeToolCall, ID: call.ToolCallID, ToolCallName: call.ToolName, ToolCallInput: call.Input,
					ProviderMetadata: call.ProviderMetadata, ProviderExecuted: call.ProviderExecuted}
			}
			if !yield(part) {
				return
			}
		}
		yield(StreamPart{Type: StreamPartTypeFinish, FinishReason: response.FinishReason, Usage: response.Usage, ProviderMetadata: response.ProviderMetadata})
	}
}
