package prompt_test

import (
	"context"
	"strings"
	"testing"

	"clara-code/pkg/core"
	"clara-code/pkg/core/prompt"
	"clara-code/pkg/model"
)

func TestSystemSelectsOnlyAvailableCapabilities(t *testing.T) {
	const ask = "request_user_input"
	const instructions = "fs_instructions"
	const skill = "read_skill"
	const stdin = "write_stdin"
	const checklist = "update_plan"
	const goal = "update_goal"
	items := []core.AgentTool{
		namedTool(ask), namedTool(instructions), namedTool(skill), namedTool("exec_command"),
		namedTool(stdin), namedTool(checklist), namedTool(goal), namedTool("spawn_agent"),
		namedTool("read"), namedTool("edit"),
	}
	minimal := prompt.System(prompt.Config{Mode: prompt.Plan})
	for _, name := range []string{ask, instructions, skill, "exec_command", stdin, checklist, goal} {
		if strings.Contains(minimal, name) {
			t.Errorf("minimal prompt references %q", name)
		}
	}
	plan := prompt.System(prompt.Config{Mode: prompt.Plan, Tools: items})
	for _, name := range []string{ask, instructions, skill, "exec_command", stdin, "# Command workflow"} {
		if !strings.Contains(plan, name) {
			t.Errorf("Plan Mode omitted %q", name)
		}
	}
	for _, text := range []string{checklist, goal, "# Anchored file edits", "# Delegation"} {
		if strings.Contains(plan, text) {
			t.Errorf("Plan Mode enables %q", text)
		}
	}
	execute := prompt.System(prompt.Config{Mode: prompt.Execute, Tools: items, Overrides: map[string]string{"coding": "Custom workflow."}})
	for _, text := range []string{stdin, checklist, goal, "# Anchored file edits", "# Delegation", "Custom workflow."} {
		if !strings.Contains(execute, text) {
			t.Errorf("execution prompt omitted %q", text)
		}
	}
	if strings.Contains(execute, "# Coding workflow") {
		t.Error("override did not replace the default fragment")
	}
}

func TestPrepareUsesFirstDeveloperAndPreservesCallerMessages(t *testing.T) {
	ctx := t.Context()
	var request model.Call
	m := summaryModel{input: &request, response: &model.Response{Content: model.ResponseContent{model.TextContent{Text: "done"}}, FinishReason: model.FinishReasonStop}}
	const defaultSystem = "default system instructions"
	engine := core.NewAgent(m, core.WithSystemPrompt(defaultSystem))
	prepare := prompt.Prepare(prompt.Config{Mode: prompt.Plan, Overrides: map[string]string{"base": "SDK base"}}, func(context.Context) (prompt.Context, error) {
		return prompt.Context{Workspace: "/work", Rules: "account rules"}, nil
	})
	history := []model.Message{model.NewSystemMessage("retained caller system"), submission("first task"), model.NewDeveloperMessage("retained caller developer"), submission("next task")}
	_, err := engine.Generate(ctx, core.AgentCall{Messages: history, PrepareStep: prepare})
	if err != nil {
		t.Fatal(err)
	}
	first := request.Prompt[0]
	if first.Role != model.MessageRoleDeveloper || len(first.Content) != 1 || !strings.HasPrefix(windowText(request.Prompt[:1]), "SDK base\n\n") || !strings.Contains(windowText(request.Prompt[:1]), "# Plan Mode") {
		t.Fatal("SDK base and plan policy did not share the leading developer message")
	}
	if request.Prompt[1].Role != model.MessageRoleUser || !strings.Contains(windowText(request.Prompt[1:2]), "account rules") {
		t.Fatal("account rules lost their user role")
	}
	if text := windowText(request.Prompt); strings.Count(text, defaultSystem) != 1 || !strings.HasSuffix(text, windowText(history)) || windowText(history) != "retained caller system\nfirst task\nretained caller developer\nnext task\n" {
		t.Fatal("prepared request modified the configured system or caller history")
	}
}

func namedTool(name string) core.AgentTool {
	return model.NewAgentTool(name, "", func(context.Context, struct{}, model.ToolCall) (model.ToolResponse, error) {
		return model.NewTextResponse(""), nil
	})
}
