package demo

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"clara-code/examples/internal/host"
	"clara-code/pkg/core/prompt"
	"clara-code/pkg/models"
	"clara-code/pkg/tools/fs"
	"clara-code/pkg/tools/fs/local"
)

func CodingLoop(ctx context.Context, output io.Writer) (err error) {
	ctx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	parent, err := local.New(os.TempDir())
	if err != nil {
		return err
	}
	dir, err := os.MkdirTemp(parent.Root(), "clara-core-coding-")
	if err != nil {
		return err
	}
	defer func() {
		err = errors.Join(err, parent.Remove(context.Background(), fs.Path("/"+filepath.Base(dir)), fs.RemoveOptions{Recursive: true}))
	}()
	config, err := host.Local(dir)
	if err != nil {
		return err
	}
	for name, source := range map[fs.Path]string{
		"/go.mod":       "module example.test/calculator\n\ngo 1.27.0\n",
		"/calc.go":      "package calculator\n\nfunc Add(a, b int) int { return a - b }\n",
		"/calc_test.go": "package calculator\n\nimport \"testing\"\n\nfunc TestAdd(t *testing.T) { if Add(2,3) != 5 { t.Fatal(\"incorrect sum\") } }\n",
	} {
		if _, err := config.FS.WriteFile(ctx, name, []byte(source), fs.WriteOptions{Mode: 0o644}); err != nil {
			return err
		}
	}
	config.Model = Model{Reply: codingReply}
	conversation, err := host.Open(ctx, config, "")
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, conversation.Close(context.Background())) }()
	run, err := conversation.Start(ctx, prompt.Execute, "Find and fix the failing calculation test.")
	if err != nil {
		return err
	}
	for cursor := uint64(0); ; {
		event, err := conversation.Session.NextEvent(ctx, cursor)
		if err != nil {
			return err
		}
		cursor = event.Sequence
		switch event.Kind {
		case "tool.call":
			var call models.ToolCallContent
			if err := json.Unmarshal(event.Data, &call); err != nil {
				return err
			}
			fmt.Fprintf(output, "> %s %s\n", call.ToolName, call.Input)
		case "tool.result":
			var result models.ToolResultContent
			if err := json.Unmarshal(event.Data, &result); err != nil {
				return err
			}
			fmt.Fprintln(output, toolOutput(result.Result))
		case "run.completed", "run.failed", "run.cancelled":
			result, err := run.Wait(ctx)
			if err == nil {
				fmt.Fprintln(output, result.Response.Content.Text())
			}
			return err
		}
	}
}

func toolOutput(value models.ToolResultOutputContent) string {
	if text, ok := models.AsToolResultOutputType[models.ToolResultOutputContentText](value); ok {
		return text.Text
	}
	if failure, ok := models.AsToolResultOutputType[models.ToolResultOutputContentError](value); ok {
		return failure.Error.Error()
	}
	return ""
}

func codingReply(_ context.Context, call models.Call) (models.Response, error) {
	var outputs []string
	for _, msg := range call.Prompt {
		for _, part := range msg.Content {
			if result, ok := models.AsMessagePart[models.ToolResultPart](part); ok {
				outputs = append(outputs, toolOutput(result.Output))
			}
		}
	}
	var tool string
	var input map[string]any
	switch len(outputs) {
	case 0, 4:
		tool, input = "exec_command", map[string]any{"command": "go test ./...", "yield_seconds": 30}
	case 1:
		if !strings.Contains(outputs[0], "incorrect sum") {
			return models.Response{}, errors.New("expected the fixture test to fail")
		}
		tool, input = "grep", map[string]any{"query": "return a - b"}
	case 2:
		if !strings.Contains(outputs[1], "calc.go") {
			return models.Response{}, errors.New("expected grep to find calc.go")
		}
		tool, input = "read", map[string]any{"path": "calc.go", "anchored": true}
	case 3:
		header, _, _ := strings.Cut(outputs[2], "\n")
		if !strings.HasPrefix(header, "[/calc.go#") {
			return models.Response{}, errors.New("expected an anchored snapshot")
		}
		tool, input = "edit", map[string]any{"input": header + "\nPUT 3.=3:\n+func Add(a, b int) int { return a + b }"}
	case 5:
		if !strings.Contains(outputs[4], "exit_code: 0") {
			return models.Response{}, errors.New("the test did not pass after the edit")
		}
		return Text("Fixed Add in calc.go. go test ./... passed."), nil
	default:
		return models.Response{}, errors.New("unexpected coding fixture step")
	}
	body, err := json.Marshal(input)
	if err != nil {
		return models.Response{}, err
	}
	return models.Response{Content: models.ResponseContent{models.ToolCallContent{
		ToolCallID: fmt.Sprintf("demo_%d", len(outputs)+1), ToolName: tool, Input: string(body),
	}}, FinishReason: models.FinishReasonToolCalls}, nil
}
