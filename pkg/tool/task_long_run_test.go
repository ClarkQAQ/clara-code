package tool

import (
	"context"
	"slices"
	"strconv"
	"testing"
	"time"

	"clara-code/pkg/model"
)

type researchStepsModel struct {
	model.LanguageModel
	requests int
}

func (m *researchStepsModel) Stream(_ context.Context, _ model.Call) (model.StreamResponse, error) {
	m.requests++
	if m.requests <= 101 {
		return slices.Values([]model.StreamPart{
			{Type: model.StreamPartTypeToolCall, ID: strconv.Itoa(m.requests), ToolCallName: "inspect", ToolCallInput: "{}"},
			{Type: model.StreamPartTypeFinish, FinishReason: model.FinishReasonToolCalls},
		}), nil
	}
	return slices.Values([]model.StreamPart{
		{Type: model.StreamPartTypeTextStart, ID: "result"},
		{Type: model.StreamPartTypeTextDelta, ID: "result", Delta: "Research complete."},
		{Type: model.StreamPartTypeTextEnd, ID: "result"},
		{Type: model.StreamPartTypeFinish, FinishReason: model.FinishReasonStop},
	}), nil
}

func TestResearchTaskCompletesBeyondOneHundredModelRequests(t *testing.T) {
	language := &researchStepsModel{}
	inspections := 0
	inspect := Func(Definition{Name: "inspect", Description: "Inspect one record.", Effects: []Effect{EffectRead}},
		func(context.Context, struct{}) (Result, error) {
			inspections++
			return TextResult("Checked."), nil
		})
	manager := NewTaskManager(func(context.Context, TaskSpec, *TaskScope) (TaskSetup, error) {
		return TaskSetup{Model: language, Tools: []Tool{inspect}}, nil
	}, 1, 1)
	scope := manager.Scope(t.Context(), "parent", ScopeOptions{AllowedTools: []string{"inspect"}})
	defer scope.Close(t.Context())
	task, err := scope.Spawn(t.Context(), TaskSpec{Name: "records", Role: "researcher", Message: "Inspect each record."})
	if err != nil {
		t.Fatal(err)
	}
	task, err = scope.Wait(t.Context(), task.ID, 30*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if task.Status != "completed" || task.Result != "Research complete." || language.requests != 102 || inspections != 101 {
		t.Fatalf("child status %s (%s), result %q, requests %d, tool executions %d", task.Status, task.Error, task.Result, language.requests, inspections)
	}
}
