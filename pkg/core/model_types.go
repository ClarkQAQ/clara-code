package core

import (
	"context"

	"clara-code/pkg/models"
)

// Aliases keep Yuzu-style call sites on the agent facade.
type (
	AgentTool                    = models.AgentTool
	ToolInfo                     = models.ToolInfo
	ToolCall                     = models.ToolCall
	ToolResponse                 = models.ToolResponse
	Schema                       = models.Schema
	MessageRole                  = models.MessageRole
	ProviderOptions              = models.ProviderOptions
	ProviderMetadata             = models.ProviderMetadata
	Usage                        = models.Usage
	ResponseContent              = models.ResponseContent
	Response                     = models.Response
	StreamPart                   = models.StreamPart
	StreamResponse               = models.StreamResponse
	ToolChoice                   = models.ToolChoice
	Call                         = models.Call
	CallWarning                  = models.CallWarning
	LanguageModel                = models.LanguageModel
	FinishReason                 = models.FinishReason
	Prompt                       = models.Prompt
	Message                      = models.Message
	MessagePart                  = models.MessagePart
	TextPart                     = models.TextPart
	ReasoningPart                = models.ReasoningPart
	FilePart                     = models.FilePart
	ToolCallPart                 = models.ToolCallPart
	ToolResultPart               = models.ToolResultPart
	ToolResultOutputContentText  = models.ToolResultOutputContentText
	ToolResultOutputContentError = models.ToolResultOutputContentError
	ToolResultOutputContentMedia = models.ToolResultOutputContentMedia
	ContentType                  = models.ContentType
	Content                      = models.Content
	TextContent                  = models.TextContent
	ReasoningContent             = models.ReasoningContent
	FileContent                  = models.FileContent
	SourceType                   = models.SourceType
	SourceContent                = models.SourceContent
	ToolCallContent              = models.ToolCallContent
	ToolResultContent            = models.ToolResultContent
	Tool                         = models.Tool
	FunctionTool                 = models.FunctionTool
	ProviderTool                 = models.ProviderTool
	ProviderDefinedTool          = models.ProviderDefinedTool
	ExecutableProviderTool       = models.ExecutableProviderTool
	Error                        = models.Error
	ProviderError                = models.ProviderError
	RetryError                   = models.RetryError
	NoObjectGeneratedError       = models.NoObjectGeneratedError
)

const (
	MessageRoleSystem            = models.MessageRoleSystem
	StreamPartTypeWarnings       = models.StreamPartTypeWarnings
	StreamPartTypeTextStart      = models.StreamPartTypeTextStart
	StreamPartTypeTextDelta      = models.StreamPartTypeTextDelta
	StreamPartTypeTextEnd        = models.StreamPartTypeTextEnd
	StreamPartTypeReasoningStart = models.StreamPartTypeReasoningStart
	StreamPartTypeReasoningDelta = models.StreamPartTypeReasoningDelta
	StreamPartTypeReasoningEnd   = models.StreamPartTypeReasoningEnd
	StreamPartTypeToolInputStart = models.StreamPartTypeToolInputStart
	StreamPartTypeToolInputDelta = models.StreamPartTypeToolInputDelta
	StreamPartTypeToolInputEnd   = models.StreamPartTypeToolInputEnd
	StreamPartTypeToolCall       = models.StreamPartTypeToolCall
	StreamPartTypeToolResult     = models.StreamPartTypeToolResult
	StreamPartTypeSource         = models.StreamPartTypeSource
	StreamPartTypeFinish         = models.StreamPartTypeFinish
	StreamPartTypeError          = models.StreamPartTypeError
	ToolChoiceAuto               = models.ToolChoiceAuto
	FinishReasonStop             = models.FinishReasonStop
	FinishReasonLength           = models.FinishReasonLength
	FinishReasonContentFilter    = models.FinishReasonContentFilter
	FinishReasonToolCalls        = models.FinishReasonToolCalls
	FinishReasonError            = models.FinishReasonError
	FinishReasonUnknown          = models.FinishReasonUnknown
	MessageRoleUser              = models.MessageRoleUser
	MessageRoleAssistant         = models.MessageRoleAssistant
	MessageRoleTool              = models.MessageRoleTool
	ContentTypeText              = models.ContentTypeText
	ContentTypeReasoning         = models.ContentTypeReasoning
	ContentTypeFile              = models.ContentTypeFile
	ContentTypeSource            = models.ContentTypeSource
	ContentTypeToolCall          = models.ContentTypeToolCall
	ContentTypeToolResult        = models.ContentTypeToolResult
)

func NewUserMessage(text string, files ...FilePart) Message {
	return models.NewUserMessage(text, files...)
}
func NewSystemMessage(text ...string) Message { return models.NewSystemMessage(text...) }

func NewAgentTool[Input any](name, description string, run func(context.Context, Input, ToolCall) (ToolResponse, error)) AgentTool {
	return models.NewAgentTool(name, description, run)
}

func NewTextResponse(text string) ToolResponse      { return models.NewTextResponse(text) }
func NewTextErrorResponse(text string) ToolResponse { return models.NewTextErrorResponse(text) }
