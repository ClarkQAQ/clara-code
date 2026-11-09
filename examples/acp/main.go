package main

import (
	"context"
	"errors"
	"flag"
	"log"
	"os"
	"path/filepath"

	sdk "github.com/coder/acp-go-sdk"

	"clara-code/examples/internal/config"
	"clara-code/examples/internal/demo"
	"clara-code/examples/internal/host"
	"clara-code/pkg/core"
	"clara-code/pkg/core/prompt"
	modelapi "clara-code/pkg/model"
	"clara-code/pkg/tool/mcp"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	demoMode := flag.Bool("demo", config.Env("ENDPOINT") == "", "use a deterministic model")
	persist := flag.String("store", "", "directory for persistent sessions")
	flag.Parse()
	ctx := context.Background()
	var model modelapi.LanguageModel = demo.Model{}
	var selectModel func(context.Context, string) (modelapi.LanguageModel, error)
	var store core.Store = core.NewMemoryStore()
	var err error
	if !*demoMode {
		model, selectModel, err = host.ModelFromEnv(ctx)
		if err != nil {
			return err
		}
	}
	if *persist != "" {
		store, err = host.FileStore(ctx, *persist)
		if err != nil {
			return err
		}
	}
	open := func(ctx context.Context, id, cwd string, servers []sdk.McpServer) (core.ACPBinding, error) {
		if !filepath.IsAbs(cwd) {
			return core.ACPBinding{}, errors.New("ACP cwd must be absolute")
		}
		config, err := host.Local(cwd)
		if err != nil {
			return core.ACPBinding{}, err
		}
		config.Model, config.SelectModel, config.Store = model, selectModel, store
		var manager *mcp.Manager
		if len(servers) > 0 {
			remote := make([]mcp.ServerConfig, 0, len(servers))
			for _, spec := range servers {
				switch {
				case spec.Stdio != nil:
					env := map[string]string{}
					for _, item := range spec.Stdio.Env {
						env[item.Name] = item.Value
					}
					remote = append(remote, mcp.ServerConfig{Name: spec.Stdio.Name, Command: spec.Stdio.Command, Args: spec.Stdio.Args, Env: env, Cwd: cwd, Required: true})
				case spec.Http != nil:
					headers := map[string]string{}
					for _, header := range spec.Http.Headers {
						headers[header.Name] = header.Value
					}
					remote = append(remote, mcp.ServerConfig{Name: spec.Http.Name, URL: spec.Http.Url, Headers: headers, Required: true})
				default:
					return core.ACPBinding{}, errors.New("the ACP example accepts stdio and HTTP MCP servers")
				}
			}
			manager, err = mcp.Connect(ctx, remote)
			if err != nil {
				return core.ACPBinding{}, err
			}
			config.DynamicTools = manager.Tools
		}
		conversation, err := host.Open(ctx, config, id)
		if err != nil {
			return core.ACPBinding{}, errors.Join(err, manager.Close())
		}
		return core.ACPBinding{Session: conversation.Session, Configure: func(context.Context) core.AgentStreamCall {
			return conversation.Call(prompt.Execute)
		}, Close: func(ctx context.Context) error {
			return errors.Join(conversation.Close(ctx), manager.Close())
		}}, nil
	}
	server, err := core.NewACPServer(core.ACPServerConfig{
		LoadSession: true, Images: true, MCPHTTP: true, Open: open,
		Resume: func(ctx context.Context, req sdk.ResumeSessionRequest) (core.ACPBinding, sdk.ResumeSessionResponse, error) {
			binding, err := open(ctx, string(req.SessionId), req.Cwd, req.McpServers)
			return binding, sdk.ResumeSessionResponse{}, err
		},
	})
	if err != nil {
		return err
	}
	return server.Serve(ctx, os.Stdin, os.Stdout)
}
