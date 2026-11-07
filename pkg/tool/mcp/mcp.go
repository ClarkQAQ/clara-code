// Package mcp connects Clara Core Agents to MCP servers.
package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"sort"
	"strings"
	"sync"
	"time"

	toolapi "clara-code/pkg/tool"
	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
)

// ServerConfig configures one MCP server.
type ServerConfig struct {
	Name      string
	Command   string
	Args      []string
	Env       map[string]string
	Cwd       string
	URL       string
	Headers   map[string]string
	Required  bool
	Timeout   time.Duration
	Transport sdkmcp.Transport
}

// Status reports one server connection.
type Status struct {
	Name      string
	OK        bool
	Error     string
	ToolCount int
}

type binding struct {
	config ServerConfig
	client *sdkmcp.ClientSession

	mu        sync.RWMutex
	tools     []toolapi.MCPTool
	resources []toolapi.MCPResource
	prompts   []toolapi.MCPPrompt
}

// Manager owns MCP Client connections.
type Manager struct {
	mu       sync.RWMutex
	bindings map[string]*binding
	status   []Status
	dirty    chan struct{}
	cancel   context.CancelFunc
}

// Connect connects all configured servers. Optional failures are retained in Status.
func Connect(ctx context.Context, configs []ServerConfig) (*Manager, error) {
	watchCtx, cancel := context.WithCancel(context.Background())
	manager := &Manager{
		bindings: map[string]*binding{}, dirty: make(chan struct{}, 1), cancel: cancel,
	}
	for _, config := range configs {
		if strings.TrimSpace(config.Name) == "" || manager.bindings[config.Name] != nil {
			return nil, errors.Join(errors.New("mcp: server name is empty or duplicated"), manager.Close())
		}
		conn, err := manager.connectOne(ctx, config)
		if err != nil {
			manager.status = append(manager.status, Status{Name: config.Name, Error: err.Error()})
			if config.Required {
				manager.Close()
				return nil, fmt.Errorf("mcp: required server %s failed: %w", config.Name, err)
			}
			continue
		}
		manager.mu.Lock()
		manager.bindings[config.Name] = conn
		manager.mu.Unlock()
		manager.status = append(manager.status, Status{Name: config.Name, OK: true, ToolCount: len(conn.tools)})
	}
	go manager.watchRefresh(watchCtx)
	return manager, nil
}

func (m *Manager) connectOne(ctx context.Context, config ServerConfig) (*binding, error) {
	timeout := config.Timeout
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	conn := &binding{config: config}
	client := sdkmcp.NewClient(
		&sdkmcp.Implementation{Name: "clara-core", Version: "dev"},
		&sdkmcp.ClientOptions{
			Capabilities: &sdkmcp.ClientCapabilities{},
			ToolListChangedHandler: func(context.Context, *sdkmcp.ToolListChangedRequest) {
				select {
				case m.dirty <- struct{}{}:
				default:
				}
			},
			ResourceListChangedHandler: func(context.Context, *sdkmcp.ResourceListChangedRequest) {
				select {
				case m.dirty <- struct{}{}:
				default:
				}
			},
			PromptListChangedHandler: func(context.Context, *sdkmcp.PromptListChangedRequest) {
				select {
				case m.dirty <- struct{}{}:
				default:
				}
			},
		},
	)
	transport, err := transportFor(ctx, config)
	if err != nil {
		return nil, err
	}
	connectCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	session, err := client.Connect(connectCtx, transport, nil)
	if err != nil {
		return nil, err
	}
	conn.client = session
	if err := refresh(connectCtx, conn); err != nil {
		session.Close()
		return nil, err
	}
	return conn, nil
}

func transportFor(ctx context.Context, config ServerConfig) (sdkmcp.Transport, error) {
	if config.Transport != nil {
		return config.Transport, nil
	}
	if config.URL != "" && config.Command != "" {
		return nil, errors.New("mcp: server cannot use both command and URL")
	}
	if config.URL != "" {
		httpClient := http.DefaultClient
		if len(config.Headers) > 0 {
			httpClient = &http.Client{Transport: headerTransport{
				base: http.DefaultTransport, headers: config.Headers,
			}}
		}
		return &sdkmcp.StreamableClientTransport{
			Endpoint: config.URL, HTTPClient: httpClient, MaxRetries: 3,
		}, nil
	}
	if config.Command == "" {
		return nil, errors.New("mcp: server needs a command or URL")
	}
	command := exec.CommandContext(context.WithoutCancel(ctx), config.Command, config.Args...)
	command.Dir = config.Cwd
	command.Env = append([]string(nil), os.Environ()...)
	for key, value := range config.Env {
		command.Env = append(command.Env, key+"="+value)
	}
	return &sdkmcp.CommandTransport{Command: command}, nil
}

type headerTransport struct {
	base    http.RoundTripper
	headers map[string]string
}

func (t headerTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	clone := request.Clone(request.Context())
	clone.Header = request.Header.Clone()
	for key, value := range t.headers {
		clone.Header.Set(key, value)
	}
	return t.base.RoundTrip(clone)
}

func refresh(ctx context.Context, conn *binding) error {
	var toolsList []*sdkmcp.Tool
	for tool, err := range conn.client.Tools(ctx, nil) {
		if err != nil {
			return err
		}
		toolsList = append(toolsList, tool)
	}
	sort.SliceStable(toolsList, func(i, j int) bool { return toolsList[i].Name < toolsList[j].Name })
	remoteTools := make([]toolapi.MCPTool, 0, len(toolsList))
	for _, tool := range toolsList {
		raw, err := json.Marshal(tool.InputSchema)
		if err != nil {
			return err
		}
		readOnly := tool.Annotations != nil && tool.Annotations.ReadOnlyHint
		remoteTools = append(remoteTools, toolapi.MCPTool{
			Server: conn.config.Name, Name: tool.Name, Description: tool.Description,
			InputSchema: raw, ReadOnly: readOnly,
		})
	}
	var resources []toolapi.MCPResource
	if result, err := conn.client.ListResources(ctx, nil); err == nil {
		for _, resource := range result.Resources {
			resources = append(resources, toolapi.MCPResource{
				Server: conn.config.Name, URI: resource.URI, Name: resource.Name,
				Title: resource.Title, Description: resource.Description, MIMEType: resource.MIMEType,
			})
		}
	}
	var prompts []toolapi.MCPPrompt
	if result, err := conn.client.ListPrompts(ctx, nil); err == nil {
		for _, prompt := range result.Prompts {
			item := toolapi.MCPPrompt{
				Server: conn.config.Name, Name: prompt.Name,
				Title: prompt.Title, Description: prompt.Description,
			}
			for _, argument := range prompt.Arguments {
				item.Arguments = append(item.Arguments, argument.Name)
			}
			prompts = append(prompts, item)
		}
	}
	conn.mu.Lock()
	conn.tools = remoteTools
	conn.resources = resources
	conn.prompts = prompts
	conn.mu.Unlock()
	return nil
}

func (m *Manager) watchRefresh(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-m.dirty:
			m.mu.RLock()
			bindings := make([]*binding, 0, len(m.bindings))
			for _, conn := range m.bindings {
				bindings = append(bindings, conn)
			}
			m.mu.RUnlock()
			for _, conn := range bindings {
				_ = refresh(ctx, conn)
			}
		}
	}
}

// Tools returns a dynamic tool source for all connected servers.
func (m *Manager) Tools() []toolapi.Tool {
	if m == nil {
		return nil
	}
	m.mu.RLock()
	names := make([]string, 0, len(m.bindings))
	for name := range m.bindings {
		names = append(names, name)
	}
	m.mu.RUnlock()
	sort.Strings(names)
	var out []toolapi.Tool
	for _, name := range names {
		out = append(out, (&toolapi.MCPTools{Caller: m, Server: name}).All()...)
	}
	return out
}

// ServerTools returns a tool source for one server.
func (m *Manager) ServerTools(server string) []toolapi.Tool {
	return (&toolapi.MCPTools{Caller: m, Server: server}).All()
}

func (m *Manager) CanonicalTools() []toolapi.MCPTool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var out []toolapi.MCPTool
	for _, conn := range m.bindings {
		conn.mu.RLock()
		out = append(out, conn.tools...)
		conn.mu.RUnlock()
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Server != out[j].Server {
			return out[i].Server < out[j].Server
		}
		return out[i].Name < out[j].Name
	})
	return out
}

func (m *Manager) CallTool(ctx context.Context, server, name string, arguments json.RawMessage) (toolapi.Result, error) {
	conn := m.binding(server)
	if conn == nil {
		return toolapi.ErrorResult("unknown MCP server " + server), errors.New("mcp: server not connected")
	}
	var decoded any
	if len(arguments) > 0 {
		if err := json.Unmarshal(arguments, &decoded); err != nil {
			return toolapi.ErrorResult(err.Error()), nil
		}
	}
	result, err := conn.client.CallTool(ctx, &sdkmcp.CallToolParams{Name: name, Arguments: decoded})
	if err != nil {
		return toolapi.ErrorResult(err.Error()), err
	}
	var lines []string
	var data []byte
	var mime string
	for _, content := range result.Content {
		switch item := content.(type) {
		case *sdkmcp.TextContent:
			lines = append(lines, item.Text)
		case *sdkmcp.ImageContent:
			if data != nil {
				return toolapi.Result{}, errors.New("MCP result has more than one media part")
			}
			data, mime = item.Data, item.MIMEType
		case *sdkmcp.ResourceLink:
			lines = append(lines, "[resource] "+item.URI)
		case *sdkmcp.EmbeddedResource:
			if item.Resource != nil {
				lines = append(lines, item.Resource.Text)
			}
		default:
			return toolapi.Result{}, fmt.Errorf("unsupported MCP result type %T", item)
		}
	}
	if result.StructuredContent != nil {
		raw, err := json.Marshal(result.StructuredContent)
		if err != nil {
			return toolapi.Result{}, err
		}
		lines = append(lines, string(raw))
	}
	converted := toolapi.TextResult(strings.Join(lines, "\n"))
	if data != nil {
		converted = toolapi.MediaResult(converted.Content, data, mime)
	}
	converted.IsError = result.IsError
	return converted, nil
}

func (m *Manager) ListResources(ctx context.Context, server string) ([]toolapi.MCPResource, error) {
	conn := m.binding(server)
	if conn == nil {
		return nil, errors.New("mcp: server not connected")
	}
	conn.mu.RLock()
	defer conn.mu.RUnlock()
	return append([]toolapi.MCPResource(nil), conn.resources...), nil
}

func (m *Manager) ReadResource(ctx context.Context, server, uri string) (toolapi.MCPResourceContent, error) {
	conn := m.binding(server)
	if conn == nil {
		return toolapi.MCPResourceContent{}, errors.New("mcp: server not connected")
	}
	result, err := conn.client.ReadResource(ctx, &sdkmcp.ReadResourceParams{URI: uri})
	if err != nil {
		return toolapi.MCPResourceContent{}, err
	}
	if len(result.Contents) == 0 {
		return toolapi.MCPResourceContent{}, errors.New("mcp: resource returned no content")
	}
	first := result.Contents[0]
	return toolapi.MCPResourceContent{
		URI: first.URI, MIMEType: first.MIMEType, Text: first.Text, Blob: first.Blob,
	}, nil
}

func (m *Manager) ListPrompts(ctx context.Context, server string) ([]toolapi.MCPPrompt, error) {
	conn := m.binding(server)
	if conn == nil {
		return nil, errors.New("mcp: server not connected")
	}
	conn.mu.RLock()
	defer conn.mu.RUnlock()
	return append([]toolapi.MCPPrompt(nil), conn.prompts...), nil
}

func (m *Manager) GetPrompt(ctx context.Context, server, name string, arguments map[string]string) (string, error) {
	conn := m.binding(server)
	if conn == nil {
		return "", errors.New("mcp: server not connected")
	}
	result, err := conn.client.GetPrompt(ctx, &sdkmcp.GetPromptParams{Name: name, Arguments: arguments})
	if err != nil {
		return "", err
	}
	var builder strings.Builder
	for _, prompt := range result.Messages {
		if text, ok := prompt.Content.(*sdkmcp.TextContent); ok {
			builder.WriteString(text.Text)
			builder.WriteByte('\n')
		}
	}
	return strings.TrimSpace(builder.String()), nil
}

func (m *Manager) binding(server string) *binding {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.bindings[server]
}

// Statuses returns connection health.
func (m *Manager) Statuses() []Status {
	if m == nil {
		return nil
	}
	return append([]Status(nil), m.status...)
}

// Close closes every MCP session.
func (m *Manager) Close() error {
	if m == nil {
		return nil
	}
	if m.cancel != nil {
		m.cancel()
	}
	m.mu.Lock()
	bindings := m.bindings
	m.bindings = map[string]*binding{}
	m.mu.Unlock()
	var first error
	for _, conn := range bindings {
		if err := conn.client.Close(); err != nil && first == nil {
			first = err
		}
	}
	return first
}

var _ toolapi.MCPCaller = (*Manager)(nil)
