package core

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"sync"
	"time"

	"clara-code/pkg/model"
	"clara-code/pkg/util/errs"
	"clara-code/pkg/util/id"
)

// Session owns a single foreground run over an injected Agent.
type Session struct {
	engine     Agent
	store      Store
	mu         sync.Mutex
	state      State
	changed    chan struct{}
	sequence   uint64
	eventBytes int
	active     *Run
	closed     bool
}

func NewSession(ctx context.Context, engine Agent, store Store) (*Session, error) {
	if engine == nil {
		return nil, errs.New(errs.CodeInvalidArgument, "session requires an Agent")
	}
	if store == nil {
		store = NewMemoryStore()
	}
	state := State{ID: id.New("session"), Revision: 1}
	if err := store.Save(ctx, state); err != nil {
		return nil, err
	}
	return &Session{engine: engine, store: store, state: state, changed: make(chan struct{})}, nil
}

func OpenSession(ctx context.Context, engine Agent, store Store, sessionID string) (*Session, error) {
	if engine == nil || store == nil {
		return nil, errs.New(errs.CodeInvalidArgument, "session requires an Agent and store")
	}
	state, err := store.Load(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	s := &Session{engine: engine, store: store, state: state, changed: make(chan struct{})}
	for _, event := range state.Events {
		s.sequence = event.Sequence
		s.eventBytes += len(event.Data)
	}
	if state.ActiveRun != "" {
		s.resolvePending()
		if err := s.emitLocked("run.cancelled", map[string]string{"reason": "recovered interrupted run"}); err != nil {
			return nil, err
		}
		s.state.ActiveRun = ""
		if err := s.saveLocked(ctx); err != nil {
			return nil, err
		}
	}
	return s, nil
}

func (s *Session) ID() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.state.ID
}

func (s *Session) Messages() []model.Message {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.state.Messages)
}

// Run is a cancellable foreground turn.
type Run struct {
	ID               string
	cancel           context.CancelFunc
	done             chan struct{}
	step             int
	mu               sync.Mutex
	steering, follow []model.Message
	sealed           bool
	result           *AgentResult
	err              error
}

func (r *Run) Wait(ctx context.Context) (*AgentResult, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-r.done:
		return r.result, r.err
	}
}

func (r *Run) Cancel()                              { r.cancel() }
func (r *Run) Steer(message model.Message) error    { return r.queue(message, false) }
func (r *Run) FollowUp(message model.Message) error { return r.queue(message, true) }

func (r *Run) queue(message model.Message, follow bool) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if message.Role != model.MessageRoleUser {
		return errs.New(errs.CodeInvalidArgument, "queued input must be a user message")
	}
	if r.sealed {
		return errs.ErrConflict
	}
	if len(r.steering)+len(r.follow) >= 32 {
		return errs.New(errs.CodeLimit, "turn input queue is full")
	}
	if follow {
		r.follow = append(r.follow, message)
	} else {
		r.steering = append(r.steering, message)
	}
	return nil
}

func (r *Run) take(idle, seal bool) []model.Message {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := r.steering
	r.steering = nil
	if idle {
		out = append(out, r.follow...)
		r.follow = nil
	}
	if seal && len(out) == 0 {
		r.sealed = true
	}
	return out
}

func (s *Session) Active() *Run {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.active
}

func (s *Session) Start(ctx context.Context, call AgentStreamCall) (*Run, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil, errs.ErrConflict
	}
	if s.active != nil {
		return nil, errs.ErrBusy
	}
	if len(call.Messages) == 0 && call.Prompt == "" {
		return nil, errs.New(errs.CodeInvalidArgument, "a new message is required")
	}
	input := slices.Clone(call.Messages)
	if call.Prompt != "" {
		input = append(input, model.NewUserMessage(call.Prompt, call.Files...))
		call.Prompt = ""
		call.Files = nil
	}
	call.Messages = append(slices.Clone(s.state.Messages), input...)
	previous := s.state
	sequence, eventBytes := s.sequence, s.eventBytes
	s.state.Events = slices.Clone(s.state.Events)
	s.state.Messages = slices.Clone(call.Messages)
	runCtx, cancel := context.WithCancel(ctx)
	run := &Run{ID: id.New("run"), cancel: cancel, done: make(chan struct{})}
	s.state.ActiveRun = run.ID
	if err := s.emitLocked("run.started", input); err != nil {
		cancel()
		s.state = previous
		return nil, err
	}
	if err := s.saveLocked(ctx); err != nil {
		cancel()
		s.state = previous
		s.sequence, s.eventBytes = sequence, eventBytes
		return nil, err
	}
	s.active = run
	go s.execute(runCtx, run, call)
	return run, nil
}

func (s *Session) execute(ctx context.Context, run *Run, call AgentStreamCall) {
	defer run.cancel()
	hostChunk, hostCall, hostResult, hostStep := call.OnChunk, call.OnToolCall, call.OnToolResult, call.OnStepFinish
	hostModel, hostHistory := call.OnModelMessages, call.OnHistory
	hostStepStart := call.OnStepStart
	call.OnStepStart = func(step int) error {
		s.mu.Lock()
		run.step = step
		s.mu.Unlock()
		if hostStepStart != nil {
			return hostStepStart(step)
		}
		return nil
	}
	hostSteer, hostFollow := call.Steer, call.FollowUp
	var queueErr error
	drain := func(idle bool) []model.Message {
		var items []model.Message
		if idle && hostFollow != nil {
			items = hostFollow()
		}
		if !idle && hostSteer != nil {
			items = hostSteer()
		}
		items = append(items, run.take(idle, idle && len(items) == 0)...)
		if len(items) > 0 {
			queueErr = errors.Join(queueErr, s.commit(ctx, func() { s.state.Messages = append(s.state.Messages, items...) }))
			if queueErr != nil {
				run.cancel()
				return nil
			}
		}
		return items
	}
	call.Steer = func() []model.Message { return drain(false) }
	call.FollowUp = func() []model.Message { return drain(true) }
	call.OnChunk = func(part model.StreamPart) error {
		switch part.Type {
		case model.StreamPartTypeTextDelta, model.StreamPartTypeReasoningDelta, model.StreamPartTypeToolInputDelta:
			if err := s.Emit(ctx, string(part.Type), part); err != nil {
				return err
			}
		}
		if hostChunk != nil {
			return hostChunk(part)
		}
		return nil
	}
	call.OnHistory = func(history []model.Message) error {
		if err := s.commit(ctx, func() { s.state.Messages = slices.Clone(history) }); err != nil {
			return err
		}
		if hostHistory != nil {
			return hostHistory(history)
		}
		return nil
	}
	call.OnModelMessages = func(messages []model.Message) error {
		if err := s.commit(ctx, func() {
			s.state.Staged = slices.Clone(messages)
			for _, msg := range messages {
				for _, part := range msg.Content {
					if item, ok := model.AsMessagePart[model.ToolCallPart](part); ok && !item.ProviderExecuted {
						s.state.Pending = append(s.state.Pending, Pending{Call: item})
					}
				}
			}
		}); err != nil {
			return err
		}
		if hostModel != nil {
			return hostModel(messages)
		}
		return nil
	}
	call.OnToolCall = func(item model.ToolCallContent) error {
		if err := s.Emit(ctx, "tool.call", item); err != nil {
			return err
		}
		if hostCall != nil {
			return hostCall(item)
		}
		return nil
	}
	call.OnToolResult = func(item model.ToolResultContent) error {
		if err := s.commit(ctx, func() {
			for i, pending := range s.state.Pending {
				if pending.Call.ToolCallID == item.ToolCallID {
					s.state.Pending[i].Result = &model.ToolResultPart{ToolCallID: item.ToolCallID, Output: item.Result, ClientMetadata: item.ClientMetadata}
					break
				}
			}
		}); err != nil {
			return err
		}
		if err := s.Emit(ctx, "tool.result", item); err != nil {
			return err
		}
		if hostResult != nil {
			return hostResult(item)
		}
		return nil
	}
	call.OnStepFinish = func(step StepResult) error {
		if err := s.commit(ctx, func() {
			s.state.Messages = append(s.state.Messages, step.Messages...)
			s.state.Pending = nil
			s.state.Staged = nil
		}); err != nil {
			return err
		}
		if err := s.Emit(ctx, "step.completed", step.Response); err != nil {
			return err
		}
		if hostStep != nil {
			return hostStep(step)
		}
		return nil
	}
	result, err := s.engine.Stream(ctx, call)
	err = errors.Join(err, queueErr)
	run.mu.Lock()
	run.sealed = true
	late := append(run.steering, run.follow...)
	run.steering, run.follow = nil, nil
	run.mu.Unlock()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.resolvePending()
	s.state.Messages = append(s.state.Messages, late...)
	kind, detail := "run.completed", ""
	if err != nil {
		kind, detail = "run.failed", err.Error()
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			kind = "run.cancelled"
		}
	}
	err = errors.Join(err, s.emitLocked(kind, map[string]string{"error": detail}))
	s.state.ActiveRun = ""
	persist, stop := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer stop()
	if saveErr := s.saveLocked(persist); saveErr != nil {
		err = errors.Join(err, saveErr)
		if events := s.state.Events; len(events) > 0 {
			terminal := &events[len(events)-1]
			body, _ := json.Marshal(map[string]string{"error": err.Error()})
			s.eventBytes += len(body) - len(terminal.Data)
			terminal.Kind, terminal.Data = "run.failed", body
		}
	}
	run.result, run.err = result, err
	s.active = nil
	close(run.done)
}

func (s *Session) resolvePending() {
	staged := len(s.state.Staged) > 0
	s.state.Messages = append(s.state.Messages, s.state.Staged...)
	s.state.Staged = nil
	if len(s.state.Pending) == 0 {
		return
	}
	assistant, results := model.Message{Role: model.MessageRoleAssistant}, model.Message{Role: model.MessageRoleTool}
	for _, pending := range s.state.Pending {
		assistant.Content = append(assistant.Content, pending.Call)
		part := pending.Result
		if part == nil {
			part = &model.ToolResultPart{ToolCallID: pending.Call.ToolCallID, Output: model.ToolResultOutputContentError{Error: errors.New("interrupted before a result was recorded. Check the workspace before retrying")}}
		}
		results.Content = append(results.Content, *part)
	}
	if !staged {
		s.state.Messages = append(s.state.Messages, assistant)
	}
	s.state.Messages = append(s.state.Messages, results)
	s.state.Pending = nil
}

func (s *Session) commit(ctx context.Context, update func()) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	update()
	persist, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer cancel()
	return s.saveLocked(persist)
}

func (s *Session) saveLocked(ctx context.Context) error {
	state := s.state
	state.Revision++
	if err := s.store.Save(ctx, state); err != nil {
		return err
	}
	s.state.Revision = state.Revision
	return nil
}

func (s *Session) Close(ctx context.Context) error {
	s.mu.Lock()
	run := s.active
	if !s.closed {
		s.closed = true
		close(s.changed)
		s.changed = make(chan struct{})
	}
	s.mu.Unlock()
	if run == nil {
		return nil
	}
	run.Cancel()
	_, err := run.Wait(ctx)
	if errors.Is(err, context.Canceled) && ctx.Err() == nil {
		return nil
	}
	return err
}
