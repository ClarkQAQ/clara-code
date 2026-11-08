package terminal

import (
	"context"
	"fmt"
	"strings"
	"time"

	"clara-code/internal/coding"
)

type skillManager struct {
	skills   []coding.Skill
	query    editor
	selected int
}

func (a *app) skillsMenu() {
	a.picker = &picker{kind: "skills", title: "Skills", items: []choice{
		{id: "list", label: "List skills"}, {id: "toggle", label: "Enable/Disable Skills"},
	}}
}

func (a *app) manageSkills() {
	skills, err := a.conv.ManageSkills(a.ctx)
	if err != nil {
		a.notice(err)
		return
	}
	a.picker = &picker{kind: "skill-toggle", title: "Enable/Disable Skills", skillState: &skillManager{skills: skills}}
}

func (s *skillManager) matches() []int {
	var result []int
	query := strings.ToLower(s.query.text)
	for i, skill := range s.skills {
		if strings.Contains(strings.ToLower(skill.Name+" "+skill.Description), query) {
			result = append(result, i)
		}
	}
	return result
}

func (a *app) handleSkillPicker(input key) bool {
	p := a.picker
	if input.code == "escape" || input.code == "ctrl-c" {
		if p.kind == "skill-toggle" {
			a.skillsMenu()
		} else {
			a.picker = nil
		}
		return false
	}
	if p.kind == "skills" {
		switch input.code {
		case "up", "down":
			p.selected = 1 - p.selected
		case "text":
			if input.text == "1" || input.text == "2" {
				p.selected = int(input.text[0] - '1')
				return a.handleSkillPicker(key{code: "enter"})
			}
		case "enter":
			if p.selected == 0 {
				a.picker = nil
				a.chooseSkill()
			} else {
				a.manageSkills()
			}
		}
		return false
	}
	s := p.skillState
	matches := s.matches()
	s.selected = min(s.selected, max(0, len(matches)-1))
	switch input.code {
	case "up", "down":
		if len(matches) > 0 {
			delta := 1
			if input.code == "up" {
				delta = -1
			}
			s.selected = (s.selected + delta + len(matches)) % len(matches)
		}
	case "home":
		s.selected = 0
	case "end":
		s.selected = max(0, len(matches)-1)
	case "page-up":
		s.selected = max(0, s.selected-8)
	case "page-down":
		s.selected = min(max(0, len(matches)-1), s.selected+8)
	case "enter":
		a.toggleSkill(s, matches)
	case "text":
		if input.text == " " {
			a.toggleSkill(s, matches)
		} else {
			s.query.key(input)
			s.selected = 0
		}
	default:
		s.query.key(input)
		s.selected = 0
	}
	return false
}

func (a *app) toggleSkill(s *skillManager, matches []int) {
	if len(matches) == 0 {
		return
	}
	skill := &s.skills[matches[s.selected]]
	ctx, cancel := context.WithTimeout(a.ctx, 2*time.Second)
	defer cancel()
	if err := a.conv.SetSkillEnabled(ctx, skill.Source, !skill.Enabled); err != nil {
		a.notice(err)
		return
	}
	skill.Enabled = !skill.Enabled
	a.skillMenu = skillMenu{}
}

func (a *app) skillPickerLayout() layout {
	p, v := a.picker, &a.view
	rows := []string{v.style(bold, truncate("  "+p.title, v.width))}
	if p.kind == "skills" {
		rows = append(rows, v.style(dim, truncate("  Choose an action", v.width)), "")
		labels := []string{"1. List skills", "2. Enable/Disable Skills"}
		descriptions := []string{"Tip: press $ to open this list directly.", "Enable or disable skills."}
		for i := range labels {
			prefix, style := "  ", ""
			if i == p.selected {
				prefix, style = "❯ ", accent+bold
			}
			left := prefix + labels[i]
			if v.width >= 52 {
				left += strings.Repeat(" ", max(2, 30-widthOf(left))) + descriptions[i]
			}
			rows = append(rows, v.style(style, truncate(left, v.width)))
		}
		rows = append(rows, "", v.style(dim, truncate(a.hints("  Press Enter to confirm or Esc to go back"), v.width)))
		return layout{rows: rows}
	}
	s := p.skillState
	rows = append(rows, v.style(dim, truncate("  Turn skills on or off. Your changes are saved automatically.", v.width)))
	if v.height < 6 {
		rows = nil
	}
	view := layout{rows: rows}
	view.append(v.inputLayout(s.query, "Type to search skills", 1))
	matches := s.matches()
	s.selected = min(s.selected, max(0, len(matches)-1))
	limit := min(8, max(0, v.height-len(view.rows)-2))
	start := min(max(0, s.selected-limit+1), max(0, len(matches)-limit))
	for at := start; at < min(len(matches), start+limit); at++ {
		skill := s.skills[matches[at]]
		prefix, style, mark := "  ", "", " "
		if at == s.selected {
			prefix, style = "❯ ", accent+bold
		}
		if skill.Enabled {
			mark = "x"
		}
		line := fmt.Sprintf("%s[%s] %s", prefix, mark, singleLine(skill.Name))
		if v.width >= 48 {
			line += strings.Repeat(" ", max(2, min(30, v.width/2)-widthOf(line))) + singleLine(skill.Description)
		}
		view.rows = append(view.rows, v.style(style, truncate(line, v.width)))
	}
	if len(matches) == 0 && limit > 0 {
		view.rows = append(view.rows, v.style(dim, truncate("  No matching skills", v.width)))
	}
	keys := a.host.Config.TUI
	help := fmt.Sprintf("  Space/Enter Toggle   %s/%s Select   Esc Back", keys.Hint("picker", "previous"), keys.Hint("picker", "next"))
	view.rows = append(view.rows, "", v.style(dim, truncate(a.hints(help), v.width)))
	return view
}
