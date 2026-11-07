package tool

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	"clara-code/pkg/core"
	"clara-code/pkg/model"
	"clara-code/pkg/util/errs"
	"clara-code/pkg/util/id"
)

type TaskSpec struct {
	Name      string `json:"task_name"`
	Role      string `json:"role"`
	Message   string `json:"message"`
	Model     string `json:"model,omitempty"`
	Reasoning string `json:"reasoning,omitempty"`
	Fork      bool   `json:"fork_history,omitempty"`
}

type TaskInfo struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Role   string `json:"role"`
	Status string `json:"status"`
	Result string `json:"result,omitempty"`
	Error  string `json:"error,omitempty"`
}

type TaskBackend interface {
	Spawn(context.Context, TaskSpec) (TaskInfo, error)
	Send(context.Context, string, string, bool) error
	Wait(context.Context, string, time.Duration) (TaskInfo, error)
	Interrupt(context.Context, string) error
	List(context.Context) ([]TaskInfo, error)
	CloseTask(context.Context, string) error
}

type TaskSetup struct {
	Model        model.LanguageModel
	Tools        []Tool
	Instructions string
	PrepareStep  core.PrepareStepFunction
	Authorize    core.ToolAuthorizer
}

type TaskFactory func(context.Context, TaskSpec, *TaskScope) (TaskSetup, error)

type TaskManager struct {
	build    TaskFactory
	maxDepth int
	slots    chan struct{}
	mu       sync.Mutex
	tasks    map[string]*taskEntry
}

type ScopeOptions struct {
	AllowedTools []string
	History      func() []model.Message
	Authorize    core.ToolAuthorizer
	Publish      func(context.Context, TaskInfo) error
}

type TaskScope struct {
	manager  *TaskManager
	lifetime context.Context
	cancel   context.CancelFunc
	id       string
	depth    int
	options  ScopeOptions
}

type taskEntry struct {
	parent     string
	info       TaskInfo
	child      *TaskScope
	session    *core.Session
	run        *core.Run
	done       chan struct{}
	call       core.AgentStreamCall
	publishErr error
}

func NewTaskManager(build TaskFactory, concurrency, depth int) *TaskManager {
	if concurrency <= 0 {
		concurrency = 4
	}
	if depth <= 0 {
		depth = 3
	}
	return &TaskManager{build: build, maxDepth: min(depth, 16), slots: make(chan struct{}, min(concurrency, 32)), tasks: map[string]*taskEntry{}}
}

func (m *TaskManager) Scope(ctx context.Context, parent string, options ScopeOptions) *TaskScope {
	lifetime, cancel := context.WithCancel(ctx)
	options.AllowedTools = slices.Clone(options.AllowedTools)
	if parent == "" {
		parent = id.New("scope")
	}
	return &TaskScope{manager: m, lifetime: lifetime, cancel: cancel, id: parent, options: options}
}

func (s *TaskScope) Spawn(ctx context.Context, spec TaskSpec) (TaskInfo, error) {
	if err := errors.Join(ctx.Err(), s.lifetime.Err()); err != nil {
		return TaskInfo{}, err
	}
	if s.depth >= s.manager.maxDepth {
		return TaskInfo{}, errs.New(errs.CodeLimit, "delegation depth reached")
	}
	if !taskNameValid(spec.Name) || spec.Role == "" || strings.TrimSpace(spec.Message) == "" || len(spec.Message) > 256<<10 {
		return TaskInfo{}, errs.New(errs.CodeInvalidArgument, "invalid delegated task")
	}
	m := s.manager
	if m.build == nil {
		return TaskInfo{}, errs.ErrUnsupported
	}
	m.mu.Lock()
	if len(m.tasks) >= 256 || s.lookupLocked(spec.Name) != nil {
		m.mu.Unlock()
		return TaskInfo{}, errs.ErrConflict
	}
	select {
	case m.slots <- struct{}{}:
	default:
		m.mu.Unlock()
		return TaskInfo{}, errs.ErrBusy
	}
	info := TaskInfo{ID: id.New("task"), Name: spec.Name, Role: spec.Role, Status: "starting"}
	entry := &taskEntry{parent: s.id, info: info, done: make(chan struct{})}
	m.tasks[info.ID] = entry
	m.mu.Unlock()

	lifetime, cancel := context.WithCancel(s.lifetime)
	child := &TaskScope{manager: m, lifetime: lifetime, cancel: cancel, id: info.ID, depth: s.depth + 1, options: s.options}
	ready := false
	defer func() {
		if !ready {
			cancel()
			m.mu.Lock()
			delete(m.tasks, info.ID)
			close(entry.done)
			m.mu.Unlock()
			<-m.slots
		}
	}()
	setup, err := m.build(ctx, spec, child)
	if err != nil {
		return TaskInfo{}, err
	}
	if setup.Model == nil {
		return TaskInfo{}, errs.New(errs.CodeInvalidArgument, "task factory returned no model")
	}
	initial := s.filter(setup.Tools)
	child.options.AllowedTools = toolNames(initial)
	authorize := func(ctx context.Context, tool model.ToolInfo, call model.ToolCall) error {
		if !slices.Contains(child.options.AllowedTools, tool.Name) {
			return errs.ErrForbidden
		}
		if s.options.Authorize != nil {
			if err := s.options.Authorize(ctx, tool, call); err != nil {
				return err
			}
		}
		if setup.Authorize != nil {
			return setup.Authorize(ctx, tool, call)
		}
		return nil
	}
	child.options.Authorize = authorize
	engine := core.NewAgent(setup.Model, core.WithTools(initial...), core.WithSystemPrompt(setup.Instructions))
	conversation, err := core.NewSession(lifetime, engine, nil)
	if err != nil {
		return TaskInfo{}, err
	}
	child.options.History = conversation.Messages
	call := core.AgentStreamCall{AuthorizeTool: authorize, PrepareStep: func(ctx context.Context, step core.PrepareStepFunctionOptions) (context.Context, core.PrepareStepResult, error) {
		if setup.PrepareStep == nil {
			return ctx, core.PrepareStepResult{}, nil
		}
		stepCtx, prepared, err := setup.PrepareStep(ctx, step)
		if prepared.Tools != nil {
			prepared.Tools = child.filter(prepared.Tools)
		}
		return stepCtx, prepared, err
	}}
	if spec.Reasoning != "" {
		effort := model.ReasoningEffort(spec.Reasoning)
		call.ProviderOptions.ReasoningEffort = &effort
	}
	input := call
	input.Prompt = spec.Message
	if spec.Fork && s.options.History != nil {
		input.Messages = s.options.History()
	}
	run, err := conversation.Start(lifetime, input)
	if err != nil {
		return TaskInfo{}, errors.Join(err, conversation.Close(context.WithoutCancel(ctx)))
	}
	info.Status = "running"
	m.mu.Lock()
	entry.child, entry.session, entry.run, entry.info, entry.call = child, conversation, run, info, call
	m.mu.Unlock()
	ready = true
	startErr := s.publish(info)
	go s.finish(entry, run)
	return info, startErr
}

func (s *TaskScope) filter(items []Tool) []Tool {
	out := make([]Tool, 0, len(items))
	for _, tool := range items {
		if tool != nil && slices.Contains(s.options.AllowedTools, tool.Info().Name) {
			out = append(out, tool)
		}
	}
	return out
}

func toolNames(items []Tool) []string {
	out := make([]string, 0, len(items))
	for _, item := range items {
		out = append(out, item.Info().Name)
	}
	return out
}

func taskNameValid(name string) bool {
	if len(name) == 0 || len(name) > 100 {
		return false
	}
	for _, c := range name {
		valid := c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '_'
		if !valid {
			return false
		}
	}
	return true
}

func (s *TaskScope) finish(entry *taskEntry, run *core.Run) {
	result, err := run.Wait(context.Background())
	m := s.manager
	m.mu.Lock()
	info := entry.info
	m.mu.Unlock()
	info.Status, info.Result, info.Error = "completed", "", ""
	if err != nil {
		info.Status, info.Error = "failed", err.Error()
		if errors.Is(err, context.Canceled) {
			info.Status = "cancelled"
		}
	}
	if result != nil {
		info.Result = result.Response.Content.Text()
		if len(info.Result) > 64<<10 {
			info.Result = strings.ToValidUTF8(info.Result[:64<<10], "") + "\n[result truncated]"
		}
	}
	publishErr := s.publish(info)
	m.mu.Lock()
	entry.info, entry.publishErr = info, publishErr
	<-m.slots
	close(entry.done)
	m.mu.Unlock()
}

func (s *TaskScope) publish(info TaskInfo) error {
	if s.options.Publish == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(s.lifetime), 5*time.Second)
	defer cancel()
	return s.options.Publish(ctx, info)
}

func (s *TaskScope) lookupLocked(target string) *taskEntry {
	for _, entry := range s.manager.tasks {
		if entry.parent == s.id && (entry.info.ID == target || entry.info.Name == target) {
			return entry
		}
	}
	return nil
}

func (s *TaskScope) Send(ctx context.Context, target, text string, follow bool) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if len(text) == 0 || len(text) > 256<<10 {
		return errs.ErrConflict
	}
	m := s.manager
	m.mu.Lock()
	defer m.mu.Unlock()
	entry := s.lookupLocked(target)
	if entry == nil {
		return errs.ErrNotFound
	}
	if entry.run == nil {
		return errs.ErrBusy
	}
	if entry.info.Status == "running" {
		if follow {
			return entry.run.FollowUp(model.NewUserMessage(text))
		}
		return entry.run.Steer(model.NewUserMessage(text))
	}
	if !follow {
		return errs.ErrConflict
	}
	select {
	case m.slots <- struct{}{}:
	default:
		return errs.ErrBusy
	}
	call := entry.call
	call.Prompt = text
	run, err := entry.session.Start(entry.child.lifetime, call)
	if err != nil {
		<-m.slots
		return err
	}
	entry.run, entry.done, entry.info.Status = run, make(chan struct{}), "running"
	go s.finish(entry, run)
	return nil
}

func (s *TaskScope) Wait(ctx context.Context, target string, timeout time.Duration) (TaskInfo, error) {
	if timeout <= 0 || timeout > 10*time.Minute {
		timeout = 10 * time.Minute
	}
	wait, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	m := s.manager
	m.mu.Lock()
	entry := s.lookupLocked(target)
	if entry == nil {
		m.mu.Unlock()
		return TaskInfo{}, errs.ErrNotFound
	}
	done := entry.done
	m.mu.Unlock()
	select {
	case <-wait.Done():
		return TaskInfo{}, wait.Err()
	case <-done:
		m.mu.Lock()
		defer m.mu.Unlock()
		return entry.info, entry.publishErr
	}
}

func (s *TaskScope) Interrupt(ctx context.Context, target string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	m := s.manager
	m.mu.Lock()
	defer m.mu.Unlock()
	entry := s.lookupLocked(target)
	if entry == nil {
		return errs.ErrNotFound
	}
	if entry.run == nil {
		return errs.ErrBusy
	}
	entry.run.Cancel()
	return nil
}

func (s *TaskScope) List(ctx context.Context) ([]TaskInfo, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	m := s.manager
	m.mu.Lock()
	defer m.mu.Unlock()
	items := []TaskInfo{}
	for _, entry := range m.tasks {
		if entry.parent == s.id {
			items = append(items, entry.info)
		}
	}
	sort.Slice(items, func(i, j int) bool { return items[i].Name < items[j].Name })
	return items, nil
}

func (s *TaskScope) CloseTask(ctx context.Context, target string) error {
	m := s.manager
	m.mu.Lock()
	entry := s.lookupLocked(target)
	if entry == nil {
		m.mu.Unlock()
		return errs.ErrNotFound
	}
	if entry.child == nil {
		m.mu.Unlock()
		return errs.ErrBusy
	}
	delete(m.tasks, entry.info.ID)
	m.mu.Unlock()
	entry.child.cancel()
	err := errors.Join(entry.session.Close(ctx), entry.child.Close(ctx))
	select {
	case <-entry.done:
		return err
	case <-ctx.Done():
		return errors.Join(err, ctx.Err())
	}
}

func (s *TaskScope) Close(ctx context.Context) error {
	s.cancel()
	items, err := s.List(ctx)
	for _, item := range items {
		if closeErr := s.CloseTask(ctx, item.ID); closeErr != nil {
			err = errors.Join(err, fmt.Errorf("close %s: %w", item.Name, closeErr))
		}
	}
	return err
}
