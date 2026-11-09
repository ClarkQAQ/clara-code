package tool

import (
	"context"
	"errors"
	"time"
)

func (s *Commands) WriteStdin() Tool {
	type input struct {
		SessionID   string `json:"session_id" description:"actual session_id returned by exec_command"`
		Chars       string `json:"chars,omitempty" description:"characters to write; omit to wait without sending input"`
		Stop        bool   `json:"stop,omitempty" description:"stop and close this background process"`
		YieldTimeMS int    `json:"yield_time_ms,omitempty" description:"wait for output in milliseconds; default 10000, maximum 30000"`
	}
	return Func(Definition{Name: "write_stdin", Description: "Send input to a background terminal or wait for its output. Empty chars only waits. Canceling a wait leaves the background process running. Use the actual session_id, not a PID.", Effects: []Effect{EffectExecute}, Timeout: 2 * time.Minute}, func(ctx context.Context, in input) (Result, error) {
		s.mu.Lock()
		item := s.sessions[in.SessionID]
		s.mu.Unlock()
		if in.Stop {
			if err := s.Stop(ctx, in.SessionID); err != nil {
				return ErrorResult(err.Error()), nil
			}
			return s.result(TextResult("Command stopped."), in.SessionID), nil
		}
		if item == nil {
			if result, ok := s.consumeCompleted(in.SessionID); ok {
				return result, nil
			}
			return ErrorResult("unknown or completed exec session"), nil
		}
		if in.Chars != "" {
			if err := item.Write(ctx, in.Chars); err != nil {
				if ctx.Err() != nil {
					return Result{}, ctx.Err()
				}
				return ErrorResult(err.Error()), nil
			}
		}
		poll, err := item.Poll(ctx, commandWait(in.YieldTimeMS, 10*time.Second))
		if ctx.Err() != nil {
			return Result{}, ctx.Err()
		}
		if snapshot, ok := snapshotProcess(item); ok {
			current, snapshotErr := snapshot.Snapshot(ctx)
			s.update(in.SessionID, current, snapshotErr, false)
		} else {
			s.update(in.SessionID, poll, err, false)
		}
		info, known := s.terminal(in.SessionID)
		if known && info.Status == "stopped" {
			result := pollResult("Command stopped\n", poll)
			result.IsError = false
			result = s.result(result, in.SessionID)
			_, _ = s.consumeCompleted(in.SessionID)
			return result, nil
		}
		if err != nil || !poll.Running {
			if closeErr := item.Close(); closeErr != nil {
				err = errors.Join(err, closeErr)
			}
			result := s.result(pollResult("session completed\n", poll), in.SessionID)
			// The current poll already owns its output; retire the completed cursor.
			_, _ = s.consumeCompleted(in.SessionID)
			if err != nil {
				return ErrorResult(err.Error()), nil
			}
			return result, nil
		}
		return s.result(TextResult(renderPoll("session still running\n", poll)), in.SessionID), nil
	})
}
