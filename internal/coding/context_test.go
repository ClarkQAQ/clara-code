package coding

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"clara-code/internal/session"
	"clara-code/internal/setting"
	"clara-code/pkg/core"
	"clara-code/pkg/core/prompt"
	"clara-code/pkg/model"
	"clara-code/pkg/tool"
)

func contextText(messages []model.Message) string {
	var body strings.Builder
	for _, message := range messages {
		if message.Metadata != nil && message.Metadata.Kind == "context" {
			body.WriteString(session.Text(message))
		}
	}
	return body.String()
}

func TestContextRefreshAndSubmissionOrder(t *testing.T) {
	c, _ := testHost(t, t.TempDir(), DemoModel{})
	path := filepath.Join(c.Workspace.Cwd, "AGENTS.md")
	writeTestFile(t, c.Workspace.Cwd, "AGENTS.md", "first scoped instruction")
	prepare := c.Call(prompt.Execute, false).PrepareStep
	history := []model.Message{model.NewUserMessage("Actual user task")}
	for step, expected := range []string{"first scoped instruction", "replacement scoped instruction", ""} {
		if step == 1 {
			writeTestFile(t, c.Workspace.Cwd, "AGENTS.md", expected)
		}
		if step == 2 {
			if err := os.Remove(path); err != nil {
				t.Fatal(err)
			}
		}
		_, result, err := prepare(t.Context(), core.PrepareStepFunctionOptions{Messages: history, StepNumber: step})
		if err != nil {
			t.Fatal(err)
		}
		if result.Messages[0].Role != model.MessageRoleDeveloper || session.Text(result.Messages[len(result.Messages)-1]) != "Actual user task" {
			t.Fatal("policy or user submission order changed")
		}
		body := contextText(result.Messages)
		if expected != "" && !strings.Contains(body, expected) {
			t.Fatal("rules not refreshed")
		}
		if step > 0 && strings.Contains(body, "first scoped instruction") {
			t.Fatal("removed rules retained")
		}
		if strings.Contains(*result.System, "scoped instruction") || result.History != nil || history[0].Metadata != nil {
			t.Fatal("dynamic context modified durable history")
		}
	}
}

func TestModelWindowBudgetAndExplicitByteLimit(t *testing.T) {
	c, _ := testHost(t, t.TempDir(), DemoModel{})
	c.Host.Config.ContextBytes = 512 << 10
	c.Host.Config.Origins = map[string]string{}
	profile := setting.Model{ContextWindow: 100000, MaxOutputTokens: 10000}
	budget := c.compactor(profile)
	if budget.BudgetBytes != 0 || budget.BudgetTokens != 85000 {
		t.Fatal("window and output reservation", budget)
	}
	c.Host.Config.Origins["context_bytes"] = "project config"
	if c.compactor(profile).BudgetBytes != 512<<10 {
		t.Fatal("explicit byte limit ignored")
	}
	if c.compactor(setting.Model{}).BudgetBytes != 512<<10 {
		t.Fatal("unknown model lost fallback")
	}
	c.Host.Config.ContextBytes = 0
	if c.compactor(profile).BudgetBytes != 0 {
		t.Fatal("zero byte limit not disabled")
	}
}

func TestHostTemplateUsesConfiguredToolDefaults(t *testing.T) {
	c, _ := testHost(t, t.TempDir(), DemoModel{})
	c.Host.Config.Tools.Exec.YieldSeconds = 2
	c.Host.Config.Tools.Exec.TimeoutSeconds = new(42)
	_, prepared, err := c.Call(prompt.Execute, false).PrepareStep(t.Context(), core.PrepareStepFunctionOptions{})
	if err != nil {
		t.Fatal(err)
	}
	policy := session.Text(prepared.Messages[0])
	for _, text := range []string{"defaults to 2 for exec_command", "defaults to 42 seconds", "[/absolute/path/main.go#ABCD]", "Current mode: execute"} {
		if !strings.Contains(policy, text) {
			t.Fatalf("missing tool semantics %s", text)
		}
	}
	if strings.Contains(policy, "{{.") || strings.Contains(policy, "logical workspace paths") {
		t.Fatal("unrendered or wrong path policy")
	}
}

func TestChildReceivesAssignmentAndCanReadCataloguedSkill(t *testing.T) {
	m := fixtureModel{reply: func(call model.Call) (*model.Response, error) {
		if !strings.Contains(contextText(call.Prompt), "task_assignment") || !strings.Contains(contextText(call.Prompt), "skill_child") {
			t.Error("child assignment missing")
		}
		if call.Prompt[0].Role != model.MessageRoleSystem || !strings.Contains(session.Text(call.Prompt[1]), "Current role: explorer") {
			t.Error("child policy not installed")
		}
		for _, message := range call.Prompt {
			if message.Role == model.MessageRoleTool {
				body, _ := json.Marshal(message)
				if !strings.Contains(string(body), "Read this workflow.") {
					t.Error("child did not receive the skill body", string(body))
				}
				return demoText("Skill read successfully."), nil
			}
		}
		for _, item := range call.Tools {
			if item.GetName() == "read_skill" {
				return toolReply("read_skill", map[string]any{"name": "example"}, 1), nil
			}
		}
		t.Error("catalogued skill reader filtered out of child tools")
		return demoText("missing reader"), nil
	}}
	c, _ := testHost(t, t.TempDir(), m)
	writeTestFile(t, c.Workspace.Cwd, ".agents/skills/example/SKILL.md", "---\nname: example\ndescription: test skill\n---\nRead this workflow.")
	ctx, cancel := context.WithTimeout(t.Context(), 8*time.Second)
	defer cancel()
	info, err := c.backend.Spawn(ctx, tool.TaskSpec{Name: "skill_child", Role: "explorer", Message: "Read the example skill."})
	if err != nil {
		t.Fatal(err)
	}
	result, err := c.backend.Wait(ctx, info.ID, 5*time.Second)
	if err != nil || result.Error != "" || !strings.Contains(result.Result, "Skill read successfully") {
		t.Fatal("child skill flow failed", result, err)
	}
}
