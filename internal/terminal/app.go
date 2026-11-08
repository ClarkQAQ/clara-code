package terminal

import (
	"context"
	"encoding/json"
	"errors"
	"math/rand/v2"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"clara-code/pkg/core"
	"clara-code/pkg/core/prompt"
	"clara-code/pkg/model"
	toolapi "clara-code/pkg/tool"
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
	picker               *picker
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
	draft          string
	branch         string
	hint           string
	primed         bool
	stream         string
	continued      bool
	streamRows     int
	reflow         bool
	resizeUntil    time.Time
	commandMenu    commandMenu
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
}

type operationResult struct {
	picker      *picker
	conv        *coding.Conversation
	draft       string
	label       string
	err         error
	attachments []session.Attachment
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
	state, err := term.MakeRaw(int(in.Fd()))
	if err != nil {
		return err
	}
	defer func() {
		err = errors.Join(err, a.view.finish(), term.Restore(int(in.Fd()), state))
	}()
	a.view.setCursorVisible(false)
	a.view.buf.WriteString("\x1b[?2004h")
	if options.PickSession {
		a.openResume(true)
	} else {
		label := ""
		if options.Resume != "" {
			label = "Resumed conversation"
		}
		result := a.open(ctx, options.Resume, false, -1, "", label)
		if result.err != nil {
			return result.err
		}
		if err := a.install(result); err != nil {
			return err
		}
	}

	keys := make(chan key, 256)
	inputErr := make(chan error, 1)
	go readInput(ctx, in, keys, inputErr)
	resizes := make(chan os.Signal, 1)
	signal.Notify(resizes, syscall.SIGWINCH)
	defer signal.Stop(resizes)
	ticks := time.NewTicker(time.Second / 45)
	defer ticks.Stop()
	if err := a.draw(); err != nil {
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
			if a.handle(input) {
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
		case result := <-a.op:
			a.op = nil
			a.stopOp()
			a.stopOp = nil
			a.opLabel = ""
			if result.err != nil {
				a.notice(result.err)
				a.editor.insert(a.operationDraft.text)
				a.operationDraft.replace("")
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
			} else if result.label != "" {
				a.view.message("notice", result.label)
				a.editor.insert(a.operationDraft.text)
				a.operationDraft.replace("")
			}
			a.dirty = true
		case <-ticks.C:
			if (a.run != nil || a.op != nil || len(a.toolGroup) > 0) && a.question == nil && time.Since(a.lastAnimation) >= a.animationInterval() {
				a.lastAnimation = time.Now()
				a.dirty = true
			}
			if a.dirty && time.Now().After(a.resizeUntil) {
				if err := a.draw(); err != nil {
					return err
				}
			}
		}
	}
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

func (a *app) open(ctx context.Context, id string, fork bool, at int, draft, label string) operationResult {
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
		store = session.New(a.host.Config.Home, session.Meta{
			Project: space.Root, Cwd: space.Cwd, Model: profile, Effort: string(a.host.Config.Models[profile].Effort),
		})
	}
	if err != nil {
		if store != nil {
			err = errors.Join(err, store.Close())
		}
		return operationResult{err: err}
	}
	next, err := a.host.Open(ctx, space, store, a.ask, nil)
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
	a.attachments = append([]session.Attachment(nil), result.attachments...)
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
	a.setDraft(result.draft, result.attachments)
	a.view.welcome(a.options.Version, modelLabel(a.conv.Store.Meta()), a.space.Cwd)
	a.prompts = nil
	if a.conv.Store.Meta().LegacyPathContext {
		a.view.message("notice", "Paths now refer to the real filesystem. Older /foo paths referred to the project root; read the intended target again before editing.")
	}
	for i := 0; i < a.conv.Store.TurnCount(); i++ {
		if err := a.ctx.Err(); err != nil {
			return err
		}
		turn, err := a.conv.Store.ReadTurn(i)
		if err != nil {
			a.notice(err)
			a.prompts = append(a.prompts, "[unavailable turn]")
		} else {
			a.prompts = append(a.prompts, turn.Prompt)
			a.view.message("user", turn.Prompt)
			a.replayAttachments(turn.Attachments)
			for _, cell := range turn.Cells {
				if a.view.cell(cell) {
					a.reflow = true
				}
			}
		}
		if a.view.buf.Len() >= 64<<10 {
			if _, err := a.view.buf.WriteTo(a.view.output); err != nil {
				return err
			}
		}
	}
	if result.label == "Rewound conversation" || result.label == "Forked conversation" {
		a.view.message("notice", "You're continuing from this point in a new conversation")
	} else if result.label != "" && result.label != "Cleared conversation" {
		a.view.message("notice", result.label+" · "+a.conv.Store.Meta().ID)
	}
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

func (a *app) switchSession(id string, fork bool, at int, draft, label string, attachments ...session.Attachment) {
	if a.conv != nil && id == a.conv.Session.ID() {
		a.picker = nil
		return
	}
	a.picker = nil
	a.operation(label, func(ctx context.Context) operationResult {
		result := a.open(ctx, id, fork, at, draft, label)
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
	run, err := a.conv.StartWithFiles(ctx, a.mode, text, review, files)
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
	go watch(ctx, conversation, a.conv.Store, run, cursor, events)
}

func watch(ctx context.Context, conversation *core.Session, store *session.Store, run *core.Run, cursor uint64, out chan<- runEvent) {
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
			_, err := run.Wait(ctx)
			send(runEvent{event: event, err: err, done: true})
			return
		case "text_start", "text_delta", "text_end", "reasoning_delta", "reasoning_end", "tool.activity", "tool.result", "step.completed", "plan.updated", "goal.updated", "task.updated", "model.started", "model.responding", "model.retry", "model.error", "context.compacted":
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
		}
		return
	}
	event := update.event
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
			if a.streamID != "" && a.streamID != part.ID {
				a.flushStream()
			}
			a.streamID = part.ID
			a.activity.retry = nil
		}
	case "text_delta":
		a.activity.quiet = true
		var chunk model.StreamPart
		if json.Unmarshal(event.Data, &chunk) == nil {
			if a.streamID != "" && a.streamID != chunk.ID {
				a.flushStream()
			}
			a.streamID = chunk.ID
			a.activity.retry = nil
			a.stream += clean(chunk.Delta)
			a.sawText = a.sawText || chunk.Delta != ""
		}
	case "text_end":
		var part model.StreamPart
		if json.Unmarshal(event.Data, &part) == nil && (a.streamID == part.ID || a.streamID == "") {
			a.flushStream()
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
	case "context.compacted":
		a.view.message("notice", "Compacted conversation context")
	case "step.completed":
		a.activity.retry = nil
		var response model.Response
		if json.Unmarshal(event.Data, &response) == nil {

			if !a.sawText {
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
	if a.continued {
		a.view.remember("assistant", a.stream)
		a.view.rebuild()
	} else {
		a.view.message("assistant", a.stream)
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
	pane = pane.slice(0, p.height)
	if a.stream != "" {
		rows := p.messageRows("assistant", a.stream)
		live := max(1, min(5, p.height-len(pane.rows)))
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
		available := max(0, p.height-len(pane.rows))
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
	if a.picker != nil {
		if a.picker.kind == "diff" {
			return layout{rows: a.diffRows()}
		}
		if a.picker.kind == "rewind" {
			return layout{rows: a.rewindRows()}
		}
		rows := a.picker.rows(p)
		if len(rows) > 0 {
			rows[len(rows)-1] = a.hints(rows[len(rows)-1])
		}
		return layout{rows: rows}
	}
	if a.question != nil {
		return a.question.layout(p)
	}
	matches := a.menuMatches()
	chrome := min(3, max(0, p.height-1))
	if len(matches) == 0 && p.height > 4 {
		chrome++
	}
	input := p.inputLayout(a.editor, "Run /review on my current changes", min(7, max(1, p.height-chrome)))
	var footer []string
	if len(matches) > 0 {
		menuHeight := max(0, p.height-min(2, chrome)-len(input.rows))
		footer = a.commandRows(menuHeight)
	} else if p.height > 3 {
		footer = []string{a.statusLine()}
		if p.height > 4 {
			footer = append(footer, "")
		}
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
	borderRows := min(2, max(0, p.height-1))
	if maxRows := max(0, p.height-borderRows-len(input.rows)-len(footer)); len(lines) > maxRows {
		lines = lines[len(lines)-maxRows:]
	}
	if p.height > 1 {
		lines = append(lines, p.style(dim, strings.Repeat("─", p.width)))
	}
	view := layout{rows: lines}
	view.append(input)
	if p.height > 2 {
		view.rows = append(view.rows, p.style(dim, strings.Repeat("─", p.width)))
	}
	view.rows = append(view.rows, footer...)
	return view
}

func modelLabel(meta session.Meta) string {
	return singleLine(strings.TrimSpace(meta.Model + " " + meta.Effort))
}
