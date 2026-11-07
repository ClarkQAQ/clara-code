package cli

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"clara-code/internal/coding"
	"clara-code/internal/session"
	"clara-code/pkg/core"
)

func TestRootEntrypoint(t *testing.T) {
	binary := filepath.Join(t.TempDir(), "clara-code")
	build := exec.CommandContext(t.Context(), "go", "build", "-o", binary, "-ldflags", "-X main.version=entry-fixture", "../..")
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("root build: %v\n%s", err, output)
	}
	for _, test := range []struct{ arg, want string }{{"--help", "clara-code resume"}, {"--version", "Clara Code entry-fixture"}} {
		output, err := exec.CommandContext(t.Context(), binary, test.arg).CombinedOutput()
		if err != nil || !strings.Contains(string(output), test.want) {
			t.Fatalf("%s: %v\n%s", test.arg, err, output)
		}
	}
	for _, signal := range []os.Signal{os.Interrupt, syscall.SIGTERM, syscall.SIGHUP} {
		t.Run(signal.String(), func(t *testing.T) {
			home, cwd := t.TempDir(), t.TempDir()
			p := startBinaryPTY(t, binary, home, "--demo", "--cwd", cwd)
			p.await(t, 0, "Ready")
			p.send(t, "entrypoint history\r")
			p.await(t, 0, "Demo response: entrypoint history")
			mark := len(p.output())
			p.send(t, "CANCEL_SLOW_TURN\r")
			p.await(t, mark, "Working")
			if err := p.cmd.Process.Signal(signal); err != nil {
				t.Fatal(err)
			}
			p.await(t, mark, "\x1b[?2004l\x1b[?25h")
			if err := p.cmd.Wait(); err != nil {
				t.Fatal("root did not exit cleanly", err)
			}
			space, err := coding.ResolveWorkspace(cwd)
			if err != nil {
				t.Fatal(err)
			}
			items, err := session.List(home, space.Root)
			if err != nil || len(items) != 1 {
				t.Fatal("saved session missing", items, err)
			}
			resumed := startBinaryPTY(t, binary, home, "resume", items[0].ID, "--demo", "--cwd", cwd)
			resumed.await(t, 0, "Demo response: entrypoint history")
			resumed.await(t, 0, "Ready")
			resumed.send(t, "continued entrypoint\r")
			resumed.await(t, 0, "Demo response: continued entrypoint")
			resumed.send(t, "/exit\r")
			resumed.await(t, 0, "\x1b[?2004l\x1b[?25h")
		})
	}
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	client, err := core.ConnectACPExternal(ctx, core.ACPExternalConfig{
		Command: binary, Args: []string{"acp", "--demo"}, Env: map[string]string{"CLARA_CODE_HOME": t.TempDir()},
	})
	if err != nil {
		t.Fatal(err)
	}
	id, err := client.NewSession(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close(ctx, id)
	if text, reason, err := client.Prompt(ctx, id, "root ACP probe"); err != nil || text != "Demo response: root ACP probe" || reason != "end_turn" {
		t.Fatal("root ACP failed", text, reason, err)
	}
}
