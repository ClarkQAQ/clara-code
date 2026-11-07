// Package core provides the Agent runtime, sessions, and ACP protocol adapters.
package core

import (
	modelapi "clara-code/pkg/model"
	"clara-code/pkg/model/schema"
	"cmp"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"
	"sync"
	"time"
)

// StepResult represents the result of a single step in an agent execution.
type StepResult struct {
	Response
	Messages []Message
}

// stepExecutionResult encapsulates the result of executing a step with stream processing.
type stepExecutionResult struct {
	StepResult     StepResult
	ShouldContinue bool
}

// StopCondition defines a function that determines when an agent should stop executing.
type StopCondition = func(steps []StepResult) bool

// StepCountIs returns a stop condition that stops after the specified number of steps.
func StepCountIs(stepCount int) StopCondition {
	return func(steps []StepResult) bool {
		return len(steps) >= stepCount
	}
}

// HasToolCall returns a stop condition that stops when the specified tool is called in the last step.
func HasToolCall(toolName string) StopCondition {
	return func(steps []StepResult) bool {
		if len(steps) == 0 {
			return false
		}
		lastStep := steps[len(steps)-1]
		toolCalls := lastStep.Content.ToolCalls()
		for _, toolCall := range toolCalls {
			if toolCall.ToolName == toolName {
				return true
			}
		}
		return false
	}
}

// HasContent returns a stop condition that stops when the specified content type appears in the last step.
func HasContent(contentType ContentType) StopCondition {
	return func(steps []StepResult) bool {
		if len(steps) == 0 {
			return false
		}
		lastStep := steps[len(steps)-1]
		for _, content := range lastStep.Content {
			if content.GetType() == contentType {
				return true
			}
		}
		return false
	}
}

// FinishReasonIs returns a stop condition that stops when the specified finish reason occurs.
func FinishReasonIs(reason FinishReason) StopCondition {
	return func(steps []StepResult) bool {
		if len(steps) == 0 {
			return false
		}
		lastStep := steps[len(steps)-1]
		return lastStep.FinishReason == reason
	}
}

// MaxTokensUsed returns a stop condition that stops when total token usage exceeds the specified limit.
func MaxTokensUsed(maxTokens int64) StopCondition {
	return func(steps []StepResult) bool {
		var totalTokens int64
		for _, step := range steps {
			totalTokens += step.Usage.TotalTokens
		}
		return totalTokens >= maxTokens
	}
}

// PrepareStepFunctionOptions contains the options for preparing a step in an agent execution.
type PrepareStepFunctionOptions struct {
	Steps      []StepResult
	StepNumber int
	Model      LanguageModel
	Messages   []Message
}

// PrepareStepResult contains the result of preparing a step in an agent execution.
type PrepareStepResult struct {
	Model    LanguageModel
	Messages []Message
	// History replaces the working transcript and, when configured, checkpoints it.
	// Messages can separately add context for the current request.
	History         []Message
	System          *string
	ToolChoice      *ToolChoice
	ActiveTools     []string
	DisableAllTools bool
	Tools           []AgentTool
}

// ToolCallRepairOptions contains the options for repairing a tool call.
type ToolCallRepairOptions struct {
	OriginalToolCall ToolCallContent
	ValidationError  error
	AvailableTools   []AgentTool
	SystemPrompt     string
	Messages         []Message
}

type (
	// PrepareStepFunction defines a function that prepares a step in an agent execution.
	PrepareStepFunction = func(ctx context.Context, options PrepareStepFunctionOptions) (context.Context, PrepareStepResult, error)

	// OnStepFinishedFunction defines a function that is called when a step finishes.
	OnStepFinishedFunction = func(step StepResult)

	// RepairToolCallFunction defines a function that repairs a tool call.
	RepairToolCallFunction = func(ctx context.Context, options ToolCallRepairOptions) (*ToolCallContent, error)
)

type agentSettings struct {
	systemPrompt     string
	maxOutputTokens  *int64
	temperature      *float64
	topP             *float64
	topK             *int64
	presencePenalty  *float64
	frequencyPenalty *float64
	headers          map[string]string
	userAgent        string
	providerOptions  ProviderOptions

	providerDefinedTools    []ProviderDefinedTool
	executableProviderTools []ExecutableProviderTool
	tools                   []AgentTool
	toolChoice              *ToolChoice
	maxRetries              *int
	maxSteps                int

	model LanguageModel

	stopWhen       []StopCondition
	prepareStep    PrepareStepFunction
	repairToolCall RepairToolCallFunction
	onRetry        OnRetryCallback
}

// AgentCall represents a call to an agent.
type AgentCall struct {
	Prompt           string     `json:"prompt"`
	Files            []FilePart `json:"files"`
	Messages         []Message  `json:"messages"`
	MaxOutputTokens  *int64
	Temperature      *float64    `json:"temperature"`
	TopP             *float64    `json:"top_p"`
	TopK             *int64      `json:"top_k"`
	PresencePenalty  *float64    `json:"presence_penalty"`
	FrequencyPenalty *float64    `json:"frequency_penalty"`
	ActiveTools      []string    `json:"active_tools"`
	ToolChoice       *ToolChoice `json:"tool_choice"`
	Headers          map[string]string
	ProviderOptions  ProviderOptions
	OnRetry          OnRetryCallback
	OnAuthRefresh    OnAuthRefreshFunc
	MaxRetries       *int

	// ModelProvider, when non-nil, is called on each retry attempt to
	// obtain the language model. This allows callers to swap in a
	// refreshed model after OnAuthRefresh rebuilds credentials. When
	// nil, the model captured at step preparation time is used.
	ModelProvider func() LanguageModel

	StopWhen       []StopCondition
	PrepareStep    PrepareStepFunction
	RepairToolCall RepairToolCallFunction
	AuthorizeTool  ToolAuthorizer
}

// Agent-level callbacks.
type (
	// OnAgentStartFunc is called when agent starts.
	OnAgentStartFunc func()

	// OnAgentFinishFunc is called when agent finishes.
	OnAgentFinishFunc func(result *AgentResult) error

	// OnStepStartFunc is called when a step starts.
	OnStepStartFunc func(stepNumber int) error

	// OnStepFinishFunc is called when a step finishes.
	OnStepFinishFunc func(stepResult StepResult) error

	// OnFinishFunc is called when entire agent completes.
	OnFinishFunc func(result *AgentResult)

	// OnErrorFunc is called when an error occurs.
	OnErrorFunc func(error)

	// OnAuthRefreshFunc is called when a stream fails with an authentication
	// error that the caller may be able to resolve (e.g. an expired SSO
	// session or OAuth token). The function should perform whatever credential
	// refresh is needed and return nil on success, in which case yuzu
	// retries the operation transparently. Returning an error surfaces the
	// original auth error to the caller without retry. Pair this with
	// ModelProvider to supply a rebuilt model carrying the refreshed
	// credentials on the retry attempt.
	OnAuthRefreshFunc func(ctx context.Context, err *ProviderError) error
)

// Stream part callbacks - called for each corresponding stream part type.
type (
	// OnChunkFunc is called for each stream part (catch-all).
	OnChunkFunc func(StreamPart) error

	// OnWarningsFunc is called for warnings.
	OnWarningsFunc func(warnings []CallWarning) error

	// OnTextStartFunc is called when text starts.
	OnTextStartFunc func(id string) error

	// OnTextDeltaFunc is called for text deltas.
	OnTextDeltaFunc func(id, text string) error

	// OnTextEndFunc is called when text ends.
	OnTextEndFunc func(id string) error

	// OnReasoningStartFunc is called when reasoning starts.
	OnReasoningStartFunc func(id string, reasoning ReasoningContent) error

	// OnReasoningDeltaFunc is called for reasoning deltas.
	OnReasoningDeltaFunc func(id, text string) error

	// OnReasoningEndFunc is called when reasoning ends.
	OnReasoningEndFunc func(id string, reasoning ReasoningContent) error

	// OnToolInputStartFunc is called when tool input starts.
	OnToolInputStartFunc func(id, toolName string) error

	// OnToolInputDeltaFunc is called for tool input deltas.
	OnToolInputDeltaFunc func(id, delta string) error

	// OnToolInputEndFunc is called when tool input ends.
	OnToolInputEndFunc func(id string) error

	// OnToolCallFunc is called when tool call is complete.
	OnToolCallFunc func(toolCall ToolCallContent) error

	// OnToolResultFunc is called when tool execution completes.
	OnToolResultFunc func(result ToolResultContent) error

	// OnSourceFunc is called for source references.
	OnSourceFunc func(source SourceContent) error

	// OnStreamFinishFunc is called when stream finishes.
	OnStreamFinishFunc func(usage Usage, finishReason FinishReason, providerMetadata ProviderMetadata) error
)

// AgentStreamCall represents a streaming call to an agent.
type AgentStreamCall struct {
	Prompt           string     `json:"prompt"`
	Files            []FilePart `json:"files"`
	Messages         []Message  `json:"messages"`
	MaxOutputTokens  *int64
	Temperature      *float64    `json:"temperature"`
	TopP             *float64    `json:"top_p"`
	TopK             *int64      `json:"top_k"`
	PresencePenalty  *float64    `json:"presence_penalty"`
	FrequencyPenalty *float64    `json:"frequency_penalty"`
	ActiveTools      []string    `json:"active_tools"`
	ToolChoice       *ToolChoice `json:"tool_choice"`
	Headers          map[string]string
	ProviderOptions  ProviderOptions
	OnRetry          OnRetryCallback
	OnAuthRefresh    OnAuthRefreshFunc
	MaxRetries       *int

	// ModelProvider, when non-nil, is called on each retry attempt to
	// obtain the language model. This allows callers to swap in a
	// refreshed model after OnAuthRefresh rebuilds credentials. When
	// nil, the model captured at step preparation time is used.
	ModelProvider func() LanguageModel

	StopWhen       []StopCondition
	PrepareStep    PrepareStepFunction
	RepairToolCall RepairToolCallFunction
	AuthorizeTool  ToolAuthorizer

	Steer    func() []Message
	FollowUp func() []Message

	// Agent-level callbacks
	OnAgentStart    OnAgentStartFunc      // Called when agent starts
	OnAgentFinish   OnAgentFinishFunc     // Called when agent finishes
	OnStepStart     OnStepStartFunc       // Called when a step starts
	OnStepFinish    OnStepFinishFunc      // Called when a step finishes
	OnHistory       func([]Message) error // Checkpoint an intentional history replacement.
	OnModelMessages func([]Message) error // Checkpoint the model response before dispatch.
	OnFinish        OnFinishFunc          // Called when entire agent completes
	OnError         OnErrorFunc           // Called when an error occurs

	// Stream part callbacks - called for each corresponding stream part type
	OnChunk          OnChunkFunc          // Called for each stream part (catch-all)
	OnWarnings       OnWarningsFunc       // Called for warnings
	OnTextStart      OnTextStartFunc      // Called when text starts
	OnTextDelta      OnTextDeltaFunc      // Called for text deltas
	OnTextEnd        OnTextEndFunc        // Called when text ends
	OnReasoningStart OnReasoningStartFunc // Called when reasoning starts
	OnReasoningDelta OnReasoningDeltaFunc // Called for reasoning deltas
	OnReasoningEnd   OnReasoningEndFunc   // Called when reasoning ends
	OnToolInputStart OnToolInputStartFunc // Called when tool input starts
	OnToolInputDelta OnToolInputDeltaFunc // Called for tool input deltas
	OnToolInputEnd   OnToolInputEndFunc   // Called when tool input ends
	OnToolCall       OnToolCallFunc       // Called when tool call is complete
	OnToolResult     OnToolResultFunc     // Called when tool execution completes
	OnSource         OnSourceFunc         // Called for source references
	OnStreamFinish   OnStreamFinishFunc   // Called when stream finishes
}

// AgentResult represents the result of an agent execution.
type AgentResult struct {
	Steps []StepResult
	// Final response. When the last step is tool-only (no text content),
	// this is the response from the most recent step that contained text,
	// so callers always see meaningful output without walking Steps manually.
	Response   Response
	TotalUsage Usage
	Messages   []Message
}

// finalResponse picks the best Response from a slice of steps. It walks
// backwards to find the most recent step with non-blank text content. If no
// step has text content (e.g. all steps were tool calls), the last step's
// response is returned as-is.
func finalResponse(steps []StepResult) Response {
	for i := len(steps) - 1; i >= 0; i-- {
		if hasNonBlankText(steps[i].Content) {
			return steps[i].Response
		}
	}
	if len(steps) > 0 {
		return steps[len(steps)-1].Response
	}
	return Response{}
}

// hasNonBlankText reports whether content contains at least one text block
// with non-whitespace characters.
func hasNonBlankText(content ResponseContent) bool {
	for _, c := range content {
		if c.GetType() == ContentTypeText {
			if tc, ok := modelapi.AsContentType[TextContent](c); ok {
				if strings.TrimSpace(tc.Text) != "" {
					return true
				}
			}
		}
	}
	return false
}

// Agent represents an AI agent that can generate responses and stream responses.
type Agent interface {
	Generate(context.Context, AgentCall) (*AgentResult, error)
	Stream(context.Context, AgentStreamCall) (*AgentResult, error)
}

// AgentOption defines a function that configures agent settings.
type AgentOption = func(*agentSettings)

type agent struct {
	settings agentSettings
}

// NewAgent creates a new agent with the given language model and options.
func NewAgent(model LanguageModel, opts ...AgentOption) Agent {
	settings := agentSettings{
		model:    model,
		maxSteps: 100,
	}
	for _, o := range opts {
		o(&settings)
	}
	return &agent{
		settings: settings,
	}
}

func (a *agent) prepareCall(call AgentCall) AgentCall {
	call.MaxOutputTokens = cmp.Or(call.MaxOutputTokens, a.settings.maxOutputTokens)
	call.Temperature = cmp.Or(call.Temperature, a.settings.temperature)
	call.TopP = cmp.Or(call.TopP, a.settings.topP)
	call.TopK = cmp.Or(call.TopK, a.settings.topK)
	call.PresencePenalty = cmp.Or(call.PresencePenalty, a.settings.presencePenalty)
	call.FrequencyPenalty = cmp.Or(call.FrequencyPenalty, a.settings.frequencyPenalty)
	call.MaxRetries = cmp.Or(call.MaxRetries, a.settings.maxRetries)
	call.ToolChoice = cmp.Or(call.ToolChoice, a.settings.toolChoice)

	if len(call.StopWhen) == 0 && len(a.settings.stopWhen) > 0 {
		call.StopWhen = a.settings.stopWhen
	}
	if call.PrepareStep == nil && a.settings.prepareStep != nil {
		call.PrepareStep = a.settings.prepareStep
	}
	if call.RepairToolCall == nil && a.settings.repairToolCall != nil {
		call.RepairToolCall = a.settings.repairToolCall
	}
	if call.OnRetry == nil && a.settings.onRetry != nil {
		call.OnRetry = a.settings.onRetry
	}

	call.ProviderOptions = modelapi.MergeProviderOptions(a.settings.providerOptions, call.ProviderOptions)

	headers := map[string]string{}

	if a.settings.headers != nil {
		maps.Copy(headers, a.settings.headers)
	}
	if call.Headers != nil {
		maps.Copy(headers, call.Headers)
	}
	call.Headers = headers

	return call
}

// Generate implements Agent.
func (a *agent) Generate(ctx context.Context, opts AgentCall) (*AgentResult, error) {
	return a.run(ctx, AgentStreamCall{
		Prompt: opts.Prompt, Files: opts.Files, Messages: opts.Messages,
		MaxOutputTokens: opts.MaxOutputTokens, Temperature: opts.Temperature, TopP: opts.TopP, TopK: opts.TopK,
		PresencePenalty: opts.PresencePenalty, FrequencyPenalty: opts.FrequencyPenalty,
		ActiveTools: opts.ActiveTools, ToolChoice: opts.ToolChoice, Headers: opts.Headers, ProviderOptions: opts.ProviderOptions,
		MaxRetries: opts.MaxRetries, OnRetry: opts.OnRetry, OnAuthRefresh: opts.OnAuthRefresh,
		ModelProvider: opts.ModelProvider, StopWhen: opts.StopWhen, PrepareStep: opts.PrepareStep,
		RepairToolCall: opts.RepairToolCall, AuthorizeTool: opts.AuthorizeTool,
	}, true)
}

func isStopConditionMet(conditions []StopCondition, steps []StepResult) bool {
	if len(conditions) == 0 {
		return false
	}

	for _, condition := range conditions {
		if condition(steps) {
			return true
		}
	}
	return false
}

func hasStopTurn(results []ToolResultContent) bool {
	for _, r := range results {
		if r.StopTurn {
			return true
		}
	}
	return false
}

func toResponseMessages(content []Content) []Message {
	var assistantParts []MessagePart
	var toolParts []MessagePart

	for _, c := range content {
		switch c.GetType() {
		case ContentTypeText:
			text, ok := modelapi.AsContentType[TextContent](c)
			if !ok {
				continue
			}
			assistantParts = append(assistantParts, TextPart{
				Text: text.Text,
			})
		case ContentTypeReasoning:
			reasoning, ok := modelapi.AsContentType[ReasoningContent](c)
			if !ok {
				continue
			}
			assistantParts = append(assistantParts, ReasoningPart{
				Text: reasoning.Text, ItemID: reasoning.ProviderMetadata.ReasoningItemID,
				Encrypted: reasoning.ProviderMetadata.EncryptedReasoning,
			})
		case ContentTypeToolCall:
			toolCall, ok := modelapi.AsContentType[ToolCallContent](c)
			if !ok {
				continue
			}
			assistantParts = append(assistantParts, ToolCallPart{
				ToolCallID:       toolCall.ToolCallID,
				ToolName:         toolCall.ToolName,
				Input:            toolCall.Input,
				ProviderExecuted: toolCall.ProviderExecuted,
			})
		case ContentTypeFile:
			file, ok := modelapi.AsContentType[FileContent](c)
			if !ok {
				continue
			}
			assistantParts = append(assistantParts, FilePart{
				Data:      file.Data,
				MediaType: file.MediaType,
			})
		case ContentTypeSource:
			// Sources are metadata about references used to generate the response.
			// They don't need to be included in the conversation messages.
			continue
		case ContentTypeToolResult:
			result, ok := modelapi.AsContentType[ToolResultContent](c)
			if !ok {
				continue
			}
			resultPart := ToolResultPart{
				ToolCallID:       result.ToolCallID,
				Output:           result.Result,
				ProviderExecuted: result.ProviderExecuted,
				ClientMetadata:   result.ClientMetadata,
			}
			if result.ProviderExecuted {
				// Provider-executed tool results (e.g. web search)
				// belong in the assistant message alongside the
				// server_tool_use block that produced them.
				assistantParts = append(assistantParts, resultPart)
			} else {
				toolParts = append(toolParts, resultPart)
			}
		}
	}

	var messages []Message
	if len(assistantParts) > 0 {
		messages = append(messages, Message{
			Role:    MessageRoleAssistant,
			Content: assistantParts,
		})
	}
	if len(toolParts) > 0 {
		messages = append(messages, Message{
			Role:    MessageRoleTool,
			Content: toolParts,
		})
	}
	return messages
}

// executeSingleTool authorizes a tool call and returns its result.
func (a *agent) executeSingleTool(ctx context.Context, toolMap map[string]AgentTool, execProviderToolMap map[string]ExecutableProviderTool, toolCall ToolCallContent, authorize ToolAuthorizer) ToolResultContent {
	result := ToolResultContent{
		ToolCallID:       toolCall.ToolCallID,
		ToolName:         toolCall.ToolName,
		ProviderExecuted: false,
	}

	// Skip invalid tool calls - create error result (not critical)
	if toolCall.Invalid {
		result.Result = ToolResultOutputContentError{
			Error: toolCall.ValidationError,
		}
		return result
	}

	// Find the run function — either from a regular AgentTool or an
	// executable provider tool.
	var runTool func(ctx context.Context, call ToolCall) (ToolResponse, error)
	info := ToolInfo{Name: toolCall.ToolName}
	if tool, exists := toolMap[toolCall.ToolName]; exists {
		runTool = tool.Run
		info = tool.Info()
	} else if ept, ok := execProviderToolMap[toolCall.ToolName]; ok {
		runTool = ept.Run
	}
	if runTool == nil {
		result.Result = ToolResultOutputContentError{
			Error: errors.New("tool not found: " + toolCall.ToolName),
		}
		return result
	}

	call := ToolCall{ID: toolCall.ToolCallID, Name: toolCall.ToolName, Input: toolCall.Input}
	if authorize != nil {
		if err := authorize(ctx, info, call); err != nil {
			result.Result = ToolResultOutputContentError{Error: err}
			return result
		}
	}
	if err := ctx.Err(); err != nil {
		result.Result = ToolResultOutputContentError{Error: err}
		return result
	}
	timeout := info.Timeout
	if timeout <= 0 || timeout > 10*time.Minute {
		timeout = 2 * time.Minute
	}
	toolCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	toolResult, err := runToolSafely(toolCtx, runTool, call)
	if err != nil {
		result.Result = ToolResultOutputContentError{
			Error: err,
		}
		result.ClientMetadata = toolResult.Metadata
		result.StopTurn = toolResult.StopTurn
		return result
	}
	if len(toolResult.Data) > 16<<20 {
		toolResult = modelapi.NewTextErrorResponse("tool media exceeds its size limit")
	}
	if len(toolResult.Content) > 16<<20 {
		toolResult.Content = strings.ToValidUTF8(toolResult.Content[:16<<20], "") + "\n[output truncated]"
	}

	result.ClientMetadata = toolResult.Metadata
	result.StopTurn = toolResult.StopTurn
	if toolResult.IsError {
		result.Result = ToolResultOutputContentError{
			Error: errors.New(toolResult.Content),
		}
	} else if toolResult.Type == "image" || toolResult.Type == "media" {
		result.Result = ToolResultOutputContentMedia{
			Data:      base64.StdEncoding.EncodeToString(toolResult.Data),
			MediaType: toolResult.MediaType,
			Text:      toolResult.Content,
		}
	} else {
		result.Result = ToolResultOutputContentText{
			Text: toolResult.Content,
		}
	}
	return result
}

// runToolSafely invokes a tool's run function and converts any panic into a
// failed tool result. Tool implementations run on goroutines spawned by the
// agent's step processor where an unrecovered panic would crash the entire
// host process, so this boundary is the last line of defense. The panic
// value is captured in the returned error.
func runToolSafely(ctx context.Context, runTool func(ctx context.Context, call ToolCall) (ToolResponse, error), call ToolCall) (toolResult ToolResponse, err error) {
	defer func() {
		if r := recover(); r != nil {
			toolResult = NewTextErrorResponse(fmt.Sprintf("tool %q panicked", call.Name))
			err = nil
		}
	}()
	return runTool(ctx, call)
}

// Stream implements Agent.
func (a *agent) Stream(ctx context.Context, opts AgentStreamCall) (*AgentResult, error) {
	return a.run(ctx, opts, false)
}

func (a *agent) run(ctx context.Context, opts AgentStreamCall, generate bool) (final *AgentResult, failure error) {
	// Convert AgentStreamCall to AgentCall for preparation
	call := AgentCall{
		Prompt:           opts.Prompt,
		Files:            opts.Files,
		Messages:         opts.Messages,
		MaxOutputTokens:  opts.MaxOutputTokens,
		Temperature:      opts.Temperature,
		TopP:             opts.TopP,
		TopK:             opts.TopK,
		PresencePenalty:  opts.PresencePenalty,
		FrequencyPenalty: opts.FrequencyPenalty,
		ActiveTools:      opts.ActiveTools,
		ToolChoice:       opts.ToolChoice,
		Headers:          opts.Headers,
		ProviderOptions:  opts.ProviderOptions,
		MaxRetries:       opts.MaxRetries,
		OnRetry:          opts.OnRetry,
		OnAuthRefresh:    opts.OnAuthRefresh,
		ModelProvider:    opts.ModelProvider,
		StopWhen:         opts.StopWhen,
		PrepareStep:      opts.PrepareStep,
		RepairToolCall:   opts.RepairToolCall,
	}

	call = a.prepareCall(call)

	initialPrompt, err := a.createPrompt(a.settings.systemPrompt, call.Prompt, call.Messages, call.Files...)
	if err != nil {
		return nil, err
	}

	var responseMessages []Message
	var steps []StepResult
	var totalUsage Usage
	stepCleanup := func() {}

	// Start agent stream
	if opts.OnAgentStart != nil {
		opts.OnAgentStart()
	}

	defer func() {
		stepCleanup()
		if failure != nil && opts.OnError != nil {
			opts.OnError(failure)
		}
	}()
	for stepNumber := 0; ; stepNumber++ {
		stepCleanup()
		stepCleanup = func() {}
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if stepNumber >= a.settings.maxSteps {
			return &AgentResult{Steps: steps, Response: finalResponse(steps), TotalUsage: totalUsage, Messages: append(initialPrompt, responseMessages...)}, fmt.Errorf("agent step limit reached")
		}
		if opts.Steer != nil {
			responseMessages = append(responseMessages, opts.Steer()...)
		}
		stepInputMessages := append(slices.Clone(initialPrompt), responseMessages...)
		stepCtx := ctx
		stepModel := a.settings.model
		stepSystemPrompt := a.settings.systemPrompt
		stepActiveTools := call.ActiveTools
		stepToolChoice := ToolChoiceAuto
		if call.ToolChoice != nil {
			stepToolChoice = *call.ToolChoice
		}
		disableAllTools := false
		stepTools := a.settings.tools
		// Apply step preparation if provided
		if call.PrepareStep != nil {
			updatedCtx, prepared, err := call.PrepareStep(ctx, PrepareStepFunctionOptions{
				Model:      stepModel,
				Steps:      steps,
				StepNumber: stepNumber,
				Messages:   stepInputMessages,
			})
			if err != nil {
				return nil, err
			}

			if updatedCtx != nil {
				var cancel context.CancelFunc
				stepCtx, cancel = context.WithCancel(updatedCtx)
				stop := context.AfterFunc(ctx, cancel)
				stepCleanup = func() {
					stop()
					cancel()
				}
			}

			if prepared.History != nil {
				initialPrompt, responseMessages = slices.Clone(prepared.History), nil
				stepInputMessages = slices.Clone(initialPrompt)
				if opts.OnHistory != nil {
					if err := opts.OnHistory(initialPrompt); err != nil {
						return nil, err
					}
				}
			}
			if prepared.Messages != nil {
				stepInputMessages = prepared.Messages
			}
			if prepared.Model != nil {
				stepModel = prepared.Model
			}
			if prepared.System != nil {
				stepSystemPrompt = *prepared.System
			}
			if prepared.ToolChoice != nil {
				stepToolChoice = *prepared.ToolChoice
			}
			if prepared.ActiveTools != nil {
				stepActiveTools = prepared.ActiveTools
			}
			disableAllTools = prepared.DisableAllTools
			if prepared.Tools != nil {
				stepTools = prepared.Tools
			}
		}

		if stepSystemPrompt != a.settings.systemPrompt {
			if len(stepInputMessages) > 0 && stepInputMessages[0].Role == MessageRoleSystem {
				stepInputMessages = slices.Clone(stepInputMessages[1:])
			}
			if stepSystemPrompt != "" {
				stepInputMessages = append([]Message{NewSystemMessage(stepSystemPrompt)}, stepInputMessages...)
			}
		}
		stepTools = filterAgentTools(stepTools, stepActiveTools, disableAllTools)

		preparedTools := a.prepareTools(stepTools, a.settings.providerDefinedTools, stepActiveTools, disableAllTools)

		// Filter executable provider tools by activeTools at the
		// step level, consistent with how stepTools (AgentTools)
		// are scoped before being passed to inner functions.
		stepExecProviderTools := a.filterExecProviderTools(stepActiveTools)
		if disableAllTools || stepActiveTools != nil && len(stepActiveTools) == 0 {
			stepExecProviderTools = nil
		}

		// Start step stream
		if opts.OnStepStart != nil {
			if err := opts.OnStepStart(stepNumber); err != nil {
				return nil, err
			}
		}
		// Create streaming call
		streamCall := Call{
			Prompt:           stepInputMessages,
			MaxOutputTokens:  call.MaxOutputTokens,
			Temperature:      call.Temperature,
			TopP:             call.TopP,
			TopK:             call.TopK,
			PresencePenalty:  call.PresencePenalty,
			FrequencyPenalty: call.FrequencyPenalty,
			Tools:            preparedTools,
			ToolChoice:       &stepToolChoice,
			UserAgent:        a.settings.userAgent,
			Headers:          call.Headers,
			ProviderOptions:  call.ProviderOptions,
		}

		retryOptions := DefaultRetryOptions()
		retryOptions.MaxRetries = 0
		if call.MaxRetries != nil {
			retryOptions.MaxRetries = min(5, max(0, *call.MaxRetries))
		}
		retryOptions.OnRetry, retryOptions.OnAuthRefresh = call.OnRetry, call.OnAuthRefresh
		retry := RetryWithExponentialBackoffRespectingRetryHeaders[StreamResponse](retryOptions)
		stream, err := retry(stepCtx, func() (StreamResponse, error) {
			if err := stepCtx.Err(); err != nil {
				return nil, err
			}
			retryModel := stepModel
			if call.ModelProvider != nil {
				retryModel = call.ModelProvider()
			}
			if retryModel == nil {
				return nil, errors.New("agent model is required")
			}
			if generate {
				response, err := retryModel.Generate(stepCtx, streamCall)
				if err != nil {
					return nil, err
				}
				if response == nil {
					return nil, errors.New("model returned no response")
				}
				return responseStream(response), nil
			}
			return retryModel.Stream(stepCtx, streamCall)
		})
		if err != nil {
			return nil, err
		}
		if stream == nil {
			return nil, errors.New("model returned no stream")
		}
		result, err := a.processStepStream(stepCtx, stream, opts, stepTools, stepExecProviderTools, stepInputMessages, stepSystemPrompt)
		if err != nil {
			return nil, err
		}

		steps = append(steps, result.StepResult)
		totalUsage = addUsage(totalUsage, result.StepResult.Usage)

		// Call step finished callback
		if opts.OnStepFinish != nil {
			if err := opts.OnStepFinish(result.StepResult); err != nil {
				return nil, err
			}
		}

		// Add step messages to response messages
		responseMessages = append(responseMessages, result.StepResult.Messages...)

		// Check stop conditions
		shouldStop := isStopConditionMet(call.StopWhen, steps)
		if shouldStop {
			break
		}
		if !result.ShouldContinue {
			var next []Message
			if opts.Steer != nil {
				next = append(next, opts.Steer()...)
			}
			if opts.FollowUp != nil {
				next = append(next, opts.FollowUp()...)
			}
			if len(next) == 0 {
				break
			}
			responseMessages = append(responseMessages, next...)
		}
	}

	// Finish agent stream
	agentResult := &AgentResult{
		Steps:      steps,
		Response:   finalResponse(steps),
		TotalUsage: totalUsage,
		Messages:   append(initialPrompt, responseMessages...),
	}

	if opts.OnFinish != nil {
		opts.OnFinish(agentResult)
	}

	if opts.OnAgentFinish != nil {
		if err := opts.OnAgentFinish(agentResult); err != nil {
			return agentResult, err
		}
	}

	return agentResult, nil
}

// filterExecProviderTools returns the subset of executable provider
// tools permitted by activeTools. When activeTools is empty every
// tool is included (no filtering).
func (a *agent) filterExecProviderTools(activeTools []string) []ExecutableProviderTool {
	if len(activeTools) == 0 {
		return a.settings.executableProviderTools
	}
	filtered := make([]ExecutableProviderTool, 0, len(a.settings.executableProviderTools))
	for _, ept := range a.settings.executableProviderTools {
		if slices.Contains(activeTools, ept.GetName()) {
			filtered = append(filtered, ept)
		}
	}
	return filtered
}

func (a *agent) prepareTools(tools []AgentTool, providerDefinedTools []ProviderDefinedTool, activeTools []string, disableAllTools bool) []Tool {
	preparedTools := make([]Tool, 0, len(tools)+len(providerDefinedTools))

	// If explicitly disabling all tools, return no tools
	if disableAllTools {
		return preparedTools
	}

	for _, tool := range tools {
		// If activeTools has items, only include tools in the list
		// If activeTools is empty, include all tools
		if activeTools != nil && !slices.Contains(activeTools, tool.Info().Name) {
			continue
		}
		info := tool.Info()
		inputSchema := info.InputSchema
		if inputSchema == nil {
			inputSchema = map[string]any{"type": "object", "properties": info.Parameters, "required": info.Required}
		}
		inputSchema = cloneSchema(inputSchema)
		schema.Normalize(inputSchema)
		preparedTools = append(preparedTools, FunctionTool{
			Name:            info.Name,
			Description:     info.Description,
			InputSchema:     inputSchema,
			ProviderOptions: tool.ProviderOptions(),
		})
	}
	for _, tool := range providerDefinedTools {
		// If activeTools has items, only include tools in the list. If
		// activeTools is empty, include all tools
		if activeTools != nil && !slices.Contains(activeTools, tool.GetName()) {
			continue
		}
		preparedTools = append(preparedTools, tool)
	}
	return preparedTools
}

// validateAndRepairToolCall validates a tool call and attempts repair if validation fails.
func (a *agent) validateAndRepairToolCall(ctx context.Context, toolCall ToolCallContent, availableTools []AgentTool, execProviderTools []ExecutableProviderTool, systemPrompt string, messages []Message, repairFunc RepairToolCallFunction) ToolCallContent {
	err := a.validateToolCall(toolCall, availableTools, execProviderTools)
	if err == nil {
		return toolCall
	}
	for _, tool := range availableTools {
		info := tool.Info()
		if info.Name != toolCall.ToolName {
			continue
		}
		names := slices.Sorted(maps.Keys(info.Parameters))
		err = fmt.Errorf("%s: invalid arguments: %w\nAllowed fields: %s. Required: %s. Use the tool schema; no arguments were executed", info.Name, err, strings.Join(names, ", "), strings.Join(info.Required, ", "))
		break
	}
	if repairFunc != nil {
		repaired, repairErr := repairFunc(ctx, ToolCallRepairOptions{
			OriginalToolCall: toolCall,
			ValidationError:  err,
			AvailableTools:   availableTools,
			SystemPrompt:     systemPrompt,
			Messages:         messages,
		})
		if repairErr == nil && repaired != nil {
			candidate := toolCall
			candidate.ToolName, candidate.Input = repaired.ToolName, repaired.Input
			if validationErr := a.validateToolCall(candidate, availableTools, execProviderTools); validationErr == nil {
				return candidate
			}
		}
	}
	toolCall.Invalid = true
	toolCall.ValidationError = err
	return toolCall
}

// validateToolCall validates a tool call against available tools and their schemas.
// Both availableTools and execProviderTools must already be filtered by the
// caller (e.g. via activeTools); this function trusts that the slices
// represent exactly the tools permitted for the current step.
func (a *agent) validateToolCall(toolCall ToolCallContent, availableTools []AgentTool, execProviderTools []ExecutableProviderTool) error {
	var input map[string]json.RawMessage
	if err := json.Unmarshal([]byte(toolCall.Input), &input); err != nil {
		return fmt.Errorf("tool arguments must be a JSON object: %w", err)
	}
	if input == nil {
		return errors.New("tool arguments must be a JSON object")
	}

	var tool AgentTool
	for _, t := range availableTools {
		if t.Info().Name == toolCall.ToolName {
			tool = t
			break
		}
	}

	if tool == nil {
		// Check if this is an executable provider tool. Provider-
		// defined tools have their schema enforced server-side.
		for _, ept := range execProviderTools {
			if ept.GetName() == toolCall.ToolName {
				return nil
			}
		}
		names := make([]string, 0, len(availableTools)+len(execProviderTools))
		for _, t := range availableTools {
			names = append(names, t.Info().Name)
		}
		for _, ept := range execProviderTools {
			names = append(names, ept.GetName())
		}
		return fmt.Errorf("tool not found: %s. Available tools: %s", toolCall.ToolName, strings.Join(names, ", "))
	}

	// Basic schema validation (check required fields)
	// TODO: more robust schema validation using JSON Schema or similar
	toolInfo := tool.Info()
	for _, required := range toolInfo.Required {
		if _, exists := input[required]; !exists {
			return fmt.Errorf("missing required parameter: %s", required)
		}
	}
	return nil
}

func (a *agent) createPrompt(system, prompt string, messages []Message, files ...FilePart) (Prompt, error) {
	// Validation: empty prompt is only allowed when there are messages,
	// no files to attach, and the last message is a user or tool message.
	if prompt == "" {
		if len(messages) < 1 {
			return nil, &Error{
				Title:   "invalid argument",
				Message: "prompt can't be empty when there are no messages",
			}
		}

		if len(files) > 0 {
			return nil, &Error{
				Title:   "invalid argument",
				Message: "prompt can't be empty when there are files",
			}
		}

		switch messages[len(messages)-1].Role {
		case MessageRoleUser, MessageRoleTool:
		default:
			return nil, &Error{
				Title:   "invalid argument",
				Message: "prompt can't be empty when the last message is not a user or tool message",
			}
		}
	}

	var preparedPrompt Prompt

	if system != "" {
		preparedPrompt = append(preparedPrompt, NewSystemMessage(system))
	}
	preparedPrompt = append(preparedPrompt, messages...)
	if prompt != "" {
		preparedPrompt = append(preparedPrompt, NewUserMessage(prompt, files...))
	}
	return preparedPrompt, nil
}

// WithSystemPrompt sets the system prompt for the agent.
func WithSystemPrompt(prompt string) AgentOption {
	return func(s *agentSettings) {
		s.systemPrompt = prompt
	}
}

// WithMaxOutputTokens sets the maximum output tokens for the agent.
func WithMaxOutputTokens(tokens int64) AgentOption {
	return func(s *agentSettings) {
		s.maxOutputTokens = &tokens
	}
}

// WithTemperature sets the temperature for the agent.
func WithTemperature(temp float64) AgentOption {
	return func(s *agentSettings) {
		s.temperature = &temp
	}
}

// WithTopP sets the top-p value for the agent.
func WithTopP(topP float64) AgentOption {
	return func(s *agentSettings) {
		s.topP = &topP
	}
}

// WithTopK sets the top-k value for the agent.
func WithTopK(topK int64) AgentOption {
	return func(s *agentSettings) {
		s.topK = &topK
	}
}

// WithPresencePenalty sets the presence penalty for the agent.
func WithPresencePenalty(penalty float64) AgentOption {
	return func(s *agentSettings) {
		s.presencePenalty = &penalty
	}
}

// WithFrequencyPenalty sets the frequency penalty for the agent.
func WithFrequencyPenalty(penalty float64) AgentOption {
	return func(s *agentSettings) {
		s.frequencyPenalty = &penalty
	}
}

// WithTools sets the tools for the agent.
func WithTools(tools ...AgentTool) AgentOption {
	return func(s *agentSettings) {
		s.tools = append(s.tools, tools...)
	}
}

// WithProviderDefinedTools registers provider-defined tools with the
// agent. Provider-executed tools (e.g. web search) are passed through
// to the API. Client-executed tools (ExecutableProviderTool) are also
// registered for local execution.
func WithProviderDefinedTools(tools ...ProviderTool) AgentOption {
	return func(s *agentSettings) {
		for _, t := range tools {
			// Every provider tool goes into providerDefinedTools
			// for wire formatting.
			s.providerDefinedTools = append(
				s.providerDefinedTools, modelapi.ProviderDefinition(t),
			)
			// Executable ones also register for local execution.
			if exec, ok := t.(ExecutableProviderTool); ok {
				s.executableProviderTools = append(
					s.executableProviderTools, exec,
				)
			}
		}
	}
}

// WithToolChoice sets the default tool choice for the agent. It is overridden
// by the ToolChoice on a specific call, and by PrepareStep at the step level.
func WithToolChoice(choice ToolChoice) AgentOption {
	return func(s *agentSettings) {
		s.toolChoice = &choice
	}
}

// WithStopConditions sets the stop conditions for the agent.
func WithStopConditions(conditions ...StopCondition) AgentOption {
	return func(s *agentSettings) {
		s.stopWhen = append(s.stopWhen, conditions...)
	}
}

// WithPrepareStep sets the prepare step function for the agent.
func WithPrepareStep(fn PrepareStepFunction) AgentOption {
	return func(s *agentSettings) {
		s.prepareStep = fn
	}
}

// WithRepairToolCall sets the repair tool call function for the agent.
func WithRepairToolCall(fn RepairToolCallFunction) AgentOption {
	return func(s *agentSettings) {
		s.repairToolCall = fn
	}
}

// WithMaxRetries sets the maximum number of retries for the agent.
func WithMaxRetries(maxRetries int) AgentOption {
	return func(s *agentSettings) {
		s.maxRetries = &maxRetries
	}
}

// WithMaxSteps sets the maximum number of model requests in a run.
func WithMaxSteps(steps int) AgentOption {
	return func(settings *agentSettings) {
		if steps > 0 {
			settings.maxSteps = steps
		}
	}
}

// WithOnRetry sets the retry callback for the agent.
func WithOnRetry(callback OnRetryCallback) AgentOption {
	return func(s *agentSettings) {
		s.onRetry = callback
	}
}

// processStepStream processes a single step's stream and returns the step result.
func (a *agent) processStepStream(ctx context.Context, stream StreamResponse, opts AgentStreamCall, stepTools []AgentTool, execProviderTools []ExecutableProviderTool, stepInputMessages []Message, system string) (stepExecutionResult, error) {
	var stepContent []Content
	var stepToolCalls []ToolCallContent
	var stepUsage Usage
	stepFinishReason := FinishReasonUnknown
	var stepWarnings []CallWarning
	var stepProviderMetadata ProviderMetadata

	activeToolCalls := make(map[string]*ToolCallContent)
	activeTextContent := make(map[string]string)
	type reasoningContent struct {
		content string
		options ProviderMetadata
	}
	activeReasoningContent := make(map[string]reasoningContent)

	// Set up concurrent tool execution
	type toolExecutionRequest struct {
		toolCall ToolCallContent
		parallel bool
	}
	var pendingDispatches []toolExecutionRequest
	// Raw tool calls as emitted by the provider, before validation/repair,
	// each with the position in stepContent at which it was emitted. They
	// are processed only after the stream ends and the finish reason is
	// known (see below); the position preserves stream order in the
	// recorded step content.
	type rawToolCall struct {
		contentIndex int
		toolCall     ToolCallContent
	}
	var rawToolCalls []rawToolCall

	// Create a map for quick tool lookup
	toolMap := make(map[string]AgentTool)
	for _, tool := range stepTools {
		toolMap[tool.Info().Name] = tool
	}

	execProviderToolMap := make(map[string]ExecutableProviderTool, len(execProviderTools))
	for _, ept := range execProviderTools {
		execProviderToolMap[ept.GetName()] = ept
	}

	// Process stream parts
	seenFinish := false
	parts, bytes := 0, 0
	for part := range stream {
		if err := ctx.Err(); err != nil {
			return stepExecutionResult{}, err
		}
		parts++
		bytes += len(part.Delta) + len(part.ToolCallInput)
		if parts > 1<<18 || bytes > 32<<20 {
			return stepExecutionResult{}, errors.New("model output limit reached")
		}
		// Forward all parts to chunk callback
		if opts.OnChunk != nil {
			err := opts.OnChunk(part)
			if err != nil {
				return stepExecutionResult{}, err
			}
		}

		switch part.Type {
		case StreamPartTypeWarnings:
			stepWarnings = append(stepWarnings, part.Warnings...)
			if opts.OnWarnings != nil {
				err := opts.OnWarnings(part.Warnings)
				if err != nil {
					return stepExecutionResult{}, err
				}
			}

		case StreamPartTypeTextStart:
			activeTextContent[part.ID] = ""
			if opts.OnTextStart != nil {
				err := opts.OnTextStart(part.ID)
				if err != nil {
					return stepExecutionResult{}, err
				}
			}

		case StreamPartTypeTextDelta:
			if _, exists := activeTextContent[part.ID]; exists {
				activeTextContent[part.ID] += part.Delta
			}
			if opts.OnTextDelta != nil {
				err := opts.OnTextDelta(part.ID, part.Delta)
				if err != nil {
					return stepExecutionResult{}, err
				}
			}

		case StreamPartTypeTextEnd:
			if text, exists := activeTextContent[part.ID]; exists {
				stepContent = append(stepContent, TextContent{
					Text:             text,
					ProviderMetadata: part.ProviderMetadata,
				})
				delete(activeTextContent, part.ID)
			}
			if opts.OnTextEnd != nil {
				err := opts.OnTextEnd(part.ID)
				if err != nil {
					return stepExecutionResult{}, err
				}
			}

		case StreamPartTypeReasoningStart:
			activeReasoningContent[part.ID] = reasoningContent{content: part.Delta, options: part.ProviderMetadata}
			if opts.OnReasoningStart != nil {
				content := ReasoningContent{
					Text:             part.Delta,
					ProviderMetadata: part.ProviderMetadata,
				}
				err := opts.OnReasoningStart(part.ID, content)
				if err != nil {
					return stepExecutionResult{}, err
				}
			}

		case StreamPartTypeReasoningDelta:
			if active, exists := activeReasoningContent[part.ID]; exists {
				active.content += part.Delta
				if !part.ProviderMetadata.IsZero() {
					active.options = part.ProviderMetadata
				}
				activeReasoningContent[part.ID] = active
			}
			if opts.OnReasoningDelta != nil {
				err := opts.OnReasoningDelta(part.ID, part.Delta)
				if err != nil {
					return stepExecutionResult{}, err
				}
			}

		case StreamPartTypeReasoningEnd:
			if active, exists := activeReasoningContent[part.ID]; exists {
				if !part.ProviderMetadata.IsZero() {
					active.options = part.ProviderMetadata
				}
				content := ReasoningContent{
					Text:             active.content,
					ProviderMetadata: active.options,
				}
				stepContent = append(stepContent, content)
				if opts.OnReasoningEnd != nil {
					err := opts.OnReasoningEnd(part.ID, content)
					if err != nil {
						return stepExecutionResult{}, err
					}
				}
				delete(activeReasoningContent, part.ID)
			}

		case StreamPartTypeToolInputStart:
			activeToolCalls[part.ID] = &ToolCallContent{
				ToolCallID:       part.ID,
				ToolName:         part.ToolCallName,
				Input:            "",
				ProviderExecuted: part.ProviderExecuted,
			}
			if opts.OnToolInputStart != nil {
				err := opts.OnToolInputStart(part.ID, part.ToolCallName)
				if err != nil {
					return stepExecutionResult{}, err
				}
			}

		case StreamPartTypeToolInputDelta:
			if toolCall, exists := activeToolCalls[part.ID]; exists {
				toolCall.Input += part.Delta
			}
			if opts.OnToolInputDelta != nil {
				err := opts.OnToolInputDelta(part.ID, part.Delta)
				if err != nil {
					return stepExecutionResult{}, err
				}
			}

		case StreamPartTypeToolInputEnd:
			if opts.OnToolInputEnd != nil {
				err := opts.OnToolInputEnd(part.ID)
				if err != nil {
					return stepExecutionResult{}, err
				}
			}

		case StreamPartTypeToolCall:
			toolCall := ToolCallContent{
				ToolCallID:       part.ID,
				ToolName:         part.ToolCallName,
				Input:            part.ToolCallInput,
				ProviderExecuted: part.ProviderExecuted,
				ProviderMetadata: part.ProviderMetadata,
			}

			// Buffer the call. Validation, repair, and the OnToolCall
			// callback all wait until the stream has ended and the finish
			// reason is known: a provider that emits a ToolCall before a
			// length/error finish must not have its possibly-truncated
			// arguments repaired (repair may be an extra model call) or
			// exposed to consumers — provider-executed calls included; the
			// provider may run them, but consumers hear about them only for
			// completed turns (CHARM-2020).
			rawToolCalls = append(rawToolCalls, rawToolCall{contentIndex: len(stepContent), toolCall: toolCall})
			delete(activeToolCalls, part.ID)

		case StreamPartTypeToolResult:
			// Provider-executed tool results (e.g. web search)
			// are emitted by the provider and added directly
			// to the step content for multi-turn round-tripping.
			if part.ProviderExecuted {
				resultContent := ToolResultContent{
					ToolCallID:       part.ID,
					ToolName:         part.ToolCallName,
					ProviderExecuted: true,
					ProviderMetadata: part.ProviderMetadata,
				}
				stepContent = append(stepContent, resultContent)
				if opts.OnToolResult != nil {
					err := opts.OnToolResult(resultContent)
					if err != nil {
						return stepExecutionResult{}, err
					}
				}
			}

		case StreamPartTypeSource:
			sourceContent := SourceContent{
				SourceType:       part.SourceType,
				ID:               part.ID,
				URL:              part.URL,
				Title:            part.Title,
				ProviderMetadata: part.ProviderMetadata,
			}
			stepContent = append(stepContent, sourceContent)
			if opts.OnSource != nil {
				err := opts.OnSource(sourceContent)
				if err != nil {
					return stepExecutionResult{}, err
				}
			}

		case modelapi.StreamPartTypeContent:
			if part.Content != nil {
				stepContent = append(stepContent, part.Content)
			}
		case StreamPartTypeFinish:
			seenFinish = true
			stepUsage = part.Usage
			stepFinishReason = part.FinishReason
			stepProviderMetadata = part.ProviderMetadata
			if opts.OnStreamFinish != nil {
				err := opts.OnStreamFinish(part.Usage, part.FinishReason, part.ProviderMetadata)
				if err != nil {
					return stepExecutionResult{}, err
				}
			}

		case StreamPartTypeError:
			return stepExecutionResult{}, part.Error
		}
	}

	if !seenFinish {
		return stepExecutionResult{}, modelapi.NewIncompleteStreamError()
	}
	if len(rawToolCalls) > 128 {
		return stepExecutionResult{}, errors.New("too many tool calls")
	}
	// Process buffered tool calls now that the finish reason is known.
	// Abnormal finishes — length, content filter, provider error, unknown —
	// can accompany arguments that were cut short: record the raw call in
	// the step content, but never repair (repair may be an extra model
	// call), never fire OnToolCall, and never dispatch (CHARM-2020).
	// Splice each processed call into the position it was emitted at in the
	// stream, so step content preserves the provider's ordering. Calls
	// emitted back-to-back share the same recorded index; track insertions
	// so they land in emission order, not reversed.
	insertContent := func(at int, c Content) {
		stepContent = append(stepContent, nil)
		copy(stepContent[at+1:], stepContent[at:])
		stepContent[at] = c
	}
	inserted := 0
	for _, raw := range rawToolCalls {
		at := raw.contentIndex + inserted
		toolCall := raw.toolCall
		// Provider-executed tool calls (e.g. web search) were already run by
		// the provider. Keep their data for continuation.
		if toolCall.ProviderExecuted && (stepFinishReason == FinishReasonStop || stepFinishReason == FinishReasonToolCalls) {
			insertContent(at, toolCall)
			inserted++
			continue
		}
		if stepFinishReason != FinishReasonToolCalls {
			continue
		}
		// Validate and potentially repair the tool call
		validatedToolCall := a.validateAndRepairToolCall(ctx, toolCall, stepTools, execProviderTools, system, stepInputMessages, opts.RepairToolCall)
		stepToolCalls = append(stepToolCalls, validatedToolCall)
		insertContent(at, validatedToolCall)
		inserted++

		// Determine if tool can run in parallel
		isParallel := false
		if tool, exists := toolMap[validatedToolCall.ToolName]; exists {
			isParallel = tool.Info().Parallel && tool.Info().Effect == modelapi.EffectRead
		}

		// Buffer dispatch until stream is fully consumed so that all
		// OnToolCall callbacks complete before any tool result is written.
		pendingDispatches = append(pendingDispatches, toolExecutionRequest{toolCall: validatedToolCall, parallel: isParallel})
	}

	var toolResults []ToolResultContent
	seen := map[string]bool{}
	for _, req := range pendingDispatches {
		key := req.toolCall.ToolCallID
		if key == "" || seen[key] {
			return stepExecutionResult{}, errors.New("missing or duplicate tool call ID")
		}
		seen[key] = true
	}
	if opts.OnModelMessages != nil {
		if err := opts.OnModelMessages(toResponseMessages(stepContent)); err != nil {
			return stepExecutionResult{}, err
		}
	}
	if opts.OnToolCall != nil {
		for _, content := range stepContent {
			if item, ok := modelapi.AsContentType[ToolCallContent](content); ok {
				if err := opts.OnToolCall(item); err != nil {
					return stepExecutionResult{}, err
				}
			}
		}
	}
	if stepFinishReason == FinishReasonToolCalls {
		for i := 0; i < len(pendingDispatches); {
			if err := ctx.Err(); err != nil {
				return stepExecutionResult{}, err
			}
			end := i + 1
			if pendingDispatches[i].parallel {
				for end < len(pendingDispatches) && end-i < 5 && pendingDispatches[end].parallel {
					end++
				}
			}
			results := make([]ToolResultContent, end-i)
			var group sync.WaitGroup
			for index, req := range pendingDispatches[i:end] {
				run := func() {
					results[index] = a.executeSingleTool(ctx, toolMap, execProviderToolMap, req.toolCall, opts.AuthorizeTool)
				}
				if end-i == 1 {
					run()
				} else {
					group.Go(run)
				}
			}
			group.Wait()
			for _, result := range results {
				if opts.OnToolResult != nil {
					if err := opts.OnToolResult(result); err != nil {
						return stepExecutionResult{}, err
					}
				}
				stepContent = append(stepContent, result)
				toolResults = append(toolResults, result)
			}
			i = end
		}
	}
	if err := ctx.Err(); err != nil {
		return stepExecutionResult{}, err
	}

	stepResult := StepResult{
		Response: Response{
			Content:          stepContent,
			FinishReason:     stepFinishReason,
			Usage:            stepUsage,
			Warnings:         stepWarnings,
			ProviderMetadata: stepProviderMetadata,
		},
		Messages: toResponseMessages(stepContent),
	}

	// Determine if we should continue (has tool calls and not stopped)
	shouldContinue := len(stepToolCalls) > 0 && stepFinishReason == FinishReasonToolCalls && !hasStopTurn(toolResults)

	return stepExecutionResult{
		StepResult:     stepResult,
		ShouldContinue: shouldContinue,
	}, nil
}

func addUsage(a, b Usage) Usage {
	return Usage{
		InputTokens:     a.InputTokens + b.InputTokens,
		OutputTokens:    a.OutputTokens + b.OutputTokens,
		TotalTokens:     a.TotalTokens + b.TotalTokens,
		ReasoningTokens: a.ReasoningTokens + b.ReasoningTokens,
		CacheReadTokens: a.CacheReadTokens + b.CacheReadTokens,
	}
}

// WithHeaders sets the headers for the agent.
func WithHeaders(headers map[string]string) AgentOption {
	return func(s *agentSettings) {
		s.headers = headers
	}
}

// WithUserAgent sets the User-Agent header for the agent. This overrides any
// provider-level User-Agent setting.
func WithUserAgent(ua string) AgentOption {
	return func(s *agentSettings) {
		s.userAgent = ua
	}
}

// WithProviderOptions sets the provider options for the agent.
func WithProviderOptions(providerOptions ProviderOptions) AgentOption {
	return func(s *agentSettings) {
		s.providerOptions = providerOptions
	}
}
