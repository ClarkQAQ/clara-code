// Package session stores Clara Core checkpoints and the independent visible transcript.
package session

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	"clara-code/pkg/core"
	modelapi "clara-code/pkg/model"
	"clara-code/pkg/util/errs"
	"clara-code/pkg/util/id"
	"golang.org/x/sys/unix"
)

const maxManifest = core.MaxStateBytes + 12<<20

var safeID = regexp.MustCompile(`^(session|run)_[a-zA-Z0-9_.-]{1,96}$`)

type Meta struct {
	PathMode                              int  `json:",omitempty"`
	LegacyPathContext                     bool `json:",omitempty"`
	ID, Project, Cwd, Parent, FirstPrompt string
	Model, Effort                         string
	Provider                              string `json:",omitempty"`
	PlanEffort                            string `json:",omitempty"`
	Updated                               time.Time
	LastUsage                             *modelapi.Usage `json:",omitempty"`
	TotalUsage                            modelapi.Usage
	UsageIncomplete                       bool
	UsageCursor                           uint64
}

type Cell struct {
	Kind, Text  string
	Attachments []Attachment  `json:",omitempty"`
	Tool        *ToolActivity `json:",omitempty"`
}

type Turn struct {
	ID, Prompt    string
	Attachments   []Attachment `json:",omitempty"`
	Cells         []Cell
	Cursor        uint64
	Staged        bool
	Partial       string
	Bytes         int
	StreamedText  string   `json:",omitempty"`
	StreamedCells []int    `json:",omitempty"`
	Failure       *Failure `json:",omitempty"`
}

type manifest struct {
	Commands map[string]ToolActivity `json:",omitempty"`
	Version  int
	Meta     Meta
	State    core.State
	Turns    []string
	Active   *Turn
}

type Store struct {
	home string
	mu   sync.Mutex
	lock *os.File
	data manifest
}

func New(home string, meta Meta) *Store {
	if meta.PathMode == 0 && meta.ID == "" && meta.Parent == "" && meta.Project != "" {
		meta.PathMode = 2
	}
	return &Store{home: home, data: manifest{Version: 1, Meta: meta}}
}

func Open(home, sessionID, project string) (*Store, error) {
	s := New(home, Meta{})
	if err := s.acquire(sessionID); err != nil {
		return nil, err
	}
	if err := read(s.path(sessionID), maxManifest, &s.data); err != nil {
		return nil, errors.Join(err, s.Close())
	}
	if s.data.Version != 1 || s.data.Meta.ID != sessionID || s.data.State.ID != sessionID || s.data.Meta.Project != project {
		return nil, errors.Join(errors.New("session does not belong to this project or has an invalid format"), s.Close())
	}
	collectUsage(&s.data.Meta, s.data.State.Events)
	return s, nil
}

func (s *Store) path(sessionID string) string {
	return filepath.Join(s.home, "sessions", sessionID+".json")
}

func (s *Store) object(runID, suffix string) string {
	return filepath.Join(s.home, "turns", runID+suffix+".json")
}

func (s *Store) acquire(sessionID string) error {
	if !safeID.MatchString(sessionID) || !strings.HasPrefix(sessionID, "session_") {
		return errors.New("invalid session ID")
	}
	dir := filepath.Join(s.home, "sessions")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	lock, err := os.OpenFile(filepath.Join(dir, sessionID+".lock"), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return err
	}
	if err := unix.Flock(int(lock.Fd()), unix.LOCK_EX|unix.LOCK_NB); err != nil {
		lock.Close()
		if errors.Is(err, unix.EWOULDBLOCK) {
			return fmt.Errorf("session %s is open in another process", sessionID)
		}
		return err
	}
	s.lock = lock
	return nil
}

func (s *Store) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.lock == nil {
		return nil
	}
	err := s.lock.Close()
	s.lock = nil
	return err
}

func (s *Store) Meta() Meta {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.data.Meta
}

func (s *Store) Load(ctx context.Context, sessionID string) (core.State, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return core.State{}, err
	}
	if s.lock == nil || sessionID != s.data.Meta.ID {
		return core.State{}, errs.ErrNotFound
	}
	return cloneState(s.data.State), nil
}

func (s *Store) Save(ctx context.Context, state core.State) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	next := s.data
	if s.lock == nil {
		if next.Meta.ID != "" || state.Revision != 1 {
			return errs.ErrConflict
		}
		if err := s.acquire(state.ID); err != nil {
			return err
		}
		if _, err := os.Stat(s.path(state.ID)); !errors.Is(err, os.ErrNotExist) {
			if err != nil {
				return err
			}
			return errs.ErrConflict
		}
		next.Meta.ID = state.ID
	}
	if state.ID != next.Meta.ID || state.Revision != next.State.Revision+1 {
		return errs.ErrConflict
	}
	if state.ActiveRun != "" && next.Active != nil && next.Active.ID != state.ActiveRun {
		previous := *next.Active
		previous.Cells = slices.Clone(previous.Cells)
		previous.collect(state.Events)
		previous.append("notice", "Previous run interrupted before its final checkpoint was saved.")
		if err := write(s.object(previous.ID, ""), previous, 10<<20); err != nil {
			return err
		}
		next.Turns = append(slices.Clone(next.Turns), previous.ID)
		next.Active = nil
	}
	if state.ActiveRun != "" && next.Active == nil {
		if !safeID.MatchString(state.ActiveRun) {
			return errors.New("invalid run ID")
		}
		before := next.State.Messages
		inputCount := 1
		for _, event := range state.Events {
			if event.RunID == state.ActiveRun && event.Kind == "run.started" {
				var input []modelapi.Message
				if json.Unmarshal(event.Data, &input) == nil && len(input) > 0 {
					inputCount = len(input)
				}
				break
			}
		}
		if len(state.Messages) >= inputCount {
			before = state.Messages[:len(state.Messages)-inputCount]
		}
		if err := write(s.object(state.ActiveRun, "-before"), before, core.MaxStateBytes); err != nil {
			return err
		}
		next.Active = &Turn{ID: state.ActiveRun}
		if at := len(before); at < len(state.Messages) {
			next.Active.Prompt = excerpt(Text(state.Messages[at]), 256<<10)
			attachments, err := attachmentsFrom(s.home, state.Messages[at])
			if err != nil {
				return err
			}
			next.Active.Attachments = attachments
			if next.Active.Prompt == "" {
				next.Active.Prompt = "[attachment]"
			}
		}
	}
	if next.Active != nil {
		turn := *next.Active
		turn.Cells = slices.Clone(turn.Cells)
		turn.StreamedCells = slices.Clone(turn.StreamedCells)
		turn.collect(state.Events)
		if len(state.Staged) > 0 && !turn.Staged {
			var text strings.Builder
			for _, message := range state.Staged {
				if message.Role == modelapi.MessageRoleAssistant {
					text.WriteString(Text(message))
				}
			}
			full := text.String()
			if strings.HasPrefix(full, turn.StreamedText) {
				turn.append("assistant", strings.TrimPrefix(full, turn.StreamedText))
			} else {
				for _, index := range turn.StreamedCells {
					if index < len(turn.Cells) {
						turn.Bytes -= len(turn.Cells[index].Text)
						turn.Cells[index].Text = ""
					}
				}
				turn.append("assistant", full)
			}
			turn.Partial = ""
		}
		turn.Staged = len(state.Staged) > 0
		if next.Meta.FirstPrompt == "" {
			next.Meta.FirstPrompt = excerpt(turn.Prompt, 240)
		}
		next.Active = &turn
		if state.ActiveRun == "" {
			if err := write(s.object(turn.ID, ""), turn, 10<<20); err != nil {
				return err
			}
			next.Turns = append(slices.Clone(next.Turns), turn.ID)
			next.Active = nil
		}
	}
	next.State = cloneState(state)
	collectUsage(&next.Meta, state.Events)
	return s.commit(next)
}

func collectUsage(meta *Meta, events []core.Event) {
	if meta.UsageCursor == 0 && len(events) > 0 && events[0].Sequence > 1 {
		meta.UsageIncomplete = true
	}
	for _, event := range events {
		if event.Sequence <= meta.UsageCursor {
			continue
		}
		meta.UsageCursor = event.Sequence
		if event.Kind == "step.completed" {
			var response modelapi.Response
			if json.Unmarshal(event.Data, &response) == nil {
				usage := response.Usage
				if usage.TotalTokens == 0 {
					usage.TotalTokens = max(0, usage.InputTokens) + max(0, usage.OutputTokens)
				}
				meta.LastUsage = nil
				if usage.TotalTokens == 0 {
					meta.UsageIncomplete = true
				}
				if usage.TotalTokens > 0 {
					meta.LastUsage = &usage
					meta.TotalUsage.InputTokens += usage.InputTokens
					meta.TotalUsage.OutputTokens += usage.OutputTokens
					meta.TotalUsage.TotalTokens += usage.TotalTokens
					meta.TotalUsage.ReasoningTokens += usage.ReasoningTokens
					meta.TotalUsage.CacheReadTokens += usage.CacheReadTokens
				}
			}
		}
	}
}

func cloneState(state core.State) core.State {
	state.Messages = slices.Clone(state.Messages)
	state.Events = slices.Clone(state.Events)
	state.Pending = slices.Clone(state.Pending)
	state.Staged = slices.Clone(state.Staged)
	return state
}

func (s *Store) commit(next manifest) error {
	next.Meta.Updated = time.Now().UTC()
	if err := write(s.path(next.Meta.ID), next, maxManifest); err != nil {
		return err
	}
	s.data = next
	return nil
}

func (s *Store) SetModel(model, effort string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	next := s.data
	next.Meta.Model, next.Meta.Effort = model, effort
	if model != s.data.Meta.Model || effort != s.data.Meta.Effort {
		next.Meta.LastUsage = nil
	}
	return s.commit(next)
}

func (s *Store) InvalidateContext() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	next := s.data
	next.Meta.LastUsage = nil
	return s.commit(next)
}

// ReplaceHistory is called with the Core session idle. Reopen the Core
// session after it succeeds so its in-memory revision matches the store.
func (s *Store) ReplaceHistory(ctx context.Context, messages []modelapi.Message) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	next := s.data
	state := cloneState(next.State)
	if state.ActiveRun != "" {
		return errs.ErrBusy
	}
	state.Messages = messages
	state.Revision++
	next.State = state
	next.Meta.LastUsage = nil
	return s.commit(next)
}

func (s *Store) Turns() ([]Turn, error) {
	count := s.TurnCount()
	turns := make([]Turn, 0, count)
	for i := range count {
		turn, err := s.ReadTurn(i)
		if err != nil {
			return nil, err
		}
		turns = append(turns, turn)
	}
	return turns, nil
}

func (s *Store) TurnCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.data.Turns)
}

func (s *Store) ReadTurn(index int) (Turn, error) {
	s.mu.Lock()
	if index < 0 || index >= len(s.data.Turns) {
		s.mu.Unlock()
		return Turn{}, errs.ErrNotFound
	}
	runID := s.data.Turns[index]
	s.mu.Unlock()
	if !safeID.MatchString(runID) || !strings.HasPrefix(runID, "run_") {
		return Turn{}, errors.New("invalid stored run ID")
	}
	var turn Turn
	err := read(s.object(runID, ""), 10<<20, &turn)
	s.mu.Lock()
	for i, cell := range turn.Cells {
		if cell.Tool == nil {
			continue
		}
		value, ok := s.data.Commands[cell.Tool.Key()]
		if !ok {
			value = interruptedCommand(*cell.Tool, "Interrupted; this terminal is not running in this process")
		}
		turn.Cells[i].Tool = &value
	}
	s.mu.Unlock()

	return turn, err
}

// Fork seeds a separate session with the context before turn index. Index -1
// branches from the current tip. Completed turn files are immutable and shared.
func (s *Store) Fork(ctx context.Context, index int) (*Store, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.data.Active != nil {
		return nil, errs.ErrBusy
	}
	meta := s.data.Meta
	meta.Parent, meta.ID = meta.ID, ""
	meta.LastUsage, meta.TotalUsage, meta.UsageCursor, meta.UsageIncomplete = nil, modelapi.Usage{}, 0, false
	messages, turns := s.data.State.Messages, s.data.Turns
	if index >= 0 {
		if index >= len(turns) || !safeID.MatchString(turns[index]) {
			return nil, errors.New("invalid rewind position")
		}
		messages = nil
		if err := read(s.object(turns[index], "-before"), core.MaxStateBytes, &messages); err != nil {
			return nil, err
		}
		turns = turns[:index]
		if index == 0 {
			meta.FirstPrompt = ""
		}
	}
	child := New(s.home, meta)
	child.data.Turns = slices.Clone(turns)
	child.data.Commands = map[string]ToolActivity{}
	allowed := map[string]bool{}
	for _, turnID := range turns {
		var turn Turn
		if err := read(s.object(turnID, ""), 10<<20, &turn); err != nil {
			return nil, err
		}
		for _, cell := range turn.Cells {
			if cell.Tool != nil {
				allowed[cell.Tool.Key()] = true
			}
		}
	}
	for key, value := range s.data.Commands {
		if index < 0 || allowed[key] {
			child.data.Commands[key] = interruptedCommand(value, "Terminal not resumed in this branch")
		}
	}
	if err := child.Save(ctx, core.State{ID: id.New("session"), Revision: 1, Messages: messages}); err != nil {
		return nil, errors.Join(err, child.Close())
	}
	return child, nil
}

func List(home, project string) ([]Meta, error) {
	dir := filepath.Join(home, "sessions")
	files, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var found []Meta
	for _, entry := range files {
		sessionID, ok := strings.CutSuffix(entry.Name(), ".json")
		if !ok || !safeID.MatchString(sessionID) || entry.Type()&os.ModeSymlink != 0 {
			continue
		}
		meta, err := readMeta(filepath.Join(dir, entry.Name()))
		if err == nil && meta.ID == sessionID && meta.Project == project {
			found = append(found, meta)
		}
	}
	sort.Slice(found, func(i, j int) bool {
		if found[i].Updated.Equal(found[j].Updated) {
			return found[i].ID < found[j].ID
		}
		return found[i].Updated.After(found[j].Updated)
	})
	return found, nil
}

func readMeta(path string) (Meta, error) {
	file, err := os.Open(path)
	if err != nil {
		return Meta{}, err
	}
	defer file.Close()
	// Version and Meta are the first fields. Listing never loads Core state.
	decoder := json.NewDecoder(io.LimitReader(file, 64<<10))
	start, err := decoder.Token()
	if err != nil || start != json.Delim('{') {
		return Meta{}, errors.New("invalid session manifest")
	}
	key, err := decoder.Token()
	if err != nil || key != "Version" {
		return Meta{}, errors.New("invalid session version")
	}
	var version int
	if err := decoder.Decode(&version); err != nil || version != 1 {
		return Meta{}, errors.New("unsupported session version")
	}
	key, err = decoder.Token()
	if err != nil || key != "Meta" {
		return Meta{}, errors.New("invalid session metadata")
	}
	var meta Meta
	err = decoder.Decode(&meta)
	return meta, err
}

func read(path string, maxBytes int64, dst any) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	body, err := io.ReadAll(io.LimitReader(file, maxBytes+1))
	if err != nil {
		return err
	}
	if int64(len(body)) > maxBytes {
		return errors.New("session data exceeds the size limit")
	}
	body, err = mediaJSON(path, body, true)
	if err != nil {
		return err
	}
	return json.Unmarshal(body, dst)
}

func write(path string, value any, maxBytes int) error {
	body, err := json.Marshal(value)
	if err != nil {
		return err
	}
	body, err = mediaJSON(path, body, false)
	if err != nil {
		return err
	}
	if len(body) > maxBytes {
		return errors.New("session data exceeds the size limit")
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	file, err := os.CreateTemp(dir, ".save-*")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	_, writeErr := file.Write(body)
	err = errors.Join(writeErr, file.Sync(), file.Close())
	if err != nil {
		return err
	}
	return os.Rename(file.Name(), path)
}

// RecordInput persists an input already accepted by the Core run queue.
// It also handles a run finishing between queue acceptance and this write.
func (s *Store) RecordInput(runID, text string) error { return s.RecordFiles(runID, text, nil) }
func (s *Store) RecordFiles(runID, text string, attachments []Attachment) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.lock == nil {
		return errs.ErrConflict
	}
	if s.data.Active != nil && s.data.Active.ID == runID {
		next := s.data
		turn := *next.Active
		turn.Cells = slices.Clone(turn.Cells)
		turn.append("user", text)
		if len(attachments) > 0 {
			turn.Cells = append(turn.Cells, Cell{Kind: "attachments", Attachments: attachments})
		}
		next.Active = &turn
		return s.commit(next)
	}
	for _, id := range s.data.Turns {
		if id == runID {
			var turn Turn
			if err := read(s.object(id, ""), 10<<20, &turn); err != nil {
				return err
			}
			turn.append("user", text)
			if len(attachments) > 0 {
				turn.Cells = append(turn.Cells, Cell{Kind: "attachments", Attachments: attachments})
			}
			return write(s.object(id, ""), turn, 10<<20)
		}
	}
	return errs.ErrNotFound
}

func (s *Store) ActiveTurn() *Turn {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.data.Active == nil {
		return nil
	}
	turn := *s.data.Active
	turn.Cells = slices.Clone(turn.Cells)
	return &turn
}

// MarkHostPaths records the native path namespace without rewriting old messages.
func (s *Store) MarkHostPaths() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.data.Meta.PathMode == 2 {
		return nil
	}
	if s.data.Meta.PathMode != 0 && s.data.Meta.PathMode != 1 {
		return errors.New("unsupported session path namespace")
	}
	next := s.data
	next.Meta.PathMode = 2
	next.Meta.LegacyPathContext = len(next.State.Messages) > 0 || len(next.Turns) > 0
	return s.commit(next)
}
