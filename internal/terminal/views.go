package terminal

import (
	"context"
	"fmt"

	"clara-code/internal/session"
)

func (a *app) chooseSkill() {
	if a.editor.at > 0 && a.editor.text[a.editor.at-1] != ' ' && a.editor.text[a.editor.at-1] != '\n' {
		a.editor.insert(" ")
	}
	a.editor.insert("$")
	a.skillMenu = skillMenu{}
}

func (a *app) diffPage(section, offset int) {
	var previous []int
	if current := a.picker; current != nil && current.kind == "diff" && current.selected == section {
		previous = append(previous, current.previousPages...)
		if offset > current.pageOffset {
			previous = append(previous, current.pageOffset)
		} else if offset < current.pageOffset && len(previous) > 0 {
			previous = previous[:len(previous)-1]
		}
	}
	a.picker = nil
	a.operation("Loading diff", func(ctx context.Context) operationResult {
		kind := []string{"staged", "unstaged", "untracked"}[section]
		text, more, err := a.space.DiffPage(ctx, kind, offset)
		if err != nil {
			return operationResult{err: err}
		}
		if text == "" {
			text = "No " + kind + " changes."
		}
		return operationResult{picker: &picker{kind: "diff", title: "Git diff · " + kind, preview: text, selected: section, pageOffset: offset, previousPages: previous, more: more, nextOffset: offset + len(text)}}
	})
}

func (a *app) diffRows() []string {
	p, v := a.picker, &a.view
	lines := []string{v.style(bold, truncate(p.title, v.width))}
	rows := v.messageRows("diff", p.preview)
	height := max(1, v.height-2)
	p.scroll = min(max(0, p.scroll), max(0, len(rows)-height))
	lines = append(lines, rows[p.scroll:min(len(rows), p.scroll+height)]...)
	help := "↑/↓ Scroll  Tab Section  n/p Page  Esc Back"
	if p.more {
		help = "More changes · " + help
	}
	lines = append(lines, v.style(dim, truncate(a.hints(help), v.width)))
	return lines[:min(len(lines), v.height)]
}

func (a *app) rewindRows() []string {
	p, v := a.picker, &a.view
	var lines []string
	selectedStart, selectedEnd := 0, 0
	for i := 0; i < a.conv.Store.TurnCount(); i++ {
		turn, err := a.conv.Store.ReadTurn(i)
		if err != nil {
			lines = append(lines, v.style(dim, truncate("[Unavailable turn]", v.width)))
			continue
		}
		start := len(lines)
		lines = append(lines, v.messageRows("user", turn.Prompt)...)
		lines = append(lines, v.attachmentHistory(turn.Attachments)...)
		if i == p.selected {
			selectedStart = start
			for j := start; j < len(lines); j++ {
				lines[j] = v.style(accent+bold, stripStyles(lines[j]))
			}
		}
		for _, cell := range turn.Cells {
			if p.detail || cell.Kind == "assistant" || cell.Kind == "user" {
				lines = append(lines, v.cellRows(cell)...)
			}
		}
		if i == p.selected {
			selectedEnd = len(lines)
		}
	}
	height := max(1, v.height-2)
	if p.followSelection {
		p.scroll = max(0, selectedStart-min(2, height/4))
		if selectedEnd-selectedStart < height {
			p.scroll = max(0, min(p.scroll, selectedEnd-height))
		}
		p.followSelection = false
	}
	p.scroll = min(max(0, p.scroll), max(0, len(lines)-height))
	result := []string{v.style(dim, truncate(fmt.Sprintf("Edit a previous message · %d/%d", p.selected+1, len(a.prompts)), v.width))}
	result = append(result, lines[p.scroll:min(len(lines), p.scroll+height)]...)
	result = append(result, v.style(dim, truncate(a.hints("←/→ Message  ↑/↓ Scroll  Ctrl+T Details  Enter Edit  Esc Cancel"), v.width)))
	return result[:min(len(result), v.height)]
}

func (a *app) reloadRunDisplay(turn *session.Turn) {
	start := max(0, a.runHistoryStart-a.view.droppedCells)
	if start <= len(a.view.history) {
		a.view.history = a.view.history[:start]
		a.view.sourceBytes = 0
		for _, cell := range a.view.history {
			a.view.sourceBytes += len(cell.Text)
			if cell.Tool != nil {
				a.view.sourceBytes += cell.Tool.DisplayBytes()
			}
		}
	}
	a.view.remember("user", turn.Prompt)
	a.replayAttachments(turn.Attachments)
	for _, cell := range turn.Cells {
		a.view.cell(cell)
	}
	a.stream, a.streamRows, a.continued = turn.Partial, 0, false
	a.sawText = turn.Staged || turn.Partial != ""
	a.view.rebuild()
}

func (a *app) handleTranscriptPicker(input key) bool {
	p := a.picker
	if input.code == "escape" || input.code == "ctrl-c" {
		a.picker = nil
		if p.kind == "rewind" {
			a.view.rebuild()
		}
		return false
	}
	switch input.code {
	case "up":
		p.scroll = max(0, p.scroll-1)
	case "down":
		p.scroll++
	case "page-up":
		p.scroll = max(0, p.scroll-max(1, a.view.height-3))
	case "page-down":
		p.scroll += max(1, a.view.height-3)
	case "home":
		p.scroll = 0
	case "end":
		p.scroll = int(^uint(0) >> 1)
	case "left", "right":
		if p.kind == "rewind" {
			delta := 1
			if input.code == "left" {
				delta = -1
			}
			p.selected = min(max(0, p.selected+delta), len(a.prompts)-1)
			p.followSelection = true
		}
	case "ctrl-t":
		if p.kind == "rewind" {
			p.detail = !p.detail
			p.followSelection = true
		}
	case "tab", "backtab":
		if p.kind == "diff" {
			next := (p.selected + 1) % 3
			if input.code == "backtab" {
				next = (p.selected + 2) % 3
			}
			a.diffPage(next, 0)
		}
	case "text":
		if p.kind == "diff" {
			if input.text == "n" && p.more {
				a.diffPage(p.selected, p.nextOffset)
			}
			if input.text == "p" && len(p.previousPages) > 0 {
				a.diffPage(p.selected, p.previousPages[len(p.previousPages)-1])
			}
		}
	case "enter":
		if p.kind == "rewind" {
			turn, err := a.conv.Store.ReadTurn(p.selected)
			if err != nil {
				a.notice(err)
				return false
			}
			a.switchSession("", true, p.selected, turn.Prompt, "Rewound conversation", turn.Attachments...)
		}
	}
	return false
}

func stripStyles(text string) string {
	return ansiStyle.ReplaceAllString(text, "")
}
