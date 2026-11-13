// Package session stores Clara Core checkpoints and the independent visible transcript.
package session

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"

	"clara-code/internal/filelock"
	"clara-code/internal/session/internal/sessiondb"
	"clara-code/pkg/core"
	modelapi "clara-code/pkg/model"
	"clara-code/pkg/util/errs"
	"clara-code/pkg/util/id"
)

const maxCheckpoint = core.MaxStateBytes + 12<<20
const maxCompactionMarkers = 1024

var safeID = regexp.MustCompile(`^(session|run)_[a-zA-Z0-9_.-]{1,96}$`)

var (
	ErrInvalidID       = errors.New("invalid session ID")
	ErrProjectMismatch = errors.New("session does not belong to this project")
)

type Meta struct {
	PathMode                              int  `json:",omitempty"`
	LegacyPathContext                     bool `json:",omitempty"`
	ID, Project, Cwd, Parent, FirstPrompt string
	Model, Effort                         string
	Provider                              string     `json:",omitempty"`
	PlanEffort                            string     `json:",omitempty"`
	ForkPoint                             *ForkPoint `json:",omitempty"`
	Updated                               time.Time
	LastUsage                             *modelapi.Usage `json:",omitempty"`
	TotalUsage                            modelapi.Usage
	UsageIncomplete                       bool
	UsageCursor                           uint64
	SessionCache                          CacheUsage `json:",omitzero"`
	RoundCache                            CacheUsage `json:",omitzero"`
	CacheRunID                            string     `json:",omitempty"`
}

type ForkPoint struct {
	AfterTurn int
	Title     string
}

type CompactionMarker struct {
	AfterTurn  int
	AfterFork  bool `json:",omitempty"`
	Compaction Compaction
}

type Cell struct {
	Kind, Text  string
	Attachments []Attachment  `json:",omitempty"`
	Tool        *ToolActivity `json:",omitempty"`
	Compaction  *Compaction   `json:",omitempty"`
}

type Turn struct {
	StartedAt     time.Time `json:",omitzero"`
	FinishedAt    time.Time `json:",omitzero"`
	Status        string    `json:",omitempty"`
	DurationMS    *int64    `json:",omitempty"`
	ID, Prompt    string
	Attachments   []Attachment `json:",omitempty"`
	Cells         []Cell
	Cursor        uint64
	Staged        bool
	Planning      bool `json:",omitempty"`
	Partial       string
	Bytes         int
	StreamedText  string   `json:",omitempty"`
	StreamedCells []int    `json:",omitempty"`
	Failure       *Failure `json:",omitempty"`
	started       time.Time
}

type checkpoint struct {
	State       core.State
	Active      *Turn
	Commands    map[string]ToolActivity `json:",omitempty"`
	Compactions []CompactionMarker      `json:",omitempty"`
}

type sessionData struct {
	checkpoint
	Meta  Meta
	Turns []string
}

type Store struct {
	home            string
	mu              sync.Mutex
	lock            *os.File
	db              *sql.DB
	draft           bool
	data            sessionData
	compactionCount uint64
}

func New(home string, meta Meta) *Store {
	if meta.PathMode == 0 && meta.ID == "" && meta.Parent == "" && meta.Project != "" {
		meta.PathMode = 2
	}
	return &Store{home: home, data: sessionData{Meta: meta}}
}

// NewDraft keeps the initial checkpoint in memory until the first run starts.
func NewDraft(home string, meta Meta) *Store {
	store := New(home, meta)
	store.draft = true
	return store
}

func Open(home, sessionID, project string) (*Store, error) {
	if !ValidID(sessionID) {
		return nil, ErrInvalidID
	}
	ctx := context.Background()
	db, err := openDatabase(ctx, home, false)
	if err != nil {
		return nil, err
	}
	savedProject, err := sessiondb.New(db).GetSessionProject(ctx, sessionID)
	if errors.Is(err, sql.ErrNoRows) {
		err = os.ErrNotExist
	}
	if err != nil {
		return nil, errors.Join(err, db.Close())
	}
	if savedProject != project {
		return nil, errors.Join(ErrProjectMismatch, db.Close())
	}
	s := New(home, Meta{})
	s.db = db
	if err := s.acquire(sessionID); err != nil {
		return nil, errors.Join(err, s.Close())
	}
	if err := s.restore(ctx, sessionID, project); err != nil {
		return nil, errors.Join(err, s.Close())
	}
	return s, nil
}

func (s *Store) restore(ctx context.Context, sessionID, project string) error {
	queries := sessiondb.New(s.db)
	row, err := queries.GetSession(ctx, sessionID)
	if errors.Is(err, sql.ErrNoRows) {
		return os.ErrNotExist
	}
	if err != nil {
		return err
	}
	meta, err := decodeMeta(row.Metadata)
	if err != nil {
		return err
	}
	if meta.ID != sessionID {
		return errors.New("invalid session metadata")
	}
	if meta.Project != project {
		return ErrProjectMismatch
	}
	data := sessionData{Meta: meta}
	if err := decodePayload(s.home, row.Checkpoint, maxCheckpoint-len(row.Metadata), &data.checkpoint); err != nil {
		return err
	}
	if data.State.ID != sessionID {
		return errors.New("invalid session checkpoint")
	}
	data.Turns, err = queries.ListSessionTurns(ctx, sessionID)
	if err != nil {
		return err
	}
	collectUsage(&data.Meta, data.State.Events)
	s.data = data
	return nil
}

func ValidID(id string) bool {
	return strings.HasPrefix(id, "session_") && safeID.MatchString(id)
}

func (s *Store) acquire(sessionID string) error {
	if !ValidID(sessionID) {
		return ErrInvalidID
	}
	dir := filepath.Join(s.home, "session-locks")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	lock, err := filelock.Try(filepath.Join(dir, sessionID+".lock"))
	if errors.Is(err, filelock.ErrBusy) {
		return fmt.Errorf("session %s is open in another process", sessionID)
	}
	if err != nil {
		return err
	}
	s.lock = lock
	return nil
}

func (s *Store) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.draft = false
	return s.closeLocked()
}

func (s *Store) closeLocked() error {
	var err error
	if s.db != nil {
		err = s.db.Close()
		s.db = nil
	}
	if s.lock != nil {
		err = errors.Join(err, s.lock.Close())
		s.lock = nil
	}
	return err
}

func (s *Store) Meta() Meta {
	s.mu.Lock()
	defer s.mu.Unlock()
	meta := s.data.Meta
	if meta.ForkPoint != nil {
		meta.ForkPoint = new(*meta.ForkPoint)
	}
	return meta
}

func (s *Store) Load(ctx context.Context, sessionID string) (core.State, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return core.State{}, err
	}
	if (!s.draft && s.db == nil) || sessionID == "" || sessionID != s.data.Meta.ID {
		return core.State{}, errs.ErrNotFound
	}
	return cloneState(s.data.State), nil
}

func (s *Store) Save(ctx context.Context, state core.State) (err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	started := time.Now()
	next := s.data
	first := s.db == nil
	if first && next.Meta.ID == "" {
		if !ValidID(state.ID) {
			return ErrInvalidID
		}
		next.Meta.ID = state.ID
	}
	if state.ID != next.Meta.ID || state.Revision != next.State.Revision+1 ||
		(first && !s.draft && s.data.Meta.ID != "") {
		return errs.ErrConflict
	}
	if s.draft && first && state.ActiveRun == "" {
		next.State = cloneState(state)
		s.data = next
		return nil
	}
	if first {
		if err := s.acquire(state.ID); err != nil {
			return err
		}
		defer func() {
			if err != nil {
				err = errors.Join(err, s.closeLocked())
			}
		}()
		s.db, err = openDatabase(ctx, s.home, true)
		if err != nil {
			return err
		}
	}
	var changes turnChanges
	if state.ActiveRun != "" && next.Active != nil && next.Active.ID != state.ActiveRun {
		previous := *next.Active
		previous.Cells = slices.Clone(previous.Cells)
		previous.collect(state.Events)
		if previous.Status == "completed" {
			previous.Status = "cancelled"
		}
		previous.DurationMS = nil
		previous.append("notice", "Previous run interrupted before its final checkpoint was saved.")
		changes.completed = &previous
		next.Turns = append(slices.Clone(next.Turns), previous.ID)
		next.Active = nil
	}
	if state.ActiveRun != "" && next.Active == nil {
		if !validRunID(state.ActiveRun) {
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
		body, err := encodePayload(s.home, before, core.MaxStateBytes)
		if err != nil {
			return err
		}
		changes.started = &sessiondb.InsertTurnParams{ID: state.ActiveRun, BeforeMessages: body}
		next.Active = &Turn{ID: state.ActiveRun, started: started}
		next.Active.Planning, _ = ctx.Value(planningKey{}).(bool)
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
			if turn.Planning {
				turn.appendAssistant(full)
			} else if strings.HasPrefix(full, turn.StreamedText) {
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
			if turn.Status == "completed" && !turn.started.IsZero() {
				turn.DurationMS = new(time.Since(turn.started).Milliseconds())
			}
			changes.completed = &turn
			next.Turns = append(slices.Clone(next.Turns), turn.ID)
			next.Active = nil
		}
	}
	next.State = cloneState(state)
	collectUsage(&next.Meta, state.Events)
	return s.saveCheckpoint(ctx, next, first, changes)
}

func collectUsage(meta *Meta, events []core.Event) {
	gap := len(events) > 0 && events[0].Sequence > meta.UsageCursor+1
	if gap {
		meta.UsageIncomplete = true
		meta.SessionCache.Incomplete, meta.RoundCache.Incomplete = true, true
	}
	if meta.SessionCache.Requests == 0 && meta.TotalUsage.TotalTokens > 0 {
		meta.SessionCache.Incomplete = true
	}
	for _, event := range events {
		if event.Sequence <= meta.UsageCursor {
			continue
		}
		meta.UsageCursor = event.Sequence
		if event.Kind == "run.started" {
			meta.CacheRunID = event.RunID
			meta.RoundCache = CacheUsage{}
			gap = false
		}
		if event.Kind == "step.completed" {
			var response modelapi.Response
			if json.Unmarshal(event.Data, &response) == nil {
				usage := response.Usage
				if meta.CacheRunID != event.RunID {
					meta.CacheRunID, meta.RoundCache = event.RunID, CacheUsage{Incomplete: gap}
				}
				meta.SessionCache.add(usage)
				meta.RoundCache.add(usage)
				meta.TotalUsage.CacheReadReported = !meta.SessionCache.Incomplete
				if usage.TotalTokens == 0 {
					usage.TotalTokens = max(0, usage.InputTokens) + max(0, usage.OutputTokens) + max(0, usage.CacheReadTokens)
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
			} else {
				meta.LastUsage = nil
				meta.UsageIncomplete = true
				meta.SessionCache.Incomplete, meta.RoundCache.Incomplete = true, true
				meta.TotalUsage.CacheReadReported = false
			}
		}
	}
}

func cloneState(state core.State) core.State {
	state.ContextState = slices.Clone(state.ContextState)
	state.Messages = slices.Clone(state.Messages)
	state.Events = slices.Clone(state.Events)
	state.Pending = slices.Clone(state.Pending)
	state.Staged = slices.Clone(state.Staged)
	return state
}

type turnChanges struct {
	started   *sessiondb.InsertTurnParams
	completed *Turn
}

func (s *Store) commit(next sessionData) error {
	if s.draft && s.db == nil && next.Meta.ID != "" {
		s.data = next
		return nil
	}
	return s.saveCheckpoint(context.Background(), next, false, turnChanges{})
}

func (s *Store) saveCheckpoint(ctx context.Context, next sessionData, first bool, changes turnChanges) error {
	if s.db == nil {
		return errs.ErrConflict
	}
	next.Meta.Updated = time.Now().UTC()
	meta, err := json.Marshal(next.Meta)
	if err != nil {
		return err
	}
	if len(meta) > maxMetadata {
		return errors.New("session metadata exceeds the size limit")
	}
	body, err := encodePayload(s.home, next.checkpoint, maxCheckpoint-len(meta))
	if err != nil {
		return err
	}
	var transcript []byte
	if changes.completed != nil {
		transcript, err = encodePayload(s.home, changes.completed, maxTurn)
		if err != nil {
			return err
		}
	}
	key := next.Meta.Updated.Format(updatedKey)
	err = transact(ctx, s.db, func(q *sessiondb.Queries) error {
		if err := saveSessionRow(ctx, q, next.Meta, key, meta, body, first); err != nil {
			return err
		}
		if changes.started != nil {
			if err := q.InsertTurn(ctx, *changes.started); err != nil {
				return err
			}
		}
		if changes.completed != nil {
			if err := updateTranscript(ctx, q, changes.completed.ID, transcript); err != nil {
				return err
			}
		}
		start := len(s.data.Turns)
		if first {
			start = 0
		}
		for index := start; index < len(next.Turns); index++ {
			if err := q.AppendSessionTurn(ctx, sessiondb.AppendSessionTurnParams{
				SessionID: next.Meta.ID, Position: int64(index), TurnID: next.Turns[index],
			}); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	s.data = next
	s.draft = false
	return nil
}

func saveSessionRow(ctx context.Context, q *sessiondb.Queries, meta Meta, key string, metadata, checkpoint []byte, first bool) error {
	var rows int64
	var err error
	if first {
		rows, err = q.InsertSession(ctx, sessiondb.InsertSessionParams{
			ID: meta.ID, Project: meta.Project, Cwd: meta.Cwd,
			UpdatedKey: key, Metadata: metadata, Checkpoint: checkpoint,
		})
	} else {
		rows, err = q.UpdateSession(ctx, sessiondb.UpdateSessionParams{
			ID: meta.ID, Project: meta.Project, Cwd: meta.Cwd,
			UpdatedKey: key, Metadata: metadata, Checkpoint: checkpoint,
		})
	}
	if err == nil && rows != 1 {
		return errs.ErrConflict
	}
	return err
}

func updateTranscript(ctx context.Context, q *sessiondb.Queries, runID string, body []byte) error {
	rows, err := q.UpdateTranscript(ctx, sessiondb.UpdateTranscriptParams{ID: runID, Transcript: body})
	if err == nil && rows != 1 {
		return errs.ErrNotFound
	}
	return err
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

// ReplaceHistory saves compacted history and its visible marker together. Reopen
// the idle Core session after it succeeds so its revision matches the store.
func (s *Store) ReplaceHistory(ctx context.Context, messages []modelapi.Message, contextState json.RawMessage, compaction Compaction) error {
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
	state.ContextState = slices.Clone(contextState)
	state.Revision++
	next.State = state
	next.Meta.LastUsage = nil
	after := len(next.Turns)
	markers := next.Compactions[max(0, len(next.Compactions)-maxCompactionMarkers+1):]
	next.Compactions = append(slices.Clone(markers), CompactionMarker{
		AfterTurn: after, Compaction: compaction.clone(),
		AfterFork: next.Meta.ForkPoint != nil && next.Meta.ForkPoint.AfterTurn == after,
	})
	if err := s.saveCheckpoint(ctx, next, false, turnChanges{}); err != nil {
		return err
	}
	s.compactionCount++
	return nil
}

// Compactions returns the saved markers outside completed or active turns.
func (s *Store) Compactions() []CompactionMarker {
	s.mu.Lock()
	defer s.mu.Unlock()
	markers := slices.Clone(s.data.Compactions)
	for i := range markers {
		markers[i].Compaction = markers[i].Compaction.clone()
	}
	return markers
}

// CompactionCount tracks successful history replacements on this open store.
func (s *Store) CompactionCount() uint64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.compactionCount
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
	defer s.mu.Unlock()
	if s.db == nil || index < 0 || index >= len(s.data.Turns) {
		return Turn{}, errs.ErrNotFound
	}
	turn, err := s.readTurn(context.Background(), sessiondb.New(s.db), s.data.Turns[index])
	if err != nil {
		return Turn{}, err
	}
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
	return turn, nil
}

func validRunID(id string) bool {
	return strings.HasPrefix(id, "run_") && safeID.MatchString(id)
}

func (s *Store) readTurn(ctx context.Context, q *sessiondb.Queries, runID string) (Turn, error) {
	var turn Turn
	if !validRunID(runID) {
		return turn, errors.New("invalid stored run ID")
	}
	body, err := q.GetTranscript(ctx, runID)
	if err != nil {
		return turn, err
	}
	err = decodePayload(s.home, body, maxTurn, &turn)
	return turn, err
}

// Fork seeds a separate session with the context before turn index. Index -1
// branches from the current tip. The branch shares completed turn records.
func (s *Store) Fork(ctx context.Context, index int) (*Store, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.data.Meta.ID == "" || (s.db == nil && !s.draft) {
		return nil, errs.ErrNotFound
	}
	if s.data.Active != nil {
		return nil, errs.ErrBusy
	}
	queries := sessiondb.New(s.db)
	meta := s.data.Meta
	meta.Parent, meta.ID = meta.ID, ""
	meta.ForkPoint = nil
	if s.draft {
		meta.Parent = ""
	}
	meta.LastUsage, meta.TotalUsage, meta.UsageCursor, meta.UsageIncomplete = nil, modelapi.Usage{}, 0, false
	meta.SessionCache, meta.RoundCache, meta.CacheRunID = CacheUsage{}, CacheUsage{}, ""
	messages, turns := s.data.State.Messages, s.data.Turns
	if index >= 0 {
		if index >= len(turns) || !validRunID(turns[index]) {
			return nil, errors.New("invalid rewind position")
		}
		body, err := queries.GetBeforeMessages(ctx, turns[index])
		if err != nil {
			return nil, err
		}
		messages = nil
		if err := decodePayload(s.home, body, core.MaxStateBytes, &messages); err != nil {
			return nil, err
		}
		turns = turns[:index]
		if index == 0 {
			meta.FirstPrompt = ""
		}
	}
	if meta.Parent != "" {
		meta.ForkPoint = &ForkPoint{AfterTurn: len(turns), Title: s.data.Meta.FirstPrompt}
	}
	child := New(s.home, meta)
	if len(turns) == 0 && len(messages) == 0 {
		child.draft = true
	}
	child.data.Turns = slices.Clone(turns)
	for _, marker := range s.data.Compactions {
		if marker.AfterTurn <= len(turns) {
			marker.AfterFork = false
			child.data.Compactions = append(child.data.Compactions, marker)
		}
	}
	child.data.Commands = map[string]ToolActivity{}
	allowed := map[string]bool{}
	for _, turnID := range turns {
		turn, err := s.readTurn(ctx, queries, turnID)
		if err != nil {
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
	if !slices.Contains(s.data.Turns, runID) {
		return errs.ErrNotFound
	}
	ctx := context.Background()
	return transact(ctx, s.db, func(q *sessiondb.Queries) error {
		turn, err := s.readTurn(ctx, q, runID)
		if err != nil {
			return err
		}
		turn.append("user", text)
		if len(attachments) > 0 {
			turn.Cells = append(turn.Cells, Cell{Kind: "attachments", Attachments: attachments})
		}
		body, err := encodePayload(s.home, turn, maxTurn)
		if err != nil {
			return err
		}
		return updateTranscript(ctx, q, runID, body)
	})
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
