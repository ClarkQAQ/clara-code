package terminal

import (
	"context"
	"fmt"
	"os"
	"strings"

	"clara-code/internal/session"
	"clara-code/pkg/core"
	"clara-code/pkg/core/prompt"
	"clara-code/pkg/model"
	"clara-code/pkg/tool/mcp"
)

const planPicker = "plan-implementation"

const planHandoff = "A previous agent produced the plan below to accomplish the user's task. " +
	"Implement the plan in a fresh context. Treat the plan as the source of user intent, " +
	"re-read files as needed, and carry the work through implementation and verification."

func completedPlan(result *core.AgentResult) string {
	if result == nil || len(result.Steps) == 0 {
		return ""
	}
	step := result.Steps[len(result.Steps)-1]
	if step.FinishReason != model.FinishReasonStop {
		return ""
	}
	plan := session.CompletePlan(planResponseText(step.Content))
	if len(planHandoff)+2+len(plan) > inputLimit {
		return ""
	}
	return plan
}

func planResponseText(content model.ResponseContent) string {
	var text strings.Builder
	for _, part := range content {
		if item, ok := model.AsContentType[model.TextContent](part); ok {
			text.WriteString(item.Text)
		}
	}
	return text.String()
}

func (a *app) confirmPlan(plan string) {
	a.picker = &picker{kind: planPicker, title: "Implement this plan?", preview: plan, items: []choice{
		{id: "implement", label: "Yes, implement this plan", description: "Switch to Agent and start coding."},
		{id: "fresh", label: "Yes, clear context and implement", description: "Fresh conversation with this plan."},
		{id: "stay", label: "No, stay in Plan mode", description: "Continue planning with the model."},
	}}
}

func (a *app) selectPlanAction(action, plan string) {
	a.picker = nil
	switch action {
	case "implement":
		a.implementPlan("Implement the plan.")
	case "fresh":
		a.conversationOperation("Opening a fresh conversation", func(ctx context.Context, report func(mcp.StartupProgress)) operationResult {
			result := a.open(ctx, "", false, -1, "", "Cleared conversation", report)
			result.plan = plan
			return result
		})
	}
}

func (a *app) implementPlan(text string) {
	draft := a.editor
	a.attachments = nil
	a.mode = prompt.Execute
	a.start(text, false)
	a.editor = draft
	a.syncDraftImages()
	if a.run == nil && draft.text == "" {
		a.setDraft(text, nil)
	}
}

func (a *app) planPickerRows() []string {
	p, view := a.picker, &a.view
	height := view.contentHeight()
	var rows []string
	if height > 1 {
		rows = append(rows, view.style(bold, truncate("  "+p.title, view.width)))
	}
	if height >= 6 {
		rows = append(rows, "")
	}
	limit := min(len(p.items), height-len(rows))
	start := min(max(0, p.selected-limit+1), len(p.items)-limit)
	for i := start; i < start+limit; i++ {
		item := p.items[i]
		prefix, style := "  ", ""
		if i == p.selected {
			prefix, style = "❯ ", accent
		}
		label := fmt.Sprintf("%s%d. %s", prefix, i+1, item.label)
		if view.width >= 60 {
			label += strings.Repeat(" ", 39-widthOf(label))
			descriptionStyle := dim
			if i == p.selected {
				descriptionStyle = accent
			}
			rows = append(rows, view.style(style, label)+view.style(descriptionStyle, truncate(item.description, view.width-39)))
		} else {
			rows = append(rows, view.style(style, truncate(label, view.width)))
		}
	}
	if help := a.hints("Press Enter to confirm or Esc to go back"); help != "" && len(rows) < height {
		if len(rows)+1 < height {
			rows = append(rows, "")
		}
		rows = append(rows, view.style(dim, truncate("  "+help, view.width)))
	}
	return rows
}

func (a *app) streamDisplayRows() []string {
	if a.mode != prompt.Plan {
		return a.view.messageRows("assistant", a.stream)
	}
	var rows []string
	for _, cell := range session.PlanCells(a.stream, true) {
		rows = append(rows, a.view.cellRows(cell)...)
	}
	return rows
}

func (p *painter) proposedPlanRows(text string) []string {
	margin := 0
	if p.width >= 8 {
		margin = 2
	}
	indent := strings.Repeat(" ", margin)
	width := p.width - margin*2
	rows := []string{p.style(dim, "• ") + p.style(bold, truncate("Proposed Plan", p.width-2)), ""}
	if p.width < 4 {
		rows[0] = truncate("Proposed Plan", p.width)
	}
	var body []string
	if p.markdown != nil && !*p.markdown {
		for _, line := range wrap(text, width) {
			body = append(body, truncate(line.text, width))
		}
	} else {
		body = p.markdownRowsWithSyntax(text, width, true)
	}
	background := "\x1b[48;5;237m"
	if _, bg, ok := strings.Cut(os.Getenv("COLORFGBG"), ";"); ok && (bg == "7" || bg == "15") {
		background = "\x1b[48;5;254m"
	}
	body = append([]string{""}, body...)
	body = append(body, "")
	for _, line := range body {
		if p.color {
			padding := max(0, p.width-margin*2-widthOf(ansiStyle.ReplaceAllString(line, "")))
			line = background + indent + strings.ReplaceAll(line, reset, reset+background) +
				strings.Repeat(" ", padding+margin) + reset
		} else if line != "" {
			line = indent + line
		}
		rows = append(rows, line)
	}
	return append(rows, "")
}
