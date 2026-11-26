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

// Compactor summarizes old turns or completed tool exchanges within a long active turn.
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
	// AssembleCompacted assembles a replacement with remapped host context. Each
	// original message boundary (including the end) maps to a replacement index.
	// Budgeting may inspect a provisional replacement before the final one.
	AssembleCompacted func(history []modelapi.Message, anchors []int) ([]modelapi.Message, error)
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
	measure := func(messages []modelapi.Message, anchors []int) (Estimate, error) {
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
	before, err := measure(history, nil)
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
	calibrated := func(estimate Estimate) Estimate {
		if c.UsageEstimate > 0 && c.UsageTokens > c.UsageEstimate {
			estimate.Tokens = max(estimate.Tokens, estimate.Tokens*c.UsageTokens/c.UsageEstimate)
		}
		return estimate
	}
	start := 0
	for start < len(history) && (history[start].Role == modelapi.MessageRoleSystem || history[start].Role == modelapi.MessageRoleDeveloper) {
		start++
	}
	active := activeSubmission(history)
	if !completeTools(history[start:max(start, active)]) {
		return nil, errors.New("prompt cannot be compacted safely: unmatched historical tool calls")
	}
	cut := active
	old := cut > start && slices.ContainsFunc(history[start:cut], func(m modelapi.Message) bool { return !handoffMessage(m) })
	recent := completedExchangeBoundary(history, max(start, active))
	if recent > active {
		useActive := !old
		if old {
			candidate, anchors := compactedHistory(history, start, active, cut, "")
			remaining, err := measure(candidate, anchors)
			if err != nil {
				return nil, err
			}
			remaining = calibrated(remaining)
			useActive = over(remaining) || trigger > 0 && remaining.Tokens > trigger
		}
		if useActive {
			cut = recent
		}
	}
	if cut <= start || !old && cut == active && !c.Force {
		if !c.Force && !over(Estimate{Bytes: before.Bytes, Tokens: tokens}) {
			return nil, nil
		}
		return nil, errors.New("prompt cannot be compacted safely: no completed history to summarize")
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
		modelapi.NewDeveloperMessage(instructions), modelapi.NewUserMessage(string(body)),
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
	out, anchors := compactedHistory(history, start, active, cut, response.Content.Text())
	after, err := measure(out, anchors)
	if err != nil {
		return nil, err
	}
	if over(calibrated(after)) {
		return nil, errors.New("prompt exceeds budget after compaction")
	}
	return out, nil
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

// completedExchangeBoundary keeps the most recent complete tool exchange and
// everything following it, including any outstanding parallel calls.
func completedExchangeBoundary(history []modelapi.Message, active int) int {
	pending := make(map[string]bool)
	start, complete, recent := 0, 0, 0
	for i := active + 1; i < len(history); i++ {
		if len(pending) == 0 {
			start = i
		}
		closed, valid := advanceToolCalls(history[i], pending)
		if !valid {
			return 0
		}
		if closed && len(pending) == 0 {
			complete++
			recent = start
		}
	}
	if complete < 2 {
		return 0
	}
	return recent
}

// The handoff replaces the first removed message. Retained user instructions
// and the verbatim suffix stay in order, along with their attachment data.
func compactedHistory(history []modelapi.Message, start, active, cut int, summary string) ([]modelapi.Message, []int) {
	handoff := modelapi.NewUserMessage("Prior conversation handoff:\n" + summary)
	handoff.Metadata = &modelapi.MessageMetadata{Kind: "handoff"}
	out := make([]modelapi.Message, 0, len(history)-cut+start+1)
	anchors := make([]int, len(history)+1)
	inserted, removing, boundary := false, false, 0
	for i, message := range history {
		protected := message.Role == modelapi.MessageRoleSystem || message.Role == modelapi.MessageRoleDeveloper ||
			message.Role == modelapi.MessageRoleUser && !handoffMessage(message)
		if i < start || i >= cut || i >= active && protected {
			anchors[i] = len(out)
			out = append(out, message)
			removing = false
			continue
		}
		if !removing {
			boundary = len(out)
			removing = true
		}
		anchors[i] = boundary
		if !inserted {
			out = append(out, handoff)
			inserted = true
		}
	}
	anchors[len(history)] = len(out)
	return out, anchors
}

func completeTools(messages []modelapi.Message) bool {
	pending := make(map[string]bool)
	for _, message := range messages {
		if _, valid := advanceToolCalls(message, pending); !valid {
			return false
		}
	}
	return len(pending) == 0
}

func advanceToolCalls(message modelapi.Message, pending map[string]bool) (closed, valid bool) {
	for _, part := range message.Content {
		if call, ok := modelapi.AsMessagePart[modelapi.ToolCallPart](part); ok && !call.ProviderExecuted {
			if call.ToolCallID == "" || pending[call.ToolCallID] {
				return false, false
			}
			pending[call.ToolCallID] = true
		}
		if result, ok := modelapi.AsMessagePart[modelapi.ToolResultPart](part); ok && !result.ProviderExecuted {
			if !pending[result.ToolCallID] {
				return false, false
			}
			delete(pending, result.ToolCallID)
			closed = true
		}
	}
	return closed, true
}
