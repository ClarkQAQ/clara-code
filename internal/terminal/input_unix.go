//go:build unix

package terminal

import (
	"context"
	"errors"
	"os"

	"golang.org/x/sys/unix"
)

func readInput(ctx context.Context, file *os.File, keys chan<- key, failure chan<- error) {
	defer close(keys)
	fd := int(file.Fd())
	fds := []unix.PollFd{{Fd: int32(fd), Events: unix.POLLIN}}
	var data [8192]byte
	var parser decoder
	for ctx.Err() == nil {
		wait := 100
		if len(parser.buffer) > 0 && !parser.pasting {
			wait = 35
		}
		n, err := unix.Poll(fds, wait)
		if errors.Is(err, unix.EINTR) {
			continue
		}
		if err != nil {
			failure <- err
			return
		}
		emit := func(input key) {
			select {
			case keys <- input:
			case <-ctx.Done():
			}
		}
		if n == 0 {
			parser.feed(nil, true, emit)
			continue
		}
		size, err := unix.Read(fd, data[:])
		if errors.Is(err, unix.EINTR) || errors.Is(err, unix.EAGAIN) {
			continue
		}
		if err != nil {
			failure <- err
			return
		}
		if size == 0 {
			return
		}
		parser.feed(data[:size], false, emit)
	}
}
