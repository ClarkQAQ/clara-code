package session

import (
	"errors"
	"sort"
	"time"
)

// UpdateCommand persists asynchronous command snapshots without rewriting turns.
func (s *Store) UpdateCommand(activity ToolActivity) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.data.Meta.ID == "" {
		return errors.New("session is not initialized")
	}
	next := s.data
	next.Commands = make(map[string]ToolActivity, len(s.data.Commands)+1)
	for key, value := range s.data.Commands {
		next.Commands[key] = value
	}
	activity = activity.Bounded(24 << 10)
	if old, ok := next.Commands[activity.Key()]; ok && activity.CommandRevision < old.CommandRevision {
		return nil
	}
	next.Commands[activity.Key()] = activity
	if len(next.Commands) > 256 {
		var oldest string
		for key, value := range next.Commands {
			if value.Active() || value.Background() || key == activity.Key() {
				continue
			}
			if oldest == "" || value.Started.Before(next.Commands[oldest].Started) {
				oldest = key
			}
		}
		if oldest != "" {
			delete(next.Commands, oldest)
		}
	}
	return s.commit(next)
}

func (s *Store) Commands() []ToolActivity {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]ToolActivity, 0, len(s.data.Commands))
	for _, value := range s.data.Commands {
		out = append(out, value)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Started.Before(out[j].Started) })
	return out
}

func interruptedCommand(activity ToolActivity, message string) ToolActivity {
	if activity.Active() || activity.Background() {
		activity.Status = "cancelled"
		activity.Error = message
		activity.Finished = time.Now()
	}
	return activity
}

func (s *Store) InterruptCommands() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	next := s.data
	next.Commands = make(map[string]ToolActivity, len(s.data.Commands))
	changed := false
	for key, value := range s.data.Commands {
		latest := interruptedCommand(value, "Interrupted when the prior process ended")
		changed = changed || latest.Status != value.Status
		next.Commands[key] = latest
	}
	if !changed {
		return nil
	}
	return s.commit(next)
}
