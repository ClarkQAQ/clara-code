//go:build unix

package coding

import (
	"context"
	"errors"
	"os"
	"os/exec"

	"golang.org/x/sys/unix"
)

// ExecACP runs inside the child process. Core forwards ACP child stderr to the
// host stderr, so redirect it here before replacing the child with the server.
func ExecACP(_ context.Context, args []string) error {
	if len(args) == 0 {
		return errors.New("external ACP command is missing")
	}
	null, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	if err != nil {
		return err
	}
	defer null.Close()
	if err := unix.Dup2(int(null.Fd()), int(os.Stderr.Fd())); err != nil {
		return err
	}
	path, err := exec.LookPath(args[0])
	if err != nil {
		return err
	}
	return unix.Exec(path, args, os.Environ())
}
