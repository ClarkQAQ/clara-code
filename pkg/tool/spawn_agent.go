package tool

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

// Both delegation tools expose the same contract as direct TaskScope.Spawn.
const taskInputSchema = `{
 "type":"object",
 "properties":{
  "task_name":{"type":"string","pattern":"^[a-z0-9_]{1,100}$","minLength":1,"maxLength":100,"description":"Unique task name in this parent scope: lowercase ASCII letters, digits and underscores only, 1-100 characters. Example: repo_survey. Hyphens, spaces and uppercase are invalid."},
  "role":{"type":"string","minLength":1,"pattern":"\\S","description":"Required host-supported role. Use a role advertised by the host; roles and tool permissions are host-specific."},
  "message":{"type":"string","minLength":1,"pattern":"\\S","description":"Required self-contained task brief, at most 262144 UTF-8 bytes (256 KiB). Include objective, paths, boundaries and expected result. Whitespace-only is invalid."},
  "model":{"type":"string","description":"Optional host model override. Omit to use the host default; only override when explicitly requested or instructed."},
  "reasoning":{"type":"string","description":"Optional reasoning effort override, using a level supported by the selected model. Omit to use the host default."},
  "fork_history":{"type":"boolean","default":false,"description":"Copy parent conversation when true; default false starts without parent conversation. Always supply a complete message. Does not copy tools, permissions or live processes. External ACP agents do not support this override."}
 },
 "required":["task_name","role","message"],
 "additionalProperties":false
}`

func taskDefinition(name, description string) Definition {
	return Definition{Name: name, Description: description, InputSchema: json.RawMessage(taskInputSchema), Effects: []Effect{EffectState}}
}

func SpawnAgent(backend TaskBackend) Tool {
	return Func(taskDefinition("spawn_agent", "Start an asynchronous delegated task after delegation is authorized. task_name must match ^[a-z0-9_]{1,100}$, for example repo_survey. Provide a host-supported role and a complete message. Returns a task id and status, not a finished result; use wait_agent with target set to that id."),
		func(ctx context.Context, spec TaskSpec) (Result, error) {
			if err := validateTaskSpec(spec); err != nil {
				return Result{}, err
			}
			return taskResult(backend.Spawn(ctx, spec))
		})
}

func RunTask(backend TaskBackend) Tool {
	definition := taskDefinition("subagent_task", "Start one focused delegated task and wait up to 9 minutes for its result. Same arguments as spawn_agent: task_name uses lowercase letters, digits and underscores only; role and a complete message are required. Does not close the child. If waiting times out, use list_agents to find the existing child and wait_agent; do not spawn the same task again.")
	definition.Timeout = 10 * time.Minute
	return Func(definition,
		func(ctx context.Context, spec TaskSpec) (Result, error) {
			if err := validateTaskSpec(spec); err != nil {
				return Result{}, err
			}
			info, err := backend.Spawn(ctx, spec)
			if err != nil {
				return Result{}, err
			}
			return taskResult(backend.Wait(ctx, info.ID, 9*time.Minute))
		})
}
