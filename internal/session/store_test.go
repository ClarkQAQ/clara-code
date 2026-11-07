package session

import (
	"testing"

	"clara-core/agent"
	"clara-core/agent/id"
	"clara-core/models"
)

func TestNewRunAfterIncompleteFinalCheckpoint(t *testing.T) {
	store := New(t.TempDir(), Meta{Project: "/project"})
	defer store.Close()
	sessionID := id.New("session")
	state := agent.State{ID: sessionID, Revision: 1}
	if err := store.Save(t.Context(), state); err != nil {
		t.Fatal(err)
	}
	firstRun := id.New("run")
	state.Revision++
	state.ActiveRun = firstRun
	state.Messages = []models.Message{models.NewUserMessage("first prompt")}
	if err := store.Save(t.Context(), state); err != nil {
		t.Fatal(err)
	}
	secondRun := id.New("run")
	state.Revision++
	state.ActiveRun = secondRun
	state.Messages = append(state.Messages, models.NewUserMessage("second prompt"))
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
