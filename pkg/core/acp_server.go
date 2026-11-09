package core

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"hash/crc32"
	"io"
	"reflect"
	"slices"
	"strings"
	"sync"

	sdk "github.com/coder/acp-go-sdk"

	"clara-code/pkg/model"
)

type ACPBinding struct {
	Session       *Session
	PrepareInput  func(context.Context, []model.Message) ([]model.Message, error)
	PreparePrompt func(context.Context, sdk.PromptRequest, []model.Message) ([]model.Message, error)
	Start         func(context.Context, AgentStreamCall) (*Run, error)
	ConfigOptions func() []sdk.SessionConfigOption
	CanClose      func(context.Context, sdk.CloseSessionRequest) error
	Plan          func(context.Context) []sdk.PlanEntry
	PromptMeta    func(*Run) map[string]any
	ToolMeta      func(model.ToolCallContent) map[string]any
	SetOption     func(context.Context, sdk.SetSessionConfigOptionRequest) error
	Configure     func(context.Context) AgentStreamCall
	Close         func(context.Context) error
	// MatchCwd optionally checks whether a requested cwd identifies this binding's workspace.
	MatchCwd func(string) bool
}

type ACPServerConfig struct {
	Name, Version string
	Open          func(context.Context, string, string, []sdk.McpServer) (ACPBinding, error)
	// List enables session/list and returns one page of session metadata.
	List func(context.Context, sdk.ListSessionsRequest) (sdk.ListSessionsResponse, error)
	// Resume enables session/resume. It opens an existing session without replaying updates.
	// Like Open, it must clean up any resources it acquires before returning an error.
	Resume      func(context.Context, sdk.ResumeSessionRequest) (ACPBinding, sdk.ResumeSessionResponse, error)
	LoadSession bool
	Images      bool
	MCPHTTP     bool
	Meta        map[string]any
	Extension   func(context.Context, string, json.RawMessage) (any, error)
}

type ACPServer struct {
	config   ACPServerConfig
	mu       sync.Mutex
	conn     *sdk.AgentSideConnection
	sessions map[sdk.SessionId]acpSession
	closed   bool
	client   sdk.ClientCapabilities
	// Serialize lifecycle calls for the same session before acquiring host resources.
	lifecycle [64]chan struct{}
}

type acpSession struct {
	ACPBinding
	cwd     string
	servers []sdk.McpServer
	resume  sdk.ResumeSessionResponse
}

func NewACPServer(config ACPServerConfig) (*ACPServer, error) {
	if config.Open == nil {
		return nil, errors.New("acp: session factory is required")
	}
	if config.Name == "" {
		config.Name = "clara-core"
	}
	if config.Version == "" {
		config.Version = "dev"
	}
	server := &ACPServer{config: config, sessions: map[sdk.SessionId]acpSession{}}
	for i := range server.lifecycle {
		server.lifecycle[i] = make(chan struct{}, 1)
	}
	return server, nil
}

func (s *ACPServer) Serve(ctx context.Context, input io.Reader, output io.Writer) error {
	conn := sdk.NewAgentSideConnection(s, output, input)
	s.mu.Lock()
	s.conn = conn
	s.mu.Unlock()
	var err error
	select {
	case <-ctx.Done():
		err = ctx.Err()
	case <-conn.Done():
	}
	s.mu.Lock()
	active := s.sessions
	s.sessions = nil
	s.closed = true
	s.mu.Unlock()
	for _, binding := range active {
		err = errors.Join(err, closeBinding(context.WithoutCancel(ctx), binding.ACPBinding))
	}
	return err
}

func (s *ACPServer) Initialize(_ context.Context, req sdk.InitializeRequest) (sdk.InitializeResponse, error) {
	s.mu.Lock()
	s.client = req.ClientCapabilities
	s.mu.Unlock()
	capabilities := sdk.SessionCapabilities{Close: &sdk.SessionCloseCapabilities{}}
	if s.config.List != nil {
		capabilities.List = &sdk.SessionListCapabilities{}
	}
	if s.config.Resume != nil {
		capabilities.Resume = &sdk.SessionResumeCapabilities{}
	}
	return sdk.InitializeResponse{
		ProtocolVersion: sdk.ProtocolVersionNumber,
		AgentInfo:       &sdk.Implementation{Name: s.config.Name, Version: s.config.Version},
		AuthMethods:     []sdk.AuthMethod{},
		AgentCapabilities: sdk.AgentCapabilities{
			Meta:                s.config.Meta,
			LoadSession:         s.config.LoadSession,
			PromptCapabilities:  sdk.PromptCapabilities{Image: s.config.Images, EmbeddedContext: true},
			McpCapabilities:     sdk.McpCapabilities{Http: s.config.MCPHTTP},
			SessionCapabilities: capabilities,
		},
	}, nil
}

func (s *ACPServer) Authenticate(context.Context, sdk.AuthenticateRequest) (sdk.AuthenticateResponse, error) {
	return sdk.AuthenticateResponse{}, nil
}

func (*ACPServer) Logout(context.Context, sdk.LogoutRequest) (sdk.LogoutResponse, error) {
	return sdk.LogoutResponse{}, nil
}

func (s *ACPServer) ListSessions(ctx context.Context, req sdk.ListSessionsRequest) (sdk.ListSessionsResponse, error) {
	if s.config.List == nil {
		return sdk.ListSessionsResponse{}, sdk.NewMethodNotFound("session/list")
	}
	response, err := s.config.List(ctx, req)
	if response.Sessions == nil {
		response.Sessions = []sdk.SessionInfo{}
	}
	return response, err
}

func (s *ACPServer) ResumeSession(ctx context.Context, req sdk.ResumeSessionRequest) (sdk.ResumeSessionResponse, error) {
	if s.config.Resume == nil {
		return sdk.ResumeSessionResponse{}, sdk.NewMethodNotFound("session/resume")
	}
	if strings.TrimSpace(string(req.SessionId)) == "" || req.Cwd == "" {
		return sdk.ResumeSessionResponse{}, sdk.NewInvalidParams("sessionId and cwd are required")
	}
	if len(req.AdditionalDirectories) > 0 {
		return sdk.ResumeSessionResponse{}, sdk.NewInvalidParams("additionalDirectories are not supported")
	}
	unlock, err := s.lockSession(ctx, req.SessionId)
	if err != nil {
		return sdk.ResumeSessionResponse{}, err
	}
	defer unlock()
	if existing, ok := s.lookup(req.SessionId); ok {
		matches := existing.cwd == req.Cwd
		if existing.MatchCwd != nil {
			matches = existing.MatchCwd(req.Cwd)
		}
		if !matches {
			return sdk.ResumeSessionResponse{}, unknownSession()
		}
		if !slices.EqualFunc(existing.servers, req.McpServers, func(a, b sdk.McpServer) bool {
			return reflect.DeepEqual(a, b)
		}) {
			return sdk.ResumeSessionResponse{}, sdk.NewInvalidParams("active session has different MCP servers; close it before reconnecting")
		}
		response := existing.resume
		if existing.ConfigOptions != nil {
			response.ConfigOptions = existing.ConfigOptions()
		}
		return response, nil
	}
	binding, response, err := s.config.Resume(ctx, req)
	if err != nil {
		return sdk.ResumeSessionResponse{}, err
	}
	_, err = s.register(ctx, req.SessionId, acpSession{
		ACPBinding: binding, cwd: req.Cwd, servers: req.McpServers, resume: response,
	})
	if err == nil && binding.ConfigOptions != nil {
		response.ConfigOptions = binding.ConfigOptions()
	}
	return response, err
}

func (*ACPServer) SetSessionMode(context.Context, sdk.SetSessionModeRequest) (sdk.SetSessionModeResponse, error) {
	return sdk.SetSessionModeResponse{}, sdk.NewMethodNotFound("session/set_mode")
}

func (s *ACPServer) SetSessionConfigOption(ctx context.Context, req sdk.SetSessionConfigOptionRequest) (sdk.SetSessionConfigOptionResponse, error) {
	var out sdk.SetSessionConfigOptionResponse
	if req.ValueId == nil {
		return out, sdk.NewInvalidParams("a string option value is required")
	}
	id := req.ValueId.SessionId
	unlock, err := s.lockSession(ctx, id)
	if err != nil {
		return out, err
	}
	defer unlock()
	b, ok := s.lookup(id)
	if !ok {
		return out, unknownSession()
	}
	if b.SetOption == nil || b.ConfigOptions == nil {
		return out, sdk.NewMethodNotFound("session/set_config_option")
	}
	if b.Session.Active() != nil {
		return out, sdk.NewInvalidParams("finish the current turn before changing configuration")
	}
	if err := b.SetOption(ctx, req); err != nil {
		return out, err
	}
	out.ConfigOptions = b.ConfigOptions()
	return out, nil
}

// ClientCapabilities returns the capabilities negotiated with the connected client.
func (s *ACPServer) ClientCapabilities() sdk.ClientCapabilities {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.client
}

// ElicitForm requests non-sensitive input from a client that supports ACP forms.
func (s *ACPServer) ElicitForm(ctx context.Context, req sdk.UnstableCreateElicitationRequest) (sdk.UnstableCreateElicitationResponse, error) {
	s.mu.Lock()
	conn, capability := s.conn, s.client.Elicitation
	s.mu.Unlock()
	if conn == nil || capability == nil || capability.Form == nil {
		return sdk.UnstableCreateElicitationResponse{}, errors.New("client does not support forms")
	}
	return conn.UnstableCreateElicitation(ctx, req)
}

func (s *ACPServer) HandleExtensionMethod(ctx context.Context, method string, params json.RawMessage) (any, error) {
	if s.config.Extension == nil {
		return nil, sdk.NewMethodNotFound(method)
	}
	return s.config.Extension(ctx, method, params)
}

func (s *ACPServer) open(ctx context.Context, id sdk.SessionId, cwd string, servers []sdk.McpServer) (ACPBinding, error) {
	if id != "" {
		unlock, err := s.lockSession(ctx, id)
		if err != nil {
			return ACPBinding{}, err
		}
		defer unlock()
		if binding, ok := s.lookup(id); ok {
			return binding.ACPBinding, nil
		}
	}
	binding, err := s.config.Open(ctx, string(id), cwd, servers)
	if err != nil {
		return ACPBinding{}, err
	}
	return s.register(ctx, id, acpSession{ACPBinding: binding, cwd: cwd, servers: servers})
}

func (s *ACPServer) register(ctx context.Context, id sdk.SessionId, binding acpSession) (ACPBinding, error) {
	cleanup := func(err error) (ACPBinding, error) {
		return ACPBinding{}, errors.Join(err, closeBinding(context.WithoutCancel(ctx), binding.ACPBinding))
	}
	if binding.Session == nil {
		return cleanup(errors.New("acp: factory returned no session"))
	}
	key := sdk.SessionId(binding.Session.ID())
	if id != "" && key != id {
		return cleanup(errors.New("acp: factory returned a different session ID"))
	}
	s.mu.Lock()
	if s.closed || ctx.Err() != nil {
		s.mu.Unlock()
		return cleanup(errors.Join(ctx.Err(), errors.New("acp: session registration interrupted")))
	}
	existing, ok := s.sessions[key]
	if !ok {
		s.sessions[key] = binding
	}
	s.mu.Unlock()
	if ok {
		return existing.ACPBinding, closeBinding(context.WithoutCancel(ctx), binding.ACPBinding)
	}
	return binding.ACPBinding, nil
}

func (s *ACPServer) NewSession(ctx context.Context, req sdk.NewSessionRequest) (sdk.NewSessionResponse, error) {
	binding, err := s.open(ctx, "", req.Cwd, req.McpServers)
	if err != nil {
		return sdk.NewSessionResponse{}, sdk.NewInvalidParams(err.Error())
	}
	response := sdk.NewSessionResponse{SessionId: sdk.SessionId(binding.Session.ID())}
	if binding.ConfigOptions != nil {
		response.ConfigOptions = binding.ConfigOptions()
	}
	return response, nil
}

func (s *ACPServer) LoadSession(ctx context.Context, req sdk.LoadSessionRequest) (sdk.LoadSessionResponse, error) {
	if !s.config.LoadSession {
		return sdk.LoadSessionResponse{}, sdk.NewMethodNotFound("session/load")
	}
	binding, err := s.open(ctx, req.SessionId, req.Cwd, req.McpServers)
	if err != nil {
		return sdk.LoadSessionResponse{}, err
	}
	for _, msg := range binding.Session.Messages() {
		for _, part := range msg.Content {
			text, ok := model.AsMessagePart[model.TextPart](part)
			if !ok || text.Text == "" {
				continue
			}
			var update sdk.SessionUpdate
			switch msg.Role {
			case model.MessageRoleUser:
				update = sdk.UpdateUserMessageText(text.Text)
			case model.MessageRoleAssistant:
				update = sdk.UpdateAgentMessageText(text.Text)
			default:
				continue
			}
			if err := s.send(ctx, req.SessionId, update); err != nil {
				return sdk.LoadSessionResponse{}, err
			}
		}
	}
	response := sdk.LoadSessionResponse{}
	if binding.ConfigOptions != nil {
		response.ConfigOptions = binding.ConfigOptions()
	}
	return response, nil
}

func (s *ACPServer) Prompt(ctx context.Context, req sdk.PromptRequest) (sdk.PromptResponse, error) {
	unlock, lockErr := s.lockSession(ctx, req.SessionId)
	if lockErr != nil {
		return sdk.PromptResponse{}, lockErr
	}
	defer func() {
		if unlock != nil {
			unlock()
		}
	}()
	binding, ok := s.lookup(req.SessionId)
	if !ok {
		return sdk.PromptResponse{}, unknownSession()
	}
	message, err := fromContent(req.Prompt, s.config.Images)
	if err != nil {
		return sdk.PromptResponse{}, sdk.NewInvalidParams(err.Error())
	}
	var call AgentStreamCall
	if binding.Configure != nil {
		call = binding.Configure(ctx)
	}
	messages := []model.Message{message}
	if binding.PreparePrompt != nil {
		messages, err = binding.PreparePrompt(ctx, req, messages)
	} else if binding.PrepareInput != nil {
		messages, err = binding.PrepareInput(ctx, messages)
	}
	if err != nil {
		return sdk.PromptResponse{}, sdk.NewInvalidParams(err.Error())
	}
	call.Messages = append(call.Messages, messages...)
	chunk, start, finish := call.OnChunk, call.OnToolCall, call.OnToolResult
	call.OnChunk = func(part model.StreamPart) error {
		var err error
		switch part.Type {
		case model.StreamPartTypeTextDelta:
			err = s.send(ctx, req.SessionId, sdk.UpdateAgentMessageText(part.Delta))
		case model.StreamPartTypeReasoningDelta:
			err = s.send(ctx, req.SessionId, sdk.UpdateAgentThoughtText(part.Delta))
		}
		if err == nil && chunk != nil {
			return chunk(part)
		}
		return err
	}
	call.OnToolCall = func(item model.ToolCallContent) error {
		update := sdk.StartToolCall(sdk.ToolCallId(item.ToolCallID), item.ToolName,
			sdk.WithStartKind(toolKind(item.ToolName)), sdk.WithStartStatus(sdk.ToolCallStatusInProgress),
			sdk.WithStartRawInput(json.RawMessage(item.Input)))
		if binding.ToolMeta != nil {
			update.ToolCall.Meta = binding.ToolMeta(item)
		}
		err := s.send(ctx, req.SessionId, update)
		if err == nil && start != nil {
			return start(item)
		}
		return err
	}
	call.OnToolResult = func(item model.ToolResultContent) error {
		status := sdk.ToolCallStatusCompleted
		if _, failed := item.Result.(model.ToolResultOutputContentError); failed {
			status = sdk.ToolCallStatusFailed
		}
		body, _ := json.Marshal(item.Result)
		if len(body) > 128<<10 {
			body = append(body[:128<<10], []byte("\n[output truncated]")...)
		}
		err := s.send(ctx, req.SessionId, sdk.UpdateToolCall(sdk.ToolCallId(item.ToolCallID), sdk.WithUpdateStatus(status),
			sdk.WithUpdateContent([]sdk.ToolCallContent{sdk.ToolContent(sdk.TextBlock(string(body)))})))
		if err == nil && finish != nil {
			err = finish(item)
		}
		if err == nil && item.ToolName == "update_plan" && binding.Plan != nil {
			if entries := binding.Plan(ctx); entries != nil {
				err = s.send(ctx, req.SessionId, sdk.UpdatePlan(entries...))
			}
		}
		return err
	}
	startRun := binding.Start
	if startRun == nil {
		startRun = binding.Session.Start
	}
	run, err := startRun(ctx, call)
	unlock()
	unlock = nil
	if err != nil {
		return sdk.PromptResponse{}, err
	}
	result, err := run.Wait(context.WithoutCancel(ctx))
	var meta map[string]any
	if binding.PromptMeta != nil {
		meta = binding.PromptMeta(run)
	}
	switch {
	case errors.Is(err, context.Canceled):
		return sdk.PromptResponse{Meta: meta, StopReason: sdk.StopReasonCancelled}, nil
	case err != nil:
		return sdk.PromptResponse{}, err
	case result.Response.FinishReason == model.FinishReasonLength:
		return sdk.PromptResponse{Meta: meta, StopReason: sdk.StopReasonMaxTokens}, nil
	default:
		return sdk.PromptResponse{Meta: meta, StopReason: sdk.StopReasonEndTurn}, nil
	}
}

func (s *ACPServer) Cancel(_ context.Context, req sdk.CancelNotification) error {
	if binding, ok := s.lookup(req.SessionId); ok {
		if run := binding.Session.Active(); run != nil {
			run.Cancel()
		}
	}
	return nil
}

func (s *ACPServer) CloseSession(ctx context.Context, req sdk.CloseSessionRequest) (sdk.CloseSessionResponse, error) {
	unlock, err := s.lockSession(ctx, req.SessionId)
	if err != nil {
		return sdk.CloseSessionResponse{}, err
	}
	defer unlock()
	binding, ok := s.lookup(req.SessionId)
	if !ok {
		return sdk.CloseSessionResponse{}, unknownSession()
	}
	if binding.CanClose != nil {
		if err := binding.CanClose(ctx, req); err != nil {
			return sdk.CloseSessionResponse{}, err
		}
	}
	s.mu.Lock()
	delete(s.sessions, req.SessionId)
	s.mu.Unlock()
	return sdk.CloseSessionResponse{}, closeBinding(ctx, binding.ACPBinding)
}

func closeBinding(ctx context.Context, binding ACPBinding) error {
	var err error
	if binding.Session != nil {
		err = binding.Session.Close(ctx)
	}
	if binding.Close != nil {
		err = errors.Join(err, binding.Close(ctx))
	}
	return err
}

func (s *ACPServer) lockSession(ctx context.Context, id sdk.SessionId) (func(), error) {
	lock := s.lifecycle[crc32.ChecksumIEEE([]byte(id))%uint32(len(s.lifecycle))]
	select {
	case lock <- struct{}{}:
		if err := ctx.Err(); err != nil {
			<-lock
			return nil, err
		}
		return func() { <-lock }, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (s *ACPServer) lookup(id sdk.SessionId) (acpSession, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	binding, ok := s.sessions[id]
	return binding, ok
}

func (s *ACPServer) send(ctx context.Context, id sdk.SessionId, update sdk.SessionUpdate) error {
	s.mu.Lock()
	conn := s.conn
	s.mu.Unlock()
	if conn == nil {
		return nil
	}
	return conn.SessionUpdate(ctx, sdk.SessionNotification{SessionId: id, Update: update})
}

func unknownSession() error {
	return &sdk.RequestError{Code: -32002, Message: "session not found"}
}

func fromContent(blocks []sdk.ContentBlock, images bool) (model.Message, error) {
	message := model.Message{Role: model.MessageRoleUser}
	for _, block := range blocks {
		var part model.MessagePart
		switch {
		case block.Text != nil:
			part = model.TextPart{Text: block.Text.Text}
		case block.Image != nil && images:
			if len(block.Image.Data) > 16<<20 {
				return model.Message{}, errors.New("ACP image exceeds limit")
			}
			data, err := base64.StdEncoding.DecodeString(block.Image.Data)
			if err != nil {
				return model.Message{}, err
			}
			part = model.FilePart{Data: data, MediaType: block.Image.MimeType}
		case block.ResourceLink != nil:
			part = model.TextPart{Text: fmt.Sprintf("[resource %s] %s", block.ResourceLink.Uri, block.ResourceLink.Name)}
		case block.Resource != nil && block.Resource.Resource.TextResourceContents != nil:
			text := block.Resource.Resource.TextResourceContents
			part = model.TextPart{Text: fmt.Sprintf("[resource %s]\n%s", text.Uri, text.Text)}
		default:
			return model.Message{}, errors.New("unsupported ACP prompt content")
		}
		message.Content = append(message.Content, part)
	}
	if len(message.Content) == 0 {
		return model.Message{}, errors.New("ACP prompt is empty")
	}
	return message, nil
}

func toolKind(name string) sdk.ToolKind {
	name = strings.ToLower(name)
	switch {
	case strings.Contains(name, "read"), strings.Contains(name, "list"), strings.Contains(name, "grep"), strings.Contains(name, "glob"):
		return sdk.ToolKindRead
	case strings.Contains(name, "write"), strings.Contains(name, "edit"), strings.Contains(name, "copy"):
		return sdk.ToolKindEdit
	case strings.Contains(name, "remove"), strings.Contains(name, "delete"):
		return sdk.ToolKindDelete
	case strings.Contains(name, "move"):
		return sdk.ToolKindMove
	case strings.Contains(name, "exec"), strings.Contains(name, "stdin"):
		return sdk.ToolKindExecute
	case strings.Contains(name, "search"), strings.Contains(name, "fetch"):
		return sdk.ToolKindSearch
	default:
		return sdk.ToolKindOther
	}
}

var _ sdk.Agent = (*ACPServer)(nil)
var _ sdk.AgentLoader = (*ACPServer)(nil)
