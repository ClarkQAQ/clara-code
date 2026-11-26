package session

import (
	"errors"
	"os"
	"testing"

	"clara-code/pkg/core"
	"clara-code/pkg/model"
	"clara-code/pkg/tool"
)

func TestGoalCheckpointMaterializesDraftAndRestoresHistory(t *testing.T) {
	ctx, home := t.Context(), t.TempDir()
	const project = "/project"
	store := NewDraft(home, Meta{Project: project})
	defer store.Close()
	state := core.State{ID: "session_goal", Revision: 1}
	if err := store.Save(ctx, state); err != nil {
		t.Fatal(err)
	}
	goal := tool.GoalState{Objective: "Verify the project", Status: tool.GoalActive, TokenBudget: 1000, TurnBudget: 5}
	if cell, err := store.SaveGoal(goal); err != nil || cell.Kind != GoalUpdatedKind || cell.Text != "Goal started" {
		t.Fatalf("start cell = %+v, err = %v", cell, err)
	}
	if found, err := List(home, project); err != nil || len(found) != 1 || found[0].ID != state.ID || found[0].FirstPrompt != goal.Objective {
		t.Fatalf("goal-only conversation is missing: %+v, %v", found, err)
	}
	state.Revision++
	state.ActiveRun = "run_goal"
	state.Messages = []model.Message{model.NewUserMessage(goal.Objective)}
	if err := store.Save(ctx, state); err != nil {
		t.Fatal(err)
	}
	goal.TokensUsed, goal.TurnsUsed = 32, 1
	if cell, err := store.SaveGoal(goal); err != nil || cell.Kind != "" {
		t.Fatalf("goal usage should save without a transcript cell: %+v, %v", cell, err)
	}
	goal.Status = tool.GoalPaused
	if cell, err := store.SaveGoal(goal); err != nil || cell.Text != "Goal paused" {
		t.Fatalf("pause cell = %+v, err = %v", cell, err)
	}
	state.Revision++
	state.ActiveRun = ""
	if err := store.Save(ctx, state); err != nil {
		t.Fatal(err)
	}
	if err := store.ReplaceHistory(ctx, state.Messages, nil, Compaction{}); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(home, state.ID, project)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	if got := reopened.GoalState(); got != goal {
		t.Fatalf("restored goal = %+v, want %+v", got, goal)
	}
	markers := reopened.GoalMarkers()
	if len(markers) != 1 || markers[0].Sequence != 1 || markers[0].AfterTurn != 0 || markers[0].Cell.Kind != GoalUpdatedKind {
		t.Fatalf("restored goal markers: %+v", markers)
	}
	turn, err := reopened.ReadTurn(0)
	if err != nil || len(turn.Cells) != 1 || turn.Cells[0].Kind != GoalUpdatedKind || turn.Cells[0].Text != "Goal paused" {
		t.Fatalf("restored turn: %+v, %v", turn, err)
	}
	child, err := reopened.Fork(ctx, -1)
	if err != nil {
		t.Fatal(err)
	}
	defer child.Close()
	if child.GoalState() != (tool.GoalState{}) || len(child.GoalMarkers()) != 1 {
		t.Fatalf("fork should only inherit goal history: %+v, %+v", child.GoalState(), child.GoalMarkers())
	}
}

func TestSaveGoalRollsBackWithItsTranscript(t *testing.T) {
	ctx, home := t.Context(), t.TempDir()
	const project = "/project"
	db, err := openDatabase(ctx, home, true)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	query := func(sql string) {
		t.Helper()
		if _, err := db.ExecContext(ctx, sql); err != nil {
			t.Fatal(err)
		}
	}
	store := NewDraft(home, Meta{Project: project})
	defer store.Close()
	state := core.State{ID: "session_goal_failure", Revision: 1}
	if err := store.Save(ctx, state); err != nil {
		t.Fatal(err)
	}
	query(`CREATE TRIGGER reject_goal_insert BEFORE INSERT ON sessions BEGIN SELECT RAISE(ABORT, 'goal insert failed'); END`)
	goal := tool.GoalState{Objective: "Verify", Status: tool.GoalActive}
	if cell, err := store.SaveGoal(goal); err == nil || cell.Kind != "" || store.GoalState() != (tool.GoalState{}) || len(store.GoalMarkers()) != 0 || store.lock != nil {
		t.Fatalf("failed first checkpoint leaked state: %+v, %v", cell, err)
	}
	if _, err := Open(home, state.ID, project); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("unsaved goal created a session: %v", err)
	}
	query("DROP TRIGGER reject_goal_insert")
	if _, err := store.SaveGoal(goal); err != nil {
		t.Fatal(err)
	}
	state.Revision++
	state.ActiveRun = "run_goal_failure"
	state.Messages = []model.Message{model.NewUserMessage(goal.Objective)}
	if err := store.Save(ctx, state); err != nil {
		t.Fatal(err)
	}
	query(`CREATE TRIGGER reject_goal_update BEFORE UPDATE ON sessions BEGIN SELECT RAISE(ABORT, 'goal update failed'); END`)
	paused := goal
	paused.Status = tool.GoalPaused
	if cell, err := store.SaveGoal(paused); err == nil || cell.Kind != "" || store.GoalState() != goal || len(store.ActiveTurn().Cells) != 0 {
		t.Fatalf("failed update changed the active goal or turn: %+v, %v", cell, err)
	}
	query("DROP TRIGGER reject_goal_update")
	if _, err := store.SaveGoal(paused); err != nil {
		t.Fatal(err)
	}
	if got := store.ActiveTurn().Cells; len(got) != 1 || got[0].Text != "Goal paused" {
		t.Fatalf("retry did not record exactly one pause: %+v", got)
	}
}

func TestInternalGoalTurnPreservesAttachments(t *testing.T) {
	store := New(t.TempDir(), Meta{Project: "/project"})
	defer store.Close()
	input := model.NewUserMessage("Resume the goal", model.FilePart{Filename: "goal.png", MediaType: "image/png", Data: []byte("fixture")})
	input.Metadata = &model.MessageMetadata{Kind: GoalContinuationKind}
	state := core.State{ID: "session_goal_internal", Revision: 1, ActiveRun: "run_goal_internal", Messages: []model.Message{input}}
	if err := store.Save(t.Context(), state); err != nil {
		t.Fatal(err)
	}
	state.Revision++
	state.ActiveRun = ""
	if err := store.Save(t.Context(), state); err != nil {
		t.Fatal(err)
	}
	turn, err := store.ReadTurn(0)
	if err != nil || !turn.Internal || turn.Prompt != "" || len(turn.Attachments) != 1 || turn.Attachments[0].Filename != "goal.png" {
		t.Fatalf("continued goal transcript: %+v, %v", turn, err)
	}
}

func TestGoalMarkersRemainBoundedWithStableSequence(t *testing.T) {
	var data sessionData
	cell := Cell{Kind: GoalUpdatedKind, Text: "Goal paused"}
	for range maxGoalMarkers + 2 {
		data.recordGoalTransition(cell)
	}
	markers := data.GoalMarkers
	if len(markers) != maxGoalMarkers || markers[0].Sequence != 3 || markers[len(markers)-1].Sequence != maxGoalMarkers+2 {
		t.Fatalf("unexpected marker retention: len=%d, first=%+v, last=%+v", len(markers), markers[0], markers[len(markers)-1])
	}
}

func TestGoalMarkersFollowForkBoundary(t *testing.T) {
	ctx := t.Context()
	store := New(t.TempDir(), Meta{Project: "/project"})
	defer store.Close()
	if err := store.Save(ctx, core.State{ID: "session_goal_fork", Revision: 1}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.SaveGoal(tool.GoalState{Objective: "First objective", Status: tool.GoalActive}); err != nil {
		t.Fatal(err)
	}
	child, err := store.Fork(ctx, -1)
	if err != nil {
		t.Fatal(err)
	}
	defer child.Close()
	if _, err := child.SaveGoal(tool.GoalState{Objective: "Second objective", Status: tool.GoalActive}); err != nil {
		t.Fatal(err)
	}
	if got := child.GoalMarkers(); len(got) != 2 || got[0].AfterFork || !got[1].AfterFork || got[0].AfterTurn != 0 || got[1].AfterTurn != 0 {
		t.Fatalf("fork boundary markers: %+v", got)
	}
	grandchild, err := child.Fork(ctx, -1)
	if err != nil {
		t.Fatal(err)
	}
	defer grandchild.Close()
	if got := grandchild.GoalMarkers(); len(got) != 2 || got[0].AfterFork || got[1].AfterFork {
		t.Fatalf("inherited fork boundary markers: %+v", got)
	}
}

func TestGoalMarkerFallsBackAfterFullTurn(t *testing.T) {
	d := sessionData{checkpoint: checkpoint{Active: &Turn{Bytes: 1 << 20}}}
	cell := Cell{Kind: GoalUpdatedKind, Text: "Goal completed"}
	if live := d.recordGoalTransition(cell); live.Kind != "" {
		t.Fatalf("boundary marker was also returned for live display: %+v", live)
	}
	markers := d.GoalMarkers
	if len(markers) != 1 || markers[0].AfterTurn != 1 || markers[0].Cell.Text != cell.Text {
		t.Fatalf("transition was not saved at the full turn's boundary: %+v", markers)
	}
}
