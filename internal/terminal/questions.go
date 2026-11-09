package terminal

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"clara-code/internal/coding"
)

const noneOfTheAbove = "None of the above"

type questionDraft struct {
	selected      int
	notes         editor
	keepNotesOpen bool
	answered      bool
}

type questionForm struct {
	request    coding.QuestionRequest
	drafts     []questionDraft
	index      int
	notesMode  bool
	noteWidth  int
	validation string
	helpText   func(string) string
}

func newQuestionForm(request coding.QuestionRequest) *questionForm {
	return &questionForm{request: request, drafts: make([]questionDraft, len(request.Items))}
}

func questionOptions(item coding.Question) []coding.QuestionOption {
	options := append([]coding.QuestionOption{}, item.Options...)
	for _, option := range options {
		if strings.EqualFold(option.Label, noneOfTheAbove) {
			return options
		}
	}
	if len(options) > 0 {
		options = append(options, coding.QuestionOption{Label: noneOfTheAbove, Description: "Optionally, add details in notes (tab)."})
	}
	return options
}

func (q *questionForm) editingNotes() bool {
	return q.notesMode && len(q.request.Items[q.index].Options) > 0
}

func (q *questionForm) layout(p *painter) layout {
	height := p.contentHeight()
	item, draft := q.request.Items[q.index], &q.drafts[q.index]
	unanswered := 0
	for _, answer := range q.drafts {
		if !answer.answered {
			unanswered++
		}
	}
	marginWidth := min(2, max(0, (p.width-1)/2))
	margin := strings.Repeat(" ", marginWidth)
	width := max(1, p.width-2*marginWidth)
	lines := []string{margin + p.style(dim, truncate(fmt.Sprintf("Question %d/%d (%d unanswered)", q.index+1, len(q.drafts), unanswered), width))}
	for _, row := range wrap(clean(item.Prompt()), width) {
		lines = append(lines, margin+p.style(accent, truncate(row.text, width)))
		if len(lines) >= max(2, height/3) {
			break
		}
	}
	options := questionOptions(item)
	if q.helpText != nil && len(options) > 0 {
		last := &options[len(options)-1]
		if last.Description == "Optionally, add details in notes (tab)." {
			if hint := q.helpText("tab"); hint != "" {
				last.Description = "Optionally, add details in notes (" + hint + ")."
			} else {
				last.Description = "Optionally, add details in notes."
			}
		}
	}
	var optionLines []string
	selectedStart, selectedEnd := 0, 0
	labelWidth := 12
	for _, option := range options {
		labelWidth = max(labelWidth, widthOf(singleLine(option.Label)))
	}
	labelWidth = min(labelWidth, 30, max(8, width/2))
	for i, option := range options {
		style, prefix := "", "  "
		if i == draft.selected {
			style, prefix, selectedStart = accent+bold, "› ", len(optionLines)
		}
		label := fmt.Sprintf("%d. %s", i+1, singleLine(option.Label))
		if width >= 55 {
			label = truncate(label, labelWidth+3)
			descriptions := wrap(clean(option.Description), max(1, width-labelWidth-7))
			for j, row := range descriptions {
				left := strings.Repeat(" ", labelWidth+5)
				rightStyle := dim
				if i == draft.selected {
					rightStyle = accent + bold
				}
				if j == 0 {
					left = prefix + label + strings.Repeat(" ", max(0, labelWidth+3-widthOf(label)))
				}
				optionLines = append(optionLines, margin+p.style(style, left)+"  "+p.style(rightStyle, truncate(row.text, max(1, width-labelWidth-7))))
			}
		} else {
			for j, row := range wrap(label, max(1, width-2)) {
				lead := "  "
				if j == 0 {
					lead = prefix
				}
				optionLines = append(optionLines, margin+p.style(style, truncate(lead+row.text, width)))
			}
			if option.Description != "" {
				descriptionStyle := dim
				if i == draft.selected {
					descriptionStyle = style
				}
				for _, row := range wrap(clean(option.Description), max(1, width-4)) {
					optionLines = append(optionLines, margin+p.style(descriptionStyle, truncate("    "+row.text, width)))
				}
			}
		}
		if i == draft.selected {
			selectedEnd = len(optionLines)
		}
	}
	var notes layout
	notesFocused := q.notesMode || len(options) == 0
	showNotes := notesFocused || (height > 1 && draft.keepNotesOpen)
	if showNotes {
		prefix := margin + inputPrefix(p.width-marginWidth)
		q.noteWidth = max(1, p.width-widthOf(prefix))
		placeholder := "Add notes"
		if len(options) == 0 {
			placeholder = "Type your answer"
		}
		notes = p.inputLayoutWithPrefix(draft.notes, placeholder, max(1, min(4, height/3)), prefix)
		if !notesFocused {
			notes.cursor = nil
		}
	}
	remaining := height - len(notes.rows)
	optionMinimum := min(1, len(optionLines))
	footer := q.footer(p, margin, width, min(max(1, height/4), max(0, remaining-min(2, len(lines))-optionMinimum)))
	remaining -= len(footer)
	if len(notes.rows) > 0 && remaining >= len(lines)+optionMinimum+2 {
		border := margin + p.style(dim, strings.Repeat("─", p.width-marginWidth))
		notes = notes.prepend([]string{border})
		notes.rows = append(notes.rows, border)
		remaining -= 2
	}
	lines = lines[:min(len(lines), max(0, remaining-optionMinimum))]
	if len(optionLines) > 0 && remaining-len(lines) > 1 {
		lines = append(lines, "")
	}
	if available := remaining - len(lines); available > 0 && len(optionLines) > 0 {
		start := max(0, min(selectedStart, selectedEnd-available))
		lines = append(lines, optionLines[start:min(len(optionLines), start+available)]...)
	}
	view := layout{rows: lines}
	view.append(notes)
	view.rows = append(view.rows, footer...)
	return view
}

func (q *questionForm) footer(p *painter, margin string, width, limit int) []string {
	if limit == 0 {
		return nil
	}
	hint := "tab to add notes | esc to interrupt"
	switch {
	case q.editingNotes():
		hint = "tab or esc to clear notes"
	case len(q.request.Items[q.index].Options) == 0:
		hint = "esc to interrupt"
	}
	if len(q.drafts) > 1 && !q.notesMode {
		hint += " | ←/→ questions"
	}
	submit := "enter to submit answer"
	if q.helpText != nil {
		hint, submit = q.helpText(hint), q.helpText(submit)
	}
	if hint != "" && submit != "" {
		hint += " | "
	}
	if q.validation != "" {
		hint, submit = "", q.validation
	}
	var footer []string
	for _, row := range wrap(hint+submit, width) {
		if row.text == "" {
			break
		}
		split := min(max(0, len(hint)-row.start), len(row.text))
		left := truncate(row.text[:split], width)
		right := truncate(row.text[split:], width-widthOf(left))
		footer = append(footer, margin+p.style(dim, left)+p.style(accent, right))
		if len(footer) == limit {
			break
		}
	}
	return footer
}

func (q *questionForm) key(input key) bool {
	select {
	case <-q.request.Done:
		return true
	default:
	}
	draft := &q.drafts[q.index]
	selected := draft.selected
	q.validation = ""
	options := questionOptions(q.request.Items[q.index])
	switch input.code {
	case "escape", "tab", "backtab":
		if q.editingNotes() {
			draft.notes = editor{}
			q.notesMode = false
		} else if input.code != "escape" {
			q.notesMode = !q.notesMode
		}
	case "up":
		if !q.notesMode {
			draft.selected = max(0, draft.selected-1)
		} else {
			draft.notes.vertical(-1, max(1, q.noteWidth))
		}
	case "down":
		if !q.notesMode {
			draft.selected = min(max(0, len(options)-1), draft.selected+1)
		} else {
			draft.notes.vertical(1, max(1, q.noteWidth))
		}
	case "left", "right":
		if q.notesMode {
			draft.notes.key(input)
			break
		}
		if input.code == "left" {
			q.index = max(0, q.index-1)
		} else {
			q.index = min(len(q.drafts)-1, q.index+1)
		}
	case "enter":
		if len(options) == 0 && strings.TrimSpace(draft.notes.text) == "" {
			q.validation = "Type an answer before submitting."
			return false
		}
		if len(draft.notes.text) > 4096 {
			q.validation = "Notes exceed 4096 bytes. Shorten them before submitting."
			return false
		}
		draft.answered = true
		for i := 1; i <= len(q.drafts); i++ {
			next := (q.index + i) % len(q.drafts)
			if !q.drafts[next].answered {
				q.index = next
				q.notesMode = false
				return false
			}
		}
		answers := make(map[string]string, len(q.drafts))
		for i, item := range q.request.Items {
			d := q.drafts[i]
			answer := coding.QuestionAnswer{Answers: []string{}, Notes: strings.TrimSpace(d.notes.text)}
			choices := questionOptions(item)
			if len(choices) > 0 {
				answer.Answers = append(answer.Answers, choices[d.selected].Label)
			} else {
				answer.Answers = append(answer.Answers, answer.Notes)
				answer.Notes = ""
			}
			body, _ := json.Marshal(answer)
			answers[item.ID] = string(body)
		}
		q.request.Answer <- answers
		return true
	default:
		if !q.notesMode && input.code == "text" {
			if n, err := strconv.Atoi(input.text); err == nil && n >= 1 && n <= len(options) {
				draft.selected = n - 1
				break
			}
		}
		if input.code == "text" || input.code == "paste" {
			q.notesMode = true
		}
		if q.notesMode || len(options) == 0 {
			draft.notes.key(input)
		}
	}
	if draft.selected != selected {
		draft.keepNotesOpen = false
	}
	if q.notesMode && len(options) > 0 && strings.EqualFold(options[draft.selected].Label, noneOfTheAbove) {
		draft.keepNotesOpen = true
	}
	return false
}
