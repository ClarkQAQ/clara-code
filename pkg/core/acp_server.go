package core

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"

	sdk "github.com/coder/acp-go-sdk"

	"clara-code/pkg/models"
)

type ACPBinding struct {
	Session   *Session
	Configure func(context.Context) AgentStreamCall
	Close     func(context.Context) error
}

type ACPServerConfig struct {
	Name, Version string
	Open          func(context.Context, string, string, []sdk.McpServer) (ACPBinding, error)
	LoadSession   bool
	Images        bool
	MCPHTTP       bool
}

type ACPServer struct {
	config   ACPServerConfig
	mu       sync.Mutex
	conn     *sdk.AgentSideConnection
	sessions map[sdk.SessionId]ACPBinding
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
	return &ACPServer{config: config, sessions: map[sdk.SessionId]ACPBinding{}}, nil
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
	s.sessions = map[sdk.SessionId]ACPBinding{}
	s.mu.Unlock()
	for _, binding := range active {
		err = errors.Join(err, closeBinding(context.WithoutCancel(ctx), binding))
	}
	return err
}

func (s *ACPServer) Initialize(context.Context, sdk.InitializeRequest) (sdk.InitializeResponse, error) {
	return sdk.InitializeResponse{
		ProtocolVersion: sdk.ProtocolVersionNumber,
		AgentInfo:       &sdk.Implementation{Name: s.config.Name, Version: s.config.Version},
		AuthMethods:     []sdk.AuthMethod{},
		AgentCapabilities: sdk.AgentCapabilities{
			LoadSession:         s.config.LoadSession,
			PromptCapabilities:  sdk.PromptCapabilities{Image: s.config.Images, EmbeddedContext: true},
			McpCapabilities:     sdk.McpCapabilities{Http: s.config.MCPHTTP},
			SessionCapabilities: sdk.SessionCapabilities{Close: &sdk.SessionCloseCapabilities{}},
		},
	}, nil
}

func (s *ACPServer) Authenticate(context.Context, sdk.AuthenticateRequest) (sdk.AuthenticateResponse, error) {
	return sdk.AuthenticateResponse{}, nil
}

func (*ACPServer) Logout(context.Context, sdk.LogoutRequest) (sdk.LogoutResponse, error) {
	return sdk.LogoutResponse{}, nil
}

func (*ACPServer) ListSessions(context.Context, sdk.ListSessionsRequest) (sdk.ListSessionsResponse, error) {
	return sdk.ListSessionsResponse{}, sdk.NewMethodNotFound("session/list")
}

func (*ACPServer) ResumeSession(context.Context, sdk.ResumeSessionRequest) (sdk.ResumeSessionResponse, error) {
	return sdk.ResumeSessionResponse{}, sdk.NewMethodNotFound("session/resume")
}

func (*ACPServer) SetSessionMode(context.Context, sdk.SetSessionModeRequest) (sdk.SetSessionModeResponse, error) {
	return sdk.SetSessionModeResponse{}, sdk.NewMethodNotFound("session/set_mode")
}

func (*ACPServer) SetSessionConfigOption(context.Context, sdk.SetSessionConfigOptionRequest) (sdk.SetSessionConfigOptionResponse, error) {
	return sdk.SetSessionConfigOptionResponse{}, sdk.NewMethodNotFound("session/set_config_option")
}

func (s *ACPServer) open(ctx context.Context, id sdk.SessionId, cwd string, servers []sdk.McpServer) (ACPBinding, error) {
	if id != "" {
		if binding, ok := s.lookup(id); ok {
			return binding, nil
		}
	}
	binding, err := s.config.Open(ctx, string(id), cwd, servers)
	if err != nil {
		return ACPBinding{}, err
	}
	if binding.Session == nil {
		return ACPBinding{}, errors.New("acp: factory returned no session")
	}
	key := sdk.SessionId(binding.Session.ID())
	s.mu.Lock()
	existing, ok := s.sessions[key]
	if !ok {
		s.sessions[key] = binding
	}
	s.mu.Unlock()
	if ok {
		return existing, closeBinding(context.WithoutCancel(ctx), binding)
	}
	return binding, nil
}

func (s *ACPServer) NewSession(ctx context.Context, req sdk.NewSessionRequest) (sdk.NewSessionResponse, error) {
	binding, err := s.open(ctx, "", req.Cwd, req.McpServers)
	if err != nil {
		return sdk.NewSessionResponse{}, sdk.NewInvalidParams(err.Error())
	}
	return sdk.NewSessionResponse{SessionId: sdk.SessionId(binding.Session.ID())}, nil
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
			text, ok := models.AsMessagePart[models.TextPart](part)
			if !ok || text.Text == "" {
				continue
			}
			var update sdk.SessionUpdate
			switch msg.Role {
			case models.MessageRoleUser:
				update = sdk.UpdateUserMessageText(text.Text)
			case models.MessageRoleAssistant:
				update = sdk.UpdateAgentMessageText(text.Text)
			default:
				continue
			}
			if err := s.send(ctx, req.SessionId, update); err != nil {
				return sdk.LoadSessionResponse{}, err
			}
		}
	}
	return sdk.LoadSessionResponse{}, nil
}

func (s *ACPServer) Prompt(ctx context.Context, req sdk.PromptRequest) (sdk.PromptResponse, error) {
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
	call.Messages = append(call.Messages, message)
	chunk, start, finish := call.OnChunk, call.OnToolCall, call.OnToolResult
	call.OnChunk = func(part models.StreamPart) error {
		var err error
		switch part.Type {
		case models.StreamPartTypeTextDelta:
			err = s.send(ctx, req.SessionId, sdk.UpdateAgentMessageText(part.Delta))
		case models.StreamPartTypeReasoningDelta:
			err = s.send(ctx, req.SessionId, sdk.UpdateAgentThoughtText(part.Delta))
		}
		if err == nil && chunk != nil {
			return chunk(part)
		}
		return err
	}
	call.OnToolCall = func(item models.ToolCallContent) error {
		err := s.send(ctx, req.SessionId, sdk.StartToolCall(sdk.ToolCallId(item.ToolCallID), item.ToolName,
			sdk.WithStartKind(toolKind(item.ToolName)), sdk.WithStartRawInput(json.RawMessage(item.Input))))
		if err == nil && start != nil {
			return start(item)
		}
		return err
	}
	call.OnToolResult = func(item models.ToolResultContent) error {
		status := sdk.ToolCallStatusCompleted
		if _, failed := item.Result.(models.ToolResultOutputContentError); failed {
			status = sdk.ToolCallStatusFailed
		}
		body, _ := json.Marshal(item.Result)
		err := s.send(ctx, req.SessionId, sdk.UpdateToolCall(sdk.ToolCallId(item.ToolCallID), sdk.WithUpdateStatus(status),
			sdk.WithUpdateContent([]sdk.ToolCallContent{sdk.ToolContent(sdk.TextBlock(string(body)))})))
		if err == nil && finish != nil {
			return finish(item)
		}
		return err
	}
	run, err := binding.Session.Start(ctx, call)
	if err != nil {
		return sdk.PromptResponse{}, err
	}
	result, err := run.Wait(ctx)
	switch {
	case errors.Is(err, context.Canceled):
		return sdk.PromptResponse{StopReason: sdk.StopReasonCancelled}, nil
	case err != nil:
		return sdk.PromptResponse{}, err
	case result.Response.FinishReason == models.FinishReasonLength:
		return sdk.PromptResponse{StopReason: sdk.StopReasonMaxTokens}, nil
	default:
		return sdk.PromptResponse{StopReason: sdk.StopReasonEndTurn}, nil
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
	s.mu.Lock()
	binding, ok := s.sessions[req.SessionId]
	delete(s.sessions, req.SessionId)
	s.mu.Unlock()
	if !ok {
		return sdk.CloseSessionResponse{}, unknownSession()
	}
	return sdk.CloseSessionResponse{}, closeBinding(ctx, binding)
}

func closeBinding(ctx context.Context, binding ACPBinding) error {
	err := binding.Session.Close(ctx)
	if binding.Close != nil {
		err = errors.Join(err, binding.Close(ctx))
	}
	return err
}

func (s *ACPServer) lookup(id sdk.SessionId) (ACPBinding, bool) {
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

func fromContent(blocks []sdk.ContentBlock, images bool) (models.Message, error) {
	message := models.Message{Role: models.MessageRoleUser}
	for _, block := range blocks {
		var part models.MessagePart
		switch {
		case block.Text != nil:
			part = models.TextPart{Text: block.Text.Text}
		case block.Image != nil && images:
			if len(block.Image.Data) > 16<<20 {
				return models.Message{}, errors.New("ACP image exceeds limit")
			}
			data, err := base64.StdEncoding.DecodeString(block.Image.Data)
			if err != nil {
				return models.Message{}, err
			}
			part = models.FilePart{Data: data, MediaType: block.Image.MimeType}
		case block.ResourceLink != nil:
			part = models.TextPart{Text: fmt.Sprintf("[resource %s] %s", block.ResourceLink.Uri, block.ResourceLink.Name)}
		case block.Resource != nil && block.Resource.Resource.TextResourceContents != nil:
			text := block.Resource.Resource.TextResourceContents
			part = models.TextPart{Text: fmt.Sprintf("[resource %s]\n%s", text.Uri, text.Text)}
		default:
			return models.Message{}, errors.New("unsupported ACP prompt content")
		}
		message.Content = append(message.Content, part)
	}
	if len(message.Content) == 0 {
		return models.Message{}, errors.New("ACP prompt is empty")
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
