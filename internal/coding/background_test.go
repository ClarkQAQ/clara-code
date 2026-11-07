package coding

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"clara-code/pkg/core/prompt"
	"clara-code/pkg/model"
	toolapi "clara-code/pkg/tool"

	"clara-code/internal/session"
)

func waitBackground(t *testing.T, c *Conversation, predicate func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if predicate() {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("background transition missing: %+v", c.Terminals())
}

func startBackgroundFixture(t *testing.T, command string) (*Conversation, Host) {
	t.Helper()
	count := 0
	c, h := testHost(t, t.TempDir(), fixtureModel{reply: func(model.Call) (*model.Response, error) {
		count++
		if count == 1 {
			return toolReply("exec_command", map[string]any{"command": command, "yield_seconds": 1}, 1), nil
		}
		return demoText("The command continues in the background."), nil
	}})
	finish(t, c, prompt.Execute, "Start the background job.")
	return c, h
}

func TestIdleBackgroundCompletionPersistsWithoutRewritingTurn(t *testing.T) {
	c, h := startBackgroundFixture(t, "printf first; sleep 1.5; printf last; exit 3")
	turns, err := c.Store.Turns()
	if err != nil || len(turns) != 1 {
		t.Fatalf("turns %v %v", turns, err)
	}
	// Completed turn objects are immutable even though their rendered command changes.
	pattern := filepath.Join(h.Config.Home, "turns", turns[0].ID+".json")
	before, err := os.ReadFile(pattern)
	if err != nil {
		t.Fatal(err)
	}
	waitBackground(t, c, func() bool {
		for _, cell := range c.Store.Commands() {
			if cell.Name == "exec_command" && cell.Status == "failed" && strings.Contains(cell.Output, "last") {
				return true
			}
		}
		return false
	})
	after, err := os.ReadFile(pattern)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatal("background update rewrote a turn", err)
	}
	latest, err := c.Store.ReadTurn(0)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, cell := range latest.Cells {
		if cell.Tool != nil && cell.Tool.Name == "exec_command" {
			found = cell.Tool.Status == "failed" && cell.Tool.ExitCode != nil && *cell.Tool.ExitCode == 3 && strings.Contains(cell.Tool.Output, "firstlast")
		}
	}
	if !found {
		t.Fatalf("latest command state missing: %+v", latest.Cells)
	}
}

func TestBackgroundStopScopeAndRecoveryDoNotResumeProcesses(t *testing.T) {
	c, h := startBackgroundFixture(t, "printf ready; sleep 30")
	ids := c.Terminals()
	if len(ids) != 1 {
		t.Fatal(ids)
	}
	child, err := c.Store.Fork(t.Context(), -1)
	if err != nil {
		t.Fatal(err)
	}
	defer child.Close()
	for _, record := range child.Commands() {
		if record.Active() || record.Background() {
			t.Fatal("branch copied a live command")
		}
	}
	if err := c.StopTerminals(t.Context(), ids[0].ID); err != nil {
		t.Fatal(err)
	}
	waitBackground(t, c, func() bool {
		for _, info := range c.Terminals() {
			if info.Status == "running" {
				return false
			}
		}
		return true
	})
	if c.Session.Active() != nil {
		t.Fatal("stop started an Agent turn")
	}
	id := c.Session.ID()
	if err := c.Close(); err != nil {
		t.Fatal(err)
	}
	store, err := session.Open(h.Config.Home, id, c.Workspace.Root)
	if err != nil {
		t.Fatal(err)
	}
	reopened, err := h.Open(t.Context(), c.Workspace, store, NewQuestions(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	if len(reopened.Terminals()) != 0 {
		t.Fatal("recovery created a background handle")
	}
	for _, record := range store.Commands() {
		if record.Active() || record.Background() {
			t.Fatal("recovery displayed stale running state")
		}
	}
}

func TestCancelOnlyInterruptsTheWait(t *testing.T) {
	c, _ := startBackgroundFixture(t, "printf ready; sleep 30")
	id := c.Terminals()[0].ID
	ctx, cancel := context.WithCancel(t.Context())
	var waiter model.AgentTool
	for _, tool := range c.items {
		if tool.Info().Name == "write_stdin" {
			waiter = tool
		}
	}
	done := make(chan error, 1)
	go func() {
		_, err := waiter.Run(ctx, model.ToolCall{ID: "wait", Input: `{"session_id":"` + id + `","yield_seconds":30}`})
		done <- err
	}()
	time.Sleep(20 * time.Millisecond)
	cancel()
	if err := <-done; err == nil {
		t.Fatal("wait was not interrupted")
	}
	if len(c.Terminals()) != 1 || c.Terminals()[0].Status != "running" {
		t.Fatal("Esc canceled background process")
	}
	interactions := 0
	for _, record := range c.Store.Commands() {
		if record.Interaction == "wait" {
			interactions++
			if record.Status != "cancelled" {
				t.Fatal(record)
			}
		}
	}
	if interactions != 1 {
		t.Fatal("waiting record missing")
	}
}

func TestTaskTerminalsBelongToTheParentClaraSession(t *testing.T) {
	c, _ := testHost(t, t.TempDir(), DemoModel{})
	setup, err := c.buildTask(t.Context(), toolapi.TaskSpec{Name: "builder", Role: "general"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	var executor model.AgentTool
	for _, tool := range setup.Tools {
		if tool.Info().Name == "exec_command" {
			executor = tool
		}
	}
	result, err := executor.Run(t.Context(), model.ToolCall{ID: "child-command", Input: `{"command":"printf child-marker; sleep 30","yield_seconds":1}`})
	if err != nil || result.IsError {
		t.Fatal(result, err)
	}
	terminals := c.Terminals()
	if len(terminals) != 1 || terminals[0].Status != "running" {
		t.Fatal("parent did not manage child terminal", terminals)
	}
	if c.Session.Active() != nil {
		t.Fatal("child progress started a foreground turn")
	}
	if err := c.StopTerminals(t.Context(), terminals[0].ID); err != nil {
		t.Fatal(err)
	}
	commands := c.Store.Commands()
	if len(commands) != 1 || commands[0].Owner != "task:builder" || commands[0].Status != "cancelled" || !strings.Contains(commands[0].Output, "child-marker") {
		t.Fatal("child snapshot was not persisted in the parent session", commands)
	}
}
