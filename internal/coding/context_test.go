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

	"github.com/pelletier/go-toml/v2"
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
		if result.System != nil || strings.Contains(session.Text(result.Messages[0]), "scoped instruction") || result.History != nil || history[0].Metadata != nil {
			t.Fatal("dynamic context modified durable history")
		}
	}
}

func TestModelWindowBudgetAndCompactionPercent(t *testing.T) {
	c, _ := testHost(t, t.TempDir(), DemoModel{})
	profile := setting.Model{ContextWindow: 100000, MaxOutputTokens: 1000}
	budget := c.compactor(profile)
	if budget.BudgetBytes != 0 || budget.BudgetTokens != 95000 || budget.TriggerTokens != 90000 {
		t.Fatal("effective window/automatic trigger", budget)
	}
	c.Host.Config.AutoCompactContextPercent = 80
	if c.compactor(profile).TriggerTokens != 80000 {
		t.Fatal("global compaction percent ignored")
	}
	profile.AutoCompactContextPercent = new(60)
	if c.compactor(profile).TriggerTokens != 60000 {
		t.Fatal("model override ignored")
	}
	if c.compactor(setting.Model{}).BudgetBytes != 0 || c.compactor(setting.Model{}).BudgetTokens <= 0 {
		t.Fatal("unknown model lost token fallback")
	}
	profile.AutoCompactContextPercent = new(100)
	if c.compactor(profile).TriggerTokens != 90000 {
		t.Fatal("trigger exceeded Codex's 90% raw-window ceiling")
	}
	profile.MaxOutputTokens = 10000
	if c.compactor(profile).TriggerTokens != 90000 {
		t.Fatal("output allowance lowered the compaction threshold")
	}
	profile.EffectiveContextWindowPercent = 70
	if c.compactor(profile).TriggerTokens != 70000 {
		t.Fatal("trigger exceeded the effective model window")
	}
	profile.MaxOutputTokens = 1000
	profile.EffectiveContextWindowPercent = 99
	if c.compactor(profile).TriggerTokens != 90000 {
		t.Fatal("effective window ratio changed the raw-window compaction threshold")
	}
}

func TestActiveContextTokensCountsOnlyLocalGrowthAfterSample(t *testing.T) {
	history := []model.Message{model.NewUserMessage(strings.Repeat("old", 3000))}
	_, previous, err := (prompt.Window{}).Prepare(history, nil)
	if err != nil {
		t.Fatal(err)
	}
	previous.RequestSignature = "model-and-tools"
	assistant := model.Message{Role: model.MessageRoleAssistant, Content: []model.MessagePart{
		model.TextPart{Text: strings.Repeat("response", 5000)},
		model.ToolCallPart{ToolCallID: "id", ToolName: "read", Input: "{}"},
	}}
	result := model.Message{Role: model.MessageRoleTool, Content: []model.MessagePart{model.ToolResultPart{
		ToolCallID: "id", Output: model.ToolResultOutputContentText{Text: "result"},
	}}}
	history = append(history, assistant, result)
	change := prompt.Fragment{Kind: "plan", Source: "host", Role: model.MessageRoleUser, Text: "new plan"}
	request, _, err := previous.Prepare(history, []prompt.Fragment{change})
	if err != nil {
		t.Fatal(err)
	}
	step := core.PrepareStepFunctionOptions{Steps: []core.StepResult{{Response: model.Response{Usage: model.Usage{TotalTokens: 3000}}, Messages: []model.Message{assistant, result}}}}
	local, err := prompt.Measure("", request[2:], nil)
	if err != nil || len(request) != 4 || local.Tokens < 3 {
		t.Fatal("unexpected fixture layout", len(request), local, err)
	}
	got, err := activeContextTokens(step, session.Meta{}, previous, "model-and-tools", history, request, nil, nil)
	if err != nil || got != 3000+local.Tokens {
		t.Fatalf("active footprint = %d, want %d: %v", got, 3000+local.Tokens, err)
	}
	step.Steps[0].Usage = model.Usage{InputTokens: 700, CacheReadTokens: 2200, OutputTokens: 100, ReasoningTokens: 40}
	got, err = activeContextTokens(step, session.Meta{}, previous, "model-and-tools", history, request, nil, nil)
	if err != nil || got != 3000+local.Tokens {
		t.Fatalf("usage with separate cache/input/output = %d: %v", got, err)
	}
	full, err := prompt.Measure("", request, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		signature string
		tokens    int64
	}{
		{"different-tools", 3000},
		{"model-and-tools", 0},
	} {
		step.Steps[0].Usage = model.Usage{TotalTokens: tc.tokens}
		got, err := activeContextTokens(step, session.Meta{}, previous, tc.signature, history, request, nil, nil)
		if err != nil || got != full.Tokens {
			t.Fatalf("mismatched/missing report produced %d tokens, want %d: %v", got, full.Tokens, err)
		}
	}
}

func TestContextEstimateProjectsProviderReasoning(t *testing.T) {
	encrypted := strings.Repeat("z", 1200)
	history := []model.Message{{Role: model.MessageRoleAssistant, Content: []model.MessagePart{
		model.ReasoningPart{ItemID: "plaintext-id", Text: "12345678"},
		model.ReasoningPart{ItemID: "encrypted-id", Encrypted: &encrypted, Text: "abcd"},
	}}}
	for _, tc := range []struct {
		completions bool
		tokens      int64
	}{
		{true, 3},
		{false, 63}, // (1200*3/4 - 650) visible bytes.
	} {
		got, err := prompt.Measure("", projectedReasoning(history, tc.completions), nil)
		if err != nil || got.Tokens != tc.tokens {
			t.Fatalf("completions=%t: got %+v, want %d: %v", tc.completions, got, tc.tokens, err)
		}
	}
	if len(history[0].Content) != 2 || history[0].Content[1].(model.ReasoningPart).Encrypted == nil {
		t.Fatal("projection changed durable reasoning")
	}
}

func TestUsageSignatureChangesWithProjection(t *testing.T) {
	signatures := make(map[string]bool)
	for _, wire := range []string{"responses", "completions"} {
		for _, images := range []bool{false, true} {
			signature, err := requestSignature(DemoModel{}, wire, images, "base", nil)
			if err != nil || signatures[signature] {
				t.Fatalf("usage sample was shared across wire APIs or image modalities: %v", err)
			}
			signatures[signature] = true
		}
	}
}

func TestHostTemplateUsesConfiguredToolDefaults(t *testing.T) {
	c, _ := testHost(t, t.TempDir(), DemoModel{})
	config := []byte("[tools.exec_command]\nyield_seconds = 2\ntimeout_seconds = 42\n")
	if err := toml.Unmarshal(config, &c.Host.Config); err != nil {
		t.Fatal(err)
	}
	_, prepared, err := c.Call(prompt.Execute, false).PrepareStep(t.Context(), core.PrepareStepFunctionOptions{})
	if err != nil {
		t.Fatal(err)
	}
	policy := session.Text(prepared.Messages[0])
	for _, text := range []string{"defaults to 2000 for exec_command", "defaults to 42 seconds", "[/absolute/path/main.go#ABCD]", "Current mode: execute"} {
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
		if call.Prompt[0].Role != model.MessageRoleDeveloper || !strings.Contains(session.Text(call.Prompt[0]), "Current role: explorer") {
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
