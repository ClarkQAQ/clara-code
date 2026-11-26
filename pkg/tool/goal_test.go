package tool_test

import (
	"errors"
	"fmt"
	"testing"

	"clara-code/pkg/model"
	"clara-code/pkg/tool"
)

func ExampleNewGoal() {
	var saved tool.GoalState
	goal := tool.NewGoal(saved, func(state tool.GoalState) error {
		saved = state
		return nil
	})
	if err := goal.Start("Verify the release", 100, 3); err != nil {
		panic(err)
	}
	if err := goal.RecordUsage(model.Usage{TotalTokens: 12}); err != nil {
		panic(err)
	}
	reopened := tool.NewGoal(saved, nil)
	fmt.Println(reopened.State().Status, reopened.State().TokensUsed)
	// Output: active 12
}

func TestGoalSaveFailurePreservesLastState(t *testing.T) {
	unavailable := errors.New("write unavailable")
	fail := false
	var saved tool.GoalState
	goal := tool.NewGoal(saved, func(state tool.GoalState) error {
		if fail {
			return unavailable
		}
		saved = state
		return nil
	})
	if err := goal.Start("Verify the release", 10, 1); err != nil {
		t.Fatal(err)
	}
	fail = true
	before := saved
	if err := goal.Pause(); !errors.Is(err, unavailable) || goal.State() != before {
		t.Fatalf("pause changed state on failure: %+v (%v)", goal.State(), err)
	}
	if got, ok, err := goal.Advance(); !errors.Is(err, unavailable) || ok || got != before {
		t.Fatalf("advance changed state on failure: %+v (%v, %v)", got, ok, err)
	}
	if err := goal.RecordUsage(model.Usage{TotalTokens: 10}); !errors.Is(err, unavailable) || goal.State() != before {
		t.Fatalf("usage changed state on failure: %+v (%v)", goal.State(), err)
	}
	if _, err := goal.Update().Run(t.Context(), model.ToolCall{Input: `{"status":"completed","summary":"Verified"}`}); !errors.Is(err, unavailable) || goal.State() != before {
		t.Fatalf("completion changed state on failure: %+v (%v)", goal.State(), err)
	}
	fail = false
	if _, ok, err := goal.Advance(); err != nil || !ok || saved.TurnsUsed != 1 {
		t.Fatalf("did not reserve continuation: %+v (%v, %v)", saved, ok, err)
	}
	if _, ok, err := goal.Advance(); err != nil || ok || saved.Status != tool.GoalExhausted {
		t.Fatalf("turn exhaustion not saved: %+v (%v, %v)", saved, ok, err)
	}
}
