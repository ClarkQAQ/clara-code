package tool

import (
	"context"
	"time"
)

type TaskTarget struct {
	Target string `json:"target"`
}

func SendMessage(backend TaskBackend) Tool  { return sendTaskTool(backend, "send_message", false) }
func FollowupTask(backend TaskBackend) Tool { return sendTaskTool(backend, "followup_task", true) }

func sendTaskTool(backend TaskBackend, name string, follow bool) Tool {
	return Func(Definition{Name: name, Description: "Send an instruction to a delegated task.", Effects: []Effect{EffectState}},
		func(ctx context.Context, input struct {
			Target  string `json:"target"`
			Message string `json:"message"`
		}) (Result, error) {
			if err := backend.Send(ctx, input.Target, input.Message, follow); err != nil {
				return Result{}, err
			}
			return TextResult("Message accepted."), nil
		})
}

func WaitAgent(backend TaskBackend) Tool {
	return Func(Definition{Name: "wait_agent", Description: "Wait for a delegated task to finish.", Effects: []Effect{EffectState}, Timeout: 10 * time.Minute},
		func(ctx context.Context, input struct {
			Target    string `json:"target"`
			TimeoutMS int    `json:"timeout_ms,omitempty"`
		}) (Result, error) {
			return taskResult(backend.Wait(ctx, input.Target, time.Duration(min(input.TimeoutMS, 600000))*time.Millisecond))
		})
}

func InterruptAgent(backend TaskBackend) Tool {
	return taskAction("interrupt_agent", "Cancel the active run of a delegated task.", backend.Interrupt)
}
func CloseAgent(backend TaskBackend) Tool {
	return taskAction("close_agent", "Close a delegated task and release its resources.", backend.CloseTask)
}
func ListAgents(backend TaskBackend) Tool {
	return Func(Definition{Name: "list_agents", Description: "List tasks owned by the current Agent.", Effects: []Effect{EffectRead}, ParallelSafe: true},
		func(ctx context.Context, _ struct{}) (Result, error) { return taskResult(backend.List(ctx)) })
}
func taskAction(name, description string, run func(context.Context, string) error) Tool {
	return Func(Definition{Name: name, Description: description, Effects: []Effect{EffectState}},
		func(ctx context.Context, input TaskTarget) (Result, error) {
			if err := run(ctx, input.Target); err != nil {
				return Result{}, err
			}
			return TextResult("Task updated."), nil
		})
}
