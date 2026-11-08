package tool

import (
	"context"
	"time"
)

type TaskTarget struct {
	Target string `json:"target" jsonschema:"Task id returned by spawn_agent or list_agents; local tasks also accept their name in this parent scope."`
}

func SendMessage(backend TaskBackend) Tool  { return sendTaskTool(backend, "send_message", false) }
func FollowupTask(backend TaskBackend) Tool { return sendTaskTool(backend, "followup_task", true) }

func sendTaskTool(backend TaskBackend, name string, follow bool) Tool {
	description := "Steer a running child using target = its returned task id. Does not restart a finished child; use followup_task for that."
	if follow {
		description = "Queue a follow-up for a running child or start another turn on a finished child. target is the existing task id; do not spawn a duplicate. Unavailable after close_agent."
	}
	return Func(Definition{Name: name, Description: description, Effects: []Effect{EffectState}},
		func(ctx context.Context, input struct {
			Target  string `json:"target" jsonschema:"Existing task id returned by spawn_agent or list_agents; local tasks also accept their name."`
			Message string `json:"message" jsonschema:"Non-empty instruction, at most 262144 UTF-8 bytes (256 KiB)."`
		}) (Result, error) {
			if err := backend.Send(ctx, input.Target, input.Message, follow); err != nil {
				return Result{}, err
			}
			return TextResult("Message accepted."), nil
		})
}

func WaitAgent(backend TaskBackend) Tool {
	return Func(Definition{Name: "wait_agent", Description: "Wait for an existing task id to finish. timeout_ms is milliseconds (maximum 600000; omitted or non-positive means 10 minutes). Timeout cancels this wait only, not the child; use list_agents and wait again rather than spawning a duplicate.", Effects: []Effect{EffectState}, Timeout: 10 * time.Minute},
		func(ctx context.Context, input struct {
			Target    string `json:"target" jsonschema:"Existing task id returned by spawn_agent or list_agents; local tasks also accept their name."`
			TimeoutMS int    `json:"timeout_ms,omitempty" jsonschema:"Wait duration in milliseconds, capped at 600000. Omitted or non-positive defaults to 600000; does not stop the task on timeout."`
		}) (Result, error) {
			return taskResult(backend.Wait(ctx, input.Target, time.Duration(min(input.TimeoutMS, 600000))*time.Millisecond))
		})
}

func InterruptAgent(backend TaskBackend) Tool {
	return taskAction("interrupt_agent", "Cancel the active turn of an existing task id, retaining its session for followup_task. Does not close the child.", backend.Interrupt)
}
func CloseAgent(backend TaskBackend) Tool {
	return taskAction("close_agent", "Cancel and close an existing task id and its descendants. Collect its result first; this releases the session and prevents followup_task.", backend.CloseTask)
}
func ListAgents(backend TaskBackend) Tool {
	return Func(Definition{Name: "list_agents", Description: "List tasks owned by the current Agent, including ids, names, statuses and collected results. Use this to find an existing child after a wait timeout.", Effects: []Effect{EffectRead}, ParallelSafe: true},
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
