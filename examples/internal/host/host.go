// Package host assembles a Coding Agent from the public Clara Core library.
package host

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"clara-code/pkg/core"
	"clara-code/pkg/core/prompt"
	modelapi "clara-code/pkg/model"
	"clara-code/pkg/tool"
	"clara-code/pkg/tool/fs"
	"clara-code/pkg/tool/fs/image"
)

type Config struct {
	Model        modelapi.LanguageModel
	SelectModel  func(context.Context, string) (modelapi.LanguageModel, error)
	FS           fs.FS
	Runner       tool.Runner
	Workspace    string
	Store        core.Store
	Ask          tool.AskUser
	Authorize    core.ToolAuthorizer
	ExtraTools   []core.AgentTool
	DynamicTools func() []core.AgentTool
	Compact      prompt.Compactor
}

type Conversation struct {
	Session  *core.Session
	Goal     *tool.Goal
	config   Config
	storage  fs.FS
	items    []core.AgentTool
	plan     *tool.MemoryPlan
	tasks    *tool.TaskScope
	commands *tool.Commands
	cancel   context.CancelFunc
}

func Open(ctx context.Context, config Config, id string) (*Conversation, error) {
	if config.Model == nil || config.FS == nil {
		return nil, errors.New("host needs a model and filesystem")
	}
	lifetime, cancel := context.WithCancel(context.WithoutCancel(ctx))
	host := &Conversation{config: config, storage: config.FS, Goal: &tool.Goal{}, plan: &tool.MemoryPlan{}, cancel: cancel}
	files := tool.NewFiles(config.FS, nil)
	host.items = append(files.All(), tool.NewImages(config.FS, image.Options{}).View(),
		tool.NewDocuments(config.FS).Read(), tool.NewDocuments(config.FS).Create(),
		tool.UpdatePlan(host.plan), tool.GetPlan(host.plan), host.Goal.Get(), host.Goal.Update())
	if config.Runner != nil {
		host.commands = tool.NewCommands(config.Runner)
		host.commands.Context = lifetime
		host.items = append(host.items, host.commands.ExecCommand(), host.commands.WriteStdin())
	}
	if config.Ask != nil {
		host.items = append(host.items, tool.RequestUserInput(config.Ask))
	}
	host.items = append(host.items, config.ExtraTools...)
	manager := tool.NewTaskManager(host.buildTask, 4, 3)
	taskTools := func(scope *tool.TaskScope) []core.AgentTool {
		return []core.AgentTool{tool.SpawnAgent(scope), tool.RunTask(scope), tool.ListAgents(scope), tool.SendMessage(scope),
			tool.FollowupTask(scope), tool.WaitAgent(scope), tool.InterruptAgent(scope), tool.CloseAgent(scope)}
	}
	allowed := append([]core.AgentTool{}, host.items...)
	allowed = append(allowed, taskTools(&tool.TaskScope{})...)
	names := make([]string, 0, len(allowed))
	for _, item := range allowed {
		names = append(names, item.Info().Name)
	}
	host.tasks = manager.Scope(lifetime, "", tool.ScopeOptions{
		AllowedTools: names, Authorize: config.Authorize,
		History: func() []modelapi.Message { return host.Session.Messages() },
		Publish: func(ctx context.Context, info tool.TaskInfo) error {
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
		call.AuthorizeTool = func(ctx context.Context, info modelapi.ToolInfo, toolCall modelapi.ToolCall) error {
			if err := tool.PlanOnly(ctx, info, toolCall); err != nil {
				return err
			}
			if h.config.Authorize != nil {
				return h.config.Authorize(ctx, info, toolCall)
			}
			return nil
		}
	} else {
		call.FollowUp = h.Goal.NextTurn
		call.OnStreamFinish = func(usage modelapi.Usage, _ modelapi.FinishReason, _ modelapi.ProviderMetadata) error {
			h.Goal.Charge(usage)
			return nil
		}
	}
	call.PrepareStep = h.prepare(mode, "", h.items, h.plan, h.Goal, h.tasks)
	call.OnToolResult = func(item modelapi.ToolResultContent) error {
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

func (h *Conversation) prepare(mode prompt.Mode, role string, registered []core.AgentTool, plan *tool.MemoryPlan, goal *tool.Goal, scope *tool.TaskScope) core.PrepareStepFunction {
	return func(ctx context.Context, step core.PrepareStepFunctionOptions) (context.Context, core.PrepareStepResult, error) {
		items := append([]core.AgentTool{}, registered...)
		if role == "" && h.config.DynamicTools != nil {
			items = append(items, h.config.DynamicTools()...)
		}
		if mode == prompt.Plan {
			items = tool.PlanTools(items)
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

func (h *Conversation) buildTask(ctx context.Context, spec tool.TaskSpec, scope *tool.TaskScope) (tool.TaskSetup, error) {
	model := h.config.Model
	if spec.Model != "" {
		if h.config.SelectModel == nil {
			return tool.TaskSetup{}, errors.New("host did not configure model selection")
		}
		var err error
		model, err = h.config.SelectModel(ctx, spec.Model)
		if err != nil {
			return tool.TaskSetup{}, err
		}
	}
	files := tool.NewFiles(h.storage, nil)
	plan := &tool.MemoryPlan{}
	items := []core.AgentTool{files.List(), files.Read(), files.Stat(), files.Grep(), files.Glob(), files.Instructions()}
	role := strings.ToLower(spec.Role)
	switch role {
	case "explorer", "reviewer":
	case "image":
		items = append(items, tool.NewImages(h.storage, image.Options{}).View())
	case "search":
		for _, item := range h.config.ExtraTools {
			if name := item.Info().Name; name == "web_search" || name == "web_fetch" {
				items = append(items, item)
			}
		}
	case "general":
		items = append(files.All(), tool.GetPlan(plan), tool.UpdatePlan(plan), tool.SpawnAgent(scope), tool.RunTask(scope),
			tool.SendMessage(scope), tool.FollowupTask(scope), tool.WaitAgent(scope), tool.InterruptAgent(scope), tool.CloseAgent(scope), tool.ListAgents(scope))
	default:
		return tool.TaskSetup{}, errors.New("unknown role. Use general, explorer, reviewer, search or image")
	}
	return tool.TaskSetup{Model: model, Tools: items, PrepareStep: h.prepare(prompt.Execute, role, items, plan, nil, scope)}, nil
}

func asJSON(value any) string {
	body, err := json.Marshal(value)
	if err != nil {
		return ""
	}
	return string(body)
}
