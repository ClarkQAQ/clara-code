package terminal

import (
	"strings"
	"unicode"
	"unicode/utf8"

	"clara-code/internal/coding"
)

type skillMenu struct {
	owner       *coding.Conversation
	catalog     []coding.Skill
	matches     []coding.Skill
	text, error string
	at          int
	start, end  int
	selected    int
	active      bool
	hidden      bool
}

// A mention starts at a word boundary. Shell expansions are ordinary text.
func skillMention(text string, at int) (start, end int, query string, ok bool) {
	if at < 0 || at > len(text) {
		return
	}
	start = at
	for start > 0 {
		r, size := utf8.DecodeLastRuneInString(text[:start])
		if unicode.IsSpace(r) {
			break
		}
		start -= size
	}
	if start == at || text[start] != '$' {
		return
	}
	query = text[start+1 : at]
	for _, r := range query {
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) && !strings.ContainsRune("_-.:/", r) {
			return start, at, query, false
		}
	}
	if query != "" && strings.ToUpper(query) == query {
		switch query {
		case "HOME", "PATH", "USER", "SHELL", "PWD", "TERM", "TZ", "EDITOR", "LANG", "TMPDIR", "XDG_DATA_HOME":
			return start, at, query, false
		}
	}
	end = at
	for end < len(text) {
		r, size := utf8.DecodeRuneInString(text[end:])
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) && !strings.ContainsRune("_-.:/", r) {
			break
		}
		end += size
	}
	return start, end, query, true
}

func (a *app) skillMatches() bool {
	m := &a.skillMenu
	start, end, query, ok := skillMention(a.editor.text, a.editor.at)
	if !ok || a.conv == nil {
		m.active = false
		return false
	}
	changed := m.text != a.editor.text || m.at != a.editor.at
	if !m.active || m.owner != a.conv {
		m.catalog, m.error = nil, ""
		skills, err := a.conv.Skills(a.ctx)
		if err != nil {
			m.error = a.host.Config.Redact(err.Error())
		} else {
			m.catalog = skills
		}
		m.owner, m.hidden = a.conv, false
	}
	if changed {
		m.selected, m.hidden = 0, false
	}
	m.active, m.text, m.at, m.start, m.end = true, a.editor.text, a.editor.at, start, end
	m.matches = m.matches[:0]
	for _, skill := range m.catalog {
		if strings.Contains(strings.ToLower(skill.Name), strings.ToLower(query)) || strings.Contains(strings.ToLower(skill.Description), strings.ToLower(query)) {
			m.matches = append(m.matches, skill)
		}
	}
	m.selected = min(m.selected, max(0, len(m.matches)-1))
	return !m.hidden
}

func (a *app) skillRows(available int) []string {
	if available <= 0 {
		return nil
	}
	m, p := &a.skillMenu, &a.view
	if m.error != "" {
		return []string{p.style(dim, truncate("  Skills unavailable: "+singleLine(m.error), p.width))}
	}
	if len(m.matches) == 0 {
		return []string{p.style(dim, truncate("  No matching enabled skills", p.width))}
	}
	limit := min(8, max(1, available-2))
	start := min(max(0, m.selected-limit+1), max(0, len(m.matches)-limit))
	nameWidth := 0
	for _, skill := range m.matches {
		nameWidth = max(nameWidth, widthOf(singleLine(skill.Name)))
	}
	nameWidth = min(nameWidth, max(8, p.width/3))
	var rows []string
	for i := start; i < min(len(m.matches), start+limit); i++ {
		skill := m.matches[i]
		style := dim
		if i == m.selected {
			style = accent + bold
		}
		left := "  " + truncate(singleLine(skill.Name), nameWidth)
		left += strings.Repeat(" ", max(2, nameWidth+4-widthOf(left)))
		if i == m.selected {
			rows = append(rows, p.style(style, truncate(left+"[Skill] "+singleLine(skill.Description), p.width)))
		} else {
			left = truncate(left, p.width)
			description := truncate("[Skill] "+singleLine(skill.Description), max(0, p.width-widthOf(left)))
			rows = append(rows, left+p.style(dim, description))
		}
	}
	if len(rows)+2 <= available {
		rows = append(rows, "", p.style(dim, truncate(a.hints("  Press Enter to insert or Esc to close"), p.width)))
	}
	return rows
}

func (a *app) handleSkillMenu(input key) bool {
	if !a.skillMatches() {
		return false
	}
	m := &a.skillMenu
	switch input.code {
	case "escape":
		m.hidden = true
	case "up", "down":
		if len(m.matches) > 0 {
			delta := 1
			if input.code == "up" {
				delta = -1
			}
			m.selected = (m.selected + delta + len(m.matches)) % len(m.matches)
		}
	case "enter", "tab":
		if len(m.matches) == 0 {
			return input.code == "tab"
		}
		text := "$" + m.matches[m.selected].Name
		end := m.end
		if end < len(a.editor.text) && a.editor.text[end] == ' ' {
			end++
		}
		if len(a.editor.text)-(end-m.start)+len(text)+1 > inputLimit {
			a.hint = "Not enough room to insert the skill"
			return true
		}
		a.editor.delete(m.start, end)
		a.editor.insert(text + " ")
		a.skillMenu = skillMenu{}
		a.syncDraftImages()
	default:
		return false
	}
	return true
}
