package coding

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	wire "clara-code/pkg/appwire"
	"clara-code/pkg/core"
	"clara-code/pkg/core/prompt"
	"clara-code/pkg/model"
	"clara-code/pkg/tool/mcp"
	acpsdk "github.com/coder/acp-go-sdk"

	"clara-code/internal/session"
)

func (h Host) ServeACP(ctx context.Context, input io.Reader, output io.Writer, version string) error {
	var server *core.ACPServer
	var live sync.Map
	var err error
	server, err = core.NewACPServer(core.ACPServerConfig{
		Name: "clara-code", Version: version, LoadSession: true, Images: true, MCPHTTP: true,
		Meta: map[string]any{wire.Capability: wire.Version},
		Extension: func(ctx context.Context, method string, params json.RawMessage) (any, error) {
			return h.acpExtension(ctx, server, &live, method, params)
		},
		Open: func(ctx context.Context, id, cwd string, servers []acpsdk.McpServer) (core.ACPBinding, error) {
			return h.openACP(ctx, id, cwd, servers, false, server, &live)
		},
		List: h.listACP,
		Resume: func(ctx context.Context, req acpsdk.ResumeSessionRequest) (core.ACPBinding, acpsdk.ResumeSessionResponse, error) {
			binding, err := h.openACP(ctx, string(req.SessionId), req.Cwd, req.McpServers, true, server, &live)
			return binding, acpsdk.ResumeSessionResponse{}, err
		},
	})
	if err != nil {
		return err
	}
	return server.Serve(ctx, input, scopedACPOutput{output})
}

func (h Host) openACP(ctx context.Context, id, cwd string, servers []acpsdk.McpServer, resume bool, server *core.ACPServer, live *sync.Map) (core.ACPBinding, error) {
	if !filepath.IsAbs(cwd) {
		return core.ACPBinding{}, acpsdk.NewInvalidParams("ACP cwd must be absolute")
	}
	workspace, err := ResolveWorkspace(cwd)
	if err != nil {
		if resume && errors.Is(err, os.ErrNotExist) {
			return core.ACPBinding{}, acpSessionNotFound()
		}
		return core.ACPBinding{}, err
	}
	var store *session.Store
	if id == "" {
		store = session.NewDraft(h.Config.Home, session.Meta{Project: workspace.Root, Cwd: workspace.Cwd})
	} else {
		store, err = session.Open(h.Config.Home, id, workspace.Root)
		if err != nil {
			if resume && (errors.Is(err, os.ErrNotExist) || errors.Is(err, session.ErrProjectMismatch) ||
				errors.Is(err, session.ErrInvalidID)) {
				return core.ACPBinding{}, acpSessionNotFound()
			}
			return core.ACPBinding{}, err
		}
		meta := store.Meta()
		saved, err := ResolveWorkspace(meta.Cwd)
		if err != nil || saved.Root != meta.Project || (resume && saved.Cwd != workspace.Cwd) {
			closeErr := store.Close()
			if resume {
				if closeErr != nil {
					return core.ACPBinding{}, closeErr
				}
				return core.ACPBinding{}, acpSessionNotFound()
			}
			return core.ACPBinding{}, errors.Join(err, errors.New("the session's project directory changed"), closeErr)
		}
		workspace = saved
	}
	extra, err := acpServers(servers, workspace.Cwd)
	if err != nil {
		return core.ACPBinding{}, errors.Join(err, store.Close())
	}
	var questions *Questions
	client := server.ClientCapabilities()
	if client.Elicitation != nil && client.Elicitation.Form != nil {
		questions = NewQuestions()
	}
	conversation, err := h.Open(ctx, workspace, store, questions, extra)
	if err != nil {
		return core.ACPBinding{}, errors.New(h.Config.Redact(err.Error()))
	}
	id = conversation.Session.ID()
	entry := &acpConversation{chat: conversation, mode: "agent"}
	live.Store(id, entry)
	lifetime, stop := context.WithCancel(context.WithoutCancel(ctx))
	if questions != nil {
		go serveACPQuestions(lifetime, server, conversation, id)
	}
	return core.ACPBinding{
		Session: conversation.Session,
		PreparePrompt: func(ctx context.Context, req acpsdk.PromptRequest, messages []model.Message) ([]model.Message, error) {
			return entry.preparePrompt(ctx, req, messages, hasClaraExtension(client))
		},
		Close: func(context.Context) error {
			live.CompareAndDelete(id, entry)
			stop()
			return conversation.Close()
		},
		ConfigOptions: entry.options,
		CanClose: func(ctx context.Context, req acpsdk.CloseSessionRequest) error {
			if req.Meta[wire.Namespace+"idleOnly"] != true || !hasClaraExtension(client) {
				return nil
			}
			if conversation.Session.Active() != nil {
				return errors.New("session still has a running turn")
			}
			for _, terminal := range conversation.Terminals() {
				if terminal.Poll.Running {
					return errors.New("session still has a running terminal")
				}
			}
			tasks, err := conversation.backend.List(ctx)
			if err != nil {
				return err
			}
			for _, task := range tasks {
				if task.Status == "starting" || task.Status == "running" {
					return errors.New("session still has a running task")
				}
			}
			return nil
		},
		SetOption: entry.setOption,
		PromptMeta: func(run *core.Run) map[string]any {
			if !hasClaraExtension(client) {
				return nil
			}
			return map[string]any{wire.Namespace + "runId": run.ID}
		},
		ToolMeta: func(call model.ToolCallContent) map[string]any {
			if !hasClaraExtension(client) {
				return nil
			}
			activity := session.NewToolActivity(call, time.Now())
			return map[string]any{wire.Namespace + "title": activity.Title, wire.Namespace + "detail": activity.Detail}
		},
		Start: func(ctx context.Context, call core.AgentStreamCall) (*core.Run, error) {
			if entry.currentMode() == "plan" {
				ctx = session.WithPlanning(ctx)
			}
			return conversation.Session.Start(ctx, call)
		},
		Plan: func(ctx context.Context) []acpsdk.PlanEntry {
			state, err := conversation.plan.GetPlan(ctx)
			if err != nil {
				return nil
			}
			entries := make([]acpsdk.PlanEntry, 0, len(state.Steps))
			for _, step := range state.Steps {
				status := acpsdk.PlanEntryStatus(step.Status)
				entries = append(entries, acpsdk.PlanEntry{Content: step.Step, Status: status, Priority: acpsdk.PlanEntryPriorityMedium})
			}
			return entries
		},
		Configure: func(context.Context) core.AgentStreamCall {
			mode := prompt.Execute
			if entry.currentMode() == "plan" {
				mode = prompt.Plan
			}
			return conversation.Call(mode, entry.currentMode() == "review")
		},
		MatchCwd: func(path string) bool {
			if !filepath.IsAbs(path) {
				return false
			}
			current, err := ResolveWorkspace(path)
			return err == nil && current.Cwd == workspace.Cwd && current.Root == workspace.Root
		},
	}, nil
}

func acpSessionNotFound() error {
	return &acpsdk.RequestError{Code: -32002, Message: "session not found"}
}

type acpListCursor struct {
	Scope   string
	ID      string
	Updated string
}

func (h Host) listACP(ctx context.Context, req acpsdk.ListSessionsRequest) (acpsdk.ListSessionsResponse, error) {
	response := acpsdk.ListSessionsResponse{Sessions: []acpsdk.SessionInfo{}}
	var cwd string
	if req.Cwd != nil {
		if !filepath.IsAbs(*req.Cwd) {
			return response, acpsdk.NewInvalidParams("ACP cwd must be absolute")
		}
		cwd = filepath.Clean(*req.Cwd)
		if resolved, err := filepath.EvalSymlinks(cwd); err == nil {
			cwd = resolved
		}
	}
	scope := sha256.Sum256([]byte(h.Config.Home + "\x00" + cwd))
	token := acpListCursor{Scope: base64.RawURLEncoding.EncodeToString(scope[:])}
	var after *session.Meta
	if req.Cursor != nil {
		var cursor acpListCursor
		if len(*req.Cursor) > 512 {
			return response, acpsdk.NewInvalidParams("invalid session list cursor")
		}
		raw, err := base64.RawURLEncoding.DecodeString(*req.Cursor)
		if err != nil || json.Unmarshal(raw, &cursor) != nil || cursor.Scope != token.Scope || !session.ValidID(cursor.ID) {
			return response, acpsdk.NewInvalidParams("invalid session list cursor")
		}
		updated, err := time.Parse(time.RFC3339Nano, cursor.Updated)
		if err != nil {
			return response, acpsdk.NewInvalidParams("invalid session list cursor")
		}
		after = &session.Meta{ID: cursor.ID, Updated: updated}
	}
	items, err := session.ListPage(ctx, h.Config.Home, cwd, after)
	if err != nil {
		return response, err
	}
	const maxPageBytes = 512 << 10
	pageBytes := 0
	for _, item := range items {
		info := acpsdk.SessionInfo{SessionId: acpsdk.SessionId(item.ID), Cwd: item.Cwd}
		if strings.TrimSpace(item.FirstPrompt) != "" {
			info.Title = &item.FirstPrompt
		}
		if !item.Updated.IsZero() {
			updated := item.Updated.Format(time.RFC3339Nano)
			info.UpdatedAt = &updated
		}
		raw, err := json.Marshal(info)
		if err != nil {
			return response, err
		}
		if len(response.Sessions) == session.SessionPageSize || pageBytes+len(raw)+1 > maxPageBytes {
			if len(response.Sessions) == 0 {
				return response, errors.New("session metadata exceeds the ACP page size limit")
			}
			raw, err := json.Marshal(token)
			if err != nil {
				return response, err
			}
			next := base64.RawURLEncoding.EncodeToString(raw)
			response.NextCursor = &next
			break
		}
		response.Sessions = append(response.Sessions, info)
		pageBytes += len(raw) + 1
		token.ID, token.Updated = item.ID, item.Updated.Format(time.RFC3339Nano)
	}
	return response, nil
}

func acpServers(servers []acpsdk.McpServer, cwd string) ([]mcp.ServerConfig, error) {
	out := make([]mcp.ServerConfig, 0, len(servers))
	for _, item := range servers {
		switch {
		case item.Stdio != nil:
			env := make(map[string]string, len(item.Stdio.Env))
			for _, setting := range item.Stdio.Env {
				env[setting.Name] = setting.Value
			}
			out = append(out, mcp.ServerConfig{Name: item.Stdio.Name, Command: item.Stdio.Command,
				Args: item.Stdio.Args, Env: env, Cwd: cwd, Required: true})
		case item.Http != nil:
			headers := make(map[string]string, len(item.Http.Headers))
			for _, header := range item.Http.Headers {
				headers[header.Name] = header.Value
			}
			out = append(out, mcp.ServerConfig{Name: item.Http.Name, URL: item.Http.Url, Headers: headers, Required: true})
		default:
			return nil, errors.New("ACP MCP server must use stdio or Streamable HTTP")
		}
	}
	return out, nil
}
