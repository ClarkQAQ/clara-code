package coding

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"time"

	"clara-code/internal/session"
	"clara-code/pkg/model"
	"clara-code/pkg/tool"
)

type activityLog struct {
	mu         sync.Mutex
	calls      map[string]session.ToolActivity
	background map[string]string
	publish    func(session.ToolActivity) error
	currentRun func() string
}

func newActivityLog(currentRun func() string, publish func(session.ToolActivity) error) *activityLog {
	return &activityLog{calls: map[string]session.ToolActivity{}, background: map[string]string{}, publish: publish, currentRun: currentRun}
}

func (l *activityLog) start(call model.ToolCallContent) error {
	return l.startKey(call, l.currentRun()+":"+call.ToolCallID, "")
}

func (l *activityLog) startKey(call model.ToolCallContent, key, owner string) error {
	if call.ToolName == "request_user_input" {
		return nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	activity := session.NewToolActivity(call, time.Now())
	activity.ID, activity.Owner = key, owner
	if call.ToolName == "write_stdin" {
		var args struct {
			SessionID string `json:"session_id"`
			Chars     string `json:"chars"`
			Stop      bool   `json:"stop"`
		}
		_ = json.Unmarshal([]byte(call.Input), &args)
		activity.SessionID = args.SessionID
		activity.Interaction = "wait"
		activity.Title = "Waiting for background terminal"
		if args.Chars != "" {
			activity.Interaction = "input"
			activity.Title = "Interacted with background terminal"
			activity.Input, _ = session.PreviewOutput(args.Chars, 8<<10)
		}
		if args.Stop {
			activity.Interaction = "stop"
			activity.Title = "Stopped background terminal"
		}
		if original, ok := l.calls[l.background[args.SessionID]]; ok {
			activity.Detail = original.Detail
		}
	}
	l.calls[key] = activity
	return l.publish(activity)
}

func (l *activityLog) complete(result model.ToolResultContent) error {
	return l.completeKey(result, l.currentRun()+":"+result.ToolCallID)
}

func (l *activityLog) completeKey(result model.ToolResultContent, key string) error {
	if result.ToolName == "request_user_input" {
		return nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	activity, ok := l.calls[key]
	if !ok {
		activity = session.NewToolActivity(model.ToolCallContent{ToolCallID: result.ToolCallID, ToolName: result.ToolName}, time.Now())
		activity.ID = key
	}
	completed := activity.WithResult(result, time.Now())
	if activity.Interaction != "" {
		completed.Output, completed.Error, completed.ExitCode = "", "", nil
		completed.Status = "succeeded"
		if _, failed := model.AsToolResultOutputType[model.ToolResultOutputContentError](result.Result); failed {
			completed.Status = "failed"
			completed.Error = session.ToolOutput(result).Text
		}
		if activity.Interaction == "wait" {
			completed.Title = "Waited for background terminal"
		}
	}
	l.calls[key] = completed
	if completed.Interaction == "" && completed.SessionID != "" {
		l.background[completed.SessionID] = key
	}
	l.trimLocked()
	err := l.publish(completed)
	if completed.SessionID == "" || completed.Interaction != "" {
		delete(l.calls, key)
	}
	return err
}

func (l *activityLog) terminal(info tool.TerminalInfo) (session.ToolActivity, bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	key := l.background[info.ID]
	activity, ok := l.calls[key]
	if !ok || info.Revision < activity.CommandRevision {
		return session.ToolActivity{}, false
	}
	previous := activity
	if info.Observable {
		activity.Output, activity.Omitted = session.PreviewOutput(info.Poll.Output, 16<<10)
	} else if info.Revision > activity.CommandRevision {
		var omitted int
		activity.Output, omitted = session.PreviewOutput(activity.Output+info.Poll.Output, 16<<10)
		activity.Omitted += omitted
	}
	if info.Poll.OutputOffset > 0 {
		activity.Omitted++
	}
	activity.CommandRevision = info.Revision
	activity.DurationMS = info.Poll.Duration.Milliseconds()
	activity.Status = "background"
	if info.Status != "running" {
		activity.Status = info.Status
		if info.Status == "stopped" {
			activity.Status = "cancelled"
		}
		if info.Status == "timed_out" {
			activity.Status = "failed"
			activity.Error = "Command timed out"
		}
		activity.Finished = activity.Started.Add(info.Poll.Duration)
		exit := info.Poll.ExitCode
		activity.ExitCode = &exit
		if info.Error != "" {
			activity.Error = info.Error
		}
	}
	l.calls[key] = activity
	if previous.CommandRevision == activity.CommandRevision && previous.Status == activity.Status && previous.Output == activity.Output {
		return session.ToolActivity{}, false
	}
	l.trimLocked()

	return activity, true
}

func (l *activityLog) failKey(key string, err error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	activity, ok := l.calls[key]
	if !ok {
		return
	}
	activity.Status = "failed"
	activity.Error = err.Error()
	activity.Finished = time.Now()
	if errors.Is(err, context.Canceled) {
		activity.Status = "cancelled"
		activity.Error = "Interrupted"
		if activity.Interaction == "wait" {
			activity.Title = "Interrupted waiting for background terminal"
		}
	}
	_ = l.publish(activity)
	delete(l.calls, key)
}

func (l *activityLog) finish(err error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	for key, activity := range l.calls {
		if !activity.Active() || (activity.Owner != "" && activity.Owner != "main") || !strings.HasPrefix(key, l.currentRun()+":") {
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
		_ = l.publish(activity)
		delete(l.calls, key)
	}
}

func (l *activityLog) trimLocked() {
	for len(l.calls) > 128 {
		oldest := ""
		for key, value := range l.calls {
			if value.Active() || value.Background() {
				continue
			}
			if oldest == "" || value.Started.Before(l.calls[oldest].Started) {
				oldest = key
			}
		}
		if oldest == "" {
			return
		}
		delete(l.background, l.calls[oldest].SessionID)
		delete(l.calls, oldest)
	}
}
