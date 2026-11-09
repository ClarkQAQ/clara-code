package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"clara-code/internal/session"
)

func TestInputHistoryJSONLSurvivesRestart(t *testing.T) {
	home, cwd := t.TempDir(), t.TempDir()
	p := startPTY(t, home, cwd)
	p.await(t, 0, "Ready")
	mark := len(p.output())
	p.send(t, "/status\r")
	p.await(t, mark, "Session  ")
	p.send(t, "/exit\r")
	p.await(t, mark, "\x1b[?2004l\x1b[?25h")
	if err := p.cmd.Wait(); err != nil {
		t.Fatal(err)
	}
	entries, err := session.ReadInputHistory(home)
	if err != nil || len(entries) != 2 || entries[0].Text != "/status" || entries[1].Text != "/exit" {
		t.Fatal("CLI did not persist command history", entries, err)
	}
	q := startPTY(t, home, cwd)
	q.await(t, 0, "Ready")
	q.send(t, "\x1b[A")
	q.await(t, 0, "/exit")
	mark = len(q.output())
	q.send(t, "\x1b[A")
	q.await(t, mark, "/status")
	mark = len(q.output())
	q.send(t, "\x1b[B")
	q.await(t, mark, "/exit")
}

func TestAgentAndPlanEffortsInTmux(t *testing.T) {
	home, cwd := t.TempDir(), t.TempDir()
	if err := os.WriteFile(filepath.Join(home, "config.toml"), []byte("plan_mode_reasoning_effort=\"high\"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	call := startTmux(t, home, "--demo", "--cwd", cwd)
	await := func(needle string) string {
		t.Helper()
		deadline := time.Now().Add(8 * time.Second)
		var text string
		for time.Now().Before(deadline) {
			text = call("capture-pane", "-p", "-t", "cursor")
			if strings.Contains(text, needle) {
				return text
			}
			time.Sleep(20 * time.Millisecond)
		}
		t.Fatalf("missing %q:\n%s", needle, text)
		return ""
	}
	await("Ready")
	call("send-keys", "-t", "cursor", "-l", "/model")
	call("send-keys", "-t", "cursor", "Enter")
	await("Select model")
	call("send-keys", "-t", "cursor", "Enter")
	await("Agent (1/2)")
	call("send-keys", "-t", "cursor", "Home", "Down", "Down", "Down", "Enter")
	text := await("Plan (2/2)")
	if !strings.Contains(text, "High (default) (current)") {
		t.Fatal("Plan annotations missing", text)
	}
	call("send-keys", "-t", "cursor", "Home", "Enter")
	await("Model set to demo low")
	call("send-keys", "-t", "cursor", "-l", "/status")
	call("send-keys", "-t", "cursor", "Enter")
	await("Agent effort  low")
	await("Plan effort  low")
}
