package session

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"clara-code/pkg/core"
	"clara-code/pkg/model"
	"clara-code/pkg/util/id"
)

func TestListPageReadsLegacyCommandSnapshots(t *testing.T) {
	home, cwd := t.TempDir(), t.TempDir()
	dir := filepath.Join(home, "sessions")
	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	stamp := "2026-11-09T10:11:12.123456789+08:00"
	for _, id := range []string{"session_b", "session_a"} {
		raw := fmt.Sprintf(`{"Commands":{"terminal":{"Output":%q}},"Version":1,"Meta":{"ID":%q,"Cwd":%q,"Project":%q,"Updated":%q},"State":{}}`,
			strings.Repeat("x", 70<<10), id, cwd, cwd, stamp)
		if err := os.WriteFile(filepath.Join(dir, id+".json"), []byte(raw), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	page, err := ListPage(t.Context(), home, cwd, nil)
	if err != nil || len(page) != 2 || page[0].ID != "session_a" || page[1].ID != "session_b" ||
		page[0].Updated.Format(time.RFC3339Nano) != stamp {
		t.Fatalf("unexpected page %+v, %v", page, err)
	}
	page, err = ListPage(t.Context(), home, cwd, &page[0])
	if err != nil || len(page) != 1 || page[0].ID != "session_b" {
		t.Fatalf("unexpected cursor result %+v, %v", page, err)
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
