package terminal

import "strings"

type commandSpec struct {
	name, description string
	args              bool
	required          bool
	busy              bool
}

var commands = []commandSpec{
	{name: "/model", description: "choose what model and reasoning effort to use", args: true},
	{name: "/skills", description: "list skills or enable/disable skills"},
	{name: "/attach", description: "attach a local image", args: true, required: true, busy: true},
	{name: "/review", description: "review current changes and find issues", args: true},
	{name: "/new", description: "start a new chat"},
	{name: "/resume", description: "resume a saved chat", args: true},
	{name: "/fork", description: "fork the current chat"},
	{name: "/clear", description: "clear the terminal and start a new chat"},
	{name: "/compact", description: "summarize conversation to free context"},
	{name: "/plan", description: "switch to Plan mode"},
	{name: "/agent", description: "switch to Agent mode"},
	{name: "/status", description: "show session configuration and token usage", busy: true},
	{name: "/diff", description: "show staged, unstaged and untracked changes"},
	{name: "/ps", description: "view running background terminals", busy: true},
	{name: "/stop", description: "stop all background terminals or a given ID", args: true, busy: true},
	{name: "/pwd", description: "show the current working directory", busy: true},
	{name: "/mcp", description: "list MCP servers and connection status", busy: true},
	{name: "/goal", description: "set a goal for a long-running task", args: true, required: true},
	{name: "/pause", description: "pause the active goal", busy: true},
	{name: "/continue", description: "continue the paused goal"},
	{name: "/follow", description: "queue a follow-up instruction", args: true, required: true, busy: true},
	{name: "/cancel", description: "interrupt the active turn", busy: true},
	{name: "/help", description: "show commands and keyboard shortcuts", busy: true},
	{name: "/exit", description: "exit Clara Code", busy: true},
	{name: "/quit", description: "exit Clara Code", busy: true},
}

type commandMenu struct {
	text     string
	selected int
	hidden   bool
}

func findCommand(name string) (commandSpec, bool) {
	for _, spec := range commands {
		if spec.name == name {
			return spec, true
		}
	}
	return commandSpec{}, false
}

func (a *app) menuMatches() []commandSpec {
	menu := &a.commandMenu
	if menu.text != a.editor.text {
		menu.text = a.editor.text
		menu.selected = 0
		menu.hidden = false
	}
	if menu.hidden || !strings.HasPrefix(a.editor.text, "/") || strings.ContainsAny(a.editor.text, " \t\n") {
		return nil
	}
	var exact, prefix []commandSpec
	filter := strings.ToLower(a.editor.text)
	for _, spec := range commands {
		if filter == "/" && spec.name == "/quit" {
			continue
		}
		if spec.name == filter {
			exact = append(exact, spec)
		} else if strings.HasPrefix(spec.name, filter) {
			prefix = append(prefix, spec)
		}
	}
	matches := append(exact, prefix...)
	menu.selected = min(menu.selected, max(0, len(matches)-1))
	return matches
}

func (a *app) commandRows(available int) []string {
	matches := a.menuMatches()
	if len(matches) == 0 || available <= 0 {
		return nil
	}
	p := &a.view
	limit := min(8, available)
	start := min(max(0, a.commandMenu.selected-limit+1), max(0, len(matches)-limit))
	nameWidth := 0
	for _, spec := range matches {
		nameWidth = max(nameWidth, widthOf(spec.name))
	}
	nameWidth = min(nameWidth, max(8, p.width/3))
	var lines []string
	for i := start; i < min(len(matches), start+limit); i++ {
		spec := matches[i]
		description := spec.description
		if a.run != nil && !spec.busy {
			description += " (after current turn)"
		}
		style := dim
		if i == a.commandMenu.selected {
			style = accent + bold
		}
		if p.width < 32 {
			lines = append(lines, p.style(style, truncate("  "+spec.name, p.width)))
			continue
		}
		left := "  " + truncate(spec.name, nameWidth)
		left += strings.Repeat(" ", max(1, nameWidth+4-widthOf(left)))
		lines = append(lines, p.style(style, left+truncate(description, max(1, p.width-widthOf(left)))))
	}
	return lines
}

func (a *app) handleCommandMenu(input key) (handled, exit bool) {
	matches := a.menuMatches()
	if len(matches) == 0 {
		return false, false
	}
	menu := &a.commandMenu
	switch input.code {
	case "escape":
		menu.hidden = true
	case "up":
		menu.selected = (menu.selected + len(matches) - 1) % len(matches)
	case "down":
		menu.selected = (menu.selected + 1) % len(matches)
	case "tab":
		spec := matches[menu.selected]
		text := spec.name
		if spec.args {
			text += " "
		}
		a.editor.replace(text)
		menu.text = text
		menu.hidden = true
	case "enter":
		spec := matches[menu.selected]
		if spec.required {
			a.editor.replace(spec.name + " ")
			menu.text = a.editor.text
			menu.hidden = true
			return true, false
		}
		if a.run != nil && !spec.busy {
			a.hint = "Finish or interrupt the current turn to use " + spec.name
			return true, false
		}
		a.editor.replace("")
		a.historyAt = -1
		menu.text = ""
		menu.hidden = false
		return true, a.submit(spec.name)
	default:
		return false, false
	}
	return true, false
}
