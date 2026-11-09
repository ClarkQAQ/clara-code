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

func TestAttachmentCardsAndConfigOverridesInTmux(t *testing.T) {
	home, cwd := t.TempDir(), t.TempDir()
	path := filepath.Join(cwd, "test image.png")
	if err := os.WriteFile(path, imagePNG(t), 0600); err != nil {
		t.Fatal(err)
	}
	call := startTmux(t, home, "--demo", "--cwd", cwd, "-c", "tui.animations=false", "-c", "tui.tool_output_lines=3")
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
		t.Fatalf("missing %s:\n%s", needle, text)
		return ""
	}
	await("Ready")
	call("send-keys", "-t", "cursor", "-l", "/attach \""+path+"\"")
	call("send-keys", "-t", "cursor", "Enter")
	await("❯ [Image #1]")
	call("send-keys", "-t", "cursor", "-l", " after")
	call("send-keys", "-t", "cursor", "Home")
	call("send-keys", "-t", "cursor", "-l", "before ")
	await("❯ before [Image #1] after")
	var ix, iy, iv int
	if _, err := fmt.Sscanf(call("display-message", "-p", "-t", "cursor", "#{cursor_x} #{cursor_y} #{cursor_flag}"), "%d %d %d", &ix, &iy, &iv); err != nil {
		t.Fatal(err)
	}
	if ix != 9 || iv != 1 {
		t.Fatal("inline editing displaced native cursor", ix, iy, iv)
	}
	call("resize-window", "-t", "cursor", "-x", "48", "-y", "16")
	await("[Image #1]")
	call("send-keys", "-t", "cursor", "C-g")
	await("Image attachments")
	call("send-keys", "-t", "cursor", "Delete")
	await("❯ before  after")
	call("send-keys", "-t", "cursor", "C-u", "C-k")
	await("Run /review")
	text := call("capture-pane", "-p", "-t", "cursor")
	if strings.Contains(text, "[Image #1]") {
		t.Fatal("deleted attachment left resize residue", text)
	}
	var x, y, visible int
	if _, err := fmt.Sscanf(call("display-message", "-p", "-t", "cursor", "#{cursor_x} #{cursor_y} #{cursor_flag}"), "%d %d %d", &x, &y, &visible); err != nil {
		t.Fatal(err)
	}
	if x != 2 || visible != 1 {
		t.Fatalf("attachment UI displaced cursor: %d %d %d", x, y, visible)
	}
}
