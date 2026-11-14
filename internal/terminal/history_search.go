package terminal

import "strings"

type historySearch struct {
	query   editor
	matches []int
}

func (a *app) openHistorySearch() {
	a.loadInputHistory()
	a.primed, a.hint = false, ""
	a.picker = &picker{history: &historySearch{matches: matchInputHistory(a.inputHistory, "")}}
}

func (a *app) handleHistorySearch(input key) {
	p := a.picker
	search := p.history
	switch input.code {
	case "escape", "ctrl-c":
		a.picker = nil
	case "enter":
		if len(search.matches) > 0 {
			a.restoreHistoryEntry(a.inputHistory[search.matches[p.selected]])
			a.historyAt = -1
			a.picker = nil
		}
	case "up":
		p.selected = max(0, p.selected-1)
	case "down":
		p.selected = min(max(0, len(search.matches)-1), p.selected+1)
	case "newline":
		return
	default:
		if input.code == "paste" {
			input.text = strings.ReplaceAll(clean(input.text), "\n", " ")
		}
		before := search.query.text
		search.query.key(input)
		if search.query.text != before {
			search.matches = matchInputHistory(a.inputHistory, search.query.text)
			p.selected = 0
		}
	}
}

func (a *app) historySearchLayout() layout {
	p, view := a.picker, &a.view
	search := p.history
	pane := view.inputLayout(search.query, "Search input history", 1)
	available := view.contentHeight() - len(pane.rows)
	if available <= 0 {
		return pane
	}
	help := a.hints("↑/↓ Select   Enter Recall   Esc Cancel")
	showHelp := help != "" && available > 1
	if showHelp {
		available--
	}
	limit := min(8, available)
	start := min(max(0, p.selected-limit+1), max(0, len(search.matches)-limit))
	for i := start; i < min(len(search.matches), start+limit); i++ {
		entry := a.inputHistory[search.matches[i]]
		label := singleLine(entry.Text)
		if label == "" && len(entry.Attachments) > 0 {
			label = "Image attachment"
			if name := singleLine(entry.Attachments[0].Filename); name != "" {
				label += " · " + name
			}
		}
		row := truncate("  "+label, view.width)
		if i == p.selected {
			if view.color {
				row = view.style(accent, row)
			} else {
				row = reverseVideo + row + reset
			}
		}
		pane.rows = append(pane.rows, row)
	}
	if len(search.matches) == 0 {
		message := "  No matching history"
		if len(a.inputHistory) == 0 {
			message = "  No input history"
		}
		pane.rows = append(pane.rows, view.style(dim, truncate(message, view.width)))
	}
	if showHelp {
		pane.rows = append(pane.rows, view.style(dim, truncate(help, view.width)))
	}
	return pane
}
