package cli

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"clara-code/pkg/core"
	"clara-code/pkg/model"
	toolapi "clara-code/pkg/tool"
	"clara-code/pkg/tool/mcp"
	sdk "github.com/coder/acp-go-sdk"
	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"clara-code/internal/session"
)

const childEnv = "CLARA_CODE_TEST_CHILD"

func TestMain(m *testing.M) {
	if os.Getenv(childEnv) == "1" {
		if len(os.Args) > 1 && os.Args[1] == "__mcp_fixture" {
			server := mcpsdk.NewServer(&mcpsdk.Implementation{Name: "fixture", Version: "1"}, nil)
			type echo struct {
				Text string `json:"text"`
			}
			mcpsdk.AddTool(server, &mcpsdk.Tool{Name: "echo", Description: "Echo input"},
				func(_ context.Context, _ *mcpsdk.CallToolRequest, input echo) (*mcpsdk.CallToolResult, echo, error) {
					return nil, input, nil
				})
			if err := server.Run(context.Background(), &mcpsdk.StdioTransport{}); err != nil {
				os.Exit(1)
			}
			os.Exit(0)
		}
		if err := Run(context.Background(), os.Args[1:], os.Stdin, os.Stdout, os.Stderr, "test"); err != nil {
			_, _ = os.Stderr.WriteString(err.Error() + "\n")
			os.Exit(1)
		}
		os.Exit(0)
	}
	os.Exit(m.Run())
}

func TestMCPStdioProcess(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 8*time.Second)
	defer cancel()
	manager, err := mcp.Connect(ctx, []mcp.ServerConfig{{
		Name: "fixture", Command: os.Args[0], Args: []string{"__mcp_fixture"},
		Env: map[string]string{childEnv: "1"}, Required: true,
	}})
	if err != nil {
		t.Fatal(err)
	}
	defer manager.Close()
	for _, tool := range manager.Tools() {
		if tool.Info().Name != "mcp__fixture__echo" {
			continue
		}
		reply, err := tool.Run(ctx, model.ToolCall{Input: `{"text":"stdio probe"}`})
		if err != nil || reply.IsError || !strings.Contains(reply.Content, "stdio probe") {
			t.Fatalf("stdio reply %+v, %v", reply, err)
		}
		return
	}
	t.Fatal("stdio echo tool was not registered")
}

func TestACPDelegateProcess(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	defer cancel()
	backend := toolapi.NewACPTaskBackend(map[string]core.ACPExternalConfig{
		"helper": {Command: os.Args[0], Args: []string{"__acp_exec", os.Args[0], "acp", "--demo"},
			Cwd: t.TempDir(), Env: map[string]string{childEnv: "1", "CLARA_CODE_HOME": t.TempDir()}},
	}, 1)
	spawn := toolapi.SpawnAgent(backend)
	runTool := func(item toolapi.Tool, args any) string {
		t.Helper()
		raw, err := json.Marshal(args)
		if err != nil {
			t.Fatal(err)
		}
		result, err := item.Run(ctx, model.ToolCall{Input: string(raw)})
		if err != nil || result.IsError {
			t.Fatalf("%s: %+v, %v", item.Info().Name, result, err)
		}
		return result.Content
	}
	var task toolapi.TaskInfo
	if err := json.Unmarshal([]byte(runTool(spawn, toolapi.TaskSpec{Role: "helper", Name: "probe", Message: "delegated work"})), &task); err != nil {
		t.Fatal(err)
	}
	result, err := backend.Wait(ctx, task.ID, 7*time.Second)
	if err != nil || result.Result != "Demo response: delegated work" {
		t.Fatalf("delegated result %+v, %v", result, err)
	}
	if err := backend.CloseTask(ctx, task.ID); err != nil {
		t.Fatal(err)
	}
	list := toolapi.ListACPSessions(backend)
	var page sdk.ListSessionsResponse
	if err := json.Unmarshal([]byte(runTool(list, map[string]string{"role": "helper"})), &page); err != nil {
		t.Fatal(err)
	}
	if len(page.Sessions) != 1 {
		t.Fatalf("expected one persisted delegated session, got %+v", page)
	}
	persisted := string(page.Sessions[0].SessionId)
	_, err = spawn.Run(ctx, model.ToolCall{Input: `{"role":"helper","task_name":"missing","message":"resume","resume_session_id":"session_missing"}`})
	requireACPCode(t, err, -32002)
	var resumed toolapi.TaskInfo
	spec := toolapi.TaskSpec{Role: "helper", Name: "continued", Message: "continued work", ResumeSessionID: persisted}
	if err := json.Unmarshal([]byte(runTool(toolapi.RunTask(backend), spec)), &resumed); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := backend.CloseTask(ctx, resumed.ID); err != nil {
			t.Error(err)
		}
	}()
	if resumed.ID == task.ID || resumed.Result != "Demo response: continued work" {
		t.Fatalf("resumed task %+v", resumed)
	}
	runTool(toolapi.FollowupTask(backend), map[string]string{"target": resumed.ID, "message": "follow-up"})
	result, err = backend.Wait(ctx, resumed.ID, 7*time.Second)
	if err != nil || result.Result != "Demo response: follow-up" {
		t.Fatalf("follow-up result %+v, %v", result, err)
	}
	if err := json.Unmarshal([]byte(runTool(list, map[string]string{"role": "helper"})), &page); err != nil {
		t.Fatal(err)
	}
	if len(page.Sessions) != 1 || string(page.Sessions[0].SessionId) != persisted {
		t.Fatalf("resuming created another session: %+v", page)
	}
}

func TestACPProcess(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	home, cwd := t.TempDir(), t.TempDir()
	client, err := core.ConnectACPExternal(ctx, core.ACPExternalConfig{
		Command: os.Args[0], Args: []string{"acp", "--demo"},
		Env: map[string]string{childEnv: "1", "CLARA_CODE_HOME": home},
	})
	if err != nil {
		t.Fatal(err)
	}
	id, err := client.NewSession(ctx, cwd)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := client.Close(ctx, id); err != nil && !errors.Is(err, context.Canceled) {
			t.Error(err)
		}
	}()
	reply, reason, err := client.Prompt(ctx, id, "ACP probe")
	if err != nil || reply != "Demo response: ACP probe" || reason != "end_turn" {
		t.Fatalf("ACP reply %q, %q, %v", reply, reason, err)
	}
	page, err := session.ListPage(ctx, home, "", nil)
	if err != nil || len(page) != 1 || page[0].ID != string(id) {
		t.Fatalf("ACP session was not persisted: %+v, %v", page, err)
	}
}
