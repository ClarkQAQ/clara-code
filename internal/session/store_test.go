package session

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"clara-code/internal/session/internal/sessiondb"
	"clara-code/pkg/core"
	"clara-code/pkg/core/prompt"
	"clara-code/pkg/model"
	"clara-code/pkg/util/id"
)

func TestNullContextCheckpointClearsUsage(t *testing.T) {
	store := New(t.TempDir(), Meta{Project: "/test"})
	defer store.Close()
	state := core.State{
		ID: id.New("session"), Revision: 1, Messages: []model.Message{model.NewUserMessage("task")},
		Events: []core.Event{usageEvent(1, "run_test", model.Usage{TotalTokens: 25})},
	}
	_, window, err := (prompt.Window{}).Prepare(state.Messages, nil)
	if err != nil {
		t.Fatal(err)
	}
	state.ContextState, err = window.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Save(t.Context(), state); err != nil {
		t.Fatal(err)
	}
	if got := store.Meta(); got.LastUsage == nil || got.ContextMessageCount != 1 {
		t.Fatalf("missing initial usage: %+v", got)
	}
	state.Revision++
	state.ContextState = json.RawMessage(" null ")
	if err := store.Save(t.Context(), state); err != nil {
		t.Fatal(err)
	}
	if got := store.Meta(); got.LastUsage != nil || got.ContextTokens != nil || got.ContextMessageCount != 0 || got.TotalUsage.TotalTokens != 25 {
		t.Fatalf("clearing the window retained usage or changed total consumption: %+v", got)
	}
}

func TestCompactedContextUsageFollowsCheckpoint(t *testing.T) {
	for _, mode := range []string{"automatic", "manual"} {
		t.Run(mode, func(t *testing.T) {
			home := t.TempDir()
			store := New(home, Meta{Project: "/test", Model: "original"})
			defer func() { store.Close() }()
			state := core.State{ID: id.New("session"), Revision: 1, Messages: []model.Message{model.NewUserMessage("original task")}}
			state.Events = []core.Event{usageEvent(1, "run_test", model.Usage{TotalTokens: 1700})}
			if err := store.Save(t.Context(), state); err != nil {
				t.Fatal(err)
			}
			before := store.Meta()
			state.Messages = []model.Message{model.NewDeveloperMessage("handoff"), model.NewUserMessage("continue")}
			_, window, err := (prompt.Window{}).Prepare(state.Messages, nil)
			if err != nil {
				t.Fatal(err)
			}
			window.CompactedTokens = new(int64(120))
			state.ContextState, err = window.Marshal()
			if err != nil {
				t.Fatal(err)
			}
			state.Revision++
			replace := func() error {
				if mode == "manual" {
					return store.ReplaceHistory(t.Context(), state.Messages, state.ContextState, Compaction{})
				}
				return store.Save(t.Context(), state)
			}
			if _, err := store.db.ExecContext(t.Context(), "CREATE TRIGGER reject_meter BEFORE UPDATE ON sessions BEGIN SELECT RAISE(ABORT, 'failed'); END"); err != nil {
				t.Fatal(err)
			}
			if err := replace(); err == nil || store.Meta().ContextTokens != nil || store.Meta().LastUsage == nil {
				t.Fatalf("failed checkpoint changed context usage: %v", err)
			}
			if _, err := store.db.ExecContext(t.Context(), "DROP TRIGGER reject_meter"); err != nil {
				t.Fatal(err)
			}
			if err := replace(); err != nil {
				t.Fatal(err)
			}
			if err := store.Close(); err != nil {
				t.Fatal(err)
			}
			reopened, err := Open(home, state.ID, before.Project)
			if err != nil {
				t.Fatal(err)
			}
			store = reopened
			meta := store.Meta()
			if meta.LastUsage != nil || meta.ContextTokens == nil || *meta.ContextTokens != 120 || meta.ContextMessageCount != 2 ||
				meta.TotalUsage != before.TotalUsage || meta.SessionCache != before.SessionCache || meta.RoundCache != before.RoundCache {
				t.Fatalf("compacted usage after recovery: %+v", meta)
			}
			fork, err := store.Fork(t.Context(), -1)
			if err != nil {
				t.Fatal(err)
			}
			defer fork.Close()
			if got := fork.Meta(); got.ContextTokens != nil || got.ContextMessageCount != 0 || got.LastUsage != nil {
				t.Fatalf("fork inherited context measurement: %+v", got)
			}
			state.Events = append(state.Events, usageEvent(2, "run_test", model.Usage{}))
			state.Revision++
			if err := store.Save(t.Context(), state); err != nil {
				t.Fatal(err)
			}
			if got := store.Meta(); got.ContextTokens == nil || *got.ContextTokens != 120 || got.TotalUsage.TotalTokens != 1700 {
				t.Fatalf("unreported usage replaced estimated context: %+v", got)
			}
			state.Messages = append(state.Messages, model.Message{Role: model.MessageRoleAssistant}, model.Message{Role: model.MessageRoleTool})
			state.Events = append(state.Events, usageEvent(3, "run_test", model.Usage{TotalTokens: 350}))
			state.Revision++
			if err := store.Save(t.Context(), state); err != nil {
				t.Fatal(err)
			}
			meta = store.Meta()
			if meta.ContextTokens != nil || meta.LastUsage == nil || meta.LastUsage.TotalTokens != 350 || meta.ContextMessageCount != 3 ||
				meta.TotalUsage.TotalTokens != 2050 || meta.SessionCache.Requests != before.SessionCache.Requests+2 {
				t.Fatalf("provider usage did not replace the estimate at the assistant boundary: %+v", meta)
			}
			if mode == "automatic" {
				state.Messages[0] = model.NewUserMessage("unrelated history")
				state.Revision++
				if err := store.Save(t.Context(), state); err != nil {
					t.Fatal(err)
				}
				if err := store.Close(); err != nil {
					t.Fatal(err)
				}
				reopened, err := Open(home, state.ID, before.Project)
				if err != nil {
					t.Fatal(err)
				}
				store = reopened
				if got := store.Meta(); got.LastUsage != nil || got.ContextTokens != nil || got.ContextMessageCount != 0 || got.TotalUsage != meta.TotalUsage {
					t.Fatalf("unrelated history reused a measurement: %+v", got)
				}
			}
			if err := store.SetSelection("provider", "other-model", "low", "low"); err != nil {
				t.Fatal(err)
			}
			if got := store.Meta(); got.LastUsage != nil || got.ContextTokens != nil || got.ContextMessageCount != 0 || got.TotalUsage != meta.TotalUsage {
				t.Fatalf("switch retained a stale measurement or changed billed usage: %+v", got)
			}
		})
	}
}

func TestLegacyJSONRemainsUntouched(t *testing.T) {
	home, cwd := t.TempDir(), t.TempDir()
	files := map[string][]byte{
		"sessions/session_old.json": []byte(`{"Version":1,"Meta":{"ID":"session_old"}}`),
		"turns/run_old.json":        []byte(`{"ID":"run_old"}`),
		"turns/run_old-before.json": []byte(`[]`),
	}
	for name, body := range files {
		path := filepath.Join(home, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, body, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if items, err := ListPage(t.Context(), home, "", nil); err != nil || len(items) != 0 {
		t.Fatalf("unexpected page %+v, %v", items, err)
	}
	if _, err := Open(home, "session_old", cwd); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("old session opened: %v", err)
	}
	if _, err := os.Stat(filepath.Join(home, databaseName)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("read created a database: %v", err)
	}
	store := New(home, Meta{Cwd: cwd, Project: cwd})
	defer store.Close()
	if err := store.Save(t.Context(), core.State{ID: "session_new", Revision: 1}); err != nil {
		t.Fatal(err)
	}
	if items, err := List(home, cwd); err != nil || len(items) != 1 || items[0].ID != "session_new" {
		t.Fatalf("unexpected project sessions %+v, %v", items, err)
	}
	if _, err := Open(home, "session_old", cwd); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("old session imported: %v", err)
	}
	for name, body := range files {
		got, err := os.ReadFile(filepath.Join(home, name))
		if err != nil || !bytes.Equal(got, body) {
			t.Fatalf("old file %s changed: %q, %v", name, got, err)
		}
	}
}

func TestListPageUsesMetadataAndPreciseCursor(t *testing.T) {
	home, cwd := t.TempDir(), t.TempDir()
	ctx := t.Context()
	db, err := openDatabase(ctx, home, true)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	stamp := "2026-11-09T10:11:12.123456789+08:00"
	updated, err := time.Parse(time.RFC3339Nano, stamp)
	if err != nil {
		t.Fatal(err)
	}
	for _, sessionID := range []string{"session_b", "session_a", "session_newest"} {
		meta := Meta{ID: sessionID, Cwd: cwd, Project: cwd, Updated: updated}
		if sessionID == "session_newest" {
			meta.Updated = updated.Add(time.Nanosecond)
		}
		body, err := json.Marshal(meta)
		if err != nil {
			t.Fatal(err)
		}
		if err := saveSessionRow(ctx, sessiondb.New(db), meta, meta.Updated.UTC().Format(updatedKey), body, []byte("invalid checkpoint"), true); err != nil {
			t.Fatal(err)
		}
	}
	page, err := ListPage(ctx, home, cwd, nil)
	if err != nil || len(page) != 3 || page[0].ID != "session_newest" || page[1].ID != "session_a" ||
		page[2].ID != "session_b" || page[1].Updated.Format(time.RFC3339Nano) != stamp {
		t.Fatalf("unexpected page %+v, %v", page, err)
	}
	page, err = ListPage(ctx, home, cwd, &page[1])
	if err != nil || len(page) != 1 || page[0].ID != "session_b" {
		t.Fatalf("unexpected cursor result %+v, %v", page, err)
	}
}

func TestSaveRollsBackCheckpointAndTurnTogether(t *testing.T) {
	ctx, home := t.Context(), t.TempDir()
	store := New(home, Meta{Project: "/project"})
	defer store.Close()
	state := core.State{ID: id.New("session"), Revision: 1}
	db, err := openDatabase(ctx, home, true)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.ExecContext(ctx, `
		CREATE TRIGGER reject_session BEFORE INSERT ON sessions
		BEGIN SELECT RAISE(ABORT, 'test initial save failure'); END`); err != nil {
		t.Fatal(err)
	}
	if err := store.Save(ctx, state); err == nil || store.lock != nil || store.db != nil || store.Meta().ID != "" {
		t.Fatalf("first save left resources or state after failure: %v", err)
	}
	if _, err := Open(home, state.ID, "/project"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("failed session was published or still locked: %v", err)
	}
	if _, err := db.ExecContext(ctx, "DROP TRIGGER reject_session"); err != nil {
		t.Fatal(err)
	}
	if err := store.Save(ctx, state); err != nil {
		t.Fatal(err)
	}

	runID := id.New("run")
	state.Revision++
	state.ActiveRun = runID
	state.Messages = []model.Message{model.NewUserMessage("hello")}
	if err := store.Save(ctx, state); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `
		CREATE TRIGGER reject_turn BEFORE INSERT ON session_turns
		BEGIN SELECT RAISE(ABORT, 'test archive failure'); END`); err != nil {
		t.Fatal(err)
	}
	state.Revision++
	state.ActiveRun = ""
	if err := store.Save(ctx, state); err == nil {
		t.Fatal("archive unexpectedly succeeded")
	}
	if current, err := store.Load(ctx, state.ID); err != nil || current.Revision != state.Revision-1 ||
		current.ActiveRun != runID || store.TurnCount() != 0 {
		t.Fatalf("failed save changed memory: %+v, %v", current, err)
	}
	queries := sessiondb.New(db)
	if body, err := queries.GetTranscript(ctx, runID); err != nil || body != nil {
		t.Fatalf("failed save published transcript: %q, %v", body, err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(home, state.ID, "/project")
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	if current, err := reopened.Load(ctx, state.ID); err != nil || current.Revision != state.Revision-1 ||
		current.ActiveRun != runID || reopened.TurnCount() != 0 {
		t.Fatalf("failed save changed disk: %+v, %v", current, err)
	}
	if _, err := db.ExecContext(ctx, "DROP TRIGGER reject_turn"); err != nil {
		t.Fatal(err)
	}
	if err := reopened.Save(ctx, state); err != nil {
		t.Fatal(err)
	}
	if turn, err := reopened.ReadTurn(0); err != nil || turn.ID != runID {
		t.Fatalf("retry lost turn: %+v, %v", turn, err)
	}
}

func TestNewRunAfterIncompleteFinalCheckpoint(t *testing.T) {
	store := New(t.TempDir(), Meta{Project: "/project"})
	defer store.Close()
	sessionID := id.New("session")
	state := core.State{ID: sessionID, Revision: 1}
	if err := store.Save(t.Context(), state); err != nil {
		t.Fatal(err)
	}
	firstRun := id.New("run")
	state.Revision++
	state.ActiveRun = firstRun
	state.Messages = []model.Message{model.NewUserMessage("first prompt")}
	if err := store.Save(t.Context(), state); err != nil {
		t.Fatal(err)
	}
	secondRun := id.New("run")
	state.Revision++
	state.ActiveRun = secondRun
	state.Messages = append(state.Messages, model.NewUserMessage("second prompt"))
	if err := store.Save(t.Context(), state); err != nil {
		t.Fatal(err)
	}
	state.Revision++
	state.ActiveRun = ""
	if err := store.Save(t.Context(), state); err != nil {
		t.Fatal(err)
	}
	turns, err := store.Turns()
	if err != nil || len(turns) != 2 || turns[0].Prompt != "first prompt" || turns[1].Prompt != "second prompt" {
		t.Fatalf("turns %+v %v", turns, err)
	}
	fork, err := store.Fork(t.Context(), 1)
	if err != nil {
		t.Fatal(err)
	}
	defer fork.Close()
	checkpoint, err := fork.Load(t.Context(), fork.Meta().ID)
	if err != nil || len(checkpoint.Messages) != 1 || Text(checkpoint.Messages[0]) != "first prompt" {
		t.Fatalf("checkpoint %+v %v", checkpoint, err)
	}
}
