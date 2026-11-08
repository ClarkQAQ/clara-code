package core

import (
	"context"

	"clara-code/pkg/model"
)

// Aliases keep Yuzu-style call sites on the agent facade.
type (
	AgentTool                    = model.AgentTool
	ToolInfo                     = model.ToolInfo
	ToolCall                     = model.ToolCall
	ToolResponse                 = model.ToolResponse
	Schema                       = model.Schema
	MessageRole                  = model.MessageRole
	MessageMetadata              = model.MessageMetadata
	ProviderOptions              = model.ProviderOptions
	ProviderMetadata             = model.ProviderMetadata
	Usage                        = model.Usage
	ResponseContent              = model.ResponseContent
	Response                     = model.Response
	StreamPart                   = model.StreamPart
	StreamResponse               = model.StreamResponse
	ToolChoice                   = model.ToolChoice
	Call                         = model.Call
	CallWarning                  = model.CallWarning
	LanguageModel                = model.LanguageModel
	FinishReason                 = model.FinishReason
	Prompt                       = model.Prompt
	Message                      = model.Message
	MessagePart                  = model.MessagePart
	TextPart                     = model.TextPart
	ReasoningPart                = model.ReasoningPart
	FilePart                     = model.FilePart
	ToolCallPart                 = model.ToolCallPart
	ToolResultPart               = model.ToolResultPart
	ToolResultOutputContentText  = model.ToolResultOutputContentText
	ToolResultOutputContentError = model.ToolResultOutputContentError
	ToolResultOutputContentMedia = model.ToolResultOutputContentMedia
	ContentType                  = model.ContentType
	Content                      = model.Content
	TextContent                  = model.TextContent
	ReasoningContent             = model.ReasoningContent
	FileContent                  = model.FileContent
	SourceType                   = model.SourceType
	SourceContent                = model.SourceContent
	ToolCallContent              = model.ToolCallContent
	ToolResultContent            = model.ToolResultContent
	Tool                         = model.Tool
	FunctionTool                 = model.FunctionTool
	ProviderTool                 = model.ProviderTool
	ProviderDefinedTool          = model.ProviderDefinedTool
	ExecutableProviderTool       = model.ExecutableProviderTool
	Error                        = model.Error
	ProviderError                = model.ProviderError
	RetryError                   = model.RetryError
	NoObjectGeneratedError       = model.NoObjectGeneratedError
)

const (
	MessageRoleSystem            = model.MessageRoleSystem
	MessageRoleDeveloper         = model.MessageRoleDeveloper
	StreamPartTypeWarnings       = model.StreamPartTypeWarnings
	StreamPartTypeTextStart      = model.StreamPartTypeTextStart
	StreamPartTypeTextDelta      = model.StreamPartTypeTextDelta
	StreamPartTypeTextEnd        = model.StreamPartTypeTextEnd
	StreamPartTypeReasoningStart = model.StreamPartTypeReasoningStart
	StreamPartTypeReasoningDelta = model.StreamPartTypeReasoningDelta
	StreamPartTypeReasoningEnd   = model.StreamPartTypeReasoningEnd
	StreamPartTypeToolInputStart = model.StreamPartTypeToolInputStart
	StreamPartTypeToolInputDelta = model.StreamPartTypeToolInputDelta
	StreamPartTypeToolInputEnd   = model.StreamPartTypeToolInputEnd
	StreamPartTypeToolCall       = model.StreamPartTypeToolCall
	StreamPartTypeToolResult     = model.StreamPartTypeToolResult
	StreamPartTypeSource         = model.StreamPartTypeSource
	StreamPartTypeFinish         = model.StreamPartTypeFinish
	StreamPartTypeError          = model.StreamPartTypeError
	ToolChoiceAuto               = model.ToolChoiceAuto
	FinishReasonStop             = model.FinishReasonStop
	FinishReasonLength           = model.FinishReasonLength
	FinishReasonContentFilter    = model.FinishReasonContentFilter
	FinishReasonToolCalls        = model.FinishReasonToolCalls
	FinishReasonError            = model.FinishReasonError
	FinishReasonUnknown          = model.FinishReasonUnknown
	MessageRoleUser              = model.MessageRoleUser
	MessageRoleAssistant         = model.MessageRoleAssistant
	MessageRoleTool              = model.MessageRoleTool
	ContentTypeText              = model.ContentTypeText
	ContentTypeReasoning         = model.ContentTypeReasoning
	ContentTypeFile              = model.ContentTypeFile
	ContentTypeSource            = model.ContentTypeSource
	ContentTypeToolCall          = model.ContentTypeToolCall
	ContentTypeToolResult        = model.ContentTypeToolResult
)

func NewUserMessage(text string, files ...FilePart) Message {
	return model.NewUserMessage(text, files...)
}
func NewSystemMessage(text ...string) Message { return model.NewSystemMessage(text...) }

func NewDeveloperMessage(text ...string) Message { return model.NewDeveloperMessage(text...) }

func NewAgentTool[Input any](name, description string, run func(context.Context, Input, ToolCall) (ToolResponse, error)) AgentTool {
	return model.NewAgentTool(name, description, run)
}

func NewTextResponse(text string) ToolResponse      { return model.NewTextResponse(text) }
func NewTextErrorResponse(text string) ToolResponse { return model.NewTextErrorResponse(text) }
