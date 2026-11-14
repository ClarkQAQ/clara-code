//go:build unix

package cli

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type tmuxCursor struct {
	x, y, visible int
}

func TestInlineNativeCursorInTmux(t *testing.T) {
	home, cwd := t.TempDir(), t.TempDir()
	call := startTmux(t, home, "--demo", "--cwd", cwd)
	inspect := func() (string, tmuxCursor) {
		t.Helper()
		text := call("capture-pane", "-p", "-S", "-2000", "-t", "cursor")
		var cursor tmuxCursor
		if _, err := fmt.Sscanf(call("display-message", "-p", "-t", "cursor", "#{cursor_x} #{cursor_y} #{cursor_flag}"), "%d %d %d", &cursor.x, &cursor.y, &cursor.visible); err != nil {
			t.Fatal(err)
		}
		return text, cursor
	}
	await := func(label string, predicate func(string, tmuxCursor) bool) {
		t.Helper()
		deadline := time.Now().Add(8 * time.Second)
		var text string
		var cursor tmuxCursor
		for time.Now().Before(deadline) {
			text, cursor = inspect()
			if predicate(text, cursor) {
				return
			}
			time.Sleep(20 * time.Millisecond)
		}
		t.Fatalf("%s: cursor %+v\n%s", label, cursor, text)
	}
	onInput := func(text string, cursor tmuxCursor, content string, column int) bool {
		// Capture only the visible pane here, since cursor_y excludes scrollback.
		visible := strings.Split(strings.TrimSuffix(call("capture-pane", "-p", "-t", "cursor"), "\n"), "\n")
		return cursor.visible == 1 && cursor.x == column && cursor.y < len(visible) && strings.HasPrefix(visible[cursor.y], "❯ "+content)
	}
	await("placeholder cursor", func(text string, cursor tmuxCursor) bool {
		return strings.Contains(text, "Ready") && onInput(text, cursor, "Run /review", 2)
	})
	call("send-keys", "-t", "cursor", "-l", "ni hao")
	await("pinyin candidate anchor", func(text string, cursor tmuxCursor) bool { return onInput(text, cursor, "ni hao", 8) })
	call("send-keys", "-t", "cursor", "Left", "Left", "Left")
	await("middle of input", func(text string, cursor tmuxCursor) bool { return onInput(text, cursor, "ni hao", 5) })
	call("send-keys", "-t", "cursor", "C-u")
	call("send-keys", "-t", "cursor", "-l", "你好👨‍👩‍👧‍👦")
	await("chinese and emoji columns", func(text string, cursor tmuxCursor) bool {
		return onInput(text, cursor, "你好👨‍👩‍👧‍👦hao", 8)
	})
	call("send-keys", "-t", "cursor", "C-u", "C-k")
	call("send-keys", "-t", "cursor", "-l", "cursor history probe")
	call("send-keys", "-t", "cursor", "Enter")
	await("completed turn", func(text string, cursor tmuxCursor) bool {
		return strings.Contains(text, "Demo response: cursor history probe") && strings.Contains(text, "Ready") && onInput(text, cursor, "Run /review", 2)
	})
	call("send-keys", "-t", "cursor", "-l", "original draft")
	call("send-keys", "-t", "cursor", "Left", "Left")
	await("original draft cursor", func(text string, cursor tmuxCursor) bool { return onInput(text, cursor, "original draft", 14) })
	call("send-keys", "-t", "cursor", "C-r")
	await("history search cursor", func(text string, cursor tmuxCursor) bool { return onInput(text, cursor, "Search input history", 2) })
	const query = "你好世界你好世界你好"
	call("send-keys", "-t", "cursor", "-l", query)
	call("send-keys", "-t", "cursor", "Left")
	await("Unicode query cursor", func(text string, cursor tmuxCursor) bool { return onInput(text, cursor, query, 20) })
	call("resize-window", "-t", "cursor", "-x", "18", "-y", "10")
	await("wrapped query cursor", func(text string, cursor tmuxCursor) bool { return onInput(text, cursor, "你好", 4) })
	call("resize-window", "-t", "cursor", "-x", "64", "-y", "24")
	await("resized query cursor", func(text string, cursor tmuxCursor) bool { return onInput(text, cursor, query, 20) })
	call("send-keys", "-t", "cursor", "Escape")
	await("restored draft cursor", func(text string, cursor tmuxCursor) bool { return onInput(text, cursor, "original draft", 14) })
	call("send-keys", "-t", "cursor", "C-u", "C-k")
	call("send-keys", "-t", "cursor", "-l", "CANCEL_SLOW_TURN")
	call("send-keys", "-t", "cursor", "Enter")
	await("working input", func(text string, cursor tmuxCursor) bool {
		return strings.Contains(text, "Working") && onInput(text, cursor, "Run /review", 2)
	})
	call("send-keys", "-t", "cursor", "-l", "ni hao")
	await("animated activity anchor", func(text string, cursor tmuxCursor) bool { return onInput(text, cursor, "ni hao", 8) })
	for range 3 {
		time.Sleep(110 * time.Millisecond)
		text, cursor := inspect()
		if !onInput(text, cursor, "ni hao", 8) {
			t.Fatalf("activity redraw moved the input cursor: %+v\n%s", cursor, text)
		}
	}
	for _, size := range [][2]string{{"38", "14"}, {"64", "24"}} {
		call("resize-window", "-t", "cursor", "-x", size[0], "-y", size[1])
		await("resized input anchor", func(text string, cursor tmuxCursor) bool {
			return strings.Count(text, "❯ cursor history probe") == 1 && strings.Count(text, "Demo response: cursor history probe") == 1 && onInput(text, cursor, "ni hao", 8)
		})
	}
	call("send-keys", "-t", "cursor", "Escape")
	await("interrupted draft", func(text string, cursor tmuxCursor) bool {
		return strings.Contains(text, "■ Conversation interrupted") && onInput(text, cursor, "ni hao", 8)
	})
	call("send-keys", "-t", "cursor", "C-u")
	call("send-keys", "-t", "cursor", "-l", "/model")
	call("send-keys", "-t", "cursor", "Enter")
	await("read-only selector", func(text string, cursor tmuxCursor) bool {
		return strings.Contains(text, "Select model") && cursor.visible == 0
	})
	call("send-keys", "-t", "cursor", "Escape")
	await("restored input cursor", func(text string, cursor tmuxCursor) bool { return onInput(text, cursor, "Run /review", 2) })
}

func startTmux(t *testing.T, home string, args ...string) func(...string) string {
	t.Helper()
	tmuxPath, err := exec.LookPath("tmux")
	if err != nil {
		t.Skip("tmux is not installed")
	}
	socket := "clara-cursor-" + filepath.Base(filepath.Dir(home))
	call := func(args ...string) string {
		t.Helper()
		command := exec.CommandContext(t.Context(), tmuxPath, append([]string{"-L", socket, "-f", "/dev/null"}, args...)...)
		output, err := command.CombinedOutput()
		if err != nil {
			t.Fatalf("tmux %v: %v\n%s", args, err, output)
		}
		return string(output)
	}
	t.Cleanup(func() {
		_ = exec.CommandContext(context.WithoutCancel(t.Context()), tmuxPath, "-L", socket, "kill-server").Run()
	})
	command := []string{"new-session", "-d", "-s", "cursor", "-x", "64", "-y", "24", "env", childEnv + "=1", "CLARA_CODE_HOME=" + home, "TERM=xterm-256color", os.Args[0]}
	call(append(command, args...)...)
	return call
}
