// Package host assembles a Coding Agent from the public Clara Core library.
package host

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"clara-code/pkg/core"
	"clara-code/pkg/core/prompt"
	"clara-code/pkg/models"
	"clara-code/pkg/tools"
	"clara-code/pkg/tools/fs"
	"clara-code/pkg/tools/fs/image"
)

type Config struct {
	Model        models.LanguageModel
	SelectModel  func(context.Context, string) (models.LanguageModel, error)
	FS           fs.FS
	Runner       tools.Runner
	Workspace    string
	Store        core.Store
	Ask          tools.AskUser
	Authorize    core.ToolAuthorizer
	ExtraTools   []core.AgentTool
	DynamicTools func() []core.AgentTool
	Compact      prompt.Compactor
}

type Conversation struct {
	Session  *core.Session
	Goal     *tools.Goal
	config   Config
	storage  fs.FS
	items    []core.AgentTool
	plan     *tools.MemoryPlan
	tasks    *tools.TaskScope
	commands *tools.Commands
	cancel   context.CancelFunc
}

func Open(ctx context.Context, config Config, id string) (*Conversation, error) {
	if config.Model == nil || config.FS == nil {
		return nil, errors.New("host needs a model and filesystem")
	}
	lifetime, cancel := context.WithCancel(context.WithoutCancel(ctx))
	host := &Conversation{config: config, storage: config.FS, Goal: &tools.Goal{}, plan: &tools.MemoryPlan{}, cancel: cancel}
	files := tools.NewFiles(config.FS, nil)
	host.items = append(files.All(), tools.NewImages(config.FS, image.Options{}).View(),
		tools.NewDocuments(config.FS).Read(), tools.NewDocuments(config.FS).Create(),
		tools.UpdatePlan(host.plan), tools.GetPlan(host.plan), host.Goal.Get(), host.Goal.Update())
	if config.Runner != nil {
		host.commands = tools.NewCommands(config.Runner)
		host.commands.Context = lifetime
		host.items = append(host.items, host.commands.ExecCommand(), host.commands.WriteStdin())
	}
	if config.Ask != nil {
		host.items = append(host.items, tools.RequestUserInput(config.Ask))
	}
	host.items = append(host.items, config.ExtraTools...)
	manager := tools.NewTaskManager(host.buildTask, 4, 3)
	taskTools := func(scope *tools.TaskScope) []core.AgentTool {
		return []core.AgentTool{tools.SpawnAgent(scope), tools.RunTask(scope), tools.ListAgents(scope), tools.SendMessage(scope),
			tools.FollowupTask(scope), tools.WaitAgent(scope), tools.InterruptAgent(scope), tools.CloseAgent(scope)}
	}
	allowed := append([]core.AgentTool{}, host.items...)
	allowed = append(allowed, taskTools(&tools.TaskScope{})...)
	names := make([]string, 0, len(allowed))
	for _, item := range allowed {
		names = append(names, item.Info().Name)
	}
	host.tasks = manager.Scope(lifetime, "", tools.ScopeOptions{
		AllowedTools: names, Authorize: config.Authorize,
		History: func() []models.Message { return host.Session.Messages() },
		Publish: func(ctx context.Context, info tools.TaskInfo) error {
			return host.Session.Emit(ctx, "task.updated", info)
		},
	})
	host.items = append(host.items, taskTools(host.tasks)...)
	engine := core.NewAgent(config.Model, core.WithTools(host.items...))
	var err error
	if id == "" {
		host.Session, err = core.NewSession(ctx, engine, config.Store)
	} else {
		host.Session, err = core.OpenSession(ctx, engine, config.Store, id)
	}
	if err != nil {
		cancel()
		return nil, err
	}
	return host, nil
}

func (h *Conversation) Call(mode prompt.Mode) core.AgentStreamCall {
	call := core.AgentStreamCall{AuthorizeTool: h.config.Authorize}
	if mode == prompt.Plan {
		call.AuthorizeTool = func(ctx context.Context, info models.ToolInfo, toolCall models.ToolCall) error {
			if err := tools.PlanOnly(ctx, info, toolCall); err != nil {
				return err
			}
			if h.config.Authorize != nil {
				return h.config.Authorize(ctx, info, toolCall)
			}
			return nil
		}
	} else {
		call.FollowUp = h.Goal.NextTurn
		call.OnStreamFinish = func(usage models.Usage, _ models.FinishReason, _ models.ProviderMetadata) error {
			h.Goal.Charge(usage)
			return nil
		}
	}
	call.PrepareStep = h.prepare(mode, "", h.items, h.plan, h.Goal, h.tasks)
	call.OnToolResult = func(item models.ToolResultContent) error {
		switch item.ToolName {
		case "update_plan":
			state, err := h.plan.GetPlan(context.Background())
			if err != nil {
				return err
			}
			return h.Session.Emit(context.Background(), "plan.updated", state)
		case "update_goal":
			return h.Session.Emit(context.Background(), "goal.updated", h.Goal.State())
		default:
			return nil
		}
	}
	return call
}

func (h *Conversation) Start(ctx context.Context, mode prompt.Mode, text string) (*core.Run, error) {
	call := h.Call(mode)
	call.Prompt = text
	return h.Session.Start(ctx, call)
}

func (h *Conversation) Close(ctx context.Context) error {
	h.cancel()
	err := errors.Join(h.Session.Close(ctx), h.tasks.Close(ctx))
	if h.commands != nil {
		err = errors.Join(err, h.commands.Close())
	}
	return err
}

func (h *Conversation) prepare(mode prompt.Mode, role string, registered []core.AgentTool, plan *tools.MemoryPlan, goal *tools.Goal, scope *tools.TaskScope) core.PrepareStepFunction {
	return func(ctx context.Context, step core.PrepareStepFunctionOptions) (context.Context, core.PrepareStepResult, error) {
		items := append([]core.AgentTool{}, registered...)
		if role == "" && h.config.DynamicTools != nil {
			items = append(items, h.config.DynamicTools()...)
		}
		if mode == prompt.Plan {
			items = tools.PlanTools(items)
		}
		config := prompt.Config{Mode: mode, Tools: items, Role: role}
		system := prompt.System(config)
		state := prompt.Context{Workspace: h.config.Workspace}
		rules, err := fs.Instructions(ctx, h.storage, "/")
		if err != nil {
			return ctx, core.PrepareStepResult{}, err
		}
		state.Rules = rules
		if plan != nil {
			checklist, err := plan.GetPlan(ctx)
			if err != nil {
				return ctx, core.PrepareStepResult{}, err
			}
			if len(checklist.Steps) > 0 {
				state.Plan = asJSON(checklist)
			}
		}
		if goal != nil && goal.State().Status != "" {
			state.Goal = asJSON(goal.State())
		}
		if scope != nil {
			tasks, err := scope.List(ctx)
			if err != nil {
				return ctx, core.PrepareStepResult{}, err
			}
			for i := range tasks {
				tasks[i].Result = ""
			}
			if len(tasks) > 0 {
				state.Tasks = asJSON(tasks)
			}
		}
		dynamic := state.Message()
		compactor := h.config.Compact
		if compactor.BudgetBytes > 0 {
			if size := len(asJSON(dynamic)); size >= compactor.BudgetBytes {
				return ctx, core.PrepareStepResult{}, errors.New("workspace state exceeds prompt budget")
			} else {
				compactor.BudgetBytes -= size
			}
		}
		history, err := compactor.Fit(ctx, step.Model, system, step.Messages, items)
		replacement := history
		if history == nil {
			history = step.Messages
		}
		return ctx, core.PrepareStepResult{System: &system, Tools: items, History: replacement, Messages: append(history, dynamic)}, err
	}
}

func (h *Conversation) buildTask(ctx context.Context, spec tools.TaskSpec, scope *tools.TaskScope) (tools.TaskSetup, error) {
	model := h.config.Model
	if spec.Model != "" {
		if h.config.SelectModel == nil {
			return tools.TaskSetup{}, errors.New("host did not configure model selection")
		}
		var err error
		model, err = h.config.SelectModel(ctx, spec.Model)
		if err != nil {
			return tools.TaskSetup{}, err
		}
	}
	files := tools.NewFiles(h.storage, nil)
	plan := &tools.MemoryPlan{}
	items := []core.AgentTool{files.List(), files.Read(), files.Stat(), files.Grep(), files.Glob(), files.Instructions()}
	role := strings.ToLower(spec.Role)
	switch role {
	case "explorer", "reviewer":
	case "image":
		items = append(items, tools.NewImages(h.storage, image.Options{}).View())
	case "search":
		for _, item := range h.config.ExtraTools {
			if name := item.Info().Name; name == "web_search" || name == "web_fetch" {
				items = append(items, item)
			}
		}
	case "general":
		items = append(files.All(), tools.GetPlan(plan), tools.UpdatePlan(plan), tools.SpawnAgent(scope), tools.RunTask(scope),
			tools.SendMessage(scope), tools.FollowupTask(scope), tools.WaitAgent(scope), tools.InterruptAgent(scope), tools.CloseAgent(scope), tools.ListAgents(scope))
	default:
		return tools.TaskSetup{}, errors.New("unknown role. Use general, explorer, reviewer, search or image")
	}
	return tools.TaskSetup{Model: model, Tools: items, PrepareStep: h.prepare(prompt.Execute, role, items, plan, nil, scope)}, nil
}

func asJSON(value any) string {
	body, err := json.Marshal(value)
	if err != nil {
		return ""
	}
	return string(body)
}
