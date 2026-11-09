package tool

import (
	"context"
	"errors"
	"fmt"
	"io"
	"maps"
	"net"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"

	"clara-code/pkg/util/errs"
	"golang.org/x/crypto/ssh"
)

// SSHConfig configures an SSH command runner.
type SSHConfig struct {
	Address         string
	User            string
	Password        string
	PrivateKey      []byte
	HostKeyCallback ssh.HostKeyCallback
	AllowInsecure   bool
	Timeout         time.Duration
}

// SSH starts commands on a remote host.
type SSH struct {
	config SSHConfig
}

// NewSSH creates an SSH runner.
func NewSSH(config SSHConfig) (*SSH, error) {
	if config.Address == "" || config.User == "" {
		return nil, errs.New(errs.CodeInvalidArgument, "SSH address and user are required")
	}
	if config.HostKeyCallback == nil {
		if !config.AllowInsecure {
			return nil, errs.New(errs.CodeForbidden, "SSH host-key callback is required")
		}
		config.HostKeyCallback = ssh.InsecureIgnoreHostKey()
	}
	return &SSH{config: config}, nil
}

func (s *SSH) Start(ctx context.Context, spec Spec) (Process, error) {
	command, err := sshCommand(spec)
	if err != nil {
		return nil, err
	}
	timeout := spec.Timeout
	if timeout <= 0 {
		timeout = 10 * time.Minute
	}
	auth, err := sshAuth(s.config)
	if err != nil {
		return nil, err
	}
	timeoutConfig := s.config.Timeout
	if timeoutConfig <= 0 {
		timeoutConfig = 20 * time.Second
	}
	dialCtx, cancel := context.WithTimeout(ctx, timeoutConfig)
	defer cancel()
	dialer := net.Dialer{Timeout: timeoutConfig}
	connection, err := dialer.DialContext(dialCtx, "tcp", s.config.Address)
	if err != nil {
		return nil, err
	}
	if err := connection.SetDeadline(time.Now().Add(timeoutConfig)); err != nil {
		return nil, errors.Join(err, connection.Close())
	}
	stopHandshake := context.AfterFunc(dialCtx, func() { _ = connection.Close() })
	defer stopHandshake()
	clientConn, channels, requests, err := ssh.NewClientConn(connection, s.config.Address, &ssh.ClientConfig{
		User: s.config.User, Auth: auth, HostKeyCallback: s.config.HostKeyCallback,
		Timeout: timeoutConfig,
	})
	if err != nil {
		connection.Close()
		return nil, err
	}
	client := ssh.NewClient(clientConn, channels, requests)
	session, err := client.NewSession()
	if err != nil {
		client.Close()
		return nil, err
	}
	if spec.TTY {
		if err := session.RequestPty("xterm", 24, 80, ssh.TerminalModes{}); err != nil {
			return nil, errors.Join(err, session.Close(), client.Close())
		}
	}
	stdin, err := session.StdinPipe()
	if err != nil {
		session.Close()
		client.Close()
		return nil, err
	}
	output := newCapture(spec.MaxOutput)
	session.Stdout = output
	session.Stderr = output
	started := time.Now()
	if err := session.Start(command); err != nil {
		session.Close()
		client.Close()
		return nil, err
	}
	if !stopHandshake() {
		return nil, errors.Join(dialCtx.Err(), client.Close())
	}
	if err := connection.SetDeadline(time.Time{}); err != nil {
		return nil, errors.Join(err, client.Close())
	}
	runCtx, cancelRun := context.WithCancel(ctx)
	if !spec.NoTimeout {
		cancelRun()
		runCtx, cancelRun = context.WithTimeout(ctx, timeout)
	}
	process := &sshProcess{
		session: session, client: client, stdin: stdin, output: output,
		done: make(chan struct{}), started: started, cancel: cancelRun,
	}
	go func() { process.finish(session.Wait()); close(process.done); cancelRun() }()
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
			_ = client.Close()
		}
	}()
	return process, nil
}

func sshAuth(config SSHConfig) ([]ssh.AuthMethod, error) {
	var methods []ssh.AuthMethod
	if len(config.PrivateKey) > 0 {
		signer, err := ssh.ParsePrivateKey(config.PrivateKey)
		if err != nil {
			return nil, err
		}
		methods = append(methods, ssh.PublicKeys(signer))
	}
	if config.Password != "" {
		methods = append(methods, ssh.Password(config.Password))
	}
	if len(methods) == 0 {
		return nil, errs.New(errs.CodeUnauthorized, "SSH requires a password or private key")
	}
	return methods, nil
}

var sshEnvName = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// SSH exec carries a single command string. Encode metadata as shell words
// and leave the requested script in the remote shell's own syntax.
func sshCommand(spec Spec) (string, error) {
	if strings.TrimSpace(spec.Command) == "" {
		return "", errs.New(errs.CodeInvalidArgument, "command is empty")
	}
	if strings.ContainsRune(spec.Command, 0) || strings.ContainsRune(spec.Workdir, 0) {
		return "", errs.New(errs.CodeInvalidArgument, "SSH command and working directory must not contain NUL bytes")
	}
	if spec.Workdir == "" && len(spec.Env) == 0 {
		return spec.Command, nil
	}

	var command strings.Builder
	if spec.Workdir != "" {
		command.WriteString("cd ")
		// An explicit relative path bypasses CDPATH and cd's special "-" operand.
		prefix := "./"
		if strings.HasPrefix(spec.Workdir, "/") {
			prefix = ""
		}
		writeSSHShellWord(&command, prefix, spec.Workdir)
		command.WriteString(" || exit\n")
	}
	if len(spec.Env) > 0 {
		command.WriteString("export")
		for _, name := range slices.Sorted(maps.Keys(spec.Env)) {
			if !sshEnvName.MatchString(name) {
				return "", errs.New(errs.CodeInvalidArgument, fmt.Sprintf("invalid SSH environment variable name %q", name))
			}
			value := spec.Env[name]
			if strings.ContainsRune(value, 0) {
				return "", errs.New(errs.CodeInvalidArgument, fmt.Sprintf("SSH environment variable %q contains a NUL byte", name))
			}
			command.WriteByte(' ')
			writeSSHShellWord(&command, name, "=", value)
		}
		command.WriteString(" || exit\n")
	}
	command.WriteString(spec.Command)
	return command.String(), nil
}

func writeSSHShellWord(command *strings.Builder, parts ...string) {
	command.WriteByte('\'')
	for _, part := range parts {
		command.WriteString(strings.ReplaceAll(part, "'", "'\\''"))
	}
	command.WriteByte('\'')
}

type sshProcess struct {
	closeMu    sync.Mutex
	closeErr   error
	writeMu    sync.Mutex
	session    *ssh.Session
	client     *ssh.Client
	stdin      io.WriteCloser
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
	closed     bool
}

func (p *sshProcess) markTimedOut() {
	p.mu.Lock()
	p.timedOut = true
	p.mu.Unlock()
}

func (p *sshProcess) finish(err error) {
	p.mu.Lock()
	if !p.finished {
		p.finished = true
		p.finishedAt = time.Now()
		p.waitErr = err
		var exit *ssh.ExitError
		if errors.As(err, &exit) {
			p.exitCode = exit.ExitStatus()
			p.waitErr = nil
		}
	}
	p.mu.Unlock()
}

func (p *sshProcess) Write(ctx context.Context, input string) error {
	return writeProcessInput(ctx, p.stdin, &p.writeMu, input)
}

func (p *sshProcess) Poll(ctx context.Context, wait time.Duration) (Poll, error) {
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
	waitErr := p.waitErr
	if p.timedOut {
		waitErr = nil
	}
	return Poll{
		Output: p.output.drain(), Running: !p.finished, ExitCode: p.exitCode,
		TimedOut: p.timedOut, Duration: commandDuration(p.started, p.finishedAt),
	}, waitErr
}

func (p *sshProcess) Kill(context.Context) error {
	return errors.Join(p.session.Signal(ssh.SIGKILL), p.session.Close())
}

func (p *sshProcess) Close() error {
	p.closeMu.Lock()
	defer p.closeMu.Unlock()
	if p.closed {
		return p.closeErr
	}
	p.closed = true
	p.cancel()
	err := p.client.Close()
	if errors.Is(err, net.ErrClosed) || errors.Is(err, io.EOF) {
		err = nil
	}
	p.closeErr = errors.Join(err, awaitProcessExit(p.done))
	return p.closeErr
}
