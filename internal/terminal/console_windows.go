package terminal

import (
	"context"
	"errors"
	"os"
	"time"

	"golang.org/x/sys/windows"
	"golang.org/x/term"
)

func setupTerminal(in, out *os.File) (func() error, error) {
	state, err := term.MakeRaw(int(in.Fd()))
	if err != nil {
		return nil, err
	}
	restoreInput := func() error { return term.Restore(int(in.Fd()), state) }
	inHandle, outHandle := windows.Handle(in.Fd()), windows.Handle(out.Fd())
	var inputMode, outputMode uint32
	if err := windows.GetConsoleMode(inHandle, &inputMode); err != nil {
		return nil, errors.Join(err, restoreInput())
	}
	// Quick Edit can freeze interactive reads in the classic console host.
	inputMode = (inputMode | windows.ENABLE_EXTENDED_FLAGS) &^ windows.ENABLE_QUICK_EDIT_MODE
	if err := windows.SetConsoleMode(inHandle, inputMode); err != nil {
		return nil, errors.Join(err, restoreInput())
	}
	if err := windows.GetConsoleMode(outHandle, &outputMode); err != nil {
		return nil, errors.Join(err, restoreInput())
	}
	if err := windows.SetConsoleMode(outHandle, outputMode|windows.ENABLE_PROCESSED_OUTPUT|windows.ENABLE_VIRTUAL_TERMINAL_PROCESSING); err != nil {
		return nil, errors.Join(err, restoreInput())
	}
	return func() error {
		return errors.Join(windows.SetConsoleMode(outHandle, outputMode), restoreInput())
	}, nil
}

func resizeEvents(ctx context.Context, out *os.File) (<-chan os.Signal, func()) {
	ctx, stop := context.WithCancel(ctx)
	resizes := make(chan os.Signal, 1)
	var width, height int
	go func() {
		ticker := time.NewTicker(50 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				w, h, err := term.GetSize(int(out.Fd()))
				if err == nil && (w != width || h != height) {
					width, height = w, h
					select {
					case resizes <- nil:
					default:
					}
				}
			}
		}
	}()
	return resizes, stop
}
