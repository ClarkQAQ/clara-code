package terminal

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"clara-code/pkg/core/prompt"
	"clara-code/pkg/model"
	"clara-code/pkg/tool"

	"clara-code/internal/coding"
	"clara-code/internal/session"
	"clara-code/pkg/util/localtime"
)

func (a *app) handle(input key) bool {
	input = a.bindKey(input)
	if a.op == nil && a.picker == nil && a.question == nil {
		if input.code == "ctrl-v" {
			a.loadAttachments(nil, true)
			return false
		}
		if input.code == "ctrl-g" {
			a.manageAttachments()
			return false
		}
		if input.code == "paste" {
			if paths, recognized, err := coding.ImagePaths(input.text, a.space.Cwd); recognized {
				if a.attachmentResults != nil {
					a.editor.insert(input.text)
					return false
				}
				if err != nil {
					a.notice(err)
					a.editor.insert(input.text)
				} else if !a.loadAttachments(paths, false, input.text) {
					a.editor.insert(input.text)
				}
				return false
			}
		}
	}

	if a.op != nil {
		if input.code == "escape" || input.code == "ctrl-c" {
			a.stopOp()
		} else {
			a.operationDraft.key(input)
		}
		return false
	}
	if a.question != nil {
		interrupt := input.code == "ctrl-c" || (input.code == "escape" && !a.question.editingNotes())
		if a.question.key(input) {
			a.question = nil
			a.activity.resume(time.Now())
		} else if interrupt {
			a.interrupt()
		}
		return false
	}
	if a.picker != nil {
		return a.handlePicker(input)
	}
	if a.historyAt >= 0 && (input.code == "up" || input.code == "down") {
		a.recall(input.code)
		return false
	}
	if a.handleSkillMenu(input) {
		return false
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
		if a.editor.text != "" || len(a.attachments) > 0 {
			a.editor.replace("")
			a.attachments = nil
			a.historyAt = -1
			return false
		}
		return true
	case "ctrl-d":
		if a.editor.text == "" && len(a.attachments) == 0 && a.run == nil {
			return true
		}
	case "escape":
		switch {
		case a.run != nil:
			a.interrupt()
		case a.editor.text == "" && len(a.attachments) == 0 && len(a.prompts) > 0:
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
		if a.attachmentResults != nil {
			a.hint = "Wait for the image to finish loading"
			return false
		}
		a.syncDraftImages()
		line := strings.TrimSpace(a.editor.text)
		if line == "" && len(a.attachments) == 0 {
			return false
		}
		a.editor.replace("")
		a.historyAt = -1
		a.hint = ""
		return a.submit(line)
	case "up", "down":
		width := max(1, a.view.width-2)
		if len(a.editor.images) > 0 || strings.Contains(a.editor.text, "\n") || len(wrap(a.editor.text, width)) > 1 {
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
	before := a.editor.text
	a.editor.key(input)
	if a.editor.text != before {
		a.historyAt = -1
	}
	a.syncDraftImages()
	return false
}

func (a *app) submit(line string) bool {
	a.recordInput(line)
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
		a.view.message("notice", strings.Join(lines, "\n")+"\n"+a.hints("Shift+Tab switches Agent/Plan. Esc interrupts. Esc Esc previews previous prompts. Alt+Enter adds a line.")+"\n"+a.host.Config.TUI.Hint("composer", "paste_image")+" pastes an image; "+a.host.Config.TUI.Hint("composer", "attachments")+" manages attachments.")
		return false
	}
	if command == "/pause" {
		a.pauseGoal()
		return false
	}
	if command == "/follow" {
		if arg == "" && len(a.attachments) == 0 {
			a.notice(errors.New("usage: /follow TEXT or attach an image"))
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
	case "/attach":
		a.editor.replace(line)
		if arg == "" {
			a.notice(errors.New("usage: /attach PATH"))
		} else if paths, ok, err := coding.ImagePaths(arg, a.space.Cwd); err != nil {
			a.notice(err)
		} else if ok {
			a.editor.replace("")
			if !a.loadAttachments(paths, false, line) {
				a.editor.replace(line)
			}
		} else {
			a.editor.replace("")
			if !a.loadAttachments([]string{arg}, false, line) {
				a.editor.replace(line)
			}
		}
	case "/plan":
		a.mode = prompt.Plan
	case "/agent":
		a.mode = prompt.Execute
	case "/review":
		a.start(line, true)
	case "/model":
		a.chooseModel(arg)
	case "/compact":
		mode := a.mode
		a.operation("Compacting", func(ctx context.Context) operationResult {
			err := a.conv.Compact(ctx, mode)
			return operationResult{err: err, label: "Compacted conversation context"}
		})
	case "/new":
		a.switchSession("", false, -1, "", "New conversation")
	case "/clear":
		a.switchSession("", false, -1, "", "Cleared conversation")
	case "/status":
		a.showStatus()
	case "/ps":
		a.showTerminals()
	case "/stop":
		a.stopTerminals(arg)
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
		a.skillsMenu()

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
	files, fileErr := coding.AttachmentFiles(a.host.Config.Home, a.attachments, a.host.Config.Images)
	if fileErr != nil {
		a.notice(fileErr)
		a.setDraft(text, a.attachments)
		return
	}
	accepted, err := a.conv.QueueFiles(a.ctx, a.run, text, follow, files)
	if err != nil && !accepted {
		if a.conv.Session.Active() == nil {
			var draft editor
			draft.restoreImages(text, a.attachments)
			if !a.pendingDraft.appendDraft(draft) {
				a.setDraft(text, a.attachments)
				a.notice(errors.New("queued input exceeds the input size limit"))
				return
			}
			a.pendingInput = append(a.pendingInput, text)
			a.attachments = nil
			return
		}
		a.setDraft(text, a.attachments)
		a.notice(err)
		return
	}
	a.flushStream()
	a.view.message("user", text)
	a.replayAttachments(a.attachments)
	a.attachments = nil
	if err != nil {
		a.notice(fmt.Errorf("input accepted, but saving its display record failed: %w", err))
	}
}

func (a *app) interrupt() {
	if a.run == nil {
		return
	}
	if a.conv.Goal.State().Status == tool.GoalActive {
		_ = a.conv.Goal.Pause()
	}
	a.question = nil
	a.run.Cancel()
}

func (a *app) pauseGoal() {
	if a.conv.Goal.State().Status != tool.GoalActive {
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
		p.items = append(p.items, choice{id: item.ID, label: localtime.DateTime(item.Updated) + " · " + text + " · " + item.ID})
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
	if len(a.host.Config.Providers) > 1 {
		p := &picker{kind: "provider", title: "Select provider"}
		names := make([]string, 0, len(a.host.Config.Providers))
		for name := range a.host.Config.Providers {
			names = append(names, name)
		}
		slices.Sort(names)
		for _, name := range names {
			label := name
			if display := a.host.Config.Providers[name].Name; display != "" {
				label = display + " · " + name
			}
			p.items = append(p.items, choice{id: name, label: label})
		}
		a.picker = p
		return
	}
	a.showModels(a.host.Config.ModelProvider)
}
func (a *app) showModels(provider string) {
	config := a.host.Config
	if len(config.Providers) > 0 {
		var err error
		config, err = config.SelectProvider(provider)
		if err != nil {
			a.notice(err)
			return
		}
	}
	p := &picker{kind: "model", title: "Select model", provider: provider}
	var names []string
	for name, m := range config.Models {
		if m.Visibility != "hidden" {
			names = append(names, name)
		}
	}
	slices.SortFunc(names, func(a, b string) int {
		if config.Models[a].Priority != config.Models[b].Priority {
			return config.Models[a].Priority - config.Models[b].Priority
		}
		return strings.Compare(a, b)
	})
	for i, name := range names {
		m := config.Models[name]
		label := name
		if m.DisplayName != "" {
			label = m.DisplayName + " · " + name
		}
		if m.Images() {
			label += " · image"
		}
		if m.Description != "" {
			label += " · " + m.Description
		}
		if name == a.conv.Store.Meta().Model && provider == a.conv.Store.Meta().Provider {
			p.selected = i
		}
		p.items = append(p.items, choice{id: name, label: label})
	}
	a.picker = p
}

func (a *app) applyModel(name, effort string) {
	a.applySelection(a.host.Config.ModelProvider, name, effort)
}
func (a *app) applySelection(provider, name, effort string) {
	previous := a.conv.Store.Meta()
	if err := a.conv.ChangeSelection(a.ctx, provider, name, effort, a.mode); err != nil {
		a.notice(err)
		return
	}
	a.host = a.conv.Host
	a.host.Config = a.conv.CurrentConfig()
	profile := a.host.Config.Models[name]
	if !profile.Allows(model.ReasoningEffort(previous.Effort)) || !profile.Allows(model.ReasoningEffort(previous.PlanEffort)) {
		a.view.message("notice", "Unsupported reasoning settings were reset for the selected model.")
	}
	a.view.message("notice", "Model set to "+modelLabel(a.conv.Store.Meta())+" · "+a.conv.ModeEffort(a.mode))
}
