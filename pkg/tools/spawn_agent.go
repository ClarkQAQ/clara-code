package tools

import (
	"context"
	"encoding/json"
	"time"
)

func taskResult(value any, err error) (Result, error) {
	if err != nil {
		return Result{}, err
	}
	raw, err := json.Marshal(value)
	return TextResult(string(raw)), err
}

func SpawnAgent(backend TaskBackend) Tool {
	return Func(Definition{Name: "spawn_agent", Description: "Start a delegated task. Give the child a complete brief.", Effects: []Effect{EffectState}},
		func(ctx context.Context, spec TaskSpec) (Result, error) { return taskResult(backend.Spawn(ctx, spec)) })
}

func RunTask(backend TaskBackend) Tool {
	return Func(Definition{Name: "subagent_task", Description: "Delegate one focused task and wait for the result.", Effects: []Effect{EffectState}, Timeout: 10 * time.Minute},
		func(ctx context.Context, spec TaskSpec) (Result, error) {
			info, err := backend.Spawn(ctx, spec)
			if err != nil {
				return Result{}, err
			}
			return taskResult(backend.Wait(ctx, info.ID, 9*time.Minute))
		})
}
