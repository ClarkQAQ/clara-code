//go:build unix

package terminal

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"golang.org/x/term"
)

func setupTerminal(in, _ *os.File) (func() error, error) {
	state, err := term.MakeRaw(int(in.Fd()))
	if err != nil {
		return nil, err
	}
	return func() error { return term.Restore(int(in.Fd()), state) }, nil
}

func resizeEvents(_ context.Context, _ *os.File) (<-chan os.Signal, func()) {
	resizes := make(chan os.Signal, 1)
	signal.Notify(resizes, syscall.SIGWINCH)
	return resizes, func() { signal.Stop(resizes) }
}
