package prompt_test

import (
	"context"
	"strings"
	"testing"

	"clara-code/pkg/core"
	"clara-code/pkg/core/prompt"
	"clara-code/pkg/models"
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
	for _, name := range []string{ask, instructions, skill, stdin, checklist, goal} {
		if strings.Contains(minimal, name) {
			t.Errorf("minimal prompt references %q", name)
		}
	}
	plan := prompt.System(prompt.Config{Mode: prompt.Plan, Tools: items})
	for _, name := range []string{ask, instructions, skill} {
		if !strings.Contains(plan, name) {
			t.Errorf("Plan Mode omitted %q", name)
		}
	}
	for _, text := range []string{stdin, checklist, goal, "# Anchored file edits", "# Delegation"} {
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

func namedTool(name string) core.AgentTool {
	return models.NewAgentTool(name, "", func(context.Context, struct{}, models.ToolCall) (models.ToolResponse, error) {
		return models.NewTextResponse(""), nil
	})
}
