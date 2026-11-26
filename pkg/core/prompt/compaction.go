package prompt

import (
	"context"
	"errors"
	"slices"

	"clara-code/pkg/core"
	modelapi "clara-code/pkg/model"
)

// Compactor summarizes the completed model-visible history and installs a handoff.
// BudgetTokens excludes output reservation. Context contains request-only messages.
type Compactor struct {
	Model        modelapi.LanguageModel
	BudgetBytes  int
	BudgetTokens int64
	// TriggerTokens optionally starts automatic compaction before BudgetTokens.
	// Zero uses BudgetTokens as the trigger. The final replacement must fit BudgetTokens.
	TriggerTokens int64
	Context       []modelapi.Message
	// Project applies the model's modality filtering to both budgeting and the summary request.
	Project func([]modelapi.Message) []modelapi.Message
	// Assemble measures the exact request layout, including anchored context.
	Assemble func([]modelapi.Message) ([]modelapi.Message, error)
	// AssembleCompacted assembles a replacement with remapped host context. Each
	// original message boundary (including the end) maps to a replacement index.
	// Budgeting may inspect a provisional replacement before the final one.
	AssembleCompacted func(history []modelapi.Message, anchors []int) ([]modelapi.Message, error)
	// ActiveContextTokens supplies the current window usage, including a known zero.
	ActiveContextTokens *int64
	// PendingFrom marks the first item of an unsent submission. It stays after the handoff.
	PendingFrom *int
	// Transcript restores historical host context for the supplied prefix.
	Transcript   func([]modelapi.Message) ([]modelapi.Message, error)
	Instructions string
	Force        bool
	// UsageTokens and UsageEstimate calibrate estimates for callers without ActiveContextTokens.
	UsageTokens   int64
	UsageEstimate int64
}

func (c Compactor) Fit(ctx context.Context, model modelapi.LanguageModel, system string, history []modelapi.Message, items []core.AgentTool) ([]modelapi.Message, error) {
	var definitions []modelapi.ToolInfo
	measure := func(messages []modelapi.Message, anchors []int) (Estimate, error) {
		if definitions == nil {
			definitions = make([]modelapi.ToolInfo, 0, len(items))
			for _, tool := range items {
				definitions = append(definitions, tool.Info())
			}
		}
		var err error
		if anchors != nil && c.AssembleCompacted != nil {
			messages, err = c.AssembleCompacted(messages, anchors)
		} else if c.Assemble != nil {
			messages, err = c.Assemble(messages)
		}
		if err != nil {
			return Estimate{}, err
		}
		if c.Project != nil {
			messages = c.Project(messages)
		}
		return Measure(system, append(slices.Clone(c.Context), messages...), definitions)
	}
	end := len(history)
	if c.PendingFrom != nil {
		end = *c.PendingFrom
		if end < 0 || end > len(history) {
			return nil, errors.New("compaction pending message index is outside history")
		}
	}
	if !c.Force {
		trigger := c.TriggerTokens
		if trigger <= 0 || c.BudgetTokens > 0 && trigger > c.BudgetTokens {
			trigger = c.BudgetTokens
		}
		var before Estimate
		if c.BudgetBytes > 0 || trigger > 0 && c.ActiveContextTokens == nil {
			var err error
			before, err = measure(history[:end], nil)
			if err != nil {
				return nil, err
			}
		}
		tokens := before.Tokens
		if c.ActiveContextTokens != nil {
			tokens = *c.ActiveContextTokens
		} else if c.UsageEstimate > 0 && c.UsageTokens > 0 {
			tokens = max(0, c.UsageTokens+before.Tokens-c.UsageEstimate)
		}
		reachedBytes := c.BudgetBytes > 0 && before.Bytes > c.BudgetBytes
		reachedTokens := trigger > 0 && tokens >= trigger
		if !reachedBytes && !reachedTokens {
			return nil, nil
		}
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if end == 0 || !completeTools(history[:end]) {
		if !c.Force {
			return nil, nil
		}
		return nil, errors.New("prompt cannot be compacted safely: empty history or unmatched tool calls")
	}
	if c.Model != nil {
		model = c.Model
	}
	if model == nil {
		return nil, errors.New("compaction model is required")
	}
	transcript := history[:end]
	if c.Transcript != nil {
		var err error
		transcript, err = c.Transcript(transcript)
		if err != nil {
			return nil, err
		}
	}
	summary, err := c.summarize(ctx, model, system, transcript)
	if err != nil {
		return nil, err
	}
	active := end
	if c.PendingFrom == nil {
		active = activeSubmission(history[:end])
	}
	// Generic hosts may set a small hard budget. Keep recent user text alongside
	// the summary, active attachments and pending input.
	remaining := retainedUserTokens
	for {
		out, anchors, historicalUsed := compactedHistory(history, active, end, remaining, summary)
		after, err := measure(out, anchors)
		if err != nil {
			return nil, err
		}
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		overBytes := c.BudgetBytes > 0 && after.Bytes > c.BudgetBytes
		overTokens := c.BudgetTokens > 0 && after.Tokens > c.BudgetTokens
		if !overBytes && !overTokens {
			return out, nil
		}
		if historicalUsed == 0 {
			return nil, errors.New("prompt exceeds budget after compaction")
		}
		// Only historical user text is optional under a hard budget.
		remaining = historicalUsed / 2
	}
}

// Follow-ups can extend a live run. Keep its original submission and later steering.
func activeSubmission(history []modelapi.Message) int {
	at := ActiveTurn(history)
	for at > 0 && history[at].Metadata != nil && history[at].Metadata.Kind == "followup" {
		at = ActiveTurn(history[:at])
	}
	return at
}

func handoffMessage(message modelapi.Message) bool {
	if message.Role != modelapi.MessageRoleUser {
		return false
	}
	if message.Metadata != nil {
		return message.Metadata.Kind == "handoff"
	}
	return isLegacyHandoff(message)
}

func completeTools(messages []modelapi.Message) bool {
	pending := make(map[string]bool)
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
