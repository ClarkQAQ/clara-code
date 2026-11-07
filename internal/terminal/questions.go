package terminal

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"clara-code/internal/coding"
)

type questionDraft struct {
	selected int
	notes    editor
	answered bool
}

type questionForm struct {
	request    coding.QuestionRequest
	drafts     []questionDraft
	index      int
	notesMode  bool
	width      int
	validation string
}

func newQuestionForm(request coding.QuestionRequest) *questionForm {
	return &questionForm{request: request, drafts: make([]questionDraft, len(request.Items))}
}

func questionOptions(item coding.Question) []coding.QuestionOption {
	options := append([]coding.QuestionOption{}, item.Options...)
	for _, option := range options {
		if strings.EqualFold(option.Label, "None of the above") {
			return options
		}
	}
	if len(options) > 0 {
		options = append(options, coding.QuestionOption{Label: "None of the above", Description: "Optionally, add details in notes (tab)."})
	}
	return options
}

func (q *questionForm) rows(p *painter) []string {
	q.width = p.width
	item, draft := q.request.Items[q.index], &q.drafts[q.index]
	unanswered := 0
	for _, answer := range q.drafts {
		if !answer.answered {
			unanswered++
		}
	}
	width := max(1, p.width-4)
	margin := strings.Repeat(" ", min(2, max(0, p.width-1)))
	lines := []string{margin + p.style(dim, truncate(fmt.Sprintf("Question %d/%d (%d unanswered)", q.index+1, len(q.drafts), unanswered), width))}
	for _, row := range wrap(clean(item.Prompt()), width) {
		lines = append(lines, margin+p.style(accent, truncate(row.text, width)))
		if len(lines) >= max(2, p.height/3) {
			break
		}
	}
	lines = append(lines, "")
	options := questionOptions(item)
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
				optionLines = append(optionLines, margin+p.style(style, lead+truncate(row.text, max(1, width-2))))
			}
			if option.Description != "" {
				for _, row := range wrap(clean(option.Description), max(1, width-4)) {
					optionLines = append(optionLines, margin+p.style(dim, "    "+truncate(row.text, max(1, width-4))))
				}
			}
		}
		if i == draft.selected {
			selectedEnd = len(optionLines)
		}
	}
	help := "tab to add notes | enter to submit answer | esc to interrupt"
	if q.notesMode {
		help = "tab to select options | enter to submit answer | esc to interrupt"
	}
	if len(q.drafts) > 1 {
		help += " | ←/→ questions"
	}
	if q.validation != "" {
		help = q.validation
	}
	var footer []string
	for _, row := range wrap(help, width) {
		footer = append(footer, margin+p.style(accent, truncate(row.text, width)))
		if len(footer) >= max(1, p.height/4) {
			break
		}
	}
	var notes []string
	if q.notesMode || len(options) == 0 {
		notes = append(notes, p.style(dim, truncate(margin+"Notes", p.width)))
		editorRows := p.inputRows(draft.notes, "Type your answer or additional details")
		limit := max(1, min(4, p.height/3))
		cursor := 0
		for i, row := range editorRows {
			if strings.Contains(row, reverse) {
				cursor = i
			}
		}
		start := max(0, cursor-limit+1)
		notes = append(notes, editorRows[start:min(len(editorRows), start+limit)]...)
	}
	available := max(0, p.height-len(lines)-len(notes)-len(footer)-1)
	if len(options) > 0 && available > 0 {
		start := max(0, min(selectedStart, selectedEnd-available))
		lines = append(lines, optionLines[start:min(len(optionLines), start+available)]...)
	}
	lines = append(lines, notes...)
	if len(lines)+len(footer) < p.height {
		lines = append(lines, "")
	}
	lines = append(lines, footer...)
	return lines[:min(len(lines), p.height)]
}

func (q *questionForm) key(input key) bool {
	select {
	case <-q.request.Done:
		return true
	default:
	}
	draft := &q.drafts[q.index]
	q.validation = ""
	options := questionOptions(q.request.Items[q.index])
	switch input.code {
	case "tab", "backtab":
		q.notesMode = !q.notesMode
	case "up":
		if !q.notesMode {
			draft.selected = max(0, draft.selected-1)
		} else {
			draft.notes.vertical(-1, max(1, q.width-2))
		}
	case "down":
		if !q.notesMode {
			draft.selected = min(max(0, len(options)-1), draft.selected+1)
		} else {
			draft.notes.vertical(1, max(1, q.width-2))
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
				return false
			}
		}
		if input.code == "text" || input.code == "paste" {
			q.notesMode = true
		}
		if q.notesMode || len(options) == 0 {
			draft.notes.key(input)
		}
	}
	return false
}
