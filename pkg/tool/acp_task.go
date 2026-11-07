package tool

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	acpsdk "github.com/coder/acp-go-sdk"

	"clara-code/pkg/core"
	"clara-code/pkg/util/errs"
	"clara-code/pkg/util/id"
)

// ACPTaskBackend exposes configured ACP Agents as subagent tasks.
type ACPTaskBackend struct {
	Agents        map[string]core.ACPExternalConfig
	MaxConcurrent int
	sem           chan struct{}

	mu    sync.Mutex
	tasks map[string]*externalTask
	names map[string]*externalTask
}

type externalTask struct {
	info    TaskInfo
	agent   *core.ACPExternal
	session acpsdk.SessionId
	run     bool
	pending []string
	done    chan struct{}
	ctx     context.Context
	cancel  context.CancelFunc
	closed  bool
}

// NewACPTaskBackend creates an ACP task backend for one trusted host scope.
func NewACPTaskBackend(agents map[string]core.ACPExternalConfig, maxConcurrent int) *ACPTaskBackend {
	if maxConcurrent <= 0 {
		maxConcurrent = 2
	}
	return &ACPTaskBackend{
		Agents: maps.Clone(agents), MaxConcurrent: maxConcurrent,
		sem:   make(chan struct{}, min(maxConcurrent, 32)),
		tasks: map[string]*externalTask{}, names: map[string]*externalTask{},
	}
}

func (b *ACPTaskBackend) Spawn(ctx context.Context, request TaskSpec) (TaskInfo, error) {
	if strings.TrimSpace(request.Name) == "" || strings.TrimSpace(request.Message) == "" || request.Fork || request.Model != "" || request.Reasoning != "" {
		return TaskInfo{}, errs.New(errs.CodeInvalidArgument, "invalid external task or unsupported model or history override")
	}
	if len(request.Message) > 256<<10 {
		return TaskInfo{}, errs.New(errs.CodeLimit, "external task brief is too large")
	}
	config, ok := b.Agents[request.Role]
	if !ok {
		return TaskInfo{}, fmt.Errorf("acp: no external Agent configured for role %q", request.Role)
	}
	select {
	case b.sem <- struct{}{}:
	default:
		return TaskInfo{}, errors.New("acp: external Agent concurrency limit reached")
	}
	release := true
	defer func() {
		if release {
			<-b.sem
		}
	}()
	agent, err := core.ConnectACPExternal(ctx, config)
	if err != nil {
		return TaskInfo{}, err
	}
	cwd := config.Cwd
	if cwd == "" {
		cwd, err = os.Getwd()
		if err != nil {
			return TaskInfo{}, errors.Join(err, agent.Close(context.WithoutCancel(ctx), ""))
		}
	}
	sessionID, err := agent.NewSession(ctx, cwd)
	if err != nil {
		return TaskInfo{}, errors.Join(err, agent.Close(context.WithoutCancel(ctx), ""))
	}
	lifetime, stop := context.WithCancel(context.WithoutCancel(ctx))
	entry := &externalTask{ctx: lifetime, cancel: stop,
		info: TaskInfo{
			ID: id.New("acp"), Name: request.Name, Role: request.Role,
			Status: "running",
		},
		agent: agent, session: sessionID, run: true, done: make(chan struct{}),
	}
	b.mu.Lock()
	if len(b.tasks) >= 256 || b.names[request.Name] != nil {
		b.mu.Unlock()
		stop()
		return TaskInfo{}, errors.Join(errs.ErrConflict, agent.Close(context.WithoutCancel(ctx), sessionID))
	}
	b.tasks[entry.info.ID] = entry
	b.names[request.Name] = entry
	b.mu.Unlock()
	release = false
	info := entry.info
	go b.execute(entry, request.Message)
	return info, nil
}

func (b *ACPTaskBackend) execute(entry *externalTask, prompt string) {
	defer func() { <-b.sem }()
	for {
		ctx, cancel := context.WithTimeout(entry.ctx, 10*time.Minute)
		text, stop, err := entry.agent.Prompt(ctx, entry.session, prompt)
		cancel()
		b.mu.Lock()
		entry.info.Result = text
		entry.info.Error = ""
		switch {
		case entry.ctx.Err() != nil || stop == acpsdk.StopReasonCancelled:
			entry.info.Status = "cancelled"
		case err != nil:
			entry.info.Status, entry.info.Error = "failed", err.Error()
		case stop == acpsdk.StopReasonEndTurn:
			entry.info.Status = "completed"
		default:
			entry.info.Status, entry.info.Error = "failed", string(stop)
		}
		if entry.info.Status == "completed" && len(entry.pending) > 0 && !entry.closed {
			prompt = entry.pending[0]
			entry.pending = entry.pending[1:]
			entry.info.Status = "running"
			b.mu.Unlock()
			continue
		}
		entry.run = false
		entry.pending = nil
		close(entry.done)
		b.mu.Unlock()
		return
	}
}

func (b *ACPTaskBackend) Send(ctx context.Context, target, text string, follow bool) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if len(text) > 256<<10 {
		return errs.New(errs.CodeLimit, "external message is too large")
	}
	entry := b.lookup(target)
	if entry == nil {
		return errs.ErrNotFound
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if entry.closed {
		return errs.ErrConflict
	}
	if entry.run {
		if !follow {
			return errs.ErrUnsupported
		}
		if len(entry.pending) >= 32 {
			return errs.New(errs.CodeLimit, "external message queue is full")
		}
		entry.pending = append(entry.pending, text)
		return nil
	}
	if !follow {
		return errs.ErrConflict
	}
	select {
	case b.sem <- struct{}{}:
	default:
		return errs.ErrBusy
	}
	entry.run = true
	entry.info.Status = "running"
	entry.done = make(chan struct{})
	go b.execute(entry, text)
	return nil
}

func (b *ACPTaskBackend) Wait(ctx context.Context, target string, timeout time.Duration) (TaskInfo, error) {
	entry := b.lookup(target)
	if entry == nil {
		return TaskInfo{}, errors.New("acp: target not found")
	}
	b.mu.Lock()
	done := entry.done
	info := entry.info
	running := entry.run
	b.mu.Unlock()
	if done == nil || !running {
		return info, nil
	}
	if timeout <= 0 || timeout > 10*time.Minute {
		timeout = 10 * time.Minute
	}
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return info, ctx.Err()
	case <-timer.C:
		return info, errors.New("acp: wait timed out")
	case <-done:
		b.mu.Lock()
		info = entry.info
		b.mu.Unlock()
		return info, nil
	}
}

func (b *ACPTaskBackend) Interrupt(ctx context.Context, target string) error {
	entry := b.lookup(target)
	if entry == nil {
		return errors.New("acp: target not found")
	}
	return entry.agent.Cancel(ctx, entry.session)
}

func (b *ACPTaskBackend) List(context.Context) ([]TaskInfo, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	out := make([]TaskInfo, 0, len(b.tasks))
	for _, entry := range b.tasks {
		out = append(out, entry.info)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

func (b *ACPTaskBackend) CloseTask(ctx context.Context, target string) error {
	entry := b.lookup(target)
	if entry == nil {
		return errors.New("acp: target not found")
	}
	b.mu.Lock()
	delete(b.tasks, entry.info.ID)
	delete(b.names, entry.info.Name)
	entry.closed = true
	entry.cancel()
	done := entry.done
	b.mu.Unlock()
	err := entry.agent.Close(ctx, entry.session)
	select {
	case <-done:
		return err
	case <-ctx.Done():
		return errors.Join(err, ctx.Err())
	}
}

func (b *ACPTaskBackend) lookup(target string) *externalTask {
	b.mu.Lock()
	defer b.mu.Unlock()
	if entry := b.tasks[target]; entry != nil {
		return entry
	}
	return b.names[target]
}

var _ TaskBackend = (*ACPTaskBackend)(nil)
