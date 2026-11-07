package coding

import (
	"context"
	"slices"
	"strings"
	"sync"
	"time"

	"clara-core/agent/errs"
	"clara-core/agent/prompt"
	"clara-core/models"
	"clara-core/tools"
)

func taskTools(backend tools.TaskBackend) []models.AgentTool {
	return []models.AgentTool{tools.SpawnAgent(backend), tools.RunTask(backend), tools.ListAgents(backend),
		tools.SendMessage(backend), tools.FollowupTask(backend), tools.WaitAgent(backend),
		tools.InterruptAgent(backend), tools.CloseAgent(backend)}
}

func (c *Conversation) buildTask(ctx context.Context, spec tools.TaskSpec, scope *tools.TaskScope) (tools.TaskSetup, error) {
	meta := c.Store.Meta()
	name, effort := meta.Model, meta.Effort
	if spec.Model != "" {
		name, effort = spec.Model, string(c.Host.Config.Models[spec.Model].Effort)
	}
	if spec.Reasoning != "" {
		effort = spec.Reasoning
	}
	model, err := c.Host.model(ctx, name, effort)
	if err != nil {
		return tools.TaskSetup{}, err
	}
	role := strings.ToLower(spec.Role)
	files := c.Workspace.files()
	items := []models.AgentTool{files.List(), files.Stat(), files.Instructions(), files.Read(), files.Glob(), files.Grep()}
	mode := prompt.Execute
	switch role {
	case "explorer", "reviewer":
		mode = prompt.Plan
	case "", "general":
		items = append(files.All(), c.command(c.commands.ExecCommand(), "task:"+spec.Name), c.command(c.commands.WriteStdin(), "task:"+spec.Name))
		items = append(items, taskTools(scope)...)
	default:
		return tools.TaskSetup{}, errs.New(errs.CodeInvalidArgument, "use general, explorer, reviewer or a configured external ACP role")
	}
	items = hostPathTools(items)
	result := tools.TaskSetup{Model: model, Tools: items, PrepareStep: c.prepare(mode, role, model, items, nil, false)}
	if mode == prompt.Plan {
		result.Authorize = tools.PlanOnly
	}
	return result, nil
}

type taskRouter struct {
	mu       sync.Mutex
	local    tools.TaskBackend
	external *tools.ACPTaskBackend
}

func (r *taskRouter) Spawn(ctx context.Context, spec tools.TaskSpec) (tools.TaskInfo, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	tasks, err := r.List(ctx)
	if err != nil {
		return tools.TaskInfo{}, err
	}
	for _, task := range tasks {
		if task.Name == spec.Name && task.Status != "closed" {
			return tools.TaskInfo{}, errs.ErrConflict
		}
	}
	if _, ok := r.external.Agents[spec.Role]; ok {
		return r.external.Spawn(ctx, spec)
	}
	return r.local.Spawn(ctx, spec)
}

func (r *taskRouter) backend(ctx context.Context, target string) tools.TaskBackend {
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

func (r *taskRouter) Wait(ctx context.Context, target string, timeout time.Duration) (tools.TaskInfo, error) {
	return r.backend(ctx, target).Wait(ctx, target, timeout)
}

func (r *taskRouter) Interrupt(ctx context.Context, target string) error {
	return r.backend(ctx, target).Interrupt(ctx, target)
}

func (r *taskRouter) CloseTask(ctx context.Context, target string) error {
	return r.backend(ctx, target).CloseTask(ctx, target)
}

func (r *taskRouter) List(ctx context.Context) ([]tools.TaskInfo, error) {
	local, err := r.local.List(ctx)
	if err != nil {
		return nil, err
	}
	external, err := r.external.List(ctx)
	return append(slices.Clone(local), external...), err
}
