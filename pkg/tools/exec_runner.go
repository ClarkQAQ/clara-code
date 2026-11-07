package tools

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"time"

	"clara-code/pkg/util/errs"
)

// Spec describes one command.
type Spec struct {
	Command   string
	Workdir   string
	Env       map[string]string
	Timeout   time.Duration
	NoTimeout bool
	MaxOutput int
	TTY       bool
}

// CommandResult is a completed command.
type CommandResult struct {
	Output   string
	ExitCode int
	TimedOut bool
	Duration time.Duration
}

// Poll is one reading of a running command.
type Poll struct {
	OutputOffset int64
	Output       string
	Running      bool
	ExitCode     int
	TimedOut     bool
	Duration     time.Duration
}

// Process is one running command.
type Process interface {
	Write(context.Context, string) error
	Poll(context.Context, time.Duration) (Poll, error)
	Kill(context.Context) error
	Close() error
}

// Runner starts commands.
type Runner interface {
	Start(context.Context, Spec) (Process, error)
}

// Local runs commands with the host process permissions.
type Local struct{}

// NewLocal creates a local runner.
func NewLocal() *Local { return &Local{} }

func (l *Local) Start(ctx context.Context, spec Spec) (Process, error) {
	if strings.TrimSpace(spec.Command) == "" {
		return nil, errs.New(errs.CodeInvalidArgument, "command is empty")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	timeout := spec.Timeout
	if timeout <= 0 {
		timeout = 10 * time.Minute
	}
	runCtx, cancel := context.WithCancel(ctx)
	if !spec.NoTimeout {
		cancel()
		runCtx, cancel = context.WithTimeout(ctx, timeout)
	}
	// The watcher below cancels the process group, including terminal children.
	var command *exec.Cmd
	if runtime.GOOS == "windows" {
		command = exec.CommandContext(context.WithoutCancel(runCtx), "cmd", "/C", spec.Command)
	} else {
		shell := os.Getenv("SHELL")
		if shell == "" {
			shell = "/bin/sh"
		}
		command = exec.CommandContext(context.WithoutCancel(runCtx), shell, "-lc", spec.Command)
	}
	command.Dir = spec.Workdir
	command.Env = environment(spec.Env)
	command.WaitDelay = 2 * time.Second
	output := newCapture(spec.MaxOutput)
	stdin, drain, err := startProcess(command, spec.TTY, output)
	if err != nil {
		cancel()
		return nil, err
	}
	process := &localProcess{command: command, stdin: stdin, output: output, done: make(chan struct{}), started: time.Now(), cancel: cancel}
	go func() {
		err := command.Wait()
		drain()
		process.finish(err)
		close(process.done)
		cancel()
	}()
	go func() {
		select {
		case <-process.done:
		case <-runCtx.Done():
			select {
			case <-process.done:
				return
			default:
			}
			if errors.Is(runCtx.Err(), context.DeadlineExceeded) {
				process.markTimedOut()
			}
			if err := process.Kill(context.Background()); err != nil {
				process.mu.Lock()
				process.killErr = err
				process.mu.Unlock()
			}
		}
	}()
	return process, nil
}

func environment(overrides map[string]string) []string {
	env := append([]string(nil), os.Environ()...)
	for key, value := range overrides {
		env = append(env, key+"="+value)
	}
	return env
}

type capture struct {
	mu            sync.Mutex
	data          []byte
	read          int
	cap           int
	dropped       int64
	snapshotValid bool
	snapshotValue string
}

func newCapture(limit int) *capture {
	if limit <= 0 || limit > 1<<20 {
		limit = 1 << 20
	}
	return &capture{cap: limit}
}

func (c *capture) Write(data []byte) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.data = append(c.data, data...)
	c.snapshotValid = false
	if len(c.data) > c.cap {
		drop := len(c.data) - c.cap
		c.dropped += int64(drop)
		c.data = append([]byte(nil), c.data[drop:]...)
		if c.read > drop {
			c.read -= drop
		} else {
			c.read = 0
		}
	}
	return len(data), nil
}

func (c *capture) drain() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.read >= len(c.data) {
		return ""
	}
	value := string(c.data[c.read:])
	c.read = len(c.data)
	return value
}

type localProcess struct {
	closeMu  sync.Mutex
	closeErr error
	writeMu  sync.Mutex
	killMu   sync.Mutex
	killSent bool
	command  *exec.Cmd
	stdin    interface {
		Write([]byte) (int, error)
		Close() error
	}
	output     *capture
	done       chan struct{}
	started    time.Time
	cancel     context.CancelFunc
	mu         sync.Mutex
	finished   bool
	finishedAt time.Time
	exitCode   int
	timedOut   bool
	waitErr    error
	killErr    error
	closed     bool
}

func (p *localProcess) markTimedOut() {
	p.mu.Lock()
	p.timedOut = true
	p.mu.Unlock()
}

func (p *localProcess) finish(err error) {
	p.mu.Lock()
	if !p.finished {
		p.finished = true
		p.finishedAt = time.Now()
		p.waitErr = err
		var exit *exec.ExitError
		if errors.As(err, &exit) {
			p.exitCode = exit.ExitCode()
			p.waitErr = nil
		}
	}
	p.mu.Unlock()
}

func (p *localProcess) Write(ctx context.Context, input string) error {
	return writeProcessInput(ctx, p.stdin, &p.writeMu, input)
}

func (p *localProcess) Poll(ctx context.Context, wait time.Duration) (Poll, error) {
	if wait <= 0 {
		wait = 5 * time.Second
	}
	timer := time.NewTimer(wait)
	defer timer.Stop()
	p.mu.Lock()
	finished := p.finished
	p.mu.Unlock()
	if !finished {
		select {
		case <-p.done:
		case <-timer.C:
		case <-ctx.Done():
			return Poll{}, ctx.Err()
		}
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	return Poll{
		Output: p.output.drain(), Running: !p.finished, ExitCode: p.exitCode,
		TimedOut: p.timedOut, Duration: commandDuration(p.started, p.finishedAt),
	}, errors.Join(p.waitErr, p.killErr)
}

func (p *localProcess) Kill(context.Context) error {
	p.killMu.Lock()
	defer p.killMu.Unlock()
	p.mu.Lock()
	finished := p.finished
	p.mu.Unlock()
	if finished || p.killSent || p.command.Process == nil {
		return nil
	}
	err := killProcess(p.command.Process)
	if err == nil {
		p.killSent = true
	}
	return err
}

func (p *localProcess) Close() error {
	p.closeMu.Lock()
	defer p.closeMu.Unlock()
	if p.closed {
		return p.closeErr
	}
	p.closed = true
	var err error
	select {
	case <-p.done:
	default:
		err = p.Kill(context.Background())
	}
	p.cancel()
	closeErr := p.stdin.Close()
	if errors.Is(closeErr, os.ErrClosed) {
		closeErr = nil
	}
	if err == nil {
		err = awaitProcessExit(p.done)
	}
	if closeErr != nil {
		closeErr = fmt.Errorf("close command stdin: %w", closeErr)
	}
	p.closeErr = errors.Join(err, closeErr)
	return p.closeErr
}

func awaitProcessExit(done <-chan struct{}) error {
	timer := time.NewTimer(3 * time.Second)
	defer timer.Stop()
	select {
	case <-done:
		return nil
	case <-timer.C:
		return errs.New(errs.CodeInternal, "command did not exit after cancellation")
	}
}

func startProcess(command *exec.Cmd, tty bool, output io.Writer) (io.WriteCloser, func(), error) {
	if tty {
		return startTerminal(command, output)
	}
	configureProcess(command)
	stdin, err := command.StdinPipe()
	if err != nil {
		return nil, nil, err
	}
	command.Stdout, command.Stderr = output, output
	if err := command.Start(); err != nil {
		return nil, nil, errors.Join(err, stdin.Close())
	}
	return stdin, func() {}, nil
}

// Commands exposes exec and write_stdin over one Runner.
type Commands struct {
	Runner  Runner
	Context context.Context
	// NoDefaultTimeout disables the implicit runtime limit, not explicit timeouts.
	NoDefaultTimeout bool
	// OnChange is called outside registry locks with immutable snapshots.
	OnChange  func(TerminalInfo)
	records   map[string]*terminalRecord
	completed []string

	mu       sync.Mutex
	sessions map[string]Process
	starting int
	closed   bool
}

// NewCommands creates an exec tool set.
func NewCommands(runner Runner) *Commands {
	return &Commands{Runner: runner, sessions: map[string]Process{}, records: map[string]*terminalRecord{}}
}

// Close stops all running commands.
func (s *Commands) Close() error {
	s.mu.Lock()
	s.closed = true
	sessions := s.sessions
	s.sessions = map[string]Process{}
	s.mu.Unlock()
	var err error
	for id, process := range sessions {
		err = errors.Join(err, s.stopProcess(context.Background(), id, process))
	}
	return err
}

// All returns exec and write_stdin.
func (s *Commands) All() []Tool {
	return []Tool{s.ExecCommand(), s.WriteStdin()}
}

type ownedProcess struct {
	Process
	cancel context.CancelFunc
}

func (p *ownedProcess) Close() error {
	defer p.cancel()
	return p.Process.Close()
}

func pollResult(prefix string, poll Poll) Result {
	result := TextResult(renderPoll(prefix, poll))
	result.IsError = poll.TimedOut || (!poll.Running && poll.ExitCode != 0)
	return result
}

func renderPoll(prefix string, poll Poll) string {
	var builder strings.Builder
	builder.WriteString(prefix)
	if poll.Output != "" {
		builder.WriteString(poll.Output)
		if !strings.HasSuffix(poll.Output, "\n") {
			builder.WriteByte('\n')
		}
	}
	fmt.Fprintf(&builder, "running: %t\n", poll.Running)
	if !poll.Running {
		fmt.Fprintf(&builder, "exit_code: %d\nduration: %s\n", poll.ExitCode, poll.Duration.Round(time.Millisecond))
	}
	if poll.TimedOut {
		builder.WriteString("timed_out: true\n")
	}
	return strings.TrimSpace(builder.String())
}

// Cancellation stops waiting; bytes already accepted by stdin cannot be recalled.
// At most one OS write remains in flight per process, and Close releases it.
func writeProcessInput(ctx context.Context, writer io.Writer, mu *sync.Mutex, input string) error {
	for !mu.TryLock() {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(10 * time.Millisecond):
		}
	}
	if err := ctx.Err(); err != nil {
		mu.Unlock()
		return err
	}
	done := make(chan error, 1)
	go func() { defer mu.Unlock(); _, err := io.WriteString(writer, input); done <- err }()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case err := <-done:
		return err
	}
}
