package terminal

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand/v2"
	"os"
	"strings"
	"time"

	"clara-code/pkg/core"
	"clara-code/pkg/core/prompt"
	"clara-code/pkg/model"
	toolapi "clara-code/pkg/tool"
	"clara-code/pkg/tool/mcp"
	"golang.org/x/term"

	"clara-code/internal/coding"
	"clara-code/internal/session"
)

type Options struct {
	Input, Output *os.File
	Host          coding.Host
	Workspace     coding.Workspace
	Resume        string
	PickSession   bool
	Version       string
}

type app struct {
	runHistoryStart      int
	resyncNeeded         bool
	ctx                  context.Context
	options              Options
	view                 painter
	title                terminalTitle
	host                 coding.Host
	space                coding.Workspace
	conv                 *coding.Conversation
	ask                  *coding.Questions
	editor               editor
	mode                 prompt.Mode
	run                  *core.Run
	events               <-chan runEvent
	stopRun              context.CancelFunc
	op                   <-chan operationResult
	stopOp               context.CancelFunc
	opLabel              string
	mcpProgress          <-chan mcp.StartupProgress
	picker               *picker
	setup                *setupForm
	question             *questionForm
	backgroundEvents     <-chan struct{}
	stopTerminalResults  chan error
	stoppingTerminals    bool
	attachments          []session.Attachment
	submittedAttachments []session.Attachment
	pendingDraft         editor
	attachmentResults    <-chan attachmentResult

	prompts        []string
	historyAt      int
	historyDraft   editor
	inputHistory   []session.InputHistoryEntry
	historyLoaded  bool
	branch         string
	hint           string
	primed         bool
	stream         string
	continued      bool
	streamRows     int
	reflow         bool
	resizeUntil    time.Time
	commandMenu    commandMenu
	skillMenu      skillMenu
	pendingInput   []string
	operationDraft editor
	activity       activityState
	toolGroup      []session.ToolActivity
	lastAnimation  time.Time
	streamID       string
	sawText        bool
	submitted      string
	dirty          bool
}

type runEvent struct {
	gap      bool
	snapshot *session.Turn
	event    core.Event
	err      error
	done     bool
	plan     string
}

type operationResult struct {
	picker      *picker
	conv        *coding.Conversation
	compaction  *session.Compaction
	draft       string
	label       string
	err         error
	attachments []session.Attachment
	plan        string
}

func Run(ctx context.Context, options Options) (err error) {
	in, out := options.Input, options.Output
	if !term.IsTerminal(int(in.Fd())) || !term.IsTerminal(int(out.Fd())) || os.Getenv("TERM") == "dumb" {
		return errors.New("clara-code needs an interactive terminal (use clara-code acp for stdio)")
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	a := &app{ctx: ctx, options: options, host: options.Host, space: options.Workspace,
		ask: coding.NewQuestions(), mode: prompt.Execute, dirty: true}
	a.view = painter{output: out, color: true}
	a.configureUI()
	if _, set := os.LookupEnv("NO_COLOR"); set {
		a.view.color = false
	}
	if err := a.resize(); err != nil {
		return err
	}
	defer func() {
		cancel()
		if a.op != nil {
			if result := <-a.op; result.conv != nil && result.conv != a.conv {
				err = errors.Join(err, result.conv.Close())
			}
		}
		if a.conv != nil {
			err = errors.Join(err, a.conv.Close())
		}
	}()
	restore, err := setupTerminal(in, out)
	if err != nil {
		return err
	}
	enableKeys, restoreKeys := keyboardMode()
	defer func() {
		err = errors.Join(err, a.view.finish(restoreKeys), a.title.close(out), restore())
	}()
	a.title.screen = os.Getenv("STY") != ""
	a.view.setCursorVisible(false)
	a.view.buf.WriteString(enableKeys)
	a.view.buf.WriteString("\x1b[?2004h")
	if !options.PickSession && options.Resume == "" && a.host.Models == nil && a.host.Config.NeedsSetup() {
		a.setup = newSetupForm(a.host.Config)
	} else {
		a.startup()
	}

	keys := make(chan key, 256)
	inputErr := make(chan error, 1)
	inputDone := make(chan struct{})
	go func() {
		defer close(inputDone)
		readInput(ctx, in, keys, inputErr)
	}()
	defer func() {
		cancel()
		<-inputDone
	}()
	resizes, stopResizes := resizeEvents(ctx, out)
	defer stopResizes()
	ticks := time.NewTicker(time.Second / 45)
	defer ticks.Stop()
	if err := a.draw(); err != nil {
		return err
	}
	if err := a.updateTitle(time.Now()); err != nil {
		return err
	}
	for {
		select {
		case <-ctx.Done():
			return nil
		case input, ok := <-keys:
			if !ok {
				select {
				case err := <-inputErr:
					return err
				default:
					return nil
				}
			}
			if a.setup != nil {
				if a.handleSetup(input) {
					return nil
				}
				if a.setup == nil {
					a.startup()
				}
			} else if a.handle(input) {
				return nil
			}
			a.dirty = true
		case <-resizes:
			if err := a.resize(); err != nil {
				return err
			}
			a.dirty = true
		case request := <-a.ask.Requests:
			a.question = newQuestionForm(request)
			a.question.helpText = a.questionHints
			a.activity.pause(time.Now())
			a.picker = nil
			a.dirty = true
		case result := <-a.attachmentResults:
			a.acceptAttachments(result)
			a.dirty = true
		case <-a.backgroundEvents:
			a.syncBackground()
			a.dirty = true
		case err := <-a.stopTerminalResults:
			a.stopTerminalResults = nil
			a.stoppingTerminals = false
			if err != nil {
				a.notice(err)
			} else {
				a.flushStream()
				a.view.message("notice", "Background terminal stop completed")
			}
			a.syncBackground()
			a.dirty = true
		case event := <-a.events:
			a.onRunEvent(event)
			a.dirty = true
		case progress := <-a.mcpProgress:
			name := singleLine(progress.Name)
			a.opLabel = "Starting MCP server " + name
			if progress.Total > 1 {
				a.opLabel = fmt.Sprintf("Starting MCP servers (%d/%d) · %s", progress.Completed, progress.Total, name)
			}
			a.dirty = true
		case result := <-a.op:
			a.op = nil
			a.mcpProgress = nil
			a.stopOp()
			a.stopOp = nil
			a.opLabel = ""
			if result.compaction != nil {
				a.view.cell(compactionCell(result.compaction))
			}
			if result.err != nil {
				if a.conv == nil && !a.options.PickSession {
					if errors.Is(result.err, context.Canceled) {
						return nil
					}
					return result.err
				}
				a.notice(result.err)
				a.editor.insert(a.operationDraft.text)
				a.operationDraft.replace("")
				if result.plan != "" {
					a.confirmPlan(result.plan)
				}
				if a.conv == nil && a.options.PickSession {
					a.openResume(true)
				}
			} else if result.picker != nil {
				a.picker = result.picker
				a.editor.insert(a.operationDraft.text)
				a.operationDraft.replace("")
			} else if result.conv != nil {
				if err := a.install(result); err != nil {
					return err
				}
				if result.plan != "" {
					a.implementPlan(planHandoff + "\n\n" + result.plan)
				}
			} else if result.label != "" || result.compaction != nil {
				if result.label != "" {
					a.view.message("notice", result.label)
				}
				a.editor.insert(a.operationDraft.text)
				a.operationDraft.replace("")
			}
			a.dirty = true
		case <-ticks.C:
			now := time.Now()
			if (a.run != nil || a.op != nil || len(a.toolGroup) > 0) && a.question == nil && now.Sub(a.lastAnimation) >= a.animationInterval() {
				a.lastAnimation = now
				a.dirty = true
			}
			if a.dirty && now.After(a.resizeUntil) {
				if err := a.draw(); err != nil {
					return err
				}
			}
			if err := a.updateTitle(now); err != nil {
				return err
			}
		}
	}
}

func (a *app) startup() {
	if a.options.PickSession {
		a.openResume(true)
		return
	}
	label := ""
	if a.options.Resume != "" {
		label = "Resumed conversation"
	}
	a.conversationOperation("Opening conversation", func(ctx context.Context, report func(mcp.StartupProgress)) operationResult {
		return a.open(ctx, a.options.Resume, false, -1, "", label, report)
	})
}

func (a *app) resize() error {
	width, height, err := term.GetSize(int(a.options.Output.Fd()))
	if err != nil {
		return err
	}
	width, height = max(1, width), max(1, height)
	if a.view.width != 0 && (width != a.view.width || height != a.view.height) {
		a.reflow = true
		a.resizeUntil = time.Now().Add(100 * time.Millisecond)
		a.editor.verticalSet = false
		if a.question != nil {
			for i := range a.question.drafts {
				a.question.drafts[i].notes.verticalSet = false
			}
		}
		if a.picker != nil && a.picker.kind == "rewind" {
			a.picker.followSelection = true
		}
	}
	a.view.width, a.view.height = width, height
	return nil
}

func (a *app) open(ctx context.Context, id string, fork bool, at int, draft, label string, report func(mcp.StartupProgress)) operationResult {
	space := a.options.Workspace
	var store *session.Store
	var err error
	switch {
	case fork:
		space = a.conv.Workspace
		store, err = a.conv.Store.Fork(ctx, at)
	case id != "":
		store, err = session.Open(a.host.Config.Home, id, space.Root)
		if err == nil {
			space, err = coding.ResolveWorkspace(store.Meta().Cwd)
			if err == nil && space.Root != store.Meta().Project {
				err = errors.New("the session's project directory changed")
			}
		}
	default:
		space = a.space
		profile := a.host.Config.DefaultModel
		store = session.NewDraft(a.host.Config.Home, session.Meta{
			Project: space.Root, Cwd: space.Cwd, Model: profile, Effort: string(a.host.Config.Models[profile].Effort),
		})
	}
	if err != nil {
		if store != nil {
			err = errors.Join(err, store.Close())
		}
		return operationResult{err: err}
	}
	next, err := a.host.OpenWithMCPProgress(ctx, space, store, a.ask, report)
	if err != nil {
		return operationResult{err: err}
	}
	if ctx.Err() != nil {
		return operationResult{err: errors.Join(ctx.Err(), next.Close())}
	}
	return operationResult{conv: next, draft: draft, label: label}
}

func (a *app) install(result operationResult) error {
	if a.conv != nil {
		if err := a.conv.Close(); err != nil {
			a.notice(err)
		}
		a.view.resetHistory()
	}
	a.conv, a.space = result.conv, result.conv.Workspace
	a.host = a.conv.Host
	a.host.Config = a.conv.CurrentConfig()
	a.configureUI()
	a.backgroundEvents = a.conv.BackgroundUpdates()
	a.stopTerminalResults = nil
	a.stoppingTerminals = false
	a.toolGroup = nil
	a.branch = a.space.Branch(a.ctx)
	a.historyAt = -1
	a.hint, a.primed = "", false
	a.picker = nil
	a.commandMenu = commandMenu{}
	a.activity.retry = nil
	if result.plan == "" {
		a.setDraft(result.draft, result.attachments)
	}
	meta := a.conv.Store.Meta()
	a.view.welcome(a.options.Version, modelLabel(meta), a.space.Cwd)
	a.loadInputHistory()
	seedHistory := len(a.inputHistory) == 0
	a.prompts = nil
	if meta.LegacyPathContext {
		a.view.message("notice", "Paths now refer to the real filesystem. Older /foo paths referred to the project root; read the intended target again before editing.")
	}
	boundary := transcriptBoundaries(meta, a.conv.Store.Compactions())
	count := a.conv.Store.TurnCount()
	for i := 0; i <= count; i++ {
		if err := a.ctx.Err(); err != nil {
			return err
		}
		for _, cell := range boundary(i) {
			a.view.cell(cell)
		}
		if i == count {
			break
		}
		turn, err := a.conv.Store.ReadTurn(i)
		if err != nil {
			a.notice(err)
			a.prompts = append(a.prompts, "[unavailable turn]")
		} else {
			a.prompts = append(a.prompts, turn.Prompt)
			if seedHistory && i >= count-1000 {
				a.inputHistory = append(a.inputHistory, session.InputHistoryEntry{SessionID: a.conv.Session.ID(), Text: turn.Prompt, Attachments: turn.Attachments})
			}
			a.view.message("user", turn.Prompt)
			a.replayAttachments(turn.Attachments)
			for _, cell := range turn.Cells {
				if a.view.cell(cell) {
					a.reflow = true
				}
			}
			a.view.message(turnCompletedKind, turnCompletion(turn))
		}
		if a.view.buf.Len() >= 64<<10 {
			if _, err := a.view.buf.WriteTo(a.view.output); err != nil {
				return err
			}
		}
	}
	switch result.label {
	case "", "Cleared conversation", "Rewound conversation", "Forked conversation":
	default:
		a.view.message("notice", result.label+" · "+meta.ID)
	}
	a.warnMCPStartup()
	a.syncBackground()
	a.editor.insert(a.operationDraft.text)
	a.operationDraft.replace("")
	return nil
}

func (a *app) operation(label string, work func(context.Context) operationResult) {
	ctx, cancel := context.WithCancel(a.ctx)
	out := make(chan operationResult, 1)
	a.operationDraft.replace("")
	a.activity.start(time.Now())
	a.op, a.stopOp, a.opLabel = out, cancel, label
	go func() { out <- work(ctx) }()
}

func (a *app) conversationOperation(label string, work func(context.Context, func(mcp.StartupProgress)) operationResult) {
	updates := make(chan mcp.StartupProgress, 1)
	a.mcpProgress = updates
	a.operation(label, func(ctx context.Context) operationResult {
		return work(ctx, func(progress mcp.StartupProgress) {
			// The connection loop is the sole sender. Replace any unread update.
			select {
			case <-updates:
			default:
			}
			updates <- progress
		})
	})
}

func (a *app) switchSession(id string, fork bool, at int, draft, label string, attachments ...session.Attachment) {
	if a.conv != nil && id == a.conv.Session.ID() {
		a.picker = nil
		return
	}
	a.picker = nil
	a.conversationOperation(label, func(ctx context.Context, report func(mcp.StartupProgress)) operationResult {
		result := a.open(ctx, id, fork, at, draft, label, report)
		result.attachments = attachments
		return result
	})
}

func (a *app) start(text string, review bool) {
	state, err := a.conv.Store.Load(a.ctx, a.conv.Session.ID())
	if err != nil {
		a.notice(err)
		return
	}
	cursor := uint64(0)
	if n := len(state.Events); n > 0 {
		cursor = state.Events[n-1].Sequence
	}
	ctx, cancel := context.WithCancel(a.ctx)
	files, fileErr := coding.AttachmentFiles(a.host.Config.Home, a.attachments, a.host.Config.Images)
	if fileErr != nil {
		cancel()
		a.notice(fileErr)
		a.setDraft(text, a.attachments)
		return
	}
	planning := a.mode == prompt.Plan
	if planning {
		ctx = session.WithPlanning(ctx)
	}
	compactions := a.conv.Store.CompactionCount()
	run, err := a.conv.StartWithFiles(ctx, a.mode, text, review, files)
	if compaction := compactionSince(a.conv.Store, compactions); compaction != nil {
		a.view.cell(compactionCell(compaction))
	}
	if err != nil {
		cancel()
		a.notice(err)
		a.setDraft(text, a.attachments)
		return
	}
	a.run, a.stopRun = run, cancel
	a.submitted = text
	a.submittedAttachments = append([]session.Attachment(nil), a.attachments...)
	a.attachments = nil
	a.activity.start(time.Now())
	a.toolGroup = nil
	a.streamID = ""
	a.runHistoryStart = a.view.droppedCells + len(a.view.history)
	a.view.message("user", text)
	a.replayAttachments(a.submittedAttachments)
	events := make(chan runEvent, 128)
	a.events = events
	conversation := a.conv.Session
	go watch(ctx, conversation, a.conv.Store, run, cursor, planning, events)
}

func watch(ctx context.Context, conversation *core.Session, store *session.Store, run *core.Run, cursor uint64, planning bool, out chan<- runEvent) {
	send := func(event runEvent) bool {
		select {
		case out <- event:
			return true
		case <-ctx.Done():
			return false
		}
	}
	for {
		event, err := conversation.NextEvent(ctx, cursor)
		var gap *core.GapError
		switch {
		case errors.As(err, &gap):
			cursor = gap.Earliest - 1
			if !send(runEvent{gap: true}) {
				return
			}
			if snapshot := store.ActiveTurn(); snapshot != nil && snapshot.ID == run.ID {
				if !send(runEvent{snapshot: snapshot}) {
					return
				}
				cursor = max(cursor, snapshot.Cursor)
			}
			continue
		case err != nil:
			send(runEvent{err: err, done: true})
			return
		}
		cursor = event.Sequence
		if event.RunID != run.ID {
			continue
		}
		switch event.Kind {
		case "run.completed", "run.cancelled", "run.failed":
			result, err := run.Wait(ctx)
			update := runEvent{event: event, err: err, done: true}
			if planning && event.Kind == "run.completed" && err == nil {
				update.plan = completedPlan(result)
			}
			send(update)
			return
		case "text_start", "text_delta", "text_end", "reasoning_delta", "reasoning_end", "tool.activity", "tool.result", "step.completed", "plan.updated", "goal.updated", "task.updated", "model.started", "model.responding", "model.retry", "model.error", session.ContextCompactedKind:
			if !send(runEvent{event: event}) {
				return
			}
		}
	}
}

func (a *app) onRunEvent(update runEvent) {
	if update.gap {
		a.resyncNeeded = true
		return
	}
	if update.snapshot != nil {
		a.resyncNeeded = true
		a.reloadRunDisplay(update.snapshot)
		return
	}
	if update.done {
		runID := a.run.ID
		deadline := errors.Is(update.err, context.DeadlineExceeded) || (a.activity.failure != nil && a.activity.failure.Kind == "timeout")
		cancelled := !deadline && (update.event.Kind == "run.cancelled" || errors.Is(update.err, context.Canceled))
		a.finishTools(cancelled)
		if a.resyncNeeded {
			if turn, err := a.conv.Store.ReadTurn(a.conv.Store.TurnCount() - 1); err == nil && a.run != nil && turn.ID == a.run.ID {
				a.reloadRunDisplay(&turn)
				a.stream = ""
			}
			a.resyncNeeded = false
		}
		a.flushStream()
		a.sawText = false
		a.activity.retry = nil
		a.stopRun()
		a.events, a.stopRun, a.run, a.question = nil, nil, nil, nil
		switch {
		case cancelled:
			a.view.message("notice", "Interrupted")
		case update.err != nil:
			a.runError(update.err)
			if a.editor.text == "" && len(a.pendingInput) == 0 {
				a.restoreDraft(a.submitted, a.submittedAttachments)
			}
		}
		a.submitted = ""
		a.submittedAttachments = nil
		a.activity = activityState{}
		for i := len(a.prompts); i < a.conv.Store.TurnCount(); i++ {
			turn, err := a.conv.Store.ReadTurn(i)
			if err != nil {
				a.notice(err)
				a.prompts = append(a.prompts, "[unavailable turn]")
			} else {
				a.prompts = append(a.prompts, turn.Prompt)
				if update.event.Kind == "run.completed" && update.err == nil && turn.ID == runID {
					a.view.message(turnCompletedKind, turnCompletion(turn))
				}
			}
		}
		a.branch = a.space.Branch(a.ctx)
		if len(a.pendingInput) > 0 {
			draft := a.editor
			a.editor = editor{}
			next := a.pendingDraft
			a.pendingInput = nil
			a.pendingDraft = editor{}
			a.attachments = next.imageAttachments()
			a.start(next.text, false)
			if a.run != nil {
				a.editor = draft
				a.syncDraftImages()
			} else if !a.editor.appendDraft(draft) {
				a.pendingDraft = a.editor
				a.pendingInput = []string{a.editor.text}
				a.editor = draft
				a.syncDraftImages()
			} else {
				a.syncDraftImages()
			}
		} else if update.plan != "" {
			a.confirmPlan(update.plan)
		}
		return
	}
	event := update.event
	planning := a.mode == prompt.Plan
	switch event.Kind {
	case "model.responding":
		a.activity.retry = nil
	case "model.started":
		a.activity.quiet = false
		a.streamID = ""
		a.activity.header = workingVerbs[rand.IntN(len(workingVerbs))]
		a.activity.retry = nil
		a.activity.failure = nil
	case "text_start":
		a.activity.quiet = true
		var part model.StreamPart
		if json.Unmarshal(event.Data, &part) == nil {
			if !planning && a.streamID != "" && a.streamID != part.ID {
				a.flushStream()
			}
			a.streamID = part.ID
			a.activity.retry = nil
		}
	case "text_delta":
		a.activity.quiet = true
		var chunk model.StreamPart
		if json.Unmarshal(event.Data, &chunk) == nil {
			if !planning && a.streamID != "" && a.streamID != chunk.ID {
				a.flushStream()
			}
			a.streamID = chunk.ID
			a.activity.retry = nil
			if planning && len(a.stream) < inputLimit {
				a.stream += chunk.Delta
			} else if !planning {
				a.stream += clean(chunk.Delta)
			}
			a.sawText = a.sawText || chunk.Delta != ""
		}
	case "text_end":
		var part model.StreamPart
		if json.Unmarshal(event.Data, &part) == nil && (a.streamID == part.ID || a.streamID == "") {
			if !planning {
				a.flushStream()
			}
			a.streamID = ""
		}
	case "reasoning_delta", "reasoning_end":
		var part model.StreamPart
		if json.Unmarshal(event.Data, &part) == nil && len(part.ProviderMetadata.ReasoningSummary) > 0 {
			title := summaryTitle(strings.Join(part.ProviderMetadata.ReasoningSummary, "\n"))
			if title != "" {
				a.activity.header = title
			}
			a.activity.retry = nil
		}
	case "model.retry":
		var retry session.RetryStatus
		if json.Unmarshal(event.Data, &retry) == nil {
			a.activity.retry = &retry
			a.activity.retryUntil = time.Now().Add(time.Duration(retry.DelayMS) * time.Millisecond)
		}
	case "model.error":
		var failure session.Failure
		if json.Unmarshal(event.Data, &failure) == nil {
			a.activity.failure = &failure
		}
	case "tool.activity":
		var activity session.ToolActivity
		if json.Unmarshal(event.Data, &activity) == nil {
			a.onToolActivity(activity)
		}
	case session.ContextCompactedKind:
		a.flushStream()
		var compaction session.Compaction
		if json.Unmarshal(event.Data, &compaction) != nil {
			compaction = session.Compaction{}
		}
		a.view.cell(compactionCell(&compaction))
	case "step.completed":
		a.activity.retry = nil
		var response model.Response
		if json.Unmarshal(event.Data, &response) == nil {
			if planning && (!a.sawText || a.stream != "") {
				a.stream = planResponseText(response.Content)
			} else if !a.sawText {
				a.stream = clean(response.Content.Text())
			}
		}
		a.flushStream()
		a.sawText = false
	case "tool.result":
		var result model.ToolResultContent
		if json.Unmarshal(event.Data, &result) == nil && result.ToolName == "request_user_input" {
			cell := session.ToolOutput(result)
			a.view.message(cell.Kind, cell.Text)
		}
	case "plan.updated":
		var plan toolapi.PlanState
		if json.Unmarshal(event.Data, &plan) == nil && len(plan.Steps) > 0 {
			var lines []string
			for _, step := range plan.Steps {
				lines = append(lines, step.Status+"  "+step.Step)
			}
			a.view.message("activity", "Updated plan\n"+strings.Join(lines, "\n"))
		}
	case "goal.updated", "task.updated":
		var state struct{ Status, Name string }
		if json.Unmarshal(event.Data, &state) == nil && state.Status != "" {
			a.view.message("activity", strings.TrimSpace(state.Name+" "+state.Status))
		}
	}
}

func (a *app) flushStream() {
	if a.stream == "" {
		return
	}
	cells := []session.Cell{{Kind: "assistant", Text: a.stream}}
	if a.mode == prompt.Plan {
		cells = session.PlanCells(a.stream, false)
	}
	if a.continued {
		for _, cell := range cells {
			a.view.remember(cell.Kind, clean(cell.Text))
		}
		a.view.rebuild()
	} else {
		for _, cell := range cells {
			a.view.message(cell.Kind, cell.Text)
		}
	}
	a.stream, a.continued, a.streamRows = "", false, 0
}

func (a *app) notice(err error) {
	a.view.message("error", a.host.Config.Redact(err.Error()))
}

func (a *app) draw() error {
	a.dirty = false
	p := &a.view
	if a.reflow {
		if a.picker != nil && a.picker.kind == "rewind" {
			p.clearScreen()
		} else {
			p.rebuild()
		}
		a.streamRows, a.continued = 0, false
		a.reflow = false
	}
	pane := a.pane()
	height := p.contentHeight()
	pane = pane.slice(0, height)
	if a.stream != "" {
		rows := a.streamDisplayRows()
		live := max(1, min(5, height-len(pane.rows)))
		commitTo := max(0, len(rows)-live)
		if commitTo < a.streamRows {
			p.rebuild()
			a.streamRows = 0
		}
		for a.streamRows < commitTo {
			p.commit(rows[a.streamRows])
			a.streamRows++
			a.continued = true
		}
		available := max(0, height-len(pane.rows))
		tail := rows[min(a.streamRows, len(rows)):]
		if len(tail) > available {
			tail = tail[len(tail)-available:]
		}
		pane = pane.prepend(tail)
	}
	return p.frame(pane)
}

func (a *app) pane() layout {
	p := &a.view
	if a.setup != nil {
		return a.setup.layout(p, a.hints)
	}
	if a.picker != nil {
		if a.picker.history != nil {
			return a.historySearchLayout()
		}
		if a.picker.kind == planPicker {
			return layout{rows: a.planPickerRows()}
		}
		if a.picker.kind == "skills" || a.picker.kind == "skill-toggle" {
			return a.skillPickerLayout()
		}
		if a.picker.kind == "diff" {
			return layout{rows: a.diffRows()}
		}
		if a.picker.kind == "rewind" {
			return layout{rows: a.rewindRows()}
		}
		help := a.hints("↑/↓ Select   Enter Confirm   Esc Back")
		return layout{rows: a.picker.rows(p, help)}
	}
	if a.question != nil {
		return a.question.layout(p)
	}
	if a.conv == nil && a.op != nil && !a.options.PickSession {
		return layout{rows: a.activityRows(time.Now())}
	}
	height := p.contentHeight()
	matches := a.menuMatches()
	skillsVisible := a.skillMatches()
	chrome := min(3, max(0, height-1))
	borderRows := min(2, height-1)
	input := p.inputBox(a.editor, "Run /review on my current changes", min(7, max(1, height-chrome)), borderRows)
	var footer []string
	if len(matches) > 0 {
		menuHeight := max(0, height-len(input.rows))
		footer = a.commandRows(menuHeight)
	} else if skillsVisible {
		footer = a.skillRows(max(0, height-len(input.rows)))
	} else if height > 3 {
		footer = []string{a.statusLine()}
	}
	var lines []string
	hasActivity := false
	if a.stream == "" {
		for _, tool := range a.toolGroup {
			lines = append(lines, p.toolRows(tool, time.Now())...)
		}
		activity := a.activityRows(time.Now())
		hasActivity = len(activity) > 0
		lines = append(lines, activity...)
	}
	if summary := a.backgroundSummary(); summary != "" && !hasActivity {
		lines = append(lines, p.style(dim, truncate("  "+summary, p.width)), "")
	}
	if maxRows := max(0, height-len(input.rows)-len(footer)); len(lines) > maxRows {
		lines = lines[len(lines)-maxRows:]
	}
	view := layout{rows: lines}
	view.append(input)
	view.rows = append(view.rows, footer...)
	return view
}

func modelLabel(meta session.Meta) string {
	return singleLine(strings.TrimSpace(meta.Model + " " + meta.Effort))
}
