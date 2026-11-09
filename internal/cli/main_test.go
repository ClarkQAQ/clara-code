package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"clara-code/pkg/core"
	"clara-code/pkg/model"
	toolapi "clara-code/pkg/tool"
	"clara-code/pkg/tool/mcp"
	sdk "github.com/coder/acp-go-sdk"
	"github.com/creack/pty"
	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"clara-code/internal/coding"
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

type cliPTY struct {
	cmd  *exec.Cmd
	file *os.File
	mu   sync.Mutex
	buf  bytes.Buffer
}

func startPTY(t *testing.T, home, cwd string, args ...string) *cliPTY {
	t.Helper()
	args = append([]string{"--demo", "--cwd", cwd}, args...)
	return startConfiguredPTY(t, home, args...)
}

func startConfiguredPTY(t *testing.T, home string, args ...string) *cliPTY {
	t.Helper()
	return startBinaryPTY(t, os.Args[0], home, args...)
}

func startBinaryPTY(t *testing.T, binary, home string, args ...string) *cliPTY {
	t.Helper()
	cmd := exec.CommandContext(t.Context(), binary, args...)
	cmd.Env = append(os.Environ(), childEnv+"=1", "TERM=xterm-256color", "CLARA_CODE_HOME="+home)
	file, err := pty.StartWithSize(cmd, &pty.Winsize{Rows: 24, Cols: 104})
	if err != nil {
		t.Fatal(err)
	}
	p := &cliPTY{cmd: cmd, file: file}
	go func() {
		var chunk [4096]byte
		for {
			n, err := file.Read(chunk[:])
			p.mu.Lock()
			p.buf.Write(chunk[:n])
			p.mu.Unlock()
			if err != nil {
				return
			}
		}
	}()
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		_ = file.Close()
	})
	return p
}

func (p *cliPTY) send(t *testing.T, keys string) {
	t.Helper()
	if _, err := io.WriteString(p.file, keys); err != nil {
		t.Fatal(err)
	}
}

func (p *cliPTY) output() string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.buf.String()
}

func (p *cliPTY) await(t *testing.T, after int, needle string) {
	t.Helper()
	deadline := time.Now().Add(6 * time.Second)
	for time.Now().Before(deadline) {
		if output := p.output(); len(output) >= after && strings.Contains(output[after:], needle) {
			return
		}
		time.Sleep(15 * time.Millisecond)
	}
	out := p.output()
	t.Fatalf("PTY did not show %q: %q", needle, out[max(0, len(out)-2500):])
}

func TestInlineRewindAndResumeAfterCrash(t *testing.T) {
	home, cwd := t.TempDir(), t.TempDir()
	space, err := coding.ResolveWorkspace(cwd)
	if err != nil {
		t.Fatal(err)
	}
	p := startPTY(t, home, cwd)
	p.await(t, 0, "Ready")
	planLabel := "Plan mode (Shift+Tab to cycle)"
	interrupted := "■ Conversation interrupted"
	replayedInterruption := interrupted + " - tell the model what to do differently."
	if _, noColor := os.LookupEnv("NO_COLOR"); !noColor {
		planLabel = "\x1b[95m" + planLabel + "\x1b[0m"
		interrupted = "\x1b[31m" + interrupted
		replayedInterruption = "\x1b[31m" + replayedInterruption + "\x1b[0m"
	}
	mark := len(p.output())
	p.send(t, "\x1b[Z")
	p.await(t, mark, planLabel)
	p.send(t, "\x1b[Z")
	p.send(t, "\x1b[200~你好\x1b[201~\r")
	p.await(t, mark, "有什么可以帮你的吗")
	p.await(t, strings.LastIndex(p.output(), "有什么可以帮你的吗"), "Ready")
	mark = len(p.output())
	p.send(t, "\x1b\x1b")
	p.await(t, mark, "Edit a previous message")
	p.send(t, "\r")
	p.await(t, mark, "You're continuing from this point in a new conversation")
	p.await(t, mark, "❯ 你好")
	if err := pty.Setsize(p.file, &pty.Winsize{Rows: 12, Cols: 47}); err != nil {
		t.Fatal(err)
	}
	_ = p.cmd.Process.Signal(syscall.SIGWINCH)
	mark = len(p.output())
	p.send(t, " changed\r")
	p.await(t, mark, "Demo response: 你好 changed")
	p.await(t, strings.LastIndex(p.output(), "Demo response: 你好 changed"), "Ready")
	mark = len(p.output())
	p.send(t, "CANCEL_SLOW_TURN\r")
	p.await(t, mark, "Working")
	p.send(t, "\x1b")
	p.await(t, mark, interrupted)
	mark = len(p.output())
	p.send(t, "CANCEL_SLOW_TURN\r")
	p.await(t, mark, "Working")
	if err := p.cmd.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	if err := p.cmd.Wait(); err == nil {
		t.Fatal("expected abrupt process exit")
	}
	items, err := session.List(home, space.Root)
	if err != nil || len(items) != 2 || items[0].Parent != items[1].ID {
		t.Fatalf("sessions %+v, %v", items, err)
	}
	resumed := startPTY(t, home, cwd, "resume", items[0].ID)
	resumed.await(t, 0, "Demo response: 你好 changed")
	resumed.await(t, 0, replayedInterruption)
	resumed.await(t, 0, "Ready")
	mark = len(resumed.output())
	resumed.send(t, "/exit\r")
	resumed.await(t, mark, "\x1b[?2004l\x1b[?25h")
	for _, output := range []string{p.output(), resumed.output()} {
		for _, forbidden := range []string{"\x1b[?1049", "\x1b[?1047"} {
			if strings.Contains(output, forbidden) {
				t.Fatalf("unexpected terminal sequence %q", forbidden)
			}
		}
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
	if _, err := os.Stat(filepath.Join(home, "sessions", string(id)+".json")); err != nil {
		t.Fatal(err)
	}
}
