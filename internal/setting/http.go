package setting

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"sync"
	"sync/atomic"
	"time"
)

type requestClient struct{ timeout, idle time.Duration }

func (c requestClient) Do(req *http.Request) (*http.Response, error) {
	ctx, cancel := context.WithCancel(req.Context())
	if c.timeout > 0 {
		cancel()
		ctx, cancel = context.WithTimeout(req.Context(), c.timeout)
	}
	resp, err := http.DefaultClient.Do(req.Clone(ctx))
	if err != nil {
		cancel()
		return nil, err
	}
	b := &timedBody{ReadCloser: resp.Body, cancel: cancel, idle: c.idle}
	if c.idle > 0 {
		b.timer = time.AfterFunc(c.idle, func() { b.expired.Store(true); _ = resp.Body.Close() })
	}
	resp.Body = b
	return resp, nil
}

type timedBody struct {
	io.ReadCloser
	cancel  context.CancelFunc
	idle    time.Duration
	timer   *time.Timer
	expired atomic.Bool
	once    sync.Once
	mu      sync.Mutex
}

func (b *timedBody) Read(p []byte) (int, error) {
	n, err := b.ReadCloser.Read(p)
	b.mu.Lock()
	if n > 0 && b.timer != nil && !b.expired.Load() {
		b.timer.Reset(b.idle)
	}
	b.mu.Unlock()
	if b.expired.Load() {
		return n, fmt.Errorf("model stream idle timeout: %w", context.DeadlineExceeded)
	}
	return n, err
}
func (b *timedBody) Close() error {
	var err error
	b.once.Do(func() {
		b.mu.Lock()
		if b.timer != nil {
			b.timer.Stop()
		}
		b.mu.Unlock()
		b.cancel()
		err = b.ReadCloser.Close()
	})
	return err
}
