package tool_test

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"clara-code/pkg/model"
	"clara-code/pkg/tool"
)

func backgroundID(t *testing.T, result model.ToolResponse) string {
	t.Helper()
	var info struct {
		Terminal tool.TerminalInfo `json:"terminal"`
	}
	if err := json.Unmarshal([]byte(result.Metadata), &info); err != nil || info.Terminal.ID == "" {
		t.Fatalf("missing terminal metadata: %+v %v", result, err)
	}
	return info.Terminal.ID
}

func TestBackgroundSnapshotsDoNotConsumeAndExitIsRetained(t *testing.T) {
	commands := tool.NewCommands(tool.NewLocal())
	defer commands.Close()
	result := callTool(t, commands.ExecCommand(), map[string]any{"command": "printf first; sleep 1.3; printf last; exit 7", "yield_seconds": 1})
	id := backgroundID(t, result)
	if !strings.Contains(result.Content, "first") {
		t.Fatal(result.Content)
	}
	deadline := time.Now().Add(3 * time.Second)
	var info tool.TerminalInfo
	for time.Now().Before(deadline) {
		if err := commands.Refresh(t.Context()); err != nil {
			t.Fatal(err)
		}
		for _, value := range commands.Terminals() {
			if value.ID == id {
				info = value
			}
		}
		if info.Status != "running" {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if info.Status != "failed" || info.Poll.ExitCode != 7 || !strings.Contains(info.Poll.Output, "firstlast") {
		t.Fatalf("snapshot %+v", info)
	}
	final := callTool(t, commands.WriteStdin(), map[string]any{"session_id": id, "yield_seconds": 1})
	if !final.IsError || !strings.Contains(final.Content, "last") || strings.Contains(final.Content, "first") {
		t.Fatalf("monitor stole or duplicated output: %s", final.Content)
	}
	if len(commands.Terminals()) != 0 {
		t.Fatal("completed cursor not retired")
	}
}

func TestCanceledWaitAndConcurrentStopKeepCorrectState(t *testing.T) {
	commands := tool.NewCommands(tool.NewLocal())
	defer commands.Close()
	commands.NoDefaultTimeout = true
	id := backgroundID(t, callTool(t, commands.ExecCommand(), map[string]any{"command": "printf ready; sleep 30", "yield_seconds": 1}))
	ctx, cancel := context.WithCancel(t.Context())
	input, _ := json.Marshal(map[string]any{"session_id": id, "yield_seconds": 30})
	done := make(chan error, 1)
	go func() { _, err := commands.WriteStdin().Run(ctx, model.ToolCall{Input: string(input)}); done <- err }()
	cancel()
	if err := <-done; err == nil {
		t.Fatal("wait not canceled")
	}
	if err := commands.Refresh(t.Context()); err != nil {
		t.Fatal(err)
	}
	if commands.Terminals()[0].Status != "running" {
		t.Fatal("canceled wait killed process")
	}
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		result, err := commands.WriteStdin().Run(t.Context(), model.ToolCall{Input: string(input)})
		if err != nil || result.IsError || !strings.Contains(result.Content, "Command stopped") {
			t.Errorf("concurrent stop lost its reason: %+v %v", result, err)
		}
	}()
	go func() {
		defer wg.Done()
		if err := commands.Stop(t.Context(), id); err != nil {
			t.Error(err)
		}
	}()
	wg.Wait()
	for _, info := range commands.Terminals() {
		if info.ID == id && info.Status != "stopped" {
			t.Fatalf("stop lost race: %+v", info)
		}
	}
}

func TestNoTimeoutIsExplicitOptInForRunners(t *testing.T) {
	runner := tool.NewLocal()
	process, err := runner.Start(t.Context(), tool.Spec{Command: "sleep 0.15; printf done", Timeout: 20 * time.Millisecond, NoTimeout: true})
	if err != nil {
		t.Fatal(err)
	}
	defer process.Close()
	result, err := process.Poll(t.Context(), time.Second)
	if err != nil || result.TimedOut || !strings.Contains(result.Output, "done") {
		t.Fatalf("no-timeout %+v %v", result, err)
	}
	limited, err := runner.Start(t.Context(), tool.Spec{Command: "sleep 5", Timeout: 20 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	defer limited.Close()
	result, err = limited.Poll(t.Context(), time.Second)
	if err != nil || !result.TimedOut {
		t.Fatalf("explicit timeout lost %+v %v", result, err)
	}
}

type snapshotFixtureRunner struct{ process *snapshotFixtureProcess }

func (r *snapshotFixtureRunner) Start(context.Context, tool.Spec) (tool.Process, error) {
	r.process = &snapshotFixtureProcess{}
	return r.process, nil
}

type snapshotFixtureProcess struct{ finished atomic.Bool }

func (p *snapshotFixtureProcess) Write(context.Context, string) error { return nil }
func (p *snapshotFixtureProcess) Kill(context.Context) error {
	p.finished.Store(true)
	return nil
}
func (p *snapshotFixtureProcess) Close() error { return p.Kill(context.Background()) }
func (p *snapshotFixtureProcess) Poll(context.Context, time.Duration) (tool.Poll, error) {
	return tool.Poll{Running: !p.finished.Load(), Output: "retained output"}, nil
}
func (p *snapshotFixtureProcess) Snapshot(ctx context.Context) (tool.Poll, error) {
	return p.Poll(ctx, 0)
}

func TestCompletedTerminalCacheIsBoundedAndNotifiesOnlyChanges(t *testing.T) {
	runner := &snapshotFixtureRunner{}
	commands := tool.NewCommands(runner)
	defer commands.Close()
	var notifications atomic.Int32
	commands.OnChange = func(tool.TerminalInfo) { notifications.Add(1) }
	var oldest, latest string
	for i := range 34 {
		id := backgroundID(t, callTool(t, commands.ExecCommand(), map[string]any{"command": "fixture", "yield_seconds": 1}))
		if i == 0 {
			oldest = id
		}
		latest = id
		before := notifications.Load()
		if err := commands.Refresh(t.Context()); err != nil {
			t.Fatal(err)
		}
		if notifications.Load() != before {
			t.Fatal("unchanged snapshot produced notification")
		}
		runner.process.finished.Store(true)
		if err := commands.Refresh(t.Context()); err != nil {
			t.Fatal(err)
		}
	}
	if len(commands.Terminals()) != 32 || notifications.Load() != 34 {
		t.Fatalf("cache %d, notifications %d", len(commands.Terminals()), notifications.Load())
	}
	missing := callTool(t, commands.WriteStdin(), map[string]any{"session_id": oldest})
	if !missing.IsError {
		t.Fatal("oldest completed entry was not evicted")
	}
	retained := callTool(t, commands.WriteStdin(), map[string]any{"session_id": latest})
	if retained.IsError || !strings.Contains(retained.Content, "retained output") {
		t.Fatal(retained)
	}
}

func TestStopAllAndExplicitBackgroundTimeout(t *testing.T) {
	commands := tool.NewCommands(tool.NewLocal())
	commands.NoDefaultTimeout = true
	defer commands.Close()
	for range 2 {
		callTool(t, commands.ExecCommand(), map[string]any{"command": "sleep 30", "yield_seconds": 1})
	}
	if err := commands.Stop(t.Context(), ""); err != nil {
		t.Fatal(err)
	}
	for _, info := range commands.Terminals() {
		if info.Status != "stopped" {
			t.Fatalf("stop all: %+v", info)
		}
		result := callTool(t, commands.WriteStdin(), map[string]any{"session_id": info.ID})
		if result.IsError || !strings.Contains(result.Content, "Command stopped") {
			t.Fatalf("user stop was not explicit: %+v", result)
		}
	}
	id := backgroundID(t, callTool(t, commands.ExecCommand(), map[string]any{"command": "sleep 30", "yield_seconds": 1, "timeout_seconds": 2}))
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if err := commands.Refresh(t.Context()); err != nil {
			t.Fatal(err)
		}
		if commands.Terminals()[0].Status != "running" {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	info := commands.Terminals()[0]
	if info.ID != id || info.Status != "timed_out" {
		t.Fatalf("explicit timeout ignored by unlimited host: %+v", info)
	}
}

func TestCanceledStdinWriteDoesNotKillTerminal(t *testing.T) {
	commands := tool.NewCommands(tool.NewLocal())
	defer commands.Close()
	id := backgroundID(t, callTool(t, commands.ExecCommand(), map[string]any{"command": "sleep 30", "yield_seconds": 1}))
	ctx, cancel := context.WithTimeout(t.Context(), 50*time.Millisecond)
	defer cancel()
	input, _ := json.Marshal(map[string]any{"session_id": id, "chars": strings.Repeat("x", 1<<20)})
	if _, err := commands.WriteStdin().Run(ctx, model.ToolCall{Input: string(input)}); err == nil {
		t.Fatal("blocked stdin write did not cancel")
	}
	if err := commands.Refresh(t.Context()); err != nil {
		t.Fatal(err)
	}
	if commands.Terminals()[0].Status != "running" {
		t.Fatal("canceling stdin killed the background process")
	}
}

func TestConcurrentStopAndSessionCloseReapTheProcess(t *testing.T) {
	commands := tool.NewCommands(tool.NewLocal())
	id := backgroundID(t, callTool(t, commands.ExecCommand(), map[string]any{"command": "sleep 30", "yield_seconds": 1}))
	var group sync.WaitGroup
	for _, close := range []func() error{func() error { return commands.Stop(t.Context(), id) }, commands.Close} {
		group.Go(func() {
			if err := close(); err != nil {
				t.Error(err)
			}
		})
	}
	group.Wait()
	for _, info := range commands.Terminals() {
		if info.Status == "running" || info.Poll.Running {
			t.Fatalf("session close left a live process: %+v", info)
		}
	}
}
