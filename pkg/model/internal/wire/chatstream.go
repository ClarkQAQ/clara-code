package openai

// ChatCompletionAccumulator rebuilds a [ChatCompletion] from the chunks of a
// streamed chat completion, so the assembled message, tool calls, logprobs and
// usage can be read the same way as a non-streamed response.
type ChatCompletionAccumulator struct {
	ChatCompletion
}

// AddChunk merges a chunk into the accumulated completion.
func (a *ChatCompletionAccumulator) AddChunk(chunk ChatCompletionChunk) {
	if a.ID == "" {
		a.ID = chunk.ID
	}
	if a.Object == "" {
		a.Object = chunk.Object
	}
	if a.Created == 0 {
		a.Created = chunk.Created
	}
	if a.Model == "" {
		a.Model = chunk.Model
	}
	if a.SystemFingerprint == "" {
		a.SystemFingerprint = chunk.SystemFingerprint
	}
	if a.ServiceTier == "" {
		a.ServiceTier = ChatCompletionServiceTier(chunk.ServiceTier)
	}
	if chunk.Usage.TotalTokens != 0 || chunk.Usage.PromptTokens != 0 || chunk.Usage.CompletionTokens != 0 {
		a.Usage = chunk.Usage
	}
	for _, choice := range chunk.Choices {
		a.addChoice(choice)
	}
}

func (a *ChatCompletionAccumulator) addChoice(chunk ChatCompletionChunkChoice) {
	index := int(chunk.Index)
	for len(a.Choices) <= index {
		a.Choices = append(a.Choices, ChatCompletionChoice{Index: int64(len(a.Choices))})
	}
	choice := &a.Choices[index]
	if chunk.FinishReason != "" {
		choice.FinishReason = chunk.FinishReason
	}
	if chunk.Delta.Role != "" {
		choice.Message.Role = chunk.Delta.Role
	}
	choice.Message.Content += chunk.Delta.Content
	choice.Message.Refusal += chunk.Delta.Refusal
	choice.Message.FunctionCall.Name += chunk.Delta.FunctionCall.Name
	choice.Message.FunctionCall.Arguments += chunk.Delta.FunctionCall.Arguments
	choice.Logprobs.Content = append(choice.Logprobs.Content, chunk.Logprobs.Content...)
	choice.Logprobs.Refusal = append(choice.Logprobs.Refusal, chunk.Logprobs.Refusal...)
	for _, delta := range chunk.Delta.ToolCalls {
		toolIndex := int(delta.Index)
		for len(choice.Message.ToolCalls) <= toolIndex {
			choice.Message.ToolCalls = append(choice.Message.ToolCalls, ChatCompletionMessageToolCallUnion{})
		}
		toolCall := &choice.Message.ToolCalls[toolIndex]
		if toolCall.ID == "" {
			toolCall.ID = delta.ID
		}
		if toolCall.Type == "" {
			toolCall.Type = delta.Type
		}
		toolCall.Function.Name += delta.Function.Name
		toolCall.Function.Arguments += delta.Function.Arguments
	}
}
