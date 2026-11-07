//go:build unix

package tool

import (
	"fmt"
	"github.com/creack/pty"
	"io"
	"os"
	"os/exec"
	"syscall"
	"time"
)

func configureProcess(command *exec.Cmd) {
	if command.SysProcAttr == nil {
		command.SysProcAttr = &syscall.SysProcAttr{}
	}
	command.SysProcAttr.Setpgid = true
}

func killProcess(process *os.Process) error {
	err := syscall.Kill(-process.Pid, syscall.SIGKILL)
	if err != nil && err != syscall.ESRCH {
		return fmt.Errorf("kill process group %d: %w", process.Pid, err)
	}
	return nil
}

func startTerminal(command *exec.Cmd, output io.Writer) (io.WriteCloser, func(), error) {
	terminal, err := pty.StartWithSize(command, &pty.Winsize{Rows: 24, Cols: 80})
	if err != nil {
		return nil, nil, err
	}
	done := make(chan struct{})
	go func() { _, _ = io.Copy(output, terminal); close(done) }()
	return terminal, func() {
		timer := time.NewTimer(500 * time.Millisecond)
		defer timer.Stop()
		select {
		case <-done:
		case <-timer.C:
		}
		_ = terminal.Close()
	}, nil
}
