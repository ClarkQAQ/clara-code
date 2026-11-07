package terminal

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"clara-core/agent/prompt"
	"clara-core/tools"

	"clara-code/internal/session"
)

func (a *app) handle(input key) bool {
	if a.op != nil {
		if input.code == "escape" || input.code == "ctrl-c" {
			a.stopOp()
		} else {
			a.operationDraft.key(input)
		}
		return false
	}
	if a.question != nil {
		if a.question.key(input) {
			a.question = nil
			a.activity.resume(time.Now())
		} else if input.code == "escape" || input.code == "ctrl-c" {
			a.interrupt()
		}
		return false
	}
	if a.picker != nil {
		return a.handlePicker(input)
	}
	if handled, exit := a.handleCommandMenu(input); handled {
		return exit
	}
	if input.code != "escape" && a.primed {
		a.primed, a.hint = false, ""
	}
	switch input.code {
	case "ctrl-c":
		if a.run != nil {
			a.interrupt()
			return false
		}
		if a.editor.text != "" {
			a.editor.replace("")
			a.historyAt = -1
			return false
		}
		return true
	case "ctrl-d":
		if a.editor.text == "" && a.run == nil {
			return true
		}
	case "escape":
		switch {
		case a.run != nil:
			a.interrupt()
		case a.editor.text == "" && len(a.prompts) > 0:
			if a.primed {
				a.openRewind()
				a.primed, a.hint = false, ""
			} else {
				a.primed = true
				a.hint = "Press Esc again to edit a previous message"
			}
		}
		return false
	case "backtab":
		if a.run == nil {
			if a.mode == prompt.Plan {
				a.mode = prompt.Execute
			} else {
				a.mode = prompt.Plan
			}
		}
		return false
	case "enter":
		line := strings.TrimSpace(a.editor.text)
		if line == "" {
			return false
		}
		a.editor.replace("")
		a.historyAt = -1
		a.hint = ""
		return a.submit(line)
	case "up", "down":
		width := max(1, a.view.width-2)
		if strings.Contains(a.editor.text, "\n") || len(wrap(a.editor.text, width)) > 1 {
			direction := 1
			if input.code == "up" {
				direction = -1
			}
			a.editor.vertical(direction, width)
			return false
		}
		a.recall(input.code)
		return false
	case "ctrl-l":
		return false
	}
	a.editor.key(input)
	return false
}

func (a *app) recall(direction string) {
	if len(a.prompts) == 0 {
		return
	}
	if a.historyAt == -1 {
		if direction == "down" || a.editor.text != "" {
			return
		}
		a.draft = a.editor.text
		a.historyAt = len(a.prompts) - 1
	} else if direction == "up" {
		a.historyAt = max(0, a.historyAt-1)
	} else {
		a.historyAt++
	}
	if a.historyAt == len(a.prompts) {
		a.historyAt = -1
		a.editor.replace(a.draft)
		return
	}
	a.editor.replace(a.prompts[a.historyAt])
}

func (a *app) submit(line string) bool {
	command, arg := splitCommand(line)
	if command == "/exit" || command == "/quit" {
		return true
	}
	if command == "/cancel" {
		a.interrupt()
		return false
	}
	if a.conv == nil {
		return false
	}
	if command == "/help" {
		var lines []string
		for _, spec := range commands {
			if spec.name != "/quit" {
				lines = append(lines, fmt.Sprintf("%-12s %s", spec.name, spec.description))
			}
		}
		a.view.message("notice", strings.Join(lines, "\n")+"\nShift+Tab switches Agent/Plan. Esc interrupts. Esc Esc previews previous prompts. Alt+Enter adds a line.")
		return false
	}
	if command == "/pause" {
		a.pauseGoal()
		return false
	}
	if command == "/follow" {
		if arg == "" {
			a.notice(errors.New("usage: /follow TEXT"))
		} else if a.run != nil {
			a.steer(arg, true)
		} else {
			a.start(arg, false)
		}
		return false
	}
	if !strings.HasPrefix(command, "/") || strings.Contains(command[1:], "/") {
		if a.run != nil {
			a.steer(line, false)
		} else {
			a.start(line, false)
		}
		return false
	}
	spec, known := findCommand(command)
	if !known {
		a.notice(errors.New("unknown command " + command + " · type /help"))
		a.editor.replace(line)
		return false
	}
	if a.run != nil && !spec.busy {
		a.notice(errors.New("finish or cancel the current turn before using " + command))
		a.editor.replace(line)
		return false
	}
	switch command {
	case "/plan":
		a.mode = prompt.Plan
	case "/agent":
		a.mode = prompt.Execute
	case "/review":
		a.start(line, true)
	case "/model":
		a.chooseModel(arg)
	case "/compact":
		a.operation("Compacting", func(ctx context.Context) operationResult {
			err := a.conv.Compact(ctx)
			return operationResult{err: err, label: "Compacted conversation context"}
		})
	case "/new":
		a.switchSession("", false, -1, "", "New conversation")
	case "/clear":
		a.switchSession("", false, -1, "", "Cleared conversation")
	case "/status":
		a.showStatus()
	case "/pwd":
		a.view.message("notice", a.space.Cwd)
	case "/diff":
		a.diffPage(0, 0)
	case "/resume":
		if arg == "" {
			a.openResume(false)
		} else {
			a.switchSession(arg, false, -1, "", "Resumed conversation")
		}
	case "/fork":
		a.switchSession("", true, -1, "", "Forked conversation")
	case "/goal":
		if a.mode == prompt.Plan {
			a.notice(errors.New("switch to Agent mode before starting a goal"))
		} else if err := a.conv.Goal.Start(arg, 100_000, 20); err != nil {
			a.notice(fmt.Errorf("cannot start goal: %w", err))
		} else {
			a.start("Begin the explicitly requested goal: "+arg, false)
			if a.run == nil {
				_ = a.conv.Goal.Pause()
			}
		}
	case "/continue":
		if a.mode == prompt.Plan {
			a.notice(errors.New("switch to Agent mode before continuing a goal"))
		} else if err := a.conv.Goal.Resume(); err != nil {
			a.notice(fmt.Errorf("cannot continue goal: %w", err))
		} else {
			a.start("Resume the active goal.", false)
			if a.run == nil {
				_ = a.conv.Goal.Pause()
			}
		}
	case "/mcp":
		var lines []string
		for _, status := range a.conv.MCPStatuses() {
			label := fmt.Sprintf("connected (%d tools)", status.ToolCount)
			if !status.OK {
				label = status.Error
			}
			lines = append(lines, status.Name+" · "+label)
		}
		a.report("MCP servers", "No MCP servers configured", lines)
	case "/skills":
		a.chooseSkill()

	default:
		a.notice(errors.New("unknown command " + command + " · type /help"))
		a.editor.replace(line)
	}
	return false
}

func splitCommand(line string) (string, string) {
	i := strings.IndexAny(line, " \t\n")
	if i < 0 {
		return line, ""
	}
	return line[:i], strings.TrimSpace(line[i:])
}

func (a *app) report(title, empty string, lines []string) {
	if len(lines) == 0 {
		a.view.message("notice", empty)
		return
	}
	a.view.message("notice", title+"\n"+strings.Join(lines, "\n"))
}

func (a *app) steer(text string, follow bool) {
	accepted, err := a.conv.QueueInput(a.ctx, a.run, text, follow)
	if err != nil && !accepted {
		if a.conv.Session.Active() == nil {
			a.pendingInput = append(a.pendingInput, text)
			return
		}
		a.editor.replace(text)
		a.notice(err)
		return
	}
	a.flushStream()
	a.view.message("user", text)
	if err != nil {
		a.notice(fmt.Errorf("input accepted, but saving its display record failed: %w", err))
	}
}

func (a *app) interrupt() {
	if a.run == nil {
		return
	}
	if a.conv.Goal.State().Status == tools.GoalActive {
		_ = a.conv.Goal.Pause()
	}
	a.question = nil
	a.run.Cancel()
}

func (a *app) pauseGoal() {
	if a.conv.Goal.State().Status != tools.GoalActive {
		a.notice(errors.New("no active goal"))
		return
	}
	if err := a.conv.Goal.Pause(); err != nil {
		a.notice(err)
		return
	}
	if a.run != nil {
		a.interrupt()
	}
	a.view.message("notice", "Goal paused")
}

func (a *app) openResume(startup bool) {
	found, err := session.List(a.host.Config.Home, a.space.Root)
	if err != nil {
		a.notice(err)
	}
	p := &picker{kind: "resume", title: "Resume a conversation", startup: startup}
	for _, item := range found {
		text := item.FirstPrompt
		if text == "" {
			text = "Empty conversation"
		}
		p.items = append(p.items, choice{id: item.ID, label: item.Updated.Local().Format("2006-01-02 15:04") + " · " + text + " · " + item.ID})
	}
	if len(p.items) == 0 {
		if !startup {
			a.view.message("notice", "No saved conversations in this project")
			return
		}
		p.items = append(p.items, choice{label: "No saved conversations · start a new one"})
	}
	a.picker = p
}

func (a *app) openRewind() {
	p := &picker{kind: "rewind", title: "Edit a previous message", followSelection: true, selected: len(a.prompts) - 1}
	for _, text := range a.prompts {
		p.items = append(p.items, choice{label: text})
	}
	a.picker = p
	a.view.clearScreen()
}

func (a *app) chooseModel(arg string) {
	if arg != "" {
		name, effort := splitCommand(arg)
		profile, ok := a.host.Config.Models[name]
		if !ok {
			a.notice(fmt.Errorf("model %q is not configured", name))
			return
		}
		if effort == "" {
			effort = string(profile.Effort)
		}
		a.applyModel(name, effort)
		return
	}
	p := &picker{kind: "model", title: "Select model"}
	var names []string
	for name := range a.host.Config.Models {
		names = append(names, name)
	}
	slices.Sort(names)
	for i, name := range names {
		if name == a.conv.Store.Meta().Model {
			p.selected = i
		}
		p.items = append(p.items, choice{id: name, label: name})
	}
	a.picker = p
}

func (a *app) applyModel(name, effort string) {
	if err := a.conv.ChangeModel(a.ctx, name, effort); err != nil {
		a.notice(err)
		return
	}
	a.view.message("notice", "Model set to "+modelLabel(a.conv.Store.Meta()))
}
