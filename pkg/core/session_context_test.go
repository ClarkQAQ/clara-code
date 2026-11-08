package core_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync/atomic"
	"testing"

	"clara-code/pkg/core"
	"clara-code/pkg/model"
)

type checkpointModel struct {
	model.LanguageModel
	calls atomic.Int32
}

func (m *checkpointModel) Provider() string { return "fixture" }
func (m *checkpointModel) Model() string    { return "fixture" }
func (m *checkpointModel) Stream(context.Context, model.Call) (model.StreamResponse, error) {
	m.calls.Add(1)
	return func(yield func(model.StreamPart) bool) {
		yield(model.StreamPart{Type: model.StreamPartTypeFinish, FinishReason: model.FinishReasonStop})
	}, nil
}

type checkpointStore struct{ core.Store }

func (s checkpointStore) Save(ctx context.Context, state core.State) error {
	if strings.Contains(string(state.ContextState), "reject") {
		return errors.New("checkpoint rejected")
	}
	return s.Store.Save(ctx, state)
}

func TestContextCheckpointRestoresAndFailurePreventsDispatch(t *testing.T) {
	ctx := t.Context()
	store := checkpointStore{Store: core.NewMemoryStore()}
	model := &checkpointModel{}
	engine := core.NewAgent(model)
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
	call.PrepareStep = func(ctx context.Context, step core.PrepareStepFunctionOptions) (context.Context, core.PrepareStepResult, error) {
		if string(step.ContextState) != string(first) {
			t.Error("window did not survive restore")
		}
		return ctx, core.PrepareStepResult{ContextState: json.RawMessage(`{"window":"reject"}`)}, nil
	}
	run, err = restored.Start(ctx, call)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := run.Wait(ctx); err == nil || !strings.Contains(err.Error(), "checkpoint rejected") {
		t.Fatal("failed checkpoint accepted", err)
	}
	if model.calls.Load() != 1 || string(restored.ContextState()) != string(first) {
		t.Fatal("failed checkpoint dispatched or replaced window")
	}
	state, err := store.Load(ctx, restored.ID())
	if err != nil || string(state.ContextState) != string(first) {
		t.Fatal("failed window persisted", err)
	}
}
