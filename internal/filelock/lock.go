// Package filelock provides process-wide coordination through persistent lock files.
package filelock

import (
	"context"
	"errors"
	"os"
	"time"
)

var ErrBusy = errors.New("file is locked by another process")

// Try acquires an exclusive lock until the returned file is closed.
func Try(path string) (*os.File, error) {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	if err := tryLock(file); err != nil {
		return nil, errors.Join(err, file.Close())
	}
	return file, nil
}

// Acquire waits for an exclusive lock until the context is canceled.
func Acquire(ctx context.Context, path string) (*os.File, error) {
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		file, err := Try(path)
		if !errors.Is(err, ErrBusy) {
			return file, err
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(10 * time.Millisecond):
		}
	}
}
