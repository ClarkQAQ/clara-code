//go:build unix

package cli

import (
	"fmt"
	"strings"
	"testing"
	"time"
	"unicode/utf8"
)

func TestTranscriptMarkersInTmux(t *testing.T) {
	call := startTmux(t, t.TempDir(), "--demo", "--cwd", t.TempDir())
	const compacted = "Context compacted · "
	const forked = "Forked from timeline source"
	const childResponse = "Demo response: timeline child"
	await := func(label string, accept func(string) bool) {
		t.Helper()
		deadline := time.Now().Add(8 * time.Second)
		var history string
		for time.Now().Before(deadline) {
			history = call("capture-pane", "-p", "-S", "-", "-t", "cursor")
			if accept(history) && strings.Contains(call("capture-pane", "-p", "-t", "cursor"), "Ready") {
				return
			}
			time.Sleep(20 * time.Millisecond)
		}
		t.Fatalf("did not see %s in the transcript:\n%s", label, history)
	}
	submit := func(text string) {
		call("send-keys", "-t", "cursor", "-l", text)
		call("send-keys", "-t", "cursor", "Enter")
	}
	await("initial composer", func(string) bool { return true })
	for _, prompt := range []string{"timeline source", "timeline second"} {
		submit(prompt)
		await(prompt, func(history string) bool { return strings.Contains(history, "Demo response: "+prompt) })
	}
	submit("/compact")
	await("compaction duration", func(history string) bool { return strings.Contains(history, compacted) })
	submit("/fork")
	await("fork divider", func(history string) bool { return strings.Contains(history, forked) })
	submit("timeline child")
	await("child reply", func(history string) bool { return strings.Contains(history, childResponse) })

	call("resize-window", "-t", "cursor", "-x", "38", "-y", "14")
	call("send-keys", "-t", "cursor", "-l", "ni hao")
	call("send-keys", "-t", "cursor", "Left", "Left", "Left")
	await("ordered markers in scrollback and the resized composer cursor", func(history string) bool {
		last := -1
		for _, line := range []string{"Demo response: timeline second", compacted, forked, childResponse} {
			at := strings.Index(history, line)
			if at <= last || strings.Count(history, line) != 1 {
				return false
			}
			last = at
		}
		for line := range strings.SplitSeq(history, "\n") {
			if strings.Contains(line, forked) && utf8.RuneCountInString(line) != 38 {
				return false
			}
		}
		var cursor tmuxCursor
		report := call("display-message", "-p", "-t", "cursor", "#{cursor_x} #{cursor_y} #{cursor_flag}")
		if _, err := fmt.Sscanf(report, "%d %d %d", &cursor.x, &cursor.y, &cursor.visible); err != nil {
			t.Fatal(err)
		}
		visible := strings.Split(call("capture-pane", "-p", "-t", "cursor"), "\n")
		return cursor.visible == 1 && cursor.x == 5 && cursor.y < len(visible) && strings.HasPrefix(visible[cursor.y], "❯ ni hao")
	})
}
