package demo

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"clara-code/examples/internal/host"
	"clara-code/pkg/core"
	"clara-code/pkg/core/prompt"
	"clara-code/pkg/models"
	"clara-code/pkg/tools/fs"
)

func TestLocalWorkspaceUsesLogicalPaths(t *testing.T) {
	ctx := t.Context()
	root := t.TempDir()
	config, err := host.Local(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := config.FS.MkdirAll(ctx, "/nested", 0o755); err != nil {
		t.Fatal(err)
	}
	for path, contents := range map[fs.Path]string{
		"/main.go":          "package main",
		"/AGENTS.md":        "Follow the root rules.",
		"/nested/AGENTS.md": "Follow the nested rules.",
	} {
		if _, err := config.FS.WriteFile(ctx, path, []byte(contents), fs.WriteOptions{}); err != nil {
			t.Fatal(err)
		}
	}

	config.Model = Model{Reply: func(_ context.Context, call models.Call) (models.Response, error) {
		state, _ := models.AsMessagePart[models.TextPart](call.Prompt[len(call.Prompt)-1].Content[0])
		if !strings.Contains(state.Text, "workspace root in file tools is /") ||
			!strings.Contains(state.Text, "Follow the root rules.") || strings.Contains(state.Text, root) {
			return models.Response{}, fmt.Errorf("unexpected workspace context: %s", state.Text)
		}
		return models.Response{
			FinishReason: models.FinishReasonToolCalls,
			Content: models.ResponseContent{
				models.ToolCallContent{ToolCallID: "list", ToolName: "list", Input: `{"path":"/"}`},
				models.ToolCallContent{ToolCallID: "rules", ToolName: "fs_instructions", Input: `{"path":"/nested"}`},
				models.ToolCallContent{ToolCallID: "new-file", ToolName: "fs_instructions", Input: `{"path":"/nested/future.go"}`},
			},
		}, nil
	}}
	conversation, err := host.Open(ctx, config, "")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := conversation.Close(context.Background()); err != nil {
			t.Error(err)
		}
	})
	call := conversation.Call(prompt.Execute)
	call.Prompt = "List the workspace and find its nested rules."
	call.StopWhen = []core.StopCondition{core.StepCountIs(1)}
	run, err := conversation.Session.Start(ctx, call)
	if err != nil {
		t.Fatal(err)
	}
	result, err := run.Wait(ctx)
	if err != nil {
		t.Fatal(err)
	}
	items := result.Steps[0].Content.ToolResults()
	if len(items) != 3 {
		t.Fatalf("unexpected tool results: %+v", items)
	}
	list := toolOutput(items[0].Result)
	rules := toolOutput(items[1].Result)
	if !strings.Contains(list, "/main.go") ||
		!strings.Contains(rules, "Follow the root rules.") ||
		!strings.Contains(rules, "Follow the nested rules.") ||
		toolOutput(items[2].Result) != rules {
		t.Fatalf("list: %q, rules: %q", list, rules)
	}
}
