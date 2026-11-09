package prompt

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strings"

	"clara-code/pkg/core"
	modelapi "clara-code/pkg/model"
)

// Compactor replaces complete old turns, leaving the active turn and media intact.
// BudgetTokens excludes output reservation. Context contains request-only messages.
type Compactor struct {
	Model        modelapi.LanguageModel
	BudgetBytes  int
	BudgetTokens int64
	// TriggerTokens optionally starts automatic compaction before BudgetTokens.
	// Zero retains the BudgetTokens trigger. The final replacement must fit BudgetTokens.
	TriggerTokens int64
	Context       []modelapi.Message
	// Project applies the model's request-only modality filtering for budgeting.
	Project func([]modelapi.Message) []modelapi.Message
	// Assemble measures the exact request layout, including anchored context.
	Assemble func([]modelapi.Message) ([]modelapi.Message, error)
	// Transcript restores historical host context for summarization as task data.
	Transcript    func([]modelapi.Message) ([]modelapi.Message, error)
	Instructions  string
	Force         bool
	UsageTokens   int64
	UsageEstimate int64
}

func (c Compactor) Fit(ctx context.Context, model modelapi.LanguageModel, system string, history []modelapi.Message, items []core.AgentTool) ([]modelapi.Message, error) {
	definitions := make([]modelapi.ToolInfo, 0, len(items))
	for _, tool := range items {
		definitions = append(definitions, tool.Info())
	}
	measure := func(messages []modelapi.Message) (Estimate, error) {
		if c.Assemble != nil {
			var err error
			messages, err = c.Assemble(messages)
			if err != nil {
				return Estimate{}, err
			}
		}
		if c.Project != nil {
			messages = c.Project(messages)
		}
		return Measure(system, append(slices.Clone(c.Context), messages...), definitions)
	}
	before, err := measure(history)
	if err != nil {
		return nil, err
	}
	tokens := before.Tokens
	if c.UsageEstimate > 0 && c.UsageTokens > 0 {
		tokens = max(tokens, c.UsageTokens+before.Tokens-c.UsageEstimate)
	}
	over := func(estimate Estimate) bool {
		return c.BudgetBytes > 0 && estimate.Bytes > c.BudgetBytes || c.BudgetTokens > 0 && estimate.Tokens > c.BudgetTokens
	}
	trigger := c.TriggerTokens
	if trigger <= 0 || c.BudgetTokens > 0 && trigger > c.BudgetTokens {
		trigger = c.BudgetTokens
	}
	triggered := c.BudgetBytes > 0 && before.Bytes > c.BudgetBytes || trigger > 0 && tokens > trigger
	if !c.Force && !triggered {
		return nil, nil
	}
	start := 0
	for start < len(history) && (history[start].Role == modelapi.MessageRoleSystem || history[start].Role == modelapi.MessageRoleDeveloper) {
		start++
	}
	cut := ActiveTurn(history)
	if cut <= start || !completeTools(history[start:cut]) {
		return nil, errors.New("prompt cannot be compacted safely: no complete old turn or unmatched tool calls")
	}
	prefix := history[start:cut]
	if c.Transcript != nil {
		prefix, err = c.Transcript(history[:cut])
		if err != nil {
			return nil, err
		}
	}
	transcript, _ := summaryMessages(prefix)
	for i := range transcript {
		transcript[i].Metadata = nil
	}
	body, err := json.Marshal(transcript)
	if err != nil {
		return nil, err
	}
	if c.Model != nil {
		model = c.Model
	}
	if model == nil {
		return nil, errors.New("compaction model is required")
	}
	instructions := c.Instructions
	if instructions == "" {
		instructions = Default("compact")
	}
	limit := int64(2048)
	response, err := model.Generate(ctx, modelapi.Call{Prompt: []modelapi.Message{
		modelapi.NewSystemMessage(instructions), modelapi.NewUserMessage(string(body)),
	}, MaxOutputTokens: &limit})
	if err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if response == nil || strings.TrimSpace(response.Content.Text()) == "" || response.FinishReason != modelapi.FinishReasonStop {
		return nil, errors.New("compaction did not return a complete summary")
	}
	handoff := modelapi.NewUserMessage("Prior conversation handoff:\n" + response.Content.Text())
	handoff.Metadata = &modelapi.MessageMetadata{Kind: "handoff"}
	out := append(slices.Clone(history[:start]), handoff)
	out = append(out, history[cut:]...)
	after, err := measure(out)
	if err != nil {
		return nil, err
	}
	if c.UsageEstimate > 0 && c.UsageTokens > c.UsageEstimate {
		after.Tokens = max(after.Tokens, after.Tokens*c.UsageTokens/c.UsageEstimate)
	}
	if over(after) {
		return nil, errors.New("prompt exceeds budget after compaction")
	}
	return out, nil
}

func completeTools(messages []modelapi.Message) bool {
	pending := map[string]bool{}
	for _, message := range messages {
		for _, part := range message.Content {
			if call, ok := modelapi.AsMessagePart[modelapi.ToolCallPart](part); ok && !call.ProviderExecuted {
				if call.ToolCallID == "" || pending[call.ToolCallID] {
					return false
				}
				pending[call.ToolCallID] = true
			}
			if result, ok := modelapi.AsMessagePart[modelapi.ToolResultPart](part); ok && !result.ProviderExecuted {
				if !pending[result.ToolCallID] {
					return false
				}
				delete(pending, result.ToolCallID)
			}
		}
	}
	return len(pending) == 0
}
