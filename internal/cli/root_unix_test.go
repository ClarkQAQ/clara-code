//go:build unix

package cli

import (
	"os"
	"syscall"
	"testing"

	"clara-code/internal/coding"
	"clara-code/internal/session"
)

func testRootSignals(t *testing.T, binary string) {
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
}
