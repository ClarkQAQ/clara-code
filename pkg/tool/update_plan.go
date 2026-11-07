package tool

import (
	"context"
	"encoding/json"
	"slices"
	"sync"

	"clara-code/pkg/core"
	"clara-code/pkg/model"
	"clara-code/pkg/util/errs"
)

type PlanStep struct {
	Step   string `json:"step"`
	Status string `json:"status" enum:"pending,in_progress,completed"`
}

type PlanState struct {
	Explanation string     `json:"explanation,omitempty"`
	Steps       []PlanStep `json:"plan"`
}

type PlanStore interface {
	GetPlan(context.Context) (PlanState, error)
	SetPlan(context.Context, PlanState) error
}

type MemoryPlan struct {
	mu    sync.Mutex
	state PlanState
}

func (p *MemoryPlan) GetPlan(context.Context) (PlanState, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	state := p.state
	state.Steps = slices.Clone(state.Steps)
	return state, nil
}

func (p *MemoryPlan) SetPlan(_ context.Context, state PlanState) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	state.Steps = slices.Clone(state.Steps)
	p.state = state
	return nil
}

func UpdatePlan(store PlanStore) Tool {
	return Func(Definition{Name: "update_plan", Description: "Update the execution checklist. Keep at most one step in progress.", Effects: []Effect{EffectState}},
		func(ctx context.Context, state PlanState) (Result, error) {
			if len(state.Steps) > 64 || len(state.Explanation) > 4096 {
				return Result{}, errs.New(errs.CodeLimit, "plan is too large")
			}
			progress := 0
			for _, step := range state.Steps {
				if step.Step == "" || len(step.Step) > 1024 {
					return Result{}, errs.New(errs.CodeInvalidArgument, "invalid plan step")
				}
				if step.Status == "in_progress" {
					progress++
				}
			}
			if progress > 1 {
				return ErrorResult("Keep at most one step in progress."), nil
			}
			if err := store.SetPlan(ctx, state); err != nil {
				return Result{}, err
			}
			return TextResult("Plan updated."), nil
		})
}

func GetPlan(store PlanStore) Tool {
	return Func(Definition{Name: "get_plan", Description: "Read the execution checklist.", Effects: []Effect{EffectRead}, ParallelSafe: true},
		func(ctx context.Context, _ struct{}) (Result, error) {
			state, err := store.GetPlan(ctx)
			if err != nil {
				return Result{}, err
			}
			raw, err := json.Marshal(state)
			return TextResult(string(raw)), err
		})
}

// PlanOnly restricts a planning turn to reads and user interaction.
func PlanOnly(ctx context.Context, info model.ToolInfo, _ model.ToolCall) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if info.Effect == EffectRead || info.Effect == EffectInteraction {
		return nil
	}
	return errs.New(errs.CodeForbidden, "this tool is unavailable in Plan Mode")
}

// PlanTools selects the tools a planning step may advertise.
func PlanTools(items []core.AgentTool) []core.AgentTool {
	out := make([]core.AgentTool, 0, len(items))
	for _, tool := range items {
		if PlanOnly(context.Background(), tool.Info(), model.ToolCall{}) == nil {
			out = append(out, tool)
		}
	}
	return out
}
