package mcp_test

import (
	"clara-code/pkg/model"
	"context"
	"fmt"
	"strings"
	"testing"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

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
