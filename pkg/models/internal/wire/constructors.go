package openai

// SystemMessage builds a system message parameter.
func SystemMessage[T string | []ChatCompletionContentPartTextParam](content T) ChatCompletionMessageParamUnion {
	var message ChatCompletionSystemMessageParam
	switch value := any(content).(type) {
	case string:
		message.Content.OfString = NewOpt(value)
	case []ChatCompletionContentPartTextParam:
		message.Content.OfArrayOfContentParts = value
	}
	return ChatCompletionMessageParamUnion{OfSystem: &message}
}

// DeveloperMessage builds a developer message parameter.
func DeveloperMessage[T string | []ChatCompletionContentPartTextParam](content T) ChatCompletionMessageParamUnion {
	var message ChatCompletionDeveloperMessageParam
	switch value := any(content).(type) {
	case string:
		message.Content.OfString = NewOpt(value)
	case []ChatCompletionContentPartTextParam:
		message.Content.OfArrayOfContentParts = value
	}
	return ChatCompletionMessageParamUnion{OfDeveloper: &message}
}

// UserMessage builds a user message parameter.
func UserMessage[T string | []ChatCompletionContentPartUnionParam](content T) ChatCompletionMessageParamUnion {
	var message ChatCompletionUserMessageParam
	switch value := any(content).(type) {
	case string:
		message.Content.OfString = NewOpt(value)
	case []ChatCompletionContentPartUnionParam:
		message.Content.OfArrayOfContentParts = value
	}
	return ChatCompletionMessageParamUnion{OfUser: &message}
}

// AssistantMessage builds an assistant message parameter.
func AssistantMessage[T string | []ChatCompletionAssistantMessageParamContentArrayOfContentPartUnion](content T) ChatCompletionMessageParamUnion {
	var message ChatCompletionAssistantMessageParam
	switch value := any(content).(type) {
	case string:
		message.Content.OfString = NewOpt(value)
	case []ChatCompletionAssistantMessageParamContentArrayOfContentPartUnion:
		message.Content.OfArrayOfContentParts = value
	}
	return ChatCompletionMessageParamUnion{OfAssistant: &message}
}

// ToolMessage builds a tool result message parameter.
func ToolMessage[T string | []ChatCompletionContentPartTextParam](content T, toolCallID string) ChatCompletionMessageParamUnion {
	var message ChatCompletionToolMessageParam
	switch value := any(content).(type) {
	case string:
		message.Content.OfString = NewOpt(value)
	case []ChatCompletionContentPartTextParam:
		message.Content.OfArrayOfContentParts = value
	}
	message.ToolCallID = toolCallID
	return ChatCompletionMessageParamUnion{OfTool: &message}
}

// FileContentPart builds a file content part.
func FileContentPart(file ChatCompletionContentPartFileFileParam) ChatCompletionContentPartUnionParam {
	return ChatCompletionContentPartUnionParam{OfFile: &ChatCompletionContentPartFileParam{File: file}}
}

// ImageContentPart builds an image content part.
func ImageContentPart(imageURL ChatCompletionContentPartImageImageURLParam) ChatCompletionContentPartUnionParam {
	return ChatCompletionContentPartUnionParam{OfImageURL: &ChatCompletionContentPartImageParam{ImageURL: imageURL}}
}

// InputAudioContentPart builds an audio content part.
func InputAudioContentPart(inputAudio ChatCompletionContentPartInputAudioInputAudioParam) ChatCompletionContentPartUnionParam {
	return ChatCompletionContentPartUnionParam{OfInputAudio: &ChatCompletionContentPartInputAudioParam{InputAudio: inputAudio}}
}

// ResponseInputItemParamOfMessage builds a message item for the Responses API.
func ResponseInputItemParamOfMessage[T string | ResponseInputMessageContentListParam](content T, role EasyInputMessageRole) ResponseInputItemUnionParam {
	var message EasyInputMessageParam
	switch value := any(content).(type) {
	case string:
		message.Content.OfString = NewOpt(value)
	case ResponseInputMessageContentListParam:
		message.Content.OfInputItemContentList = value
	}
	message.Role = role
	return ResponseInputItemUnionParam{OfMessage: &message}
}

// ResponseInputItemParamOfItemReference builds a reference to a stored item.
func ResponseInputItemParamOfItemReference(id string) ResponseInputItemUnionParam {
	return ResponseInputItemUnionParam{OfItemReference: &ResponseInputItemItemReferenceParam{ID: id}}
}

// ResponseInputItemParamOfFunctionCall builds a function call item.
func ResponseInputItemParamOfFunctionCall(arguments, callID, name string) ResponseInputItemUnionParam {
	return ResponseInputItemUnionParam{OfFunctionCall: &ResponseFunctionToolCallParam{
		Arguments: arguments,
		CallID:    callID,
		Name:      name,
	}}
}

// ResponseInputItemParamOfFunctionCallOutput builds a function call result item.
func ResponseInputItemParamOfFunctionCallOutput[T string | ResponseFunctionCallOutputItemListParam](output T) ResponseInputItemUnionParam {
	var item ResponseInputItemFunctionCallOutputParam
	switch value := any(output).(type) {
	case string:
		item.Output.OfString = NewOpt(value)
	case ResponseFunctionCallOutputItemListParam:
		item.Output.OfResponseFunctionCallOutputItemArray = value
	}
	return ResponseInputItemUnionParam{OfFunctionCallOutput: &item}
}

// ResponseFormatTextConfigParamOfJSONSchema builds a structured output format.
func ResponseFormatTextConfigParamOfJSONSchema(name string, schema map[string]any) ResponseFormatTextConfigUnionParam {
	return ResponseFormatTextConfigUnionParam{OfJSONSchema: &ResponseFormatTextJSONSchemaConfigParam{
		Name:   name,
		Schema: schema,
	}}
}
