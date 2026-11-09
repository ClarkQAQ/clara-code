package session

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"sync"
	"testing"
	"time"

	"clara-code/internal/session/internal/sessiondb"
	"clara-code/pkg/core"
	"clara-code/pkg/model"
)

func TestDraftSurvivesFailedFirstSave(t *testing.T) {
	ctx, home := t.Context(), t.TempDir()
	store := NewDraft(home, Meta{Project: "/project"})
	defer store.Close()
	state := core.State{ID: "session_draft", Revision: 1}
	if err := store.Save(ctx, state); err != nil {
		t.Fatal(err)
	}
	if err := store.SetSelection("provider", "selected", "high", "low"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(home, databaseName)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("draft or model selection touched the database: %v", err)
	}

	state.Revision++
	state.ActiveRun = "invalid"
	state.Messages = []model.Message{model.NewUserMessage("first message")}
	if err := store.Save(ctx, state); err == nil {
		t.Fatal("invalid first run was saved")
	}
	if loaded, err := store.Load(ctx, state.ID); err != nil || loaded.Revision != 1 || loaded.ActiveRun != "" {
		t.Fatalf("draft could not be retried: %+v, %v", loaded, err)
	}
	if _, err := Open(home, state.ID, "/project"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("failed first run left a row or lock: %v", err)
	}

	state.ActiveRun = "run_first"
	if err := store.Save(ctx, state); err != nil {
		t.Fatal(err)
	}
	items, err := List(home, "/project")
	if err != nil || len(items) != 1 || items[0].ID != state.ID || items[0].FirstPrompt != "first message" ||
		items[0].Provider != "provider" || items[0].Model != "selected" || items[0].Effort != "high" || items[0].PlanEffort != "low" {
		t.Fatalf("draft metadata was not saved with the first run: %+v, %v", items, err)
	}
}

func TestConcurrentSQLiteInitialization(t *testing.T) {
	const homeEnv = "CLARA_TEST_SQLITE_HOME"
	const idEnv = "CLARA_TEST_SQLITE_SESSION"
	if home := os.Getenv(homeEnv); home != "" {
		store := New(home, Meta{Project: "/project", Cwd: "/project"})
		defer store.Close()
		state := core.State{ID: os.Getenv(idEnv)}
		for revision := range 5 {
			state.Revision = uint64(revision + 1)
			if err := store.Save(t.Context(), state); err != nil {
				t.Fatal(err)
			}
		}
		return
	}
	home := filepath.Join(t.TempDir(), "home # %")
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancel()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	var writers sync.WaitGroup
	for index := range 4 {
		writers.Go(func() {
			cmd := exec.CommandContext(ctx, exe, "-test.run=^TestConcurrentSQLiteInitialization$")
			cmd.Env = append(os.Environ(), homeEnv+"="+home, fmt.Sprintf("%s=session_writer_%d", idEnv, index))
			if out, err := cmd.CombinedOutput(); err != nil {
				t.Errorf("writer %d: %v\n%s", index, err, out)
			}
		})
	}
	writers.Wait()
	items, err := List(home, "/project")
	if err != nil || len(items) != 4 {
		t.Fatalf("sessions %+v, %v", items, err)
	}
	db, err := openDatabase(ctx, home, false)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var applied int
	if err := db.QueryRowContext(ctx, "SELECT count(*) FROM goose_db_version WHERE version_id = 1 AND is_applied").Scan(&applied); err != nil || applied != 1 {
		t.Fatalf("initial migration applied %d times: %v", applied, err)
	}
	for _, item := range items {
		store, err := Open(home, item.ID, item.Project)
		if err != nil {
			t.Fatal(err)
		}
		state, loadErr := store.Load(ctx, item.ID)
		closeErr := store.Close()
		if loadErr != nil || closeErr != nil || state.Revision != 5 {
			t.Fatalf("writer state %+v, load %v, close %v", state, loadErr, closeErr)
		}
	}
}

func TestForkSharesTurnAndSerializesLateInput(t *testing.T) {
	ctx := t.Context()
	parent := New(t.TempDir(), Meta{Project: "/project"})
	defer parent.Close()
	state := core.State{ID: "session_parent", Revision: 1}
	if err := parent.Save(ctx, state); err != nil {
		t.Fatal(err)
	}
	state.Revision++
	state.ActiveRun = "run_shared"
	state.Messages = []model.Message{model.NewUserMessage("hello")}
	if err := parent.Save(ctx, state); err != nil {
		t.Fatal(err)
	}
	state.Revision++
	state.ActiveRun = ""
	if err := parent.Save(ctx, state); err != nil {
		t.Fatal(err)
	}
	child, err := parent.Fork(ctx, -1)
	if err != nil {
		t.Fatal(err)
	}
	defer child.Close()
	refs, err := sessiondb.New(child.db).ListSessionTurns(ctx, child.Meta().ID)
	if err != nil || !slices.Equal(refs, []string{"run_shared"}) {
		t.Fatalf("branch references %+v, %v", refs, err)
	}
	var writers sync.WaitGroup
	for i, store := range []*Store{parent, child} {
		writers.Go(func() {
			if err := store.RecordInput("run_shared", fmt.Sprintf("late input %d", i)); err != nil {
				t.Error(err)
			}
		})
	}
	writers.Wait()
	for _, store := range []*Store{parent, child} {
		turn, err := store.ReadTurn(0)
		if err != nil {
			t.Fatal(err)
		}
		var inputs []string
		for _, cell := range turn.Cells {
			if cell.Kind == "user" {
				inputs = append(inputs, cell.Text)
			}
		}
		slices.Sort(inputs)
		if !slices.Equal(inputs, []string{"late input 0", "late input 1"}) {
			t.Fatalf("late input lost: %+v", turn.Cells)
		}
	}
}
