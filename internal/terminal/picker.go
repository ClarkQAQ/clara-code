package terminal

import (
	"clara-code/pkg/core/prompt"
)

type choice struct {
	id, label   string
	description string
}

type picker struct {
	skillState             *skillManager
	kind, title, model     string
	provider               string
	effortMode             prompt.Mode
	agentEffort            string
	items                  []choice
	selected               int
	startup, detail        bool
	preview                string
	scroll                 int
	pageOffset, nextOffset int
	previousPages          []int
	more, followSelection  bool
}

func (p *picker) rows(view *painter, help string) []string {
	if p.kind == "effort" {
		return p.effortRows(view, help)
	}
	return listRows(view, p.title, p.items, p.selected, help, 0)
}

func listRows(view *painter, title string, items []choice, selected int, help string, reserved int) []string {
	lines := []string{view.style(bold, truncate(title, view.width))}
	limit := min(8, max(1, view.contentHeight()-2-reserved))
	start := min(max(0, selected-limit+1), max(0, len(items)-limit))
	for i := start; i < min(len(items), start+limit); i++ {
		prefix, style := "  ", ""
		if i == selected {
			prefix, style = "❯ ", accent
		}
		lines = append(lines, view.style(style, truncate(prefix+singleLine(items[i].label), view.width)))
	}
	if help != "" && view.contentHeight() > 2+reserved {
		lines = append(lines, view.style(dim, truncate(help, view.width)))
	}
	return lines
}

func (a *app) handlePicker(input key) bool {
	p := a.picker
	if p.kind == "skills" || p.kind == "skill-toggle" {
		return a.handleSkillPicker(input)
	}
	if p.kind == "rewind" || p.kind == "diff" {
		return a.handleTranscriptPicker(input)
	}
	if input.code == "escape" || input.code == "ctrl-c" {
		if p.kind == "effort" && input.code == "escape" {
			if p.effortMode == prompt.Plan {
				a.showEfforts(p.provider, p.model, prompt.Execute, p.agentEffort, true)
			} else {
				a.showModels(p.provider)
			}
			return false
		}
		a.picker = nil
		return p.startup
	}
	if len(p.items) == 0 {
		return false
	}
	if p.kind == "attachments" && (input.code == "delete" || input.code == "backspace") {
		index := 0
		for _, span := range a.editor.images {
			if span.pending {
				continue
			}
			if index == p.selected {
				a.editor.delete(span.start, span.end)
				break
			}
			index++
		}
		a.syncDraftImages()
		a.picker = nil
		if len(a.attachments) > 0 {
			a.manageAttachments()
		}
		return false
	}
	if p.kind == "effort" && input.code == "text" && len(input.text) == 1 && input.text[0] >= '1' && input.text[0] <= '9' {
		index := int(input.text[0] - '1')
		if index < len(p.items) {
			p.selected = index
			return a.handlePicker(key{code: "enter"})
		}
	}
	switch input.code {
	case "up", "left":
		p.selected = max(0, p.selected-1)
	case "down", "right":
		p.selected = min(len(p.items)-1, p.selected+1)
	case "home":
		p.selected = 0
	case "end":
		p.selected = len(p.items) - 1
	case "page-up":
		p.selected = max(0, p.selected-8)
	case "page-down":
		p.selected = min(len(p.items)-1, p.selected+8)
	case "enter":
		selected := p.items[p.selected]
		switch p.kind {
		case "resume":
			a.switchSession(selected.id, false, -1, "", "Resumed conversation")

		case "provider":
			a.showModels(selected.id)
		case "attachments":
			a.picker = nil
		case "model":
			a.showEfforts(p.provider, selected.id, prompt.Execute, "", false)
		case "effort":
			if p.effortMode != prompt.Plan {
				a.showEfforts(p.provider, p.model, prompt.Plan, selected.id, false)
			} else if err := a.conv.ChangeReasoningSelection(a.ctx, p.provider, p.model, p.agentEffort, selected.id); err != nil {
				a.notice(err)
			} else {
				a.picker = nil
				a.host = a.conv.Host
				a.host.Config = a.conv.CurrentConfig()
				a.view.message("notice", "Model set to "+modelLabel(a.conv.Store.Meta())+" · Plan "+a.conv.ModeEffort(prompt.Plan))
			}
		}
	}
	return false
}
