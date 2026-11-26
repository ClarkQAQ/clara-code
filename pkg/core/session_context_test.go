package core_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"testing/synctest"

	"clara-code/pkg/core"
	"clara-code/pkg/model"
)

type checkpointModel struct {
	model.LanguageModel
	calls      atomic.Int32
	incomplete bool
}

func (m *checkpointModel) Provider() string { return "fixture" }
func (m *checkpointModel) Model() string    { return "fixture" }
func (m *checkpointModel) Stream(context.Context, model.Call) (model.StreamResponse, error) {
	m.calls.Add(1)
	return func(yield func(model.StreamPart) bool) {
		if m.incomplete {
			yield(model.StreamPart{Type: model.StreamPartTypeTextDelta, Delta: "partial"})
			return
		}
		yield(model.StreamPart{Type: model.StreamPartTypeFinish, FinishReason: model.FinishReasonStop})
	}, nil
}

type checkpointStore struct {
	core.Store
	rejectReset bool
}

func (s checkpointStore) Save(ctx context.Context, state core.State) error {
	if strings.Contains(string(state.ContextState), "reject") {
		return errors.New("checkpoint rejected")
	}
	if s.rejectReset && len(state.Events) > 0 && state.Events[len(state.Events)-1].Kind == "model.stream_reset" {
		return errors.New("stream reset checkpoint rejected")
	}
	return s.Store.Save(ctx, state)
}

func TestFailedStreamResetCheckpointKeepsPartial(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx := t.Context()
		store := checkpointStore{Store: core.NewMemoryStore(), rejectReset: true}
		m := &checkpointModel{incomplete: true}
		session, err := core.NewSession(ctx, core.NewAgent(m, core.WithMaxRetries(1)), store)
		if err != nil {
			t.Fatal(err)
		}
		run, err := session.Start(ctx, core.AgentStreamCall{Prompt: "test reset", OnStreamReset: func() error { return nil }})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := run.Wait(ctx); err == nil || !strings.Contains(err.Error(), "stream reset checkpoint rejected") {
			t.Fatalf("expected reset checkpoint failure, got %v", err)
		}
		state, err := store.Load(ctx, session.ID())
		if err != nil {
			t.Fatal(err)
		}
		var parts []string
		for _, event := range state.Events {
			parts = append(parts, event.Kind)
		}
		if m.calls.Load() != 1 || strings.Join(parts, ",") != "run.started,text_delta,run.failed" {
			t.Fatalf("requests %d, events %v", m.calls.Load(), parts)
		}
	})
}

func TestContextCheckpointRestoresAndFailurePreventsDispatch(t *testing.T) {
	ctx := t.Context()
	store := checkpointStore{Store: core.NewMemoryStore()}
	m := &checkpointModel{}
	engine := core.NewAgent(m)
	session, err := core.NewSession(ctx, engine, store)
	if err != nil {
		t.Fatal(err)
	}
	first := json.RawMessage(`{"window":"first"}`)
	call := core.AgentStreamCall{Prompt: "first", PrepareStep: func(ctx context.Context, step core.PrepareStepFunctionOptions) (context.Context, core.PrepareStepResult, error) {
		if len(step.ContextState) != 0 {
			t.Error("new session already had host state")
		}
		return ctx, core.PrepareStepResult{ContextState: first}, nil
	}}
	run, err := session.Start(ctx, call)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := run.Wait(ctx); err != nil {
		t.Fatal(err)
	}
	if err := session.Close(ctx); err != nil {
		t.Fatal(err)
	}
	restored, err := core.OpenSession(ctx, engine, store, session.ID())
	if err != nil {
		t.Fatal(err)
	}
	defer restored.Close(ctx)
	copy := restored.ContextState()
	copy[0] = 'x'
	if string(restored.ContextState()) != string(first) {
		t.Fatal("mutable state escaped session")
	}
	call.Prompt = "second"
	replacement := []core.Message{model.NewUserMessage("replacement")}
	call.PrepareStep = func(ctx context.Context, step core.PrepareStepFunctionOptions) (context.Context, core.PrepareStepResult, error) {
		if string(step.ContextState) != string(first) {
			t.Error("window did not survive restore")
		}
		return ctx, core.PrepareStepResult{History: replacement, ContextState: json.RawMessage(`{"window":"reject"}`)}, nil
	}
	run, err = restored.Start(ctx, call)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := run.Wait(ctx); err == nil || !strings.Contains(err.Error(), "checkpoint rejected") {
		t.Fatal("failed checkpoint accepted", err)
	}
	if m.calls.Load() != 1 || string(restored.ContextState()) != string(first) {
		t.Fatal("failed checkpoint dispatched or replaced window")
	}
	state, err := store.Load(ctx, restored.ID())
	if err != nil || string(state.ContextState) != string(first) {
		t.Fatal("failed window persisted", err)
	}
	if len(state.Messages) < 2 || len(restored.Messages()) != len(state.Messages) {
		t.Fatal("failed replacement discarded the conversation")
	}
	nextWindow := json.RawMessage(`{"window":"compacted"}`)
	call.PrepareStep = func(ctx context.Context, _ core.PrepareStepFunctionOptions) (context.Context, core.PrepareStepResult, error) {
		return ctx, core.PrepareStepResult{History: replacement, ContextState: nextWindow}, nil
	}
	call.OnHistory = func([]core.Message) error {
		saved, err := store.Load(ctx, restored.ID())
		if err == nil && (len(saved.Messages) != 1 || string(saved.ContextState) != string(nextWindow)) {
			return errors.New("replacement history and window were checkpointed separately")
		}
		return err
	}
	run, err = restored.Start(ctx, call)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := run.Wait(ctx); err != nil {
		t.Fatal(err)
	}
}
