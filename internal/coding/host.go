package coding

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"clara-code/pkg/core"
	"clara-code/pkg/core/prompt"
	modelapi "clara-code/pkg/model"
	toolapi "clara-code/pkg/tool"
	"clara-code/pkg/tool/mcp"

	"clara-code/internal/session"
	"clara-code/internal/setting"
)

type Host struct {
	Config setting.Config
	Models func(context.Context, string) (modelapi.LanguageModel, error)
	now    func() time.Time
}

type Conversation struct {
	Host      Host
	Workspace Workspace
	Store     *session.Store
	Session   *core.Session
	Goal      *toolapi.Goal
	Questions *Questions
	sessionMu sync.RWMutex

	model             modelapi.LanguageModel
	plan              *toolapi.MemoryPlan
	items             []modelapi.AgentTool
	commands          *toolapi.Commands
	mcp               *mcp.Manager
	tasks             *toolapi.TaskScope
	backend           toolapi.TaskBackend
	external          *toolapi.ACPTaskBackend
	cancel            context.CancelFunc
	activities        *activityLog
	commandSequence   atomic.Uint64
	backgroundUpdates chan struct{}
	monitorDone       chan struct{}
	backgroundMu      sync.Mutex
	backgroundErr     error
}

func (h Host) Open(ctx context.Context, workspace Workspace, store *session.Store, ask *Questions, extra []mcp.ServerConfig) (*Conversation, error) {
	meta := store.Meta()
	reloaded, err := h.Config.Reload(ctx, workspace.Cwd, workspace.Root)
	if err != nil {
		return nil, errors.Join(err, store.Close())
	}
	h.Config = reloaded
	if meta.Provider != "" && len(h.Config.Providers) > 0 {
		h.Config, err = h.Config.SelectProvider(meta.Provider)
		if err != nil {
			return nil, errors.Join(err, store.Close())
		}
	}
	if meta.ID == "" && h.Config.DefaultModel != "" {
		store.SetInitialModel(h.Config.DefaultModel, string(h.Config.Models[h.Config.DefaultModel].Effort))
		meta = store.Meta()
	}
	if meta.ID == "" || meta.Provider == "" {
		planEffort := meta.PlanEffort
		if planEffort == "" {
			planEffort = string(h.Config.PlanEffort)
		}
		store.SetPendingSelection(h.Config.ModelProvider, planEffort)
	}
	lifetime, cancel := context.WithCancel(context.WithoutCancel(ctx))
	c := &Conversation{Host: h, Workspace: workspace, Store: store, Questions: ask,
		Goal: &toolapi.Goal{}, plan: &toolapi.MemoryPlan{}, cancel: cancel}
	cleanup := func(err error) (*Conversation, error) {
		return nil, errors.Join(err, c.Close())
	}
	model, err := h.model(ctx, meta.Model, meta.Effort)
	if err != nil {
		return cleanup(err)
	}
	c.model = model
	c.commands = toolapi.NewCommands(observedRunner{runner{workspace, toolapi.NewLocal()}})
	c.commands.Context = lifetime
	c.commands.NoDefaultTimeout = true
	files := workspace.files()
	c.items = append(hostPathTools(files.All()), c.command(c.commands.ExecCommand(), "main"), c.command(c.commands.WriteStdin(), "main"),
		toolapi.GetPlan(c.plan), toolapi.UpdatePlan(c.plan), c.Goal.Get(), c.Goal.Update())
	if ask != nil {
		c.items = append(c.items, ask.Tool())
	}
	if len(h.Config.MCP)+len(extra) > 0 {
		names := make([]string, 0, len(h.Config.MCP))
		for name := range h.Config.MCP {
			names = append(names, name)
		}
		slices.Sort(names)
		configs := make([]mcp.ServerConfig, 0, len(names)+len(extra))
		for _, name := range names {
			if setting.Enabled(h.Config.MCP[name].Enabled) {
				configs = append(configs, h.Config.MCP[name].Server(name, workspace.Cwd))
			}
		}
		configs = append(configs, extra...)
		c.mcp, err = mcp.Connect(ctx, configs)
		if err != nil {
			return cleanup(err)
		}
	}
	manager := toolapi.NewTaskManager(c.buildTask, 4, 3)
	allowed := make([]string, 0, len(c.items)+8)
	for _, tool := range append(slices.Clone(c.items), taskTools(&toolapi.TaskScope{})...) {
		allowed = append(allowed, tool.Info().Name)
	}
	allowed = append(allowed, "image_view", "read_skill", "read_skill_resource")
	c.tasks = manager.Scope(lifetime, "", toolapi.ScopeOptions{
		AllowedTools: allowed,
		History: func() []modelapi.Message {
			c.sessionMu.RLock()
			defer c.sessionMu.RUnlock()
			return c.Session.Messages()
		},
		Publish: func(ctx context.Context, info toolapi.TaskInfo) error {
			c.sessionMu.RLock()
			defer c.sessionMu.RUnlock()
			return c.Session.Emit(ctx, "task.updated", info)
		},
	})
	var backend toolapi.TaskBackend = c.tasks
	if len(h.Config.ACP) > 0 {
		configs := make(map[string]core.ACPExternalConfig, len(h.Config.ACP))
		executable, err := os.Executable()
		if err != nil {
			return cleanup(err)
		}
		for name, config := range h.Config.ACP {
			if config.Command == "" || name == "" {
				return cleanup(errors.New("external ACP agent needs a name and command"))
			}
			args := append([]string{"__acp_exec", config.Command}, config.Args...)
			configs[name] = core.ACPExternalConfig{Command: executable, Args: args, Cwd: workspace.Cwd, Env: config.Env}
		}
		c.external = toolapi.NewACPTaskBackend(configs, 2)
		backend = &taskRouter{local: c.tasks, external: c.external}
	}
	c.backend = backend
	c.activities = newActivityLog(func() string {
		if active := c.Session.Active(); active != nil {
			return active.ID
		}
		return ""
	}, func(activity session.ToolActivity) error {
		return c.publishActivity(activity)
	})
	c.items = append(c.items, taskTools(backend)...)
	if c.external != nil {
		c.items = append(c.items, toolapi.ListACPSessions(c.external))
	}
	engine := core.NewAgent(model)
	if meta.ID == "" {
		c.Session, err = core.NewSession(ctx, engine, store)
	} else {
		c.Session, err = core.OpenSession(ctx, engine, store, meta.ID)
	}
	if err != nil {
		return cleanup(err)
	}
	if err := store.InterruptCommands(); err != nil {
		return cleanup(err)
	}
	if err := store.MarkHostPaths(); err != nil {
		return cleanup(err)
	}
	c.backgroundUpdates = make(chan struct{}, 1)
	c.monitorDone = make(chan struct{})
	c.commands.OnChange = c.onTerminal
	go c.watchTerminals(lifetime)
	return c, nil
}

func (h Host) model(ctx context.Context, name, effort string) (modelapi.LanguageModel, error) {
	profile, ok := h.Config.Models[name]
	if !ok {
		return nil, fmt.Errorf("model %q is not configured", name)
	}
	level := modelapi.ReasoningEffort(effort)
	if !setting.ValidEffort(level) {
		return nil, errors.New("invalid reasoning effort")
	}
	if !profile.Allows(level) {
		return nil, errors.New("reasoning effort is not supported by this model")
	}
	factory := h.Models
	if factory == nil {
		var err error
		factory, err = h.Config.Factory()
		if err != nil {
			return nil, err
		}
	}
	model, err := factory(ctx, profile.ID)
	if err != nil {
		return nil, err
	}
	model = diagnosticModel{LanguageModel: model, redact: h.Config.Redact}
	if level != "" {
		model = effortModel{LanguageModel: model, effort: level}
	}
	return capabilityModel{LanguageModel: model, profile: profile}, nil
}

type effortModel struct {
	modelapi.LanguageModel
	effort modelapi.ReasoningEffort
}

func (m effortModel) Generate(ctx context.Context, call modelapi.Call) (*modelapi.Response, error) {
	if call.ProviderOptions.ReasoningEffort == nil {
		call.ProviderOptions.ReasoningEffort = &m.effort
	}
	return m.LanguageModel.Generate(ctx, call)
}

func (m effortModel) Stream(ctx context.Context, call modelapi.Call) (modelapi.StreamResponse, error) {
	if call.ProviderOptions.ReasoningEffort == nil {
		call.ProviderOptions.ReasoningEffort = &m.effort
	}
	return m.LanguageModel.Stream(ctx, call)
}

func (c *Conversation) ChangeModel(ctx context.Context, name, effort string) error {
	return c.ChangeSelection(ctx, c.Host.Config.ModelProvider, name, effort, prompt.Execute)
}

func (c *Conversation) Call(mode prompt.Mode, review bool) core.AgentStreamCall {
	role := ""
	if review {
		mode, role = prompt.Plan, "reviewer"
	}
	call := core.AgentStreamCall{PrepareStep: c.prepare(mode, role, c.model, c.items, c.plan, true)}
	retries := 3
	if n := c.CurrentConfig().Connection().RequestMaxRetries; n != nil {
		retries = *n
	}
	call.MaxRetries = new(retries)
	effort := modelapi.ReasoningEffort(c.ModeEffort(mode))
	if effort != "" {
		call.ProviderOptions.ReasoningEffort = &effort
	}
	attempt := 0
	responseStarted := false
	call.OnStepStart = func(step int) error {
		attempt = 0
		responseStarted = false
		return c.Session.Emit(context.Background(), "model.started", map[string]int{"step": step})
	}
	call.OnChunk = func(part modelapi.StreamPart) error {
		if !responseStarted && part.Type != modelapi.StreamPartTypeWarnings {
			responseStarted = true
			if err := c.Session.Emit(context.Background(), "model.responding", nil); err != nil {
				return err
			}
		}
		switch part.Type {
		case modelapi.StreamPartTypeTextStart, modelapi.StreamPartTypeTextEnd, modelapi.StreamPartTypeReasoningStart, modelapi.StreamPartTypeReasoningEnd:
			return c.Session.Emit(context.Background(), string(part.Type), part)
		}
		return nil
	}
	call.OnRetry = func(err *core.ProviderError, delay time.Duration) {
		attempt++
		failure := session.Failure{Kind: "connection", Message: "Connection interrupted"}
		if err != nil {
			failure = session.DescribeFailure(err)
		}
		failure.Message = c.Host.Config.Redact(failure.Message)
		_ = c.Session.Emit(context.Background(), "model.retry", session.RetryStatus{Attempt: attempt, Limit: retries, DelayMS: delay.Milliseconds(), Kind: failure.Kind, Message: failure.Message})
	}
	call.OnToolCall = func(call modelapi.ToolCallContent) error {
		if call.ToolName == "exec_command" || call.ToolName == "write_stdin" {
			return nil
		}
		return c.activities.start(call)
	}
	call.OnError = func(err error) {
		c.activities.finish(err)
		failure := session.DescribeFailure(err)
		if responseStarted && failure.Kind == "connection" {
			failure.Kind = "stream"
		}
		failure.Message = c.Host.Config.Redact(failure.Message)
		_ = c.Session.Emit(context.Background(), "model.error", failure)
	}
	call.OnAgentFinish = func(*core.AgentResult) error {
		c.activities.finish(nil)
		return nil
	}
	call.OnHistory = func([]modelapi.Message) error {
		if err := c.Store.InvalidateContext(); err != nil {
			return err
		}
		return c.Session.Emit(context.Background(), "context.compacted", nil)
	}
	if mode == prompt.Plan {
		call.AuthorizeTool = toolapi.PlanOnly
	} else {
		call.FollowUp = c.Goal.NextTurn
		call.OnStreamFinish = func(usage modelapi.Usage, _ modelapi.FinishReason, _ modelapi.ProviderMetadata) error {
			c.Goal.Charge(usage)
			return nil
		}
	}
	call.OnToolResult = func(result modelapi.ToolResultContent) error {
		if result.ToolName != "exec_command" && result.ToolName != "write_stdin" {
			if err := c.activities.complete(result); err != nil {
				return err
			}
		}
		switch result.ToolName {
		case "update_plan":
			plan, err := c.plan.GetPlan(context.Background())
			if err != nil {
				return err
			}
			return c.Session.Emit(context.Background(), "plan.updated", plan)
		case "update_goal":
			return c.Session.Emit(context.Background(), "goal.updated", c.Goal.State())
		default:
			return nil
		}
	}
	return call
}

func (c *Conversation) Start(ctx context.Context, mode prompt.Mode, text string, review bool) (*core.Run, error) {
	return c.StartWithFiles(ctx, mode, text, review, nil)
}
func (c *Conversation) StartWithFiles(ctx context.Context, mode prompt.Mode, text string, review bool, files []modelapi.FilePart) (*core.Run, error) {
	files, err := c.prepareFiles(ctx, files)
	if err != nil {
		return nil, err
	}
	if session.MediaBytes(c.Session.Messages()) > 16<<20 {
		if err := c.Compact(ctx, mode); err != nil {
			return nil, err
		}
	}
	if review && !c.Workspace.Git {
		return nil, errors.New("/review requires a Git working tree")
	}
	call := c.Call(mode, review)
	call.Prompt = text
	call.Files = files
	return c.Session.Start(ctx, call)
}

type preparation struct {
	compact    bool
	assignment *prompt.Fragment
}

func (c *Conversation) prepare(mode prompt.Mode, role string, model modelapi.LanguageModel, registered []modelapi.AgentTool, plan *toolapi.MemoryPlan, foreground bool, options ...preparation) core.PrepareStepFunction {
	var previousEstimate int64
	return func(ctx context.Context, step core.PrepareStepFunctionOptions) (context.Context, core.PrepareStepResult, error) {
		rules, err := c.Workspace.instructionFragments(ctx, c.Host.Config.Home)
		if err != nil {
			return ctx, core.PrepareStepResult{}, err
		}
		skills, skillTools, err := c.Workspace.skills(ctx, c.Host.Config.Home)
		if err != nil {
			return ctx, core.PrepareStepResult{}, err
		}
		profile := profileOf(model)
		items := slices.Clone(registered)
		if profile.Images() {
			items = append(items, c.imageTool(profile))
		}
		if skillTools != nil {
			items = append(items, skillTools...)
		}
		if foreground && mode == prompt.Execute {
			items = append(items, c.mcp.Tools()...)
		}
		items = c.configureTools(items, profile)
		overrides := map[string]string{}
		overrides["tasks"] = prompt.Default("tasks") + "\n\nClara built-in roles: general can implement changes and run commands; explorer and reviewer are read-only and cannot run Shell commands. Configured external ACP roles are listed separately when available. Do not invent roles such as worker or planner. A minimal read-only delegation is spawn_agent {\"task_name\":\"repo_survey\",\"role\":\"explorer\",\"message\":\"Inspect go.mod and the top-level directories using file tools. Do not modify files. Report the module name and directory responsibilities with supporting paths.\"}. After it returns an id, wait with wait_agent {\"target\":\"<returned id>\",\"timeout_ms\":10000}. Use the returned id, not the placeholder."
		overrides["workspace_rules"] = prompt.Default("workspace_rules") + "\nClara Code already injects global instructions and current-directory project rules. fs_instructions queries project files only; no project instructions does not mean no global instructions. For new files, query the destination path before writing."
		if role == "reviewer" && foreground {
			items = append(items, c.Workspace.reviewTool())
			overrides["review"] = prompt.Default("review") + "\nCall review_changes to inspect staged, unstaged and untracked changes. " +
				"Read untracked text with the read tool. Report actionable findings with file and line locations, or state that none were found."
		}
		if mode == prompt.Plan {
			items = toolapi.PlanTools(items)
		}
		config := prompt.Config{Mode: mode, Role: role, Tools: items, Overrides: overrides, Parameters: c.promptParameters()}
		system, _ := prompt.Instructions(config)
		fragments := append(c.hostPolicies(mode, role, foreground), prompt.Policy(config)...)
		fragments = append(fragments, rules...)
		fragments = append(fragments, c.environmentFragments(mode)...)
		state := prompt.Context{}
		if plan != nil {
			list, err := plan.GetPlan(ctx)
			if err != nil {
				return ctx, core.PrepareStepResult{}, err
			}
			if len(list.Steps) > 0 {
				state.Plan = jsonText(list)
			}
		}
		if foreground && mode == prompt.Execute {
			if goal := c.Goal.State(); goal.Status != "" {
				state.Goal = jsonText(goal)
			}
			if tasks, err := c.backend.List(ctx); err == nil && len(tasks) > 0 {
				for i := range tasks {
					tasks[i].Result = ""
				}
				state.Tasks = jsonText(tasks)
			}
		}
		for _, item := range []struct{ kind, text string }{{"plan", state.Plan}, {"goal", state.Goal}, {"tasks", state.Tasks}} {
			fragments = append(fragments, prompt.Fragment{Kind: item.kind, Source: "host", Lifecycle: "step", Role: modelapi.MessageRoleUser, Text: item.text})
		}
		if terminals := c.terminalSummaries(); len(terminals) > 0 {
			fragments = append(fragments, prompt.Fragment{Kind: "background_terminals", Source: "current process", Lifecycle: "step", Role: modelapi.MessageRoleUser, Text: jsonText(terminals)})
		}
		if c.Store.Meta().LegacyPathContext {
			fragments = append(fragments, prompt.Fragment{Kind: "path_migration", Source: "host", Lifecycle: "session", Role: modelapi.MessageRoleUser, Text: "Older transcript entries used /foo to mean a file under " + c.Workspace.Root + ". That old virtual mapping no longer applies. Re-resolve the target and read it again before editing; old snapshots are invalid. New calls use native host paths."})
		}
		if len(skills) > 0 {
			var catalog strings.Builder
			for _, skill := range skills {
				fmt.Fprintf(&catalog, "- %s: %s (source: %s)\n", skill.Name, skill.Description, skill.Source)
			}
			fragments = append(fragments, prompt.Fragment{Kind: "skills", Source: "skill discovery", Scope: c.Workspace.Cwd, Lifecycle: "step", Role: modelapi.MessageRoleUser, Text: catalog.String()})
		}
		if foreground && mode == prompt.Execute && len(c.Host.Config.ACP) > 0 {
			names := make([]string, 0, len(c.Host.Config.ACP))
			for name := range c.Host.Config.ACP {
				names = append(names, name)
			}
			slices.Sort(names)
			fragments = append(fragments, prompt.Fragment{Kind: "external_agents", Source: "host configuration", Lifecycle: "step", Role: modelapi.MessageRoleUser, Text: jsonText(names)})
		}
		history := step.Messages
		if len(options) > 0 && options[0].assignment != nil {
			fragments = append(fragments, *options[0].assignment)
		}
		compactor := c.compactor(profile)
		compactor.Force = len(options) > 0 && options[0].compact
		window, err := prompt.LoadWindow(step.ContextState)
		if err != nil {
			return ctx, core.PrepareStepResult{}, err
		}
		window = migrateEnvironmentWindow(window)
		current := fragments
		if compactor.Force && window.Matches(history) {
			current = window.Snapshot
		}
		request, nextWindow, err := window.Prepare(history, current)
		if err != nil {
			return ctx, core.PrepareStepResult{}, err
		}
		compactor.Transcript = nextWindow.Prefix
		compactor.Assemble = func(messages []modelapi.Message) ([]modelapi.Message, error) {
			if slices.EqualFunc(messages, history, func(a, b modelapi.Message) bool { return jsonText(a) == jsonText(b) }) {
				return request, nil
			}
			fresh, _, err := (prompt.Window{}).Prepare(messages, fragments)
			return fresh, err
		}
		if len(step.Steps) > 0 {
			compactor.UsageTokens = step.Steps[len(step.Steps)-1].Usage.TotalTokens
			compactor.UsageEstimate = previousEstimate
		} else if usage := c.Store.Meta().LastUsage; foreground && !compactor.Force && usage != nil {
			baselineRequest, _, err := window.Prepare(history[:prompt.ActiveTurn(history)], fragments)
			if err != nil {
				return ctx, core.PrepareStepResult{}, err
			}
			baseline, err := prompt.Measure(system, baselineRequest, toolDefinitions(items))
			if err != nil {
				return ctx, core.PrepareStepResult{}, err
			}
			compactor.UsageTokens, compactor.UsageEstimate = usage.TotalTokens, baseline.Tokens
		}
		compacted, err := compactor.Fit(ctx, model, system, history, items)
		if err != nil {
			return ctx, core.PrepareStepResult{}, err
		}
		if compacted != nil {
			history = compacted
			request, nextWindow, err = (prompt.Window{}).Prepare(history, fragments)
			if err != nil {
				return ctx, core.PrepareStepResult{}, err
			}
		}
		estimate, err := prompt.Measure(system, request, toolDefinitions(items))
		if err != nil {
			return ctx, core.PrepareStepResult{}, err
		}
		previousEstimate = estimate.Tokens
		checkpoint, err := nextWindow.Marshal()
		if err != nil {
			return ctx, core.PrepareStepResult{}, err
		}
		return ctx, core.PrepareStepResult{Model: model, System: &system, Tools: items, Messages: request, History: compacted, ContextState: checkpoint}, nil
	}
}

func jsonText(value any) string {
	data, _ := json.Marshal(value)
	return string(data)
}

func (c *Conversation) Skills(ctx context.Context) ([]Skill, error) {
	skills, _, err := c.Workspace.skills(ctx, c.Host.Config.Home)
	return skills, err
}

func (c *Conversation) MCPStatuses() []mcp.Status {
	if c.mcp == nil {
		return nil
	}
	return c.mcp.Statuses()
}

func (c *Conversation) Compact(ctx context.Context, modes ...prompt.Mode) error {
	old := c.Session
	if old.Active() != nil {
		return errors.New("finish or cancel the current turn before compacting")
	}
	history := old.Messages()
	if len(history) == 0 {
		return errors.New("there is no conversation to compact")
	}
	mode := prompt.Execute
	if len(modes) > 0 {
		mode = modes[0]
	}
	_, result, err := c.prepare(mode, "", c.model, c.items, c.plan, true, preparation{compact: true})(ctx, core.PrepareStepFunctionOptions{Messages: history, ContextState: old.ContextState()})
	if err != nil {
		return err
	}
	if result.History == nil {
		return errors.New("there are no complete old turns to compact")
	}
	if err := c.Store.ReplaceHistory(ctx, result.History, result.ContextState); err != nil {
		return err
	}
	next, err := core.OpenSession(context.WithoutCancel(ctx), core.NewAgent(c.model), c.Store, old.ID())
	if err != nil {
		return err
	}
	c.sessionMu.Lock()
	c.Session = next
	c.sessionMu.Unlock()
	return old.Close(context.WithoutCancel(ctx))
}

func (c *Conversation) Close() error {
	c.cancel()
	if c.monitorDone != nil {
		<-c.monitorDone
	}
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
	defer cancel()
	var err error
	if c.Session != nil {
		err = errors.Join(err, c.Session.Close(ctx))
	}
	if c.tasks != nil {
		err = errors.Join(err, c.tasks.Close(ctx))
	}
	if c.external != nil {
		tasks, listErr := c.external.List(ctx)
		err = errors.Join(err, listErr)
		for _, task := range tasks {
			err = errors.Join(err, c.external.CloseTask(ctx, task.ID))
		}
	}
	if c.commands != nil {
		err = errors.Join(err, c.commands.Close())
	}
	return errors.Join(err, c.mcp.Close(), c.Store.Close())
}

func (c *Conversation) ProjectLabel() string {
	return filepath.Base(c.Workspace.Root)
}

func (c *Conversation) QueueInput(ctx context.Context, run *core.Run, text string, follow bool) (bool, error) {
	return c.QueueFiles(ctx, run, text, follow, nil)
}
func (c *Conversation) QueueFiles(ctx context.Context, run *core.Run, text string, follow bool, files []modelapi.FilePart) (bool, error) {
	files, err := c.prepareFiles(ctx, files)
	if err != nil {
		return false, err
	}
	attachments, err := AttachmentsFromFiles(c.Host.Config.Home, files)
	if err != nil {
		return false, err
	}
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if follow {
		err = run.FollowUp(modelapi.NewUserMessage(text, files...))
	} else {
		err = run.Steer(modelapi.NewUserMessage(text, files...))
	}
	if err != nil {
		return false, err
	}
	return true, c.Store.RecordFiles(run.ID, text, attachments)
}
