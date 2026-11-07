package terminal

type choice struct {
	id, label string
}

type picker struct {
	kind, title, model     string
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

		case "model":
			profile := a.host.Config.Models[selected.id]
			levels := []string{"", "none", "minimal", "low", "medium", "high", "xhigh", "max"}
			next := &picker{kind: "effort", title: "Select reasoning effort", model: selected.id}
			effort := string(profile.Effort)
			if meta := a.conv.Store.Meta(); meta.Model == selected.id {
				effort = meta.Effort
			}
			for i, level := range levels {
				label := level
				if level == "" {
					label = "Provider default"
				}
				if effort == level {
					next.selected = i
				}
				next.items = append(next.items, choice{id: level, label: label})
			}
			a.picker = next
		case "skill":
			a.picker = nil
			a.editor.insert("$" + selected.id + " ")
		case "effort":
			a.picker = nil
			a.applyModel(p.model, selected.id)
		}
	}
	return false
}
