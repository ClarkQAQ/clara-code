package coding

import (
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"

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
			if !filepath.IsAbs(cwd) {
				return core.ACPBinding{}, errors.New("ACP cwd must be absolute")
			}
			workspace, err := ResolveWorkspace(cwd)
			if err != nil {
				return core.ACPBinding{}, err
			}
			extra, err := acpServers(servers, workspace.Cwd)
			if err != nil {
				return core.ACPBinding{}, err
			}
			var store *session.Store
			if id == "" {
				name := h.Config.DefaultModel
				store = session.New(h.Config.Home, session.Meta{Project: workspace.Root, Cwd: workspace.Cwd,
					Model: name, Effort: string(h.Config.Models[name].Effort)})
			} else {
				store, err = session.Open(h.Config.Home, id, workspace.Root)
				if err != nil {
					return core.ACPBinding{}, err
				}
				workspace, err = ResolveWorkspace(store.Meta().Cwd)
				if err != nil || workspace.Root != store.Meta().Project {
					return core.ACPBinding{}, errors.Join(err, errors.New("the session's project directory changed"), store.Close())
				}
			}
			conversation, err := h.Open(ctx, workspace, store, nil, extra)
			if err != nil {
				return core.ACPBinding{}, err
			}
			return core.ACPBinding{
				Session: conversation.Session, Close: func(context.Context) error { return conversation.Close() },
				Configure: func(context.Context) core.AgentStreamCall {
					return conversation.Call(prompt.Execute, false)
				},
			}, nil
		},
	})
	if err != nil {
		return err
	}
	return server.Serve(ctx, input, output)
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
