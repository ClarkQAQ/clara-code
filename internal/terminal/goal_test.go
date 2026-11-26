package terminal

import (
	"context"
	"slices"
	"strings"
	"testing"
	"time"

	"clara-code/internal/coding"
	"clara-code/internal/session"
	"clara-code/pkg/core/prompt"
	"clara-code/pkg/tool"
)

func TestGoalStatusAndTimelineAfterInterrupt(t *testing.T) {
	a := testApp(t)
	const objective = "CANCEL_SLOW_TURN Verify the workspace"
	a.submit("/goal " + objective)
	if a.run == nil || !strings.HasSuffix(stripStyles(a.statusLine()), "Goal active") {
		t.Fatalf("starting goal did not update the footer: %q", a.statusLine())
	}
	a.question = newQuestionForm(coding.QuestionRequest{Items: []coding.Question{{Question: "What should be verified?"}}})
	view := a.pane()
	if len(view.rows) > a.view.contentHeight() || view.cursor == nil || view.cursor.row >= len(view.rows)-1 || !strings.HasSuffix(stripStyles(view.rows[len(view.rows)-1]), "Goal active") {
		t.Fatalf("goal footer displaced the question cursor or disappeared while waiting: %+v", view)
	}
	a.handle(key{code: "escape"})
	finishGoalViewTurn(t, a)
	if state := a.conv.Goal.State(); state.Status != tool.GoalPaused || state.Objective != objective {
		t.Fatalf("interrupt lost goal state: %+v", state)
	}
	if want := []string{"Goal started", "Goal paused"}; !slices.Equal(goalTimeline(a), want) {
		t.Fatalf("live goal activity = %v, want %v", goalTimeline(a), want)
	}

	reopenGoalView(t, a, "")
	if a.run != nil || !strings.HasSuffix(stripStyles(a.statusLine()), "Goal paused (/continue)") {
		t.Fatalf("reopening did not restore paused goal: %q", a.statusLine())
	}
	if want := []string{"Goal started", "Goal paused"}; !slices.Equal(goalTimeline(a), want) {
		t.Fatalf("restored goal activity = %v, want %v", goalTimeline(a), want)
	}

	a.submit("/continue")
	if a.run == nil || a.conv.Goal.State().Status != tool.GoalActive {
		t.Fatal("could not continue the restored goal")
	}
	a.submit("/pause")
	finishGoalViewTurn(t, a)
	a.start("An ordinary prompt", false)
	finishGoalViewTurn(t, a)
	if want := []rewindPrompt{{turn: 0, text: objective}, {turn: 2, text: "An ordinary prompt"}}; !slices.Equal(a.prompts, want) {
		t.Fatalf("rewind entries retained an internal goal turn: %+v", a.prompts)
	}
	a.openRewind()
	if rows := strings.Join(a.rewindRows(), "\n"); !strings.Contains(rows, "2/2") || strings.Contains(rows, "Continue the active goal") {
		t.Fatalf("rewind exposed an internal goal prompt: %q", rows)
	}
}

func TestSavedActiveGoalDefersUntilAgentMode(t *testing.T) {
	a := testApp(t)
	if err := a.conv.Goal.Start("CANCEL_SLOW_TURN Check the output", 0, 0); err != nil {
		t.Fatal(err)
	}
	a.mode = prompt.Plan
	a.host.Config.TUI.StatusLine = []string{}
	reopenGoalView(t, a, "Unsubmitted draft")
	if a.run != nil || !strings.Contains(a.statusLine(), "Goal active") {
		t.Fatalf("active goal was lost or started during Plan mode: %q", a.statusLine())
	}
	a.view.width = 20
	if text := stripStyles(a.statusLine()); text != "  Plan · Goal active" {
		t.Fatalf("compact status line = %q", text)
	}
	a.view.width = 15
	if text := stripStyles(a.statusLine()); !strings.HasSuffix(text, "Goal active") || widthOf(text) != 15 {
		t.Fatalf("narrow status line = %q", text)
	}
	a.handle(key{code: "backtab"})
	if a.run == nil || a.editor.text != "Unsubmitted draft" {
		t.Fatalf("automatic continuation did not preserve the draft: %q", a.editor.text)
	}
	a.handle(key{code: "escape"})
	finishGoalViewTurn(t, a)
	if a.conv.Goal.State().Status != tool.GoalPaused || a.editor.text != "Unsubmitted draft" || len(a.prompts) != 0 {
		t.Fatalf("interruption changed the draft or exposed the goal worker: %q, %+v", a.editor.text, a.prompts)
	}
}

func reopenGoalView(t *testing.T, a *app, draft string) {
	t.Helper()
	id := a.conv.Session.ID()
	if err := a.conv.Close(); err != nil {
		t.Fatal(err)
	}
	store, err := session.Open(a.host.Config.Home, id, a.space.Root)
	if err != nil {
		t.Fatal(err)
	}
	conversation, err := a.host.Open(t.Context(), a.space, store, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	a.conv = nil
	a.view.resetHistory()
	if err := a.install(operationResult{conv: conversation, draft: draft, label: "Resumed conversation", resumeGoal: true}); err != nil {
		t.Fatal(err)
	}
}

func finishGoalViewTurn(t *testing.T, a *app) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	for a.run != nil {
		select {
		case event := <-a.events:
			a.onRunEvent(event)
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		}
	}
}

func goalTimeline(a *app) []string {
	var labels []string
	for _, cell := range a.view.history {
		if cell.Kind == session.GoalUpdatedKind {
			labels = append(labels, cell.Text)
		}
	}
	return labels
}
