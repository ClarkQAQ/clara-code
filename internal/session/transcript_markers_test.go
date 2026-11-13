package session

import (
	"encoding/json"
	"testing"
	"time"

	"clara-code/pkg/core"
	"clara-code/pkg/model"
)

func TestTurnArchivesCompactionAndSuccessfulDuration(t *testing.T) {
	ctx := t.Context()
	store := New(t.TempDir(), Meta{Project: "/project"})
	defer store.Close()
	state := core.State{ID: "session_markers"}
	save := func() {
		t.Helper()
		state.Revision++
		if err := store.Save(ctx, state); err != nil {
			t.Fatal(err)
		}
	}
	emit := func(kind string, data json.RawMessage, at time.Time) {
		state.Events = append(state.Events, core.Event{
			Sequence: uint64(len(state.Events) + 1), RunID: state.ActiveRun, Kind: kind, Data: data, At: at,
		})
	}
	begin := func(runID, input string, at time.Time) {
		t.Helper()
		state.ActiveRun = runID
		state.Messages = append(state.Messages, model.NewUserMessage(input))
		emit("run.started", nil, at)
		save()
	}
	walled := time.Now().UTC()
	save()
	begin("run_success", "first prompt", walled)
	emit("text_delta", json.RawMessage(`{"delta":"Before"}`), walled)
	emit("text_end", nil, walled)
	emit(ContextCompactedKind, json.RawMessage(`{"DurationMS":2000}`), walled)
	save()
	emit(ContextCompactedKind, json.RawMessage(`null`), walled)
	emit("text_delta", json.RawMessage(`{"delta":"After"}`), walled)
	emit("text_end", nil, walled)
	// Move only the synthetic wall clock backwards; elapsed stays non-negative.
	emit("run.completed", nil, walled.Add(-time.Hour))
	state.ActiveRun = ""
	save()

	begin("run_cancelled", "second prompt", walled)
	emit("run.cancelled", nil, walled.Add(time.Hour))
	state.ActiveRun = ""
	save()
	turns, err := store.Turns()
	if err != nil || len(turns) != 2 {
		t.Fatalf("saved turns: %+v, %v", turns, err)
	}
	finished, cancelled := turns[0], turns[1]
	if finished.Status != "completed" || finished.DurationMS == nil || *finished.DurationMS < 0 || cancelled.Status != "cancelled" || cancelled.DurationMS != nil {
		t.Fatalf("wrong recorded durations or states: %+v, %+v", finished, cancelled)
	}
	cells := finished.Cells
	if len(cells) != 4 || cells[0].Text != "Before" || cells[3].Text != "After" {
		t.Fatalf("wrong transcript sequence: %+v", cells)
	}
	for _, cell := range cells[1:3] {
		if cell.Kind != ContextCompactedKind || cell.Compaction == nil {
			t.Fatalf("missing compaction: %+v", cell)
		}
	}
	if duration := cells[1].Compaction.DurationMS; duration == nil || *duration != 2000 || cells[2].Compaction.DurationMS != nil {
		t.Fatalf("wrong compaction timing: %+v, %+v", cells[1].Compaction, cells[2].Compaction)
	}
}

func TestCompactionAndForkMarkersFollowSessionBoundaries(t *testing.T) {
	ctx := t.Context()
	home := t.TempDir()
	store := New(home, Meta{Project: "/project"})
	defer store.Close()
	state := core.State{ID: "session_parent"}
	save := func() {
		t.Helper()
		state.Revision++
		if err := store.Save(ctx, state); err != nil {
			t.Fatal(err)
		}
	}
	finish := func(runID, input string) {
		t.Helper()
		state.ActiveRun = runID
		state.Messages = append(state.Messages, model.NewUserMessage(input))
		save()
		state.ActiveRun = ""
		save()
	}
	compact := func(store *Store, elapsed int64) {
		t.Helper()
		state.Messages = []model.Message{model.NewUserMessage("summary")}
		if err := store.ReplaceHistory(ctx, state.Messages, nil, Compaction{DurationMS: &elapsed}); err != nil {
			t.Fatal(err)
		}
	}
	save()
	finish("run_first", "Source title")
	if _, err := store.db.ExecContext(ctx, `CREATE TRIGGER reject_compaction BEFORE UPDATE ON sessions
		BEGIN SELECT RAISE(ABORT, 'compaction checkpoint failed'); END`); err != nil {
		t.Fatal(err)
	}
	if err := store.ReplaceHistory(ctx, nil, nil, Compaction{}); err == nil || store.CompactionCount() != 0 || len(store.Compactions()) != 0 {
		t.Fatalf("failed replacement recorded a marker: %v", err)
	}
	if saved, err := store.Load(ctx, state.ID); err != nil || len(saved.Messages) != 1 || Text(saved.Messages[0]) != "Source title" {
		t.Fatalf("failed compaction changed model history: %+v, %v", saved.Messages, err)
	}
	if _, err := store.db.ExecContext(ctx, `DROP TRIGGER reject_compaction`); err != nil {
		t.Fatal(err)
	}
	compact(store, 1200)
	state.Revision++
	finish("run_second", "second prompt")
	compact(store, 2400)
	if markers := store.Compactions(); store.CompactionCount() != 2 || len(markers) != 2 || markers[0].AfterTurn != 1 || markers[1].AfterTurn != 2 {
		t.Fatalf("manual compactions not recorded in order: %+v", markers)
	}

	fromStart, err := store.Fork(ctx, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer fromStart.Close()
	if meta := fromStart.Meta(); meta.FirstPrompt != "" || meta.ForkPoint == nil || *meta.ForkPoint != (ForkPoint{AfterTurn: 0, Title: "Source title"}) {
		t.Fatalf("fork of first message lost original title: %+v", meta)
	}

	rewound, err := store.Fork(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { rewound.Close() }()
	if meta := rewound.Meta(); meta.Parent != state.ID || meta.ForkPoint == nil || *meta.ForkPoint != (ForkPoint{AfterTurn: 1, Title: "Source title"}) {
		t.Fatalf("wrong rewind provenance: %+v", meta)
	}
	compact(rewound, 3600)
	id := rewound.Meta().ID
	if err := rewound.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(home, id, "/project")
	if err != nil {
		t.Fatal(err)
	}
	rewound = reopened
	markers := rewound.Compactions()
	if len(markers) != 2 || markers[0].AfterTurn != 1 || markers[0].AfterFork || markers[1].AfterTurn != 1 || !markers[1].AfterFork {
		t.Fatalf("wrong compaction placement around fork after reopening: %+v", markers)
	}
	for i, want := range []int64{1200, 3600} {
		if duration := markers[i].Compaction.DurationMS; duration == nil || *duration != want {
			t.Fatalf("wrong duration for marker %d: %v", i, duration)
		}
	}
	child, err := rewound.Fork(ctx, -1)
	if err != nil {
		t.Fatal(err)
	}
	defer child.Close()
	if inherited := child.Compactions(); child.Meta().Parent != id || len(inherited) != 2 || inherited[0].AfterFork || inherited[1].AfterFork {
		t.Fatalf("fork of a fork did not reset inherited markers: %+v", inherited)
	}
}

func TestRunWithoutFinalCheckpointDoesNotArchiveSuccessfulDuration(t *testing.T) {
	ctx := t.Context()
	store := New(t.TempDir(), Meta{Project: "/project"})
	defer store.Close()
	started := time.Now().UTC()
	state := core.State{
		ID: "session_interrupted", Revision: 1, ActiveRun: "run_first",
		Messages: []model.Message{model.NewUserMessage("first prompt")},
		Events:   []core.Event{{Sequence: 1, RunID: "run_first", Kind: "run.started", At: started}},
	}
	if err := store.Save(ctx, state); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.ExecContext(ctx, `CREATE TRIGGER reject_final_checkpoint BEFORE UPDATE OF transcript ON turns
		BEGIN SELECT RAISE(ABORT, 'test final checkpoint failure'); END`); err != nil {
		t.Fatal(err)
	}
	state.Revision++
	state.ActiveRun = ""
	state.Events = append(state.Events, core.Event{Sequence: 2, RunID: "run_first", Kind: "run.completed", At: started.Add(time.Hour)})
	if err := store.Save(ctx, state); err == nil || store.TurnCount() != 0 {
		t.Fatalf("failed checkpoint archived an ostensibly successful turn: %v", err)
	}
	if _, err := store.db.ExecContext(ctx, `DROP TRIGGER reject_final_checkpoint`); err != nil {
		t.Fatal(err)
	}
	state.ActiveRun = "run_second"
	state.Messages = append(state.Messages, model.NewUserMessage("second prompt"))
	state.Events = append(state.Events, core.Event{Sequence: 3, RunID: "run_second", Kind: "run.started", At: started.Add(time.Hour)})
	if err := store.Save(ctx, state); err != nil {
		t.Fatal(err)
	}
	turn, err := store.ReadTurn(0)
	if err != nil || turn.Status == "completed" || turn.DurationMS != nil {
		t.Fatalf("previous interrupted run was archived as successful: %+v, %v", turn, err)
	}
}
