package core

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	acpsdk "github.com/coder/acp-go-sdk"

	"clara-code/pkg/util/errs"
)

// ACPExternalConfig configures an external ACP Agent process.
type ACPExternalConfig struct {
	Command           string
	Args              []string
	Env               map[string]string
	Cwd               string
	PermissionHandler func(context.Context, acpsdk.RequestPermissionRequest) (bool, error)
}

// ACPExternal is one ACP client connection to an external Agent.
type ACPExternal struct {
	process   *exec.Cmd
	conn      *acpsdk.ClientSideConnection
	client    *clientCallbacks
	info      acpsdk.InitializeResponse
	closeOnce sync.Once
	closeErr  error
}

// ConnectACPExternal starts and initializes an external ACP Agent.
func ConnectACPExternal(ctx context.Context, config ACPExternalConfig) (*ACPExternal, error) {
	if strings.TrimSpace(config.Command) == "" {
		return nil, errors.New("acp: external Agent command is required")
	}
	command := exec.CommandContext(context.WithoutCancel(ctx), config.Command, config.Args...)
	command.Dir = config.Cwd
	command.Env = append([]string(nil), os.Environ()...)
	for key, value := range config.Env {
		command.Env = append(command.Env, key+"="+value)
	}
	stdin, err := command.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := command.StdoutPipe()
	if err != nil {
		return nil, err
	}
	command.Stderr = os.Stderr
	if err := command.Start(); err != nil {
		return nil, err
	}
	callbacks := &clientCallbacks{
		permission: config.PermissionHandler, updates: map[acpsdk.SessionId]*sessionBuffer{},
	}
	conn := acpsdk.NewClientSideConnection(callbacks, stdin, stdout)
	initializeCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	info, err := conn.Initialize(initializeCtx, acpsdk.InitializeRequest{
		ProtocolVersion:    acpsdk.ProtocolVersionNumber,
		ClientCapabilities: acpsdk.ClientCapabilities{},
		ClientInfo:         &acpsdk.Implementation{Name: "clara-core", Version: "dev"},
	})
	if err != nil {
		killErr := command.Process.Kill()
		waitErr := command.Wait()
		var exit *exec.ExitError
		if errors.As(waitErr, &exit) {
			waitErr = nil
		}
		return nil, errors.Join(err, killErr, waitErr)
	}
	return &ACPExternal{process: command, conn: conn, client: callbacks, info: info}, nil
}

// Initialize returns the negotiated Agent information.
func (e *ACPExternal) Initialize() acpsdk.InitializeResponse { return e.info }

// ListSessions fetches one page of session metadata when the Agent advertises support.
func (e *ACPExternal) ListSessions(ctx context.Context, req acpsdk.ListSessionsRequest) (acpsdk.ListSessionsResponse, error) {
	if e.info.AgentCapabilities.SessionCapabilities.List == nil {
		return acpsdk.ListSessionsResponse{}, errs.New(errs.CodeUnsupported, "external ACP Agent does not support session/list")
	}
	return e.conn.ListSessions(ctx, req)
}

// ResumeSession reconnects to an existing session without requesting history replay.
func (e *ACPExternal) ResumeSession(ctx context.Context, req acpsdk.ResumeSessionRequest) (acpsdk.ResumeSessionResponse, error) {
	if e.info.AgentCapabilities.SessionCapabilities.Resume == nil {
		return acpsdk.ResumeSessionResponse{}, errs.New(errs.CodeUnsupported, "external ACP Agent does not support session/resume")
	}
	response, err := e.conn.ResumeSession(ctx, req)
	if err == nil {
		e.client.ensure(req.SessionId)
	}
	return response, err
}

// NewSession creates an ACP session.
func (e *ACPExternal) NewSession(ctx context.Context, cwd string) (acpsdk.SessionId, error) {
	response, err := e.conn.NewSession(ctx, acpsdk.NewSessionRequest{
		Cwd: cwd, McpServers: []acpsdk.McpServer{},
	})
	if err != nil {
		return "", err
	}
	e.client.ensure(response.SessionId)
	return response.SessionId, nil
}

// Prompt sends one text prompt and returns aggregated Agent text.
func (e *ACPExternal) Prompt(ctx context.Context, sessionID acpsdk.SessionId, text string) (string, acpsdk.StopReason, error) {
	buffer := e.client.ensure(sessionID)
	buffer.reset()
	response, err := e.conn.Prompt(ctx, acpsdk.PromptRequest{
		SessionId: sessionID,
		Prompt:    []acpsdk.ContentBlock{acpsdk.TextBlock(text)},
	})
	if err != nil {
		return "", "", err
	}
	buffer.mu.Lock()
	defer buffer.mu.Unlock()
	return buffer.text.String(), response.StopReason, buffer.err
}

// Updates returns one session update channel.
func (e *ACPExternal) Updates(sessionID acpsdk.SessionId) <-chan acpsdk.SessionUpdate {
	return e.client.ensure(sessionID).updates
}

// Cancel cancels the active prompt.
func (e *ACPExternal) Cancel(ctx context.Context, sessionID acpsdk.SessionId) error {
	return e.conn.Cancel(ctx, acpsdk.CancelNotification{SessionId: sessionID})
}

// Close closes the session and the external process.
func (e *ACPExternal) Close(ctx context.Context, sessionID acpsdk.SessionId) error {
	e.closeOnce.Do(func() {
		if sessionID != "" && ctx.Err() == nil {
			closeCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
			_, e.closeErr = e.conn.CloseSession(closeCtx, acpsdk.CloseSessionRequest{SessionId: sessionID})
			cancel()
			var requestErr *acpsdk.RequestError
			if errors.As(e.closeErr, &requestErr) && requestErr.Code == -32601 {
				e.closeErr = nil
			}
		}
		killErr := e.process.Process.Kill()
		if errors.Is(killErr, os.ErrProcessDone) {
			killErr = nil
		}
		waitErr := e.process.Wait()
		var exit *exec.ExitError
		if errors.As(waitErr, &exit) {
			waitErr = nil
		}
		e.closeErr = errors.Join(e.closeErr, killErr, waitErr)
	})
	return e.closeErr
}

type sessionBuffer struct {
	mu      sync.Mutex
	text    strings.Builder
	err     error
	updates chan acpsdk.SessionUpdate
}

func (b *sessionBuffer) reset() {
	b.mu.Lock()
	b.text.Reset()
	b.err = nil
	b.mu.Unlock()
}

type clientCallbacks struct {
	mu         sync.Mutex
	permission func(context.Context, acpsdk.RequestPermissionRequest) (bool, error)
	updates    map[acpsdk.SessionId]*sessionBuffer
}

func (c *clientCallbacks) ensure(id acpsdk.SessionId) *sessionBuffer {
	c.mu.Lock()
	defer c.mu.Unlock()
	buffer := c.updates[id]
	if buffer == nil {
		buffer = &sessionBuffer{updates: make(chan acpsdk.SessionUpdate, 256)}
		c.updates[id] = buffer
	}
	return buffer
}

func (c *clientCallbacks) SessionUpdate(_ context.Context, params acpsdk.SessionNotification) error {
	buffer := c.ensure(params.SessionId)
	if params.Update.AgentMessageChunk != nil {
		if text := params.Update.AgentMessageChunk.Content.Text; text != nil {
			buffer.mu.Lock()
			if buffer.text.Len()+len(text.Text) <= 1<<20 {
				buffer.text.WriteString(text.Text)
			} else {
				buffer.err = errors.New("external Agent text exceeds the size limit")
			}
			buffer.mu.Unlock()
		}
	}
	select {
	case buffer.updates <- params.Update:
	default:
	}
	return nil
}

func (c *clientCallbacks) RequestPermission(
	ctx context.Context,
	params acpsdk.RequestPermissionRequest,
) (acpsdk.RequestPermissionResponse, error) {
	allow := true
	if c.permission != nil {
		value, err := c.permission(ctx, params)
		if err != nil {
			return acpsdk.RequestPermissionResponse{}, err
		}
		allow = value
	}
	option := "reject-once"
	if allow {
		option = "allow-once"
	}
	return acpsdk.RequestPermissionResponse{
		Outcome: acpsdk.NewRequestPermissionOutcomeSelected(acpsdk.PermissionOptionId(option)),
	}, nil
}

func (c *clientCallbacks) ReadTextFile(context.Context, acpsdk.ReadTextFileRequest) (acpsdk.ReadTextFileResponse, error) {
	return acpsdk.ReadTextFileResponse{}, errors.New("acp: client file read capability is not advertised")
}

func (c *clientCallbacks) WriteTextFile(context.Context, acpsdk.WriteTextFileRequest) (acpsdk.WriteTextFileResponse, error) {
	return acpsdk.WriteTextFileResponse{}, errors.New("acp: client file write capability is not advertised")
}

func (c *clientCallbacks) CreateTerminal(context.Context, acpsdk.CreateTerminalRequest) (acpsdk.CreateTerminalResponse, error) {
	return acpsdk.CreateTerminalResponse{}, errors.New("acp: client terminal capability is not advertised")
}

func (c *clientCallbacks) TerminalOutput(context.Context, acpsdk.TerminalOutputRequest) (acpsdk.TerminalOutputResponse, error) {
	return acpsdk.TerminalOutputResponse{}, errors.New("acp: client terminal capability is not advertised")
}

func (c *clientCallbacks) WaitForTerminalExit(context.Context, acpsdk.WaitForTerminalExitRequest) (acpsdk.WaitForTerminalExitResponse, error) {
	return acpsdk.WaitForTerminalExitResponse{}, errors.New("acp: client terminal capability is not advertised")
}

func (c *clientCallbacks) KillTerminal(context.Context, acpsdk.KillTerminalRequest) (acpsdk.KillTerminalResponse, error) {
	return acpsdk.KillTerminalResponse{}, errors.New("acp: client terminal capability is not advertised")
}

func (c *clientCallbacks) ReleaseTerminal(context.Context, acpsdk.ReleaseTerminalRequest) (acpsdk.ReleaseTerminalResponse, error) {
	return acpsdk.ReleaseTerminalResponse{}, errors.New("acp: client terminal capability is not advertised")
}

var _ acpsdk.Client = (*clientCallbacks)(nil)
