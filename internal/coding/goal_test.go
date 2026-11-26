package coding

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"clara-code/internal/session"
	"clara-code/pkg/core/prompt"
	"clara-code/pkg/model"
	"clara-code/pkg/tool"
)

func TestGoalWaitPauseAndRestore(t *testing.T) {
	const objective = "Verify the result with the user"
	step := 0
	fixture := fixtureModel{reply: func(call model.Call) (*model.Response, error) {
		step++
		switch step {
		case 1:
			response := toolReply("request_user_input", map[string]any{
				"questions": []Question{{ID: "check", Question: "Has the result been verified?"}},
			}, step)
			response.Usage.TotalTokens = 11
			return response, nil
		case 2:
			boundary := call.Prompt[prompt.ActiveTurn(call.Prompt)].Metadata
			if !strings.Contains(contextText(call.Prompt), objective) ||
				boundary == nil || boundary.Kind != model.MessageKindInternalTurn {
				return nil, errors.New("resumed goal missing from request")
			}
			response := toolReply("update_goal", map[string]string{"status": "completed", "summary": "Verified"}, step)
			response.Usage.TotalTokens = 7
			return response, nil
		default:
			return demoText("Verified."), nil
		}
	}}
	c, host := testHost(t, t.TempDir(), fixture)
	if err := c.Goal.Start(objective, 100, 5); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 8*time.Second)
	defer cancel()
	run, err := c.Start(ctx, prompt.Execute, objective, false)
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-c.Questions.Requests:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	wait, endWait := context.WithDeadline(ctx, time.Now())
	defer endWait()
	if _, err := run.Wait(wait); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("wait result: %v", err)
	}
	active := c.Store.GoalState()
	if active.Status != tool.GoalActive || active.TokensUsed != 11 || c.Session.Active() != run {
		t.Fatalf("observation timeout changed the goal or run: %+v", active)
	}
	if err := c.PauseGoal(); err != nil {
		t.Fatal(err)
	}
	run.Cancel()
	if _, err := run.Wait(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("interrupt result: %v", err)
	}
	sessionID, workspace := c.Session.ID(), c.Workspace
	if err := c.Close(); err != nil {
		t.Fatal(err)
	}
	store, err := session.Open(host.Config.Home, sessionID, workspace.Root)
	if err != nil {
		t.Fatal(err)
	}
	resumed, err := host.Open(ctx, workspace, store, NewQuestions(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer resumed.Close()
	active.Status = tool.GoalPaused
	if got := resumed.Goal.State(); got != active {
		t.Fatalf("restored goal %+v, want %+v", got, active)
	}
	run, err = resumed.ContinueGoal(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := run.Wait(ctx); err != nil {
		t.Fatal(err)
	}
	want := tool.GoalState{Objective: objective, Status: tool.GoalCompleted, TokenBudget: 100, TokensUsed: 18, TurnBudget: 5, Summary: "Verified"}
	if got := store.GoalState(); got != want || resumed.Goal.State() != got {
		t.Fatalf("completed goal %+v, want %+v", got, want)
	}
	turn, err := store.ReadTurn(1)
	if err != nil || !turn.Internal || turn.Prompt != "" {
		t.Fatalf("resumed worker turn %+v (%v)", turn, err)
	}
}

func TestGoalTerminalErrorBlocksAutomaticContinuation(t *testing.T) {
	failure := errors.New("fixture provider failure")
	requests := 0
	fixture := fixtureModel{reply: func(model.Call) (*model.Response, error) {
		requests++
		return nil, failure
	}}
	c, _ := testHost(t, t.TempDir(), fixture)
	ctx, cancel := context.WithTimeout(t.Context(), 8*time.Second)
	defer cancel()
	run, err := c.StartGoal(ctx, "Keep the goal after failures", nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := run.Wait(ctx); !errors.Is(err, failure) {
		t.Fatalf("run result: %v", err)
	}
	state := c.Store.GoalState()
	if state.Status != tool.GoalBlocked || state.Blocker != failure.Error() || requests != 1 {
		t.Fatalf("terminal failure produced %d requests and goal %+v", requests, state)
	}
}
