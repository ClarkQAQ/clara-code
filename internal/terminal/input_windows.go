package terminal

import (
	"context"
	"errors"
	"io"
	"os"
	"runtime"
	"time"
	"unicode/utf16"

	"golang.org/x/sys/windows"
)

var cancelConsoleRead = windows.NewLazySystemDLL("kernel32.dll").NewProc("CancelSynchronousIo")

func readInput(ctx context.Context, file *os.File, keys chan<- key, failure chan<- error) {
	defer close(keys)
	type chunk struct {
		data []byte
		err  error
	}
	chunks := make(chan chunk)
	ready := make(chan windows.Handle, 1)
	done := make(chan struct{})
	released := make(chan struct{})
	ctx, stop := context.WithCancel(ctx)
	go func() {
		runtime.LockOSThread()
		defer func() {
			close(done)
			<-released
			runtime.UnlockOSThread()
		}()
		var thread windows.Handle
		process := windows.CurrentProcess()
		err := windows.DuplicateHandle(process, windows.CurrentThread(), process, &thread, 0, false, windows.DUPLICATE_SAME_ACCESS)
		ready <- thread
		if err != nil {
			select {
			case chunks <- chunk{err: err}:
			case <-ctx.Done():
			}
			return
		}
		console := windows.Handle(file.Fd())
		var data [4097]uint16
		pending := 0
		for ctx.Err() == nil {
			var n uint32
			err := windows.ReadConsole(console, &data[pending], uint32(len(data)-pending), &n, nil)
			count := pending + int(n)
			if n == 0 && err == nil {
				err = io.EOF
			}
			pending = 0
			if err == nil && count > 0 && data[count-1] >= 0xd800 && data[count-1] <= 0xdbff {
				count--
				pending = 1
			}
			text := string(utf16.Decode(data[:count]))
			if pending != 0 {
				data[0] = data[count]
			}
			if text == "" && err == nil {
				continue
			}
			select {
			case chunks <- chunk{data: []byte(text), err: err}:
			case <-ctx.Done():
				return
			}
			if err != nil {
				return
			}
		}
	}()
	thread := <-ready
	defer func() {
		stop()
		defer close(released)
		if thread == 0 {
			<-done
			return
		}
		defer windows.CloseHandle(thread)
		// Repeat to cover cancellation between the context check and Read.
		ticker := time.NewTicker(5 * time.Millisecond)
		defer ticker.Stop()
		for {
			_, _, _ = cancelConsoleRead.Call(uintptr(thread))
			select {
			case <-done:
				return
			case <-ticker.C:
			}
		}
	}()

	var parser decoder
	emit := func(input key) {
		select {
		case keys <- input:
		case <-ctx.Done():
		}
	}
	timer := time.NewTimer(time.Hour)
	defer timer.Stop()
	timer.Stop()
	var timeout <-chan time.Time
	for {
		select {
		case <-ctx.Done():
			return
		case <-timeout:
			parser.feed(nil, true, emit)
			timeout = nil
		case part := <-chunks:
			parser.feed(part.data, false, emit)
			if part.err != nil {
				if !errors.Is(part.err, io.EOF) && ctx.Err() == nil {
					failure <- part.err
				}
				return
			}
			timer.Stop()
			timeout = nil
			if len(parser.buffer) > 0 && !parser.pasting {
				timer.Reset(35 * time.Millisecond)
				timeout = timer.C
			}
		}
	}
}
