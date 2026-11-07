package coding

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"time"

	"clara-core/models"

	"clara-code/internal/session"
)

type activityLog struct {
	mu         sync.Mutex
	calls      map[string]session.ToolActivity
	aliases    map[string]string
	background map[string]string
	publish    func(session.ToolActivity) error
	currentRun func() string
}

func newActivityLog(currentRun func() string, publish func(session.ToolActivity) error) *activityLog {
	return &activityLog{calls: map[string]session.ToolActivity{}, aliases: map[string]string{}, background: map[string]string{}, publish: publish, currentRun: currentRun}
}

func (l *activityLog) start(call models.ToolCallContent) error {
	if call.ToolName == "request_user_input" {
		return nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	key := l.currentRun() + ":" + call.ToolCallID
	if call.ToolName == "write_stdin" {
		var args struct {
			SessionID string `json:"session_id"`
		}
		_ = json.Unmarshal([]byte(call.Input), &args)
		if original, ok := l.background[args.SessionID]; ok {
			l.aliases[key] = original
			activity := l.calls[original]
			activity.Status = "running"
			activity.Finished = time.Time{}
			l.calls[original] = activity
			return l.publish(activity)
		}
	}
	activity := session.NewToolActivity(call, time.Now())
	activity.ID = key
	l.calls[key] = activity
	return l.publish(activity)
}

func (l *activityLog) complete(result models.ToolResultContent) error {
	if result.ToolName == "request_user_input" {
		return nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	id := l.currentRun() + ":" + result.ToolCallID
	if original, ok := l.aliases[id]; ok {
		delete(l.aliases, id)
		id = original
	}
	activity, ok := l.calls[id]
	if !ok {
		activity = session.NewToolActivity(models.ToolCallContent{ToolCallID: result.ToolCallID, ToolName: result.ToolName}, time.Now())
	}
	if activity.ID == "" {
		activity.ID = id
	}
	previousSession := activity.SessionID
	activity = activity.WithResult(result, time.Now())
	l.calls[id] = activity
	if previousSession != "" {
		delete(l.background, previousSession)
	}
	if activity.Background() && activity.SessionID != "" {
		l.background[activity.SessionID] = id
	}
	err := l.publish(activity)
	if err == nil && !activity.Background() {
		delete(l.calls, id)
	}
	return err
}

func (l *activityLog) finish(err error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	for id, activity := range l.calls {
		if !activity.Active() {
			continue
		}
		activity.Finished = time.Now()
		activity.DurationMS = max(0, activity.Finished.Sub(activity.Started).Milliseconds())
		activity.Status = "failed"
		activity.Error = "Execution ended before a result was recorded"
		if errors.Is(err, context.Canceled) {
			activity.Status = "cancelled"
			activity.Error = "Interrupted"
		}
		if errors.Is(err, context.DeadlineExceeded) {
			activity.Error = "Timed out before a result was recorded"
		}
		l.calls[id] = activity
		_ = l.publish(activity)
		delete(l.calls, id)
		delete(l.background, activity.SessionID)
		for alias, original := range l.aliases {
			if original == id {
				delete(l.aliases, alias)
			}
		}
	}
}
