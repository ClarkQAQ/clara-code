//go:build unix

package cli

import (
	"strings"
	"testing"

	"clara-code/internal/coding"
	"clara-code/internal/session"
	"clara-code/pkg/tool"
)

func TestInlineGoalSurvivesInterruptAndProcessRestart(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	home, cwd := t.TempDir(), t.TempDir()
	space, err := coding.ResolveWorkspace(cwd)
	if err != nil {
		t.Fatal(err)
	}
	const objective = "CANCEL_SLOW_TURN"
	footer := func(p *cliPTY, mark int, state, goal string) {
		t.Helper()
		p.await(t, mark, state+" · ")
		p.await(t, mark, goal)
		for line := range strings.Lines(p.output()[mark:]) {
			if strings.Contains(line, state+" · ") && strings.Contains(line, goal) {
				return
			}
		}
		t.Fatalf("status bar did not show %q while %s", goal, state)
	}
	interrupt := func(p *cliPTY) {
		t.Helper()
		mark := len(p.output())
		p.send(t, "\x1b")
		p.await(t, mark, "Conversation interrupted")
		footer(p, mark, "Ready", "Goal paused")
	}
	exit := func(p *cliPTY) {
		t.Helper()
		p.send(t, "/exit\r")
		if err := p.cmd.Wait(); err != nil {
			t.Fatal(err)
		}
	}

	p := startPTY(t, home, cwd)
	p.await(t, 0, "Ready")
	mark := len(p.output())
	p.send(t, "/goal "+objective+"\r")
	p.await(t, mark, "Goal started")
	p.await(t, mark, "❯ "+objective)
	footer(p, mark, "Working", "Goal active")
	interrupt(p)
	exit(p)

	items, err := session.List(home, space.Root)
	if err != nil || len(items) != 1 {
		t.Fatalf("goal conversation was not saved: %+v, %v", items, err)
	}
	id := items[0].ID
	assertSaved := func(wantStatus tool.GoalStatus, wantTurns int, wantActiveRun bool) {
		t.Helper()
		store, err := session.Open(home, id, space.Root)
		if err != nil {
			t.Fatal(err)
		}
		defer store.Close()
		if goal := store.GoalState(); goal.Status != wantStatus || goal.Objective != objective {
			t.Fatalf("saved goal = %+v, want %q with status %q", goal, objective, wantStatus)
		}
		if count, active := store.TurnCount(), store.ActiveTurn() != nil; count != wantTurns || active != wantActiveRun {
			t.Fatalf("saved turns = %d, active run = %t, want %d, %t", count, active, wantTurns, wantActiveRun)
		}
	}
	assertSaved(tool.GoalPaused, 1, false)

	paused := startPTY(t, home, cwd, "resume", id)
	paused.await(t, 0, "Goal started")
	paused.await(t, 0, "❯ "+objective)
	footer(paused, 0, "Ready", "Goal paused")
	mark = len(paused.output())
	paused.send(t, "/continue\r")
	footer(paused, mark, "Working", "Goal active")
	if err := paused.cmd.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	if err := paused.cmd.Wait(); err == nil {
		t.Fatal("expected abrupt process exit")
	}
	assertSaved(tool.GoalActive, 1, true)

	resumed := startPTY(t, home, cwd, "resume", id)
	footer(resumed, 0, "Working", "Goal active")
	interrupt(resumed)
	mark = len(resumed.output())
	resumed.send(t, "/new\r")
	resumed.await(t, mark, "New conversation · ")
	opened := strings.LastIndex(resumed.output(), "New conversation · ")
	resumed.await(t, opened, "Ready")
	if output := resumed.output()[opened:]; strings.Contains(output, "Goal paused") || strings.Contains(output, "Goal active") {
		t.Fatalf("new conversation displayed the previous goal: %q", output)
	}
	exit(resumed)
	assertSaved(tool.GoalPaused, 3, false)
}
