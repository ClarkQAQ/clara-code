package tool

import (
	"context"
	"encoding/json"
	"strings"
	"sync"

	"clara-code/pkg/model"
	"clara-code/pkg/util/errs"
)

type GoalStatus string

const (
	GoalActive    GoalStatus = "active"
	GoalPaused    GoalStatus = "paused"
	GoalCompleted GoalStatus = "completed"
	GoalBlocked   GoalStatus = "blocked"
	GoalExhausted GoalStatus = "budget_exhausted"
)

type GoalState struct {
	Objective               string
	Status                  GoalStatus
	TokenBudget, TokensUsed int64
	TurnBudget, TurnsUsed   int
	Summary                 string
	Blocker                 string
	BlockerTurns            int
	LastBlockerTurn         int
}

type Goal struct {
	mu    sync.Mutex
	state GoalState
}

func (g *Goal) State() GoalState {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.state
}

// Start records an explicitly requested objective.
func (g *Goal) Start(objective string, tokens int64, turns int) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	objective = strings.TrimSpace(objective)
	if objective == "" || len(objective) > 8192 || tokens < 0 || turns < 0 {
		return errs.ErrConflict
	}
	if g.state.Status == GoalActive || g.state.Status == GoalPaused {
		return errs.ErrBusy
	}
	g.state = GoalState{Objective: objective, Status: GoalActive, TokenBudget: tokens, TurnBudget: turns}
	return nil
}

func (g *Goal) Pause() error {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.state.Status != GoalActive {
		return errs.ErrConflict
	}
	g.state.Status = GoalPaused
	return nil
}

func (g *Goal) Resume() error {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.state.Status != GoalPaused && g.state.Status != GoalBlocked {
		return errs.ErrConflict
	}
	g.state.Status = GoalActive
	g.state.Blocker, g.state.BlockerTurns, g.state.LastBlockerTurn = "", 0, 0
	return nil
}

// NextTurn returns continuation input for the host to schedule.
func (g *Goal) NextTurn() []model.Message {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.state.Status != GoalActive {
		return nil
	}
	if g.state.TurnBudget > 0 && g.state.TurnsUsed >= g.state.TurnBudget {
		g.state.Status = GoalExhausted
		return nil
	}
	g.state.TurnsUsed++
	return []model.Message{model.NewUserMessage("Continue the active goal: " + g.state.Objective)}
}

// Charge records actual provider usage and stops future work at the budget.
func (g *Goal) Charge(usage model.Usage) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.state.Status != GoalActive {
		return
	}
	count := usage.TotalTokens
	if count <= 0 {
		count = usage.InputTokens + usage.OutputTokens
	}
	g.state.TokensUsed += max(count, 0)
	if g.state.TokenBudget > 0 && g.state.TokensUsed >= g.state.TokenBudget {
		g.state.Status = GoalExhausted
	}
}

func (g *Goal) Get() Tool {
	return Func(Definition{Name: "get_goal", Description: "Read the active objective and its remaining budget.", Effects: []Effect{EffectRead}, ParallelSafe: true},
		func(context.Context, struct{}) (Result, error) {
			raw, err := json.Marshal(g.State())
			return TextResult(string(raw)), err
		})
}

func (g *Goal) Update() Tool {
	return Func(Definition{Name: "update_goal", Description: "Record verified completion or report a concrete blocker.", Effects: []Effect{EffectState}},
		func(_ context.Context, input struct {
			Status  string `json:"status" enum:"completed,blocked"`
			Summary string `json:"summary"`
		}) (Result, error) {
			summary := strings.TrimSpace(input.Summary)
			if summary == "" || len(summary) > 4096 {
				return Result{}, errs.ErrConflict
			}
			g.mu.Lock()
			defer g.mu.Unlock()
			if g.state.Status != GoalActive {
				return Result{}, errs.ErrConflict
			}
			g.state.Summary = summary
			if input.Status == "completed" {
				g.state.Status = GoalCompleted
				return TextResult("Goal completed."), nil
			}
			if g.state.Blocker != summary || g.state.LastBlockerTurn+1 != g.state.TurnsUsed && g.state.LastBlockerTurn != g.state.TurnsUsed {
				g.state.Blocker, g.state.BlockerTurns = summary, 0
			}
			if g.state.LastBlockerTurn != g.state.TurnsUsed || g.state.BlockerTurns == 0 {
				g.state.LastBlockerTurn = g.state.TurnsUsed
				g.state.BlockerTurns++
			}
			if g.state.BlockerTurns >= 3 {
				g.state.Status = GoalBlocked
				return TextResult("Goal blocked."), nil
			}
			return TextResult("Blocker recorded. Continue any independent work."), nil
		})
}
