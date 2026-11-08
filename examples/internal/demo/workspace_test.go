package demo

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"clara-code/examples/internal/host"
	"clara-code/pkg/core"
	"clara-code/pkg/core/prompt"
	"clara-code/pkg/model"
	"clara-code/pkg/tool/fs"
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

	config.Model = Model{Reply: func(_ context.Context, call model.Call) (model.Response, error) {
		var state strings.Builder
		for _, message := range call.Prompt {
			if message.Metadata == nil || message.Metadata.Kind != "context" {
				continue
			}
			for _, part := range message.Content {
				if text, ok := model.AsMessagePart[model.TextPart](part); ok {
					state.WriteString(text.Text)
				}
			}
		}
		if !strings.Contains(state.String(), "workspace root in file tools is /") ||
			!strings.Contains(state.String(), "Follow the root rules.") || strings.Contains(state.String(), root) {
			return model.Response{}, fmt.Errorf("unexpected workspace context: %s", state.String())
		}
		return model.Response{
			FinishReason: model.FinishReasonToolCalls,
			Content: model.ResponseContent{
				model.ToolCallContent{ToolCallID: "list", ToolName: "list", Input: `{"path":"/"}`},
				model.ToolCallContent{ToolCallID: "rules", ToolName: "fs_instructions", Input: `{"path":"/nested"}`},
				model.ToolCallContent{ToolCallID: "new-file", ToolName: "fs_instructions", Input: `{"path":"/nested/future.go"}`},
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
