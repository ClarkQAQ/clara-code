package terminal

import (
	"clara-code/pkg/core/prompt"
	"clara-code/pkg/model"
)

type choice struct {
	id, label string
}

type picker struct {
	kind, title, model     string
	provider               string
	items                  []choice
	selected               int
	startup, detail        bool
	preview                string
	scroll                 int
	pageOffset, nextOffset int
	previousPages          []int
	more, followSelection  bool
}

func (p *picker) rows(view *painter) []string {
	return listRows(view, p.title, p.items, p.selected, "↑/↓ Select   Enter Confirm   Esc Back", 0)
}

func listRows(view *painter, title string, items []choice, selected int, help string, reserved int) []string {
	lines := []string{view.style(bold, truncate(title, view.width))}
	limit := min(8, max(1, view.height-2-reserved))
	start := min(max(0, selected-limit+1), max(0, len(items)-limit))
	for i := start; i < min(len(items), start+limit); i++ {
		prefix, style := "  ", ""
		if i == selected {
			prefix, style = "❯ ", accent
		}
		lines = append(lines, view.style(style, truncate(prefix+singleLine(items[i].label), view.width)))
	}
	if view.height > 2+reserved {
		lines = append(lines, view.style(dim, truncate(help, view.width)))
	}
	return lines
}

func (a *app) handlePicker(input key) bool {
	p := a.picker
	if p.kind == "rewind" || p.kind == "diff" {
		return a.handleTranscriptPicker(input)
	}
	if input.code == "escape" || input.code == "ctrl-c" {
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
			config := a.host.Config
			if len(config.Providers) > 0 {
				var err error
				config, err = config.SelectProvider(p.provider)
				if err != nil {
					a.notice(err)
					return false
				}
			}
			profile := config.Models[selected.id]
			levels := profile.Levels()
			if len(levels) == 0 || levels[0] != "" {
				levels = append([]model.ReasoningEffort{""}, levels...)
			}
			next := &picker{kind: "effort", title: "Select reasoning effort", model: selected.id, provider: p.provider}
			effort := string(profile.Effort)
			if meta := a.conv.Store.Meta(); meta.Model == selected.id && meta.Provider == p.provider {
				effort = a.conv.ModeEffort(a.mode)
			}
			for i, level := range levels {
				label := string(level)
				if level == "" {
					label = "Provider default"
					if a.mode == prompt.Plan {
						label = "Inherit Agent reasoning effort"
					}
				}
				if effort == string(level) {
					next.selected = i
				}
				next.items = append(next.items, choice{id: string(level), label: label})
			}
			a.picker = next
		case "skill":
			a.picker = nil
			a.editor.insert("$" + selected.id + " ")
		case "effort":
			a.picker = nil
			a.applySelection(p.provider, p.model, selected.id)
		}
	}
	return false
}
