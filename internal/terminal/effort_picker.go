package terminal

import (
	"fmt"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	"clara-code/pkg/core/prompt"
	"clara-code/pkg/model"
)

func (a *app) showEfforts(provider, name string, mode prompt.Mode, stagedAgent string, restoreAgent bool) {
	config := a.host.Config
	if len(config.Providers) > 0 {
		var err error
		config, err = config.SelectProvider(provider)
		if err != nil {
			a.notice(err)
			return
		}
	}
	profile := config.Models[name]
	levels := profile.Levels()
	if !slices.Contains(levels, model.ReasoningEffort("")) {
		levels = append([]model.ReasoningEffort{""}, levels...)
	}
	meta := a.conv.Store.Meta()
	current := meta.Effort
	defaultEffort := string(profile.DefaultReasoningLevel)
	if defaultEffort == "" {
		defaultEffort = string(profile.Effort)
	}
	selected := string(profile.Effort)
	label := "Agent (1/2)"
	if mode == prompt.Plan {
		label = "Plan (2/2)"
		current = meta.PlanEffort
		defaultEffort = string(config.PlanEffort)
		if !profile.Allows(model.ReasoningEffort(defaultEffort)) {
			defaultEffort = string(profile.Effort)
		}
		selected = defaultEffort
	}
	same := meta.Model == name && meta.Provider == provider
	if same && profile.Allows(model.ReasoningEffort(current)) {
		selected = current
	}
	if mode == prompt.Execute && restoreAgent {
		selected = stagedAgent
	}
	p := &picker{kind: "effort", title: "Select Reasoning Level for " + name + " · " + label, provider: provider, model: name, effortMode: mode, agentEffort: stagedAgent}
	for i, level := range levels {
		text, description := reasoningChoice(string(level))
		if level == "" && mode == prompt.Plan {
			text = "Inherit Agent"
			agentLabel := stagedAgent
			if agentLabel == "" {
				agentLabel = "provider default"
			}
			description = "Use Agent reasoning: " + agentLabel
		}
		if string(level) == defaultEffort {
			text += " (default)"
		}
		if same && string(level) == current {
			text += " (current)"
		}
		if string(level) == selected {
			p.selected = i
		}
		p.items = append(p.items, choice{id: string(level), label: text, description: description})
	}
	a.picker = p
}

func reasoningChoice(level string) (string, string) {
	switch level {
	case "":
		return "Provider default", "Let the provider choose its reasoning level."
	case "none":
		return "None", "No additional reasoning, where supported."
	case "minimal":
		return "Minimal", "Smallest reasoning budget."
	case "low":
		return "Low", "Fast responses with lighter reasoning."
	case "medium":
		return "Medium", "Balance speed and reasoning depth."
	case "high":
		return "High", "Greater reasoning depth for complex problems."
	case "xhigh":
		return "Extra high", "More reasoning for demanding tasks."
	case "max":
		return "Max", "Highest supported reasoning effort."
	}
	first, size := utf8.DecodeRuneInString(level)
	return string(unicode.ToUpper(first)) + level[size:], "Provider-defined reasoning level."
}

func (p *picker) effortRows(v *painter, help string) []string {
	rows := []string{v.style(bold, truncate(p.title, v.width))}
	limit := min(8, max(1, v.contentHeight()-2))
	start := min(max(0, p.selected-limit+1), max(0, len(p.items)-limit))
	leftWidth := 0
	for i, item := range p.items {
		leftWidth = max(leftWidth, widthOf(fmt.Sprintf("  %d. %s", i+1, singleLine(item.label))))
	}
	leftWidth = min(leftWidth+2, max(12, v.width*3/5))
	for i := start; i < min(len(p.items), start+limit); i++ {
		item := p.items[i]
		prefix, style := "  ", ""
		if i == p.selected {
			prefix, style = "❯ ", accent+bold
		}
		left := fmt.Sprintf("%s%d. %s", prefix, i+1, singleLine(item.label))
		if v.width < 45 {
			rows = append(rows, v.style(style, truncate(left, v.width)))
			continue
		}
		left = truncate(left, leftWidth)
		left += strings.Repeat(" ", max(1, leftWidth-widthOf(left)))
		description := truncate(singleLine(item.description), max(0, v.width-widthOf(left)))
		if i == p.selected {
			rows = append(rows, v.style(style, left+description))
		} else {
			rows = append(rows, left+v.style(dim, description))
		}
	}
	if help != "" && v.contentHeight() > 2 {
		rows = append(rows, v.style(dim, truncate(help, v.width)))
	}
	return rows
}
