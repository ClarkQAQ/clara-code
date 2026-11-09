//go:build unix

package cli

import (
	"bytes"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"clara-code/internal/coding"
	"clara-code/internal/session"
	"github.com/creack/pty"
)

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

func TestInlineShiftEnterPTY(t *testing.T) {
	t.Setenv("TMUX", "")
	t.Setenv("NO_COLOR", "1")
	p := startPTY(t, t.TempDir(), t.TempDir())
	p.await(t, 0, "\x1b[>1u")
	p.await(t, 0, "Ready")
	mark := len(p.output())
	p.send(t, "discard\x1b[117;5ufirst\x1b[13;2usecond\x1b[27;2;13~third\x1b\rfourth\x1b[106;5ufifth")
	p.await(t, mark, "❯ first\x1b[K\r\n  second\x1b[K\r\n  third\x1b[K\r\n  fourth\x1b[K\r\n  fifth")
	if output := p.output()[mark:]; strings.Contains(output, "Demo response:") {
		t.Fatalf("expected an unsubmitted draft: %q", output)
	}
	p.send(t, "\r")
	p.await(t, mark, "Demo response: first")
	p.await(t, strings.LastIndex(p.output(), "Demo response: first"), "Ready")
	mark = len(p.output())
	p.send(t, "/exit\r")
	p.await(t, mark, "\x1b[<1u")
}

func TestInlineSavesConversationOnFirstMessage(t *testing.T) {
	home, cwd := t.TempDir(), t.TempDir()
	space, err := coding.ResolveWorkspace(cwd)
	if err != nil {
		t.Fatal(err)
	}
	assertNoSessions := func() {
		t.Helper()
		items, err := session.List(home, space.Root)
		if err != nil || len(items) != 0 {
			t.Fatalf("empty conversation was saved: %+v, %v", items, err)
		}
	}
	p := startPTY(t, home, cwd)
	p.await(t, 0, "Ready")
	assertNoSessions()
	p.send(t, "/resume\r")
	p.await(t, 0, "No saved conversations in this project")
	p.send(t, "/exit\r")
	if err := p.cmd.Wait(); err != nil {
		t.Fatal(err)
	}
	assertNoSessions()

	p = startPTY(t, home, cwd)
	p.await(t, 0, "Ready")
	p.send(t, "First saved message\r")
	p.await(t, 0, "Demo response: First saved message")
	p.await(t, strings.LastIndex(p.output(), "Demo response: First saved message"), "Ready")
	items, err := session.List(home, space.Root)
	if err != nil || len(items) != 1 || items[0].FirstPrompt != "First saved message" {
		t.Fatalf("first message was not saved: %+v, %v", items, err)
	}
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
