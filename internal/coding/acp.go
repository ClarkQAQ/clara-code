package coding

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"clara-code/pkg/core"
	"clara-code/pkg/core/prompt"
	"clara-code/pkg/tool/mcp"
	acpsdk "github.com/coder/acp-go-sdk"
	"golang.org/x/sys/unix"

	"clara-code/internal/session"
)

func (h Host) ServeACP(ctx context.Context, input io.Reader, output io.Writer, version string) error {
	server, err := core.NewACPServer(core.ACPServerConfig{
		Name: "clara-code", Version: version, LoadSession: true, Images: true, MCPHTTP: true,
		Open: func(ctx context.Context, id, cwd string, servers []acpsdk.McpServer) (core.ACPBinding, error) {
			return h.openACP(ctx, id, cwd, servers, false)
		},
		List: h.listACP,
		Resume: func(ctx context.Context, req acpsdk.ResumeSessionRequest) (core.ACPBinding, acpsdk.ResumeSessionResponse, error) {
			binding, err := h.openACP(ctx, string(req.SessionId), req.Cwd, req.McpServers, true)
			return binding, acpsdk.ResumeSessionResponse{}, err
		},
	})
	if err != nil {
		return err
	}
	return server.Serve(ctx, input, output)
}

func (h Host) openACP(ctx context.Context, id, cwd string, servers []acpsdk.McpServer, resume bool) (core.ACPBinding, error) {
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
		store = session.New(h.Config.Home, session.Meta{Project: workspace.Root, Cwd: workspace.Cwd})
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
	conversation, err := h.Open(ctx, workspace, store, nil, extra)
	if err != nil {
		return core.ACPBinding{}, err
	}
	return core.ACPBinding{
		Session: conversation.Session, PrepareInput: conversation.PrepareImages,
		Close: func(context.Context) error { return conversation.Close() },
		Configure: func(context.Context) core.AgentStreamCall {
			return conversation.Call(prompt.Execute, false)
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

// ExecACP runs inside the child process. Core forwards ACP child stderr to the
// host stderr, so redirect it here before replacing the child with the server.
func ExecACP(args []string) error {
	if len(args) == 0 {
		return errors.New("external ACP command is missing")
	}
	null, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	if err != nil {
		return err
	}
	defer null.Close()
	if err := unix.Dup2(int(null.Fd()), int(os.Stderr.Fd())); err != nil {
		return err
	}
	path, err := exec.LookPath(args[0])
	if err != nil {
		return err
	}
	return unix.Exec(path, args, os.Environ())
}
