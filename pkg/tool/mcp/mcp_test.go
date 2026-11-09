package mcp_test

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"testing"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"clara-code/pkg/model"
	mcpconnector "clara-code/pkg/tool/mcp"
)

func ExampleConnectWithProgress() {
	manager, err := mcpconnector.ConnectWithProgress(context.Background(), []mcpconnector.ServerConfig{
		{Name: "files"},
		{Name: "docs"},
	}, func(progress mcpconnector.StartupProgress) {
		fmt.Printf("%d/%d · %s\n", progress.Completed, progress.Total, progress.Name)
	})
	if err != nil {
		panic(err)
	}
	defer manager.Close()
	fmt.Println("Recorded results:", len(manager.Statuses()))
	// Output:
	// 0/2 · files
	// 1/2 · docs
	// Recorded results: 2
}

type echoInput struct {
	Text string `json:"text" jsonschema:"text to echo"`
}

func TestManagerConnectsCallsAndCloses(t *testing.T) {
	ctx := t.Context()
	server := sdkmcp.NewServer(&sdkmcp.Implementation{Name: "test", Version: "1"}, nil)
	discovered := make(chan struct{}, 1)
	server.AddReceivingMiddleware(func(next sdkmcp.MethodHandler) sdkmcp.MethodHandler {
		return func(ctx context.Context, method string, req sdkmcp.Request) (sdkmcp.Result, error) {
			if method == "server/discover" {
				select {
				case discovered <- struct{}{}:
				default:
				}
			}
			return next(ctx, method, req)
		}
	})
	sdkmcp.AddTool(server, &sdkmcp.Tool{
		Name: "echo", Description: "Echo text",
		Annotations: &sdkmcp.ToolAnnotations{ReadOnlyHint: true},
	}, func(_ context.Context, _ *sdkmcp.CallToolRequest, input echoInput) (*sdkmcp.CallToolResult, any, error) {
		return nil, map[string]string{"echo": input.Text}, nil
	})
	serverTransport, clientTransport := sdkmcp.NewInMemoryTransports()
	go func() {
		_ = server.Run(ctx, serverTransport)
	}()
	manager, err := mcpconnector.Connect(ctx, []mcpconnector.ServerConfig{{
		Name: "test", Transport: clientTransport, Required: true,
	}})
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-discovered:
	default:
		t.Error("default connection did not discover the server")
	}
	t.Cleanup(func() {
		if err := manager.Close(); err != nil {
			t.Error(err)
		}
	})
	tools := manager.CanonicalTools()
	if len(tools) != 1 || tools[0].Name != "echo" || !tools[0].ReadOnly {
		t.Fatalf("tools = %+v", tools)
	}
	items := manager.Tools()
	if len(items) == 0 || items[0].Info().Name != "mcp__test__echo" {
		t.Fatalf("missing registered echo tool")
	}
	result, err := items[0].Run(ctx, model.ToolCall{Input: `{"text":"hi"}`})
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError || !strings.Contains(result.Content, "hi") {
		t.Fatalf("result = %+v", result)
	}
}

func TestManagerConnectsToLegacyStdio(t *testing.T) {
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	config := mcpconnector.ServerConfig{
		Name: "legacy", Command: executable, Args: []string{"-test.run=^TestLegacyStdioServer$"},
		Env: map[string]string{"CLARA_TEST_LEGACY_MCP": "1"}, Required: true,
	}
	if manager, err := mcpconnector.Connect(t.Context(), []mcpconnector.ServerConfig{config}); err == nil {
		_ = manager.Close()
		t.Fatal("legacy server accepted the default handshake")
	}
	config.ProtocolVersion = "2025-03-26"
	manager, err := mcpconnector.Connect(t.Context(), []mcpconnector.ServerConfig{config})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := manager.Close(); err != nil {
			t.Error(err)
		}
	})
	tools := manager.CanonicalTools()
	if len(tools) != 1 || tools[0].Name != "echo" {
		t.Fatalf("legacy tools = %+v", tools)
	}
	result, err := manager.CallTool(t.Context(), "legacy", "echo", json.RawMessage(`{"text":"legacy probe"}`))
	if err != nil || result.IsError || !strings.Contains(result.Content, "legacy probe") {
		t.Fatalf("result = %+v, error = %v", result, err)
	}
}

func TestLegacyStdioServer(t *testing.T) {
	if os.Getenv("CLARA_TEST_LEGACY_MCP") != "1" {
		return
	}
	input := bufio.NewReader(os.Stdin)
	first, err := input.ReadBytes('\n')
	if err != nil {
		t.Fatal(err)
	}
	var request struct {
		Method string                  `json:"method"`
		Params sdkmcp.InitializeParams `json:"params"`
	}
	if err := json.Unmarshal(first, &request); err != nil {
		t.Fatal(err)
	}
	if request.Method == "server/discover" {
		os.Exit(1)
	}
	if request.Method != "initialize" || request.Params.ProtocolVersion != "2025-03-26" {
		t.Fatalf("unexpected first request: %s", first)
	}
	server := sdkmcp.NewServer(&sdkmcp.Implementation{Name: "legacy", Version: "1"}, nil)
	sdkmcp.AddTool(server, &sdkmcp.Tool{Name: "echo"}, func(_ context.Context, _ *sdkmcp.CallToolRequest, input echoInput) (*sdkmcp.CallToolResult, any, error) {
		return nil, map[string]string{"echo": input.Text}, nil
	})
	transport := &sdkmcp.IOTransport{Reader: io.NopCloser(io.MultiReader(bytes.NewReader(first), input)), Writer: os.Stdout}
	if err := server.Run(t.Context(), transport); err != nil {
		t.Fatal(err)
	}
	os.Exit(0)
}
