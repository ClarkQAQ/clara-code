package coding

import (
	"context"
	"slices"
	"strings"
	"sync"
	"time"

	"clara-code/pkg/core/prompt"
	modelapi "clara-code/pkg/model"
	"clara-code/pkg/tool"
	"clara-code/pkg/util/errs"
)

func taskTools(backend tool.TaskBackend) []modelapi.AgentTool {
	return []modelapi.AgentTool{tool.SpawnAgent(backend), tool.RunTask(backend), tool.ListAgents(backend),
		tool.SendMessage(backend), tool.FollowupTask(backend), tool.WaitAgent(backend),
		tool.InterruptAgent(backend), tool.CloseAgent(backend)}
}

func (c *Conversation) buildTask(ctx context.Context, spec tool.TaskSpec, scope *tool.TaskScope) (tool.TaskSetup, error) {
	meta := c.Store.Meta()
	name, effort := meta.Model, meta.Effort
	if spec.Model != "" {
		name, effort = spec.Model, string(c.CurrentConfig().Models[spec.Model].Effort)
	}
	if spec.Reasoning != "" {
		effort = spec.Reasoning
	}
	role := strings.ToLower(spec.Role)
	if (role == "explorer" || role == "reviewer") && spec.Reasoning == "" {
		effort = c.ModeEffort(prompt.Plan)
	}
	model, err := c.currentModel(ctx, name, effort)
	if err != nil {
		return tool.TaskSetup{}, err
	}
	files := c.Workspace.files()
	items := []modelapi.AgentTool{files.List(), files.Stat(), files.Instructions(), files.Read(), files.Glob(), files.Grep()}
	mode := prompt.Execute
	switch role {
	case "explorer", "reviewer":
		mode = prompt.Plan
	case "", "general":
		items = append(files.All(), c.command(c.commands.ExecCommand(), "task:"+spec.Name), c.command(c.commands.WriteStdin(), "task:"+spec.Name))
		items = append(items, taskTools(scope)...)
	default:
		return tool.TaskSetup{}, errs.New(errs.CodeInvalidArgument, "use general, explorer, reviewer or a configured external ACP role")
	}
	items = hostPathTools(items)
	assignment := prompt.Fragment{Kind: "task_assignment", Source: "host task manager", Scope: spec.Name, Lifecycle: "session", Role: modelapi.MessageRoleUser, Text: jsonText(struct {
		Name, Role    string
		ForkedHistory bool
	}{spec.Name, role, spec.Fork})}
	result := tool.TaskSetup{MaxRetries: c.CurrentConfig().Connection().RequestMaxRetries, Model: model, Tools: items, PrepareStep: c.prepare(mode, role, model, items, nil, false, preparation{assignment: &assignment})}
	result.DynamicToolNames = []string{"image_view", "read_skill", "read_skill_resource"}
	if mode == prompt.Plan {
		result.Authorize = tool.PlanOnly
	}
	return result, nil
}

type taskRouter struct {
	mu       sync.Mutex
	local    tool.TaskBackend
	external *tool.ACPTaskBackend
}

func (r *taskRouter) Spawn(ctx context.Context, spec tool.TaskSpec) (tool.TaskInfo, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	tasks, err := r.List(ctx)
	if err != nil {
		return tool.TaskInfo{}, err
	}
	for _, task := range tasks {
		if task.Name == spec.Name && task.Status != "closed" {
			return tool.TaskInfo{}, errs.New(errs.CodeConflict, "task_name is already in use; reuse its existing id with followup_task, or choose a different task_name")
		}
	}
	if _, ok := r.external.Agents[spec.Role]; ok {
		return r.external.Spawn(ctx, spec)
	}
	return r.local.Spawn(ctx, spec)
}

func (r *taskRouter) backend(ctx context.Context, target string) tool.TaskBackend {
	if strings.HasPrefix(target, "acp_") {
		return r.external
	}
	tasks, _ := r.external.List(ctx)
	for _, task := range tasks {
		if task.Name == target {
			return r.external
		}
	}
	return r.local
}

func (r *taskRouter) Send(ctx context.Context, target, text string, follow bool) error {
	return r.backend(ctx, target).Send(ctx, target, text, follow)
}

func (r *taskRouter) Wait(ctx context.Context, target string, timeout time.Duration) (tool.TaskInfo, error) {
	return r.backend(ctx, target).Wait(ctx, target, timeout)
}

func (r *taskRouter) Interrupt(ctx context.Context, target string) error {
	return r.backend(ctx, target).Interrupt(ctx, target)
}

func (r *taskRouter) CloseTask(ctx context.Context, target string) error {
	return r.backend(ctx, target).CloseTask(ctx, target)
}

func (r *taskRouter) List(ctx context.Context) ([]tool.TaskInfo, error) {
	local, err := r.local.List(ctx)
	if err != nil {
		return nil, err
	}
	external, err := r.external.List(ctx)
	return append(slices.Clone(local), external...), err
}
