package openai

// Reasoning effort levels accepted by the chat completions and responses APIs.
const (
	ReasoningEffortNone    ReasoningEffort = "none"
	ReasoningEffortMinimal ReasoningEffort = "minimal"
	ReasoningEffortLow     ReasoningEffort = "low"
	ReasoningEffortMedium  ReasoningEffort = "medium"
	ReasoningEffortHigh    ReasoningEffort = "high"
	ReasoningEffortXhigh   ReasoningEffort = "xhigh"
	ReasoningEffortMax     ReasoningEffort = "max"
)

// EasyInputMessageRole is the author role of a Responses API message item.
const (
	EasyInputMessageRoleUser      EasyInputMessageRole = "user"
	EasyInputMessageRoleAssistant EasyInputMessageRole = "assistant"
	EasyInputMessageRoleSystem    EasyInputMessageRole = "system"
	EasyInputMessageRoleDeveloper EasyInputMessageRole = "developer"
)

// ToolChoiceOptions selects how the model may call tools.
const (
	ToolChoiceOptionsNone     ToolChoiceOptions = "none"
	ToolChoiceOptionsAuto     ToolChoiceOptions = "auto"
	ToolChoiceOptionsRequired ToolChoiceOptions = "required"
)

// WebSearchToolTypeWebSearch marks the hosted web search tool.
const WebSearchToolTypeWebSearch WebSearchToolType = "web_search"

// Search context sizes accepted by the hosted web search tool.
const (
	WebSearchToolSearchContextSizeLow    WebSearchToolSearchContextSize = "low"
	WebSearchToolSearchContextSizeMedium WebSearchToolSearchContextSize = "medium"
	WebSearchToolSearchContextSizeHigh   WebSearchToolSearchContextSize = "high"
)

// ResponseNewParamsTruncation controls how a truncated response is handled.
const (
	ResponseNewParamsTruncationAuto     ResponseNewParamsTruncation = "auto"
	ResponseNewParamsTruncationDisabled ResponseNewParamsTruncation = "disabled"
)
