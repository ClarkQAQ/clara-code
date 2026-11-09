//go:build !unix && !windows

package tool

import (
	"clara-code/pkg/util/errs"
	"io"
	"os"
	"os/exec"
)

func configureProcess(*exec.Cmd) {}

func killProcess(process *os.Process) error {
	return process.Kill()
}

func startTerminal(*exec.Cmd, io.Writer) (io.WriteCloser, func(), error) {
	return nil, nil, errs.New(errs.CodeUnsupported, "local PTY is unavailable on this platform")
}
