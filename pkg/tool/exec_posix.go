//go:build !windows

package tool

import (
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"time"
)

func resolveLocalShell() (string, error) {
	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "/bin/sh"
	}
	return shell, nil
}

func startLocalProcess(ctx context.Context, shell string, spec Spec, output io.Writer) (io.WriteCloser, func() (int, error), func() error, error) {
	command := exec.CommandContext(ctx, shell, "-lc", spec.Command)
	command.Dir = spec.Workdir
	command.Env = environment(spec.Env)
	command.WaitDelay = 2 * time.Second
	stdin, drain, err := startProcess(command, spec.TTY, output)
	if err != nil {
		return nil, nil, nil, err
	}
	wait := func() (int, error) {
		err := command.Wait()
		drain()
		var exit *exec.ExitError
		if errors.As(err, &exit) {
			return exit.ExitCode(), nil
		}
		return 0, err
	}
	return stdin, wait, func() error { return killProcess(command.Process) }, nil
}

func startProcess(command *exec.Cmd, tty bool, output io.Writer) (io.WriteCloser, func(), error) {
	if tty {
		return startTerminal(command, output)
	}
	configureProcess(command)
	stdin, err := command.StdinPipe()
	if err != nil {
		return nil, nil, err
	}
	command.Stdout, command.Stderr = output, output
	if err := command.Start(); err != nil {
		return nil, nil, errors.Join(err, stdin.Close())
	}
	return stdin, func() {}, nil
}
