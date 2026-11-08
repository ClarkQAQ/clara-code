package core

import (
	"context"
	"encoding/json"
	"errors"
	"sync"

	"clara-code/pkg/model"
	"clara-code/pkg/util/errs"
)

type Pending struct {
	Call   model.ToolCallPart
	Result *model.ToolResultPart
}

type State struct {
	ID        string
	Revision  uint64
	Messages  []model.Message
	Events    []Event
	Pending   []Pending
	Staged    []model.Message
	ActiveRun string
	// ContextState is optional host-owned model-window state, separate from Messages.
	ContextState json.RawMessage `json:",omitempty"`
}

// Store persists generic state. Save replaces the previous revision atomically.
type Store interface {
	Load(context.Context, string) (State, error)
	Save(context.Context, State) error
}

const MaxStateBytes = 48 << 20

const MaxContextStateBytes = 8 << 20

type MemoryStore struct {
	mu        sync.Mutex
	data      map[string][]byte
	revisions map[string]uint64
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{data: map[string][]byte{}, revisions: map[string]uint64{}}
}

func (m *MemoryStore) Load(ctx context.Context, id string) (State, error) {
	if err := ctx.Err(); err != nil {
		return State{}, err
	}
	m.mu.Lock()
	data := m.data[id]
	m.mu.Unlock()
	if data == nil {
		return State{}, errs.ErrNotFound
	}
	return decodeState(data)
}

func (m *MemoryStore) Save(ctx context.Context, state State) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	raw, err := encodeState(state)
	if err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if state.Revision != m.revisions[state.ID]+1 {
		return errs.ErrConflict
	}
	if len(m.data) >= 1024 && m.revisions[state.ID] == 0 {
		return errs.New(errs.CodeLimit, "session limit reached")
	}
	m.data[state.ID], m.revisions[state.ID] = raw, state.Revision
	return nil
}

// Blobs supplies atomic writes and bounded reads for a file-backed session store.
type Blobs interface {
	Read(context.Context, string, int64) ([]byte, error)
	Write(context.Context, string, []byte) error
}

type FileStore struct {
	mu    sync.Mutex
	blobs Blobs
}

func NewFileStore(blobs Blobs) *FileStore { return &FileStore{blobs: blobs} }

func (f *FileStore) Load(ctx context.Context, id string) (State, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.load(ctx, id)
}

func (f *FileStore) load(ctx context.Context, id string) (State, error) {
	if !validID(id) {
		return State{}, errs.New(errs.CodeInvalidArgument, "invalid session ID")
	}
	data, err := f.blobs.Read(ctx, id+".json", MaxStateBytes+1)
	if err != nil {
		return State{}, err
	}
	state, err := decodeState(data)
	if err == nil && state.ID != id {
		return State{}, errs.ErrConflict
	}
	return state, err
}

func (f *FileStore) Save(ctx context.Context, state State) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	previous, err := f.load(ctx, state.ID)
	if err != nil && !errors.Is(err, errs.ErrNotFound) && !errs.IsCode(err, errs.CodeNotFound) {
		return err
	}
	if state.Revision != previous.Revision+1 {
		return errs.ErrConflict
	}
	raw, err := encodeState(state)
	if err != nil {
		return err
	}
	return f.blobs.Write(ctx, state.ID+".json", raw)
}

func validID(id string) bool {
	if len(id) == 0 || len(id) > 100 || id == "." || id == ".." {
		return false
	}
	for _, c := range id {
		valid := c >= '0' && c <= '9' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c == '_' || c == '-' || c == '.'
		if !valid {
			return false
		}
	}
	return true
}

type storedState struct {
	Format int
	State  State
}

func encodeState(state State) ([]byte, error) {
	if !validID(state.ID) {
		return nil, errs.New(errs.CodeInvalidArgument, "invalid session ID")
	}
	data, err := json.Marshal(storedState{1, state})
	if err == nil && len(data) > MaxStateBytes {
		return nil, errs.New(errs.CodeLimit, "session state exceeds its size limit")
	}
	return data, err
}

func decodeState(raw []byte) (State, error) {
	if len(raw) > MaxStateBytes {
		return State{}, errs.New(errs.CodeLimit, "session state exceeds its size limit")
	}
	var file storedState
	if err := json.Unmarshal(raw, &file); err != nil {
		return State{}, err
	}
	if file.Format != 1 {
		return State{}, errs.ErrUnsupported
	}
	return file.State, nil
}
