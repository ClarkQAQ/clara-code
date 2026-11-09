package filelock

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"
)

func TestLockContentionAndRelease(t *testing.T) {
	path := filepath.Join(t.TempDir(), "fixture.lock")
	first, err := Try(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { first.Close() })
	if other, err := Try(path); !errors.Is(err, ErrBusy) {
		if other != nil {
			other.Close()
		}
		t.Fatalf("second lock: %v", err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 25*time.Millisecond)
	defer cancel()
	if other, err := Acquire(ctx, path); !errors.Is(err, context.DeadlineExceeded) {
		if other != nil {
			other.Close()
		}
		t.Fatalf("wait for lock: %v", err)
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}
	second, err := Try(path)
	if err != nil {
		t.Fatal(err)
	}
	second.Close()
}
