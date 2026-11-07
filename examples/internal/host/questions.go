package host

import (
	"context"
	"errors"
	"sync"

	"clara-code/pkg/tools"
)

// Questions bridges a tool request to a UI. Each conversation has one active prompt.
type Questions struct {
	mu      sync.Mutex
	pending chan map[string]string
	Publish func(context.Context, []tools.Question) error
}

func (q *Questions) Ask(ctx context.Context, questions []tools.Question) (map[string]string, error) {
	replies := make(chan map[string]string, 1)
	q.mu.Lock()
	if q.pending != nil {
		q.mu.Unlock()
		return nil, errors.New("an input request is already pending")
	}
	q.pending = replies
	q.mu.Unlock()
	defer func() {
		q.mu.Lock()
		q.pending = nil
		q.mu.Unlock()
	}()
	if err := q.Publish(ctx, questions); err != nil {
		return nil, err
	}
	select {
	case answer := <-replies:
		return answer, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (q *Questions) Answer(answers map[string]string) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.pending == nil {
		return errors.New("no input request is pending")
	}
	select {
	case q.pending <- answers:
		return nil
	default:
		return errors.New("input request already has an answer")
	}
}
