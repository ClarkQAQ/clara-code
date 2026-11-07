package session

import (
	"encoding/json"
	"os"
	"testing"
	"time"

	"clara-code/pkg/core"
	"clara-code/pkg/model"
	"clara-code/pkg/util/id"
)

func TestPersistedTimestampsKeepOffsetsPrecisionAndOldMessages(t *testing.T) {
	home := t.TempDir()
	store := New(home, Meta{Project: "/project"})
	defer store.Close()
	start, err := time.Parse(time.RFC3339Nano, "2026-11-08T01:02:03.123456789+08:00")
	if err != nil {
		t.Fatal(err)
	}
	ended := start.Add(3 * time.Second).UTC()
	sessionID := id.New("session")
	const oldText = "Historical timestamp from an old tool: 2026-11-07 17:02:03"
	state := core.State{ID: sessionID, Revision: 1, Messages: []model.Message{model.NewUserMessage(oldText)},
		Events: []core.Event{{Sequence: 1, Kind: "fixture", At: ended}}}
	if err := store.Save(t.Context(), state); err != nil {
		t.Fatal(err)
	}
	command := ToolActivity{ID: "command", CallID: "call", Name: "exec_command", Status: "succeeded", Started: start, Finished: ended}
	if err := store.UpdateCommand(command); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(store.path(sessionID))
	if err != nil {
		t.Fatal(err)
	}
	var document struct {
		Meta     struct{ Updated string }
		State    struct{ Events []struct{ At string } }
		Commands map[string]struct{ Started, Finished string }
	}
	if err := json.Unmarshal(raw, &document); err != nil {
		t.Fatal(err)
	}
	for raw, expected := range map[string]time.Time{
		document.Meta.Updated:                 store.Meta().Updated,
		document.State.Events[0].At:           ended,
		document.Commands["command"].Started:  start,
		document.Commands["command"].Finished: ended,
	} {
		parsed, err := time.Parse(time.RFC3339Nano, raw)
		if err != nil || !parsed.Equal(expected) {
			t.Fatalf("timestamp changed: %q, expected %s: %v", raw, expected.Format(time.RFC3339Nano), err)
		}
	}
	if document.Commands["command"].Started != "2026-11-08T01:02:03.123456789+08:00" {
		t.Fatal("existing offset or fractional seconds were lost")
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(home, sessionID, "/project")
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	loaded, err := reopened.Load(t.Context(), sessionID)
	if err != nil || len(loaded.Messages) != 1 || Text(loaded.Messages[0]) != oldText {
		t.Fatal("recovery rewrote historical date text", err)
	}
	if commands := reopened.Commands(); len(commands) != 1 || !commands[0].Started.Equal(start) || !commands[0].Finished.Equal(ended) {
		t.Fatal("recovery changed command timestamps", commands)
	}
}
