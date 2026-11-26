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
	save  func(GoalState) error
}

// NewGoal restores state and saves changes before making them visible. The
// optional save callback is serialized and must not call back into the Goal.
func NewGoal(state GoalState, save func(GoalState) error) *Goal {
	return &Goal{state: state, save: save}
}

func (g *Goal) State() GoalState {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.state
}

// Start records an explicitly requested objective. A zero token or turn budget is unlimited.
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
	return g.commit(GoalState{Objective: objective, Status: GoalActive, TokenBudget: tokens, TurnBudget: turns})
}

func (g *Goal) Pause() error {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.state.Status != GoalActive {
		return errs.ErrConflict
	}
	next := g.state
	next.Status = GoalPaused
	return g.commit(next)
}

func (g *Goal) Resume() error {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.state.Status != GoalPaused && g.state.Status != GoalBlocked {
		return errs.ErrConflict
	}
	next := g.state
	next.Status = GoalActive
	next.Blocker, next.BlockerTurns, next.LastBlockerTurn = "", 0, 0
	return g.commit(next)
}

// Block records a terminal execution failure. A goal that already stopped is
// left in its current state.
func (g *Goal) Block(summary string) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.state.Status != GoalActive {
		return nil
	}
	summary = strings.TrimSpace(summary)
	if summary == "" || len(summary) > 4096 {
		return errs.ErrConflict
	}
	next := g.state
	next.Status, next.Summary, next.Blocker = GoalBlocked, summary, summary
	return g.commit(next)
}

// Advance reserves the next automatic continuation if budget remains.
func (g *Goal) Advance() (GoalState, bool, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	next := g.state
	if next.Status != GoalActive {
		return next, false, nil
	}
	if next.TurnBudget > 0 && next.TurnsUsed >= next.TurnBudget {
		next.Status = GoalExhausted
		err := g.commit(next)
		return g.state, false, err
	}
	next.TurnsUsed++
	err := g.commit(next)
	return g.state, err == nil, err
}

// NextTurn returns continuation input for an in-memory goal. Hosts that persist
// goals should use Advance to handle write errors.
func (g *Goal) NextTurn() []model.Message {
	state, ok, _ := g.Advance()
	if !ok {
		return nil
	}
	return []model.Message{model.NewUserMessage("Continue the active goal: " + state.Objective)}
}

// RecordUsage records actual provider usage and stops future work at the budget.
func (g *Goal) RecordUsage(usage model.Usage) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	next := g.state
	if next.Status != GoalActive {
		return nil
	}
	count := usage.TotalTokens
	if count <= 0 {
		count = usage.InputTokens + usage.OutputTokens
	}
	next.TokensUsed += max(count, 0)
	if next.TokenBudget > 0 && next.TokensUsed >= next.TokenBudget {
		next.Status = GoalExhausted
	}
	return g.commit(next)
}

// Charge records provider usage for an in-memory goal. Hosts that persist goals
// should use RecordUsage to handle write errors.
func (g *Goal) Charge(usage model.Usage) {
	_ = g.RecordUsage(usage)
}

func (g *Goal) commit(next GoalState) error {
	if next == g.state {
		return nil
	}
	if g.save != nil {
		if err := g.save(next); err != nil {
			return err
		}
	}
	g.state = next
	return nil
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
			next := g.state
			if next.Status != GoalActive {
				return Result{}, errs.ErrConflict
			}
			next.Summary = summary
			message := "Blocker recorded. Continue any independent work."
			if input.Status == "completed" {
				next.Status = GoalCompleted
				message = "Goal completed."
			} else {
				if next.Blocker != summary || next.LastBlockerTurn+1 != next.TurnsUsed && next.LastBlockerTurn != next.TurnsUsed {
					next.Blocker, next.BlockerTurns = summary, 0
				}
				if next.LastBlockerTurn != next.TurnsUsed || next.BlockerTurns == 0 {
					next.LastBlockerTurn = next.TurnsUsed
					next.BlockerTurns++
				}
				if next.BlockerTurns >= 3 {
					next.Status = GoalBlocked
					message = "Goal blocked."
				}
			}
			if err := g.commit(next); err != nil {
				return Result{}, err
			}
			return TextResult(message), nil
		})
}
