//go:build unix

package cli

import "testing"

func TestPTYStatusFollowsTZ(t *testing.T) {
	t.Setenv("TZ", "Asia/Shanghai")
	p := startPTY(t, t.TempDir(), t.TempDir())
	p.await(t, 0, "Ready")
	p.send(t, "/status\r")
	p.await(t, 0, "Time zone  Asia/Shanghai (UTC+08:00)")
	p.await(t, 0, "Local time  ")
	p.send(t, "/exit\r")
	p.await(t, 0, "\x1b[?2004l\x1b[?25h")
}
