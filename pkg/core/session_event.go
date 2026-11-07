package core

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"clara-code/pkg/model"
	"clara-code/pkg/util/errs"
)

// Event is one replayable session notification.
type Event struct {
	Sequence   uint64          `json:"sequence"`
	SessionID  string          `json:"session_id"`
	RunID      string          `json:"run_id,omitempty"`
	MessageID  string          `json:"message_id,omitempty"`
	ToolCallID string          `json:"tool_call_id,omitempty"`
	Kind       string          `json:"kind"`
	At         time.Time       `json:"at"`
	Data       json.RawMessage `json:"data,omitempty"`
	Truncated  bool            `json:"truncated,omitempty"`
}

type GapError struct{ Earliest uint64 }

func (e *GapError) Error() string {
	return fmt.Sprintf("event cursor expired. Replay from %d", e.Earliest-1)
}

func (s *Session) Emit(ctx context.Context, kind string, payload any) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.emitLocked(kind, payload)
}

func (s *Session) emitLocked(kind string, payload any) error {
	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	s.sequence++
	ev := Event{Sequence: s.sequence, SessionID: s.state.ID, RunID: s.state.ActiveRun, Kind: kind, At: time.Now().UTC(), Data: raw}
	switch item := payload.(type) {
	case model.StreamPart:
		if item.Type == model.StreamPartTypeToolInputDelta {
			ev.ToolCallID = item.ID
		} else if s.active != nil {
			ev.MessageID = fmt.Sprintf("%s:%d:%s:%s", ev.RunID, s.active.step, item.Type, item.ID)
		}
	case model.ToolCallContent:
		ev.ToolCallID = item.ToolCallID
	case model.ToolResultContent:
		ev.ToolCallID = item.ToolCallID
	}
	if len(raw) > 1<<20 {
		ev.Data, ev.Truncated = nil, true
	}
	s.state.Events = append(s.state.Events, ev)
	s.eventBytes += len(ev.Data)
	for len(s.state.Events) > 4096 || s.eventBytes > 4<<20 {
		s.eventBytes -= len(s.state.Events[0].Data)
		s.state.Events[0] = Event{}
		s.state.Events = s.state.Events[1:]
	}
	close(s.changed)
	s.changed = make(chan struct{})
	return nil
}

// NextEvent waits for the next event after a replay cursor.
func (s *Session) NextEvent(ctx context.Context, after uint64) (Event, error) {
	for {
		if err := ctx.Err(); err != nil {
			return Event{}, err
		}
		s.mu.Lock()
		events := s.state.Events
		if len(events) > 0 {
			first := events[0].Sequence
			if after < first-1 {
				s.mu.Unlock()
				return Event{}, &GapError{Earliest: first}
			}
			if after < events[len(events)-1].Sequence {
				ev := events[after-(first-1)]
				ev.Data = append(json.RawMessage(nil), ev.Data...)
				s.mu.Unlock()
				return ev, nil
			}
		}
		changed := s.changed
		closed := s.closed
		s.mu.Unlock()
		if closed {
			return Event{}, errs.ErrConflict
		}
		select {
		case <-ctx.Done():
			return Event{}, ctx.Err()
		case <-changed:
		}
	}
}
