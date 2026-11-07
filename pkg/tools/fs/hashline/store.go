package hashline

import (
	"fmt"
	"sync"
)

const (
	maxPaths      = 256
	maxVersions   = 4
	maxStoreBytes = 64 << 20
)

// Snapshot is one observed file version.
type Snapshot struct {
	Path  string
	Text  string
	Tag   string
	Seen  map[int]bool
	touch uint64
	bytes int
}

type noopState struct {
	payload string
	count   int
}

// Store keeps snapshots and named registers for one session.
type Store struct {
	mu        sync.Mutex
	histories map[string][]Snapshot
	named     map[string][]string
	noops     map[string]noopState
	clock     uint64
	bytes     int
}

// NewStore creates an empty snapshot store.
func NewStore() *Store {
	return &Store{
		histories: map[string][]Snapshot{}, named: map[string][]string{},
		noops: map[string]noopState{},
	}
}

// Record stores content and merges newly visible lines.
func (s *Store) Record(path, raw string, seen []int) Snapshot {
	text := Normalize(raw)
	tag := Tag(text)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.clock++
	history := s.histories[path]
	for index := range history {
		if history[index].Tag == tag && history[index].Text == text {
			if history[index].Seen == nil {
				history[index].Seen = map[int]bool{}
			}
			for _, line := range seen {
				if line > 0 {
					history[index].Seen[line] = true
				}
			}
			history[index].touch = s.clock
			snapshot := clone(history[index])
			s.histories[path] = append([]Snapshot{history[index]}, append(history[:index], history[index+1:]...)...)
			s.recount()
			return snapshot
		}
	}
	snapshot := Snapshot{Path: path, Text: text, Tag: tag, Seen: map[int]bool{}, touch: s.clock}
	for _, line := range seen {
		if line > 0 {
			snapshot.Seen[line] = true
		}
	}
	history = append([]Snapshot{snapshot}, history...)
	if len(history) > maxVersions {
		history = history[:maxVersions]
	}
	s.histories[path] = history
	s.recount()
	s.evict()
	return clone(snapshot)
}

// Lookup returns a snapshot by path and tag.
func (s *Store) Lookup(path, tag string) (Snapshot, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	history := s.histories[path]
	for index := range history {
		if history[index].Tag == tag {
			s.clock++
			history[index].touch = s.clock
			s.histories[path] = history
			return clone(history[index]), true
		}
	}
	return Snapshot{}, false
}

// Current returns the newest observed version for path.
func (s *Store) Current(path string) (Snapshot, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	history := s.histories[path]
	if len(history) == 0 {
		return Snapshot{}, false
	}
	return clone(history[0]), true
}

// Register returns a named clipboard register.
func (s *Store) Register(name string) []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.named[name]...)
}

// CommitRegisters publishes registers after edits land.
func (s *Store) CommitRegisters(registers map[string][]string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for name, lines := range registers {
		s.named[name] = append([]string(nil), lines...)
	}
}

// Noop records a byte-identical edit.
func (s *Store) Noop(path, payload string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	state := s.noops[path]
	if state.payload != payload {
		state = noopState{payload: payload, count: 1}
	} else {
		state.count++
	}
	s.noops[path] = state
	if state.count >= 3 {
		return "", fmt.Errorf("%s was unchanged by the same edit three times; read it again", path)
	}
	return fmt.Sprintf("%s was already in that state", path), nil
}

// Success clears a no-op streak.
func (s *Store) Success(path string) {
	s.mu.Lock()
	delete(s.noops, path)
	s.mu.Unlock()
}

func (s *Store) recount() {
	s.bytes = 0
	for _, history := range s.histories {
		for _, snapshot := range history {
			snapshot.bytes = len(snapshot.Path) + len(snapshot.Text) + len(snapshot.Tag) + len(snapshot.Seen)*8
			s.bytes += snapshot.bytes
		}
	}
}

func (s *Store) evict() {
	for len(s.histories) > maxPaths || s.bytes > maxStoreBytes {
		oldestPath := ""
		oldest := ^uint64(0)
		for path, history := range s.histories {
			touched := uint64(0)
			for _, snapshot := range history {
				if snapshot.touch > touched {
					touched = snapshot.touch
				}
			}
			if touched < oldest {
				oldest, oldestPath = touched, path
			}
		}
		if oldestPath == "" {
			return
		}
		delete(s.histories, oldestPath)
		s.recount()
	}
}

func clone(snapshot Snapshot) Snapshot {
	seen := make(map[int]bool, len(snapshot.Seen))
	for line := range snapshot.Seen {
		seen[line] = true
	}
	snapshot.Seen = seen
	return snapshot
}
