//go:build unix

package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestSkillsMentionAndManagementInTmux(t *testing.T) {
	home, cwd := t.TempDir(), t.TempDir()
	path := filepath.Join(cwd, ".agents", "skills", "review_example")
	if err := os.MkdirAll(path, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(path, "SKILL.md"), []byte("---\nname: review_example\ndescription: Review example changes\n---\nRead the changes first."), 0600); err != nil {
		t.Fatal(err)
	}
	call := startTmux(t, home, "--demo", "--cwd", cwd)
	await := func(label string, predicate func(string) bool) string {
		t.Helper()
		deadline := time.Now().Add(8 * time.Second)
		var text string
		for time.Now().Before(deadline) {
			text = call("capture-pane", "-p", "-t", "cursor")
			if predicate(text) {
				return text
			}
			time.Sleep(20 * time.Millisecond)
		}
		t.Fatalf("%s:\n%s", label, text)
		return ""
	}
	send := func(text string) { call("send-keys", "-t", "cursor", "-l", text) }
	await("ready", func(s string) bool { return strings.Contains(s, "Ready") })
	send("$rev")
	await("inline skill menu", func(s string) bool { return strings.Contains(s, "[Skill]") && strings.Contains(s, "review_example") })
	call("send-keys", "-t", "cursor", "Enter")
	await("mention inserted without submitting", func(s string) bool {
		return strings.Contains(s, "❯ $review_example") && !strings.Contains(s, "[Skill]")
	})
	call("send-keys", "-t", "cursor", "BSpace", "BSpace")
	await("atomic skill deletion", func(s string) bool {
		return strings.Contains(s, "❯ Run /review") && !strings.Contains(s, "❯ $review_example")
	})
	send("/skills")
	call("send-keys", "-t", "cursor", "Enter")
	await("skills management", func(s string) bool {
		return strings.Contains(s, "Choose an action") && strings.Contains(s, "Enable/Disable Skills")
	})
	call("send-keys", "-t", "cursor", "2")
	await("toggle view", func(s string) bool { return strings.Contains(s, "[x] review_example") })
	call("send-keys", "-t", "cursor", "Space")
	await("disabled", func(s string) bool { return strings.Contains(s, "[ ] review_example") })
	for _, size := range [][2]int{{34, 12}, {86, 28}} {
		call("resize-window", "-t", "cursor", "-x", fmt.Sprint(size[0]), "-y", fmt.Sprint(size[1]))
		await("resize", func(s string) bool { return strings.Count(s, "[ ] review_example") == 1 })
		var x, y, visible int
		if _, err := fmt.Sscanf(call("display-message", "-p", "-t", "cursor", "#{cursor_x} #{cursor_y} #{cursor_flag}"), "%d %d %d", &x, &y, &visible); err != nil || visible != 1 || x != 2 || y < 0 || y >= size[1] {
			t.Fatal("search cursor misplaced", x, y, visible, err)
		}
	}
	call("send-keys", "-t", "cursor", "Escape")
	await("back to management", func(s string) bool { return strings.Contains(s, "Choose an action") })
	call("send-keys", "-t", "cursor", "Escape")
	await("back to composer", func(s string) bool { return strings.Contains(s, "Ready") && !strings.Contains(s, "Choose an action") })
	send("$")
	await("disabled mention omitted", func(s string) bool { return strings.Contains(s, "No matching enabled skills") })
	call("send-keys", "-t", "cursor", "Escape", "C-u")
	send("/skills")
	call("send-keys", "-t", "cursor", "Enter")
	await("reopened manager", func(s string) bool { return strings.Contains(s, "Choose an action") })
	call("send-keys", "-t", "cursor", "2")
	await("persisted disabled state", func(s string) bool { return strings.Contains(s, "[ ] review_example") })
	call("send-keys", "-t", "cursor", "Enter")
	await("re-enabled", func(s string) bool { return strings.Contains(s, "[x] review_example") })
	call("send-keys", "-t", "cursor", "Escape")
	await("management parent", func(s string) bool { return strings.Contains(s, "Choose an action") })
	call("send-keys", "-t", "cursor", "Escape")
	await("composer restored", func(s string) bool { return strings.Contains(s, "Ready") })
	history := call("capture-pane", "-p", "-S", "-2000", "-t", "cursor")
	if strings.Contains(history, "Choose an action") || strings.Contains(history, "[x] review_example") || strings.Contains(history, "[ ] review_example") {
		t.Fatal("skills transient UI leaked into scrollback", history)
	}
}
