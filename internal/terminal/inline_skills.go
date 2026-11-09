package terminal

import (
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	"clara-code/internal/coding"
)

type inlineSpan struct {
	start, end int
}

func (e editor) inlineSpans() []inlineSpan {
	spans := slices.Clone(e.skills)
	for _, image := range e.images {
		spans = append(spans, inlineSpan{start: image.start, end: image.end})
	}
	slices.SortFunc(spans, func(a, b inlineSpan) int { return a.start - b.start })
	return spans
}

func (e *editor) insertSkill(name string) {
	start := e.at
	text := "$" + name
	e.insert(text)
	if e.at-start == len(text) {
		e.skills = append(e.skills, inlineSpan{start: start, end: e.at})
	}
}

func (e *editor) shiftSkills(from, delta int) {
	for i := range e.skills {
		if e.skills[i].start >= from {
			e.skills[i].start += delta
			e.skills[i].end += delta
		}
	}
}

func (e *editor) restoreSkills(catalog []coding.Skill) {
	e.skills = nil
	names := slices.Clone(catalog)
	slices.SortFunc(names, func(a, b coding.Skill) int { return len(b.Name) - len(a.Name) })
	for at := 0; at < len(e.text); {
		r, size := utf8.DecodeRuneInString(e.text[at:])
		if r != '$' {
			at += size
			continue
		}
		previous, _ := utf8.DecodeLastRuneInString(e.text[:at])
		boundary := at == 0 || unicode.IsSpace(previous)
		for _, image := range e.images {
			boundary = boundary || image.end == at
		}
		matched := false
		if boundary {
			for _, skill := range names {
				text := "$" + skill.Name
				if !strings.HasPrefix(e.text[at:], text) {
					continue
				}
				end := at + len(text)
				if end < len(e.text) {
					next, _ := utf8.DecodeRuneInString(e.text[end:])
					if skillNameRune(next) {
						continue
					}
				}
				e.skills = append(e.skills, inlineSpan{start: at, end: end})
				at, matched = end, true
				break
			}
		}
		if !matched {
			at += size
		}
	}
}
