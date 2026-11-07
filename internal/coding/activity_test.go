package coding

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"clara-core/agent/prompt"
	"clara-core/models"
	"clara-core/tools"

	"clara-code/internal/session"
)

func TestActualExitCodeDrivesActivity(t *testing.T) {
	commands := tools.NewCommands(observedRunner{runnerForTest(t)})
	defer commands.Close()
	result, err := observeCommand(commands.ExecCommand()).Run(t.Context(), models.ToolCall{Input: `{"command":"printf 'literal exit_code: 0\\n'; exit 7"}`})
	if err != nil {
		t.Fatal(err)
	}
	tool := session.NewToolActivity(models.ToolCallContent{ToolCallID: "failed", ToolName: "exec_command"}, time.Now())
	tool = tool.WithResult(models.ToolResultContent{ToolCallID: "failed", Result: models.ToolResultOutputContentError{Error: errors.New(result.Content)}, ClientMetadata: result.Metadata}, time.Now())
	if tool.Status != "failed" || tool.ExitCode == nil || *tool.ExitCode != 7 || !strings.Contains(tool.Output, "exit_code: 0") {
		t.Fatalf("activity trusted stdout instead of the process exit status: %+v", tool)
	}
}

func runnerForTest(t *testing.T) runner {
	t.Helper()
	workspace, err := ResolveWorkspace(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return runner{workspace, tools.NewLocal()}
}

func TestBackgroundPollingUpdatesTheOriginalRecordAcrossTurns(t *testing.T) {
	step := 0
	model := fixtureModel{reply: func(call models.Call) (*models.Response, error) {
		step++
		if step == 1 {
			return toolReply("exec_command", map[string]any{"command": "printf first; sleep 1.2; printf last", "yield_seconds": 1}, 1), nil
		}
		var output string
		for _, message := range call.Prompt {
			for _, part := range message.Content {
				if result, ok := models.AsMessagePart[models.ToolResultPart](part); ok {
					if text, ok := models.AsToolResultOutputType[models.ToolResultOutputContentText](result.Output); ok {
						output = text.Text
					}
				}
			}
		}
		if step == 2 {
			return demoText("The command is running in the background."), nil
		}
		if step == 3 {
			var id string
			for _, line := range strings.Split(output, "\n") {
				if value, ok := strings.CutPrefix(line, "session_id: "); ok {
					id = value
					break
				}
			}
			if id == "" {
				return nil, errors.New("background session id missing")
			}
			return toolReply("write_stdin", map[string]any{"session_id": id, "yield_seconds": 1}, 2), nil
		}
		return demoText("Completed."), nil
	}}
	c, _ := testHost(t, t.TempDir(), model)
	finish(t, c, prompt.Execute, "Start a background command.")
	finish(t, c, prompt.Execute, "Wait for it.")
	turns, err := c.Store.Turns()
	if err != nil || len(turns) != 2 {
		t.Fatalf("turns %d %v", len(turns), err)
	}
	var first, last *session.ToolActivity
	for _, cell := range turns[0].Cells {
		if cell.Tool != nil {
			first = cell.Tool
		}
	}
	for _, cell := range turns[1].Cells {
		if cell.Tool != nil {
			last = cell.Tool
		}
	}
	if first == nil || last == nil || first.Status != "background" || last.Status != "succeeded" || first.Key() != last.Key() {
		t.Fatalf("background record not associated: %+v %+v", first, last)
	}
	if !strings.Contains(last.Output, "first") || !strings.Contains(last.Output, "last") {
		t.Fatalf("background output lost: %q", last.Output)
	}
}

func TestParallelActivityResultsAndRepeatedCallIDs(t *testing.T) {
	currentRun := "run1"
	var records []session.ToolActivity
	log := newActivityLog(func() string { return currentRun }, func(activity session.ToolActivity) error { records = append(records, activity); return nil })
	for _, id := range []string{"a", "b"} {
		if err := log.start(models.ToolCallContent{ToolCallID: id, ToolName: "read", Input: `{"path":"file.go"}`}); err != nil {
			t.Fatal(err)
		}
	}
	for _, id := range []string{"b", "a"} {
		if err := log.complete(models.ToolResultContent{ToolCallID: id, ToolName: "read", Result: models.ToolResultOutputContentText{Text: id}}); err != nil {
			t.Fatal(err)
		}
	}
	if len(records) != 4 || records[2].CallID != "b" || records[3].CallID != "a" || records[2].Status != "succeeded" {
		t.Fatalf("out-of-order results mixed: %+v", records)
	}
	currentRun = "run2"
	_ = log.start(models.ToolCallContent{ToolCallID: "a", ToolName: "read"})
	if records[4].Key() == records[0].Key() {
		t.Fatal("a later turn reused a prior tool display record")
	}
	log.finish(context.Canceled)
	if records[len(records)-1].Status != "cancelled" {
		t.Fatal("unfinished tool was not cancelled")
	}
}

func TestInterruptedBackgroundPollClearsAliases(t *testing.T) {
	log := newActivityLog(func() string { return "run" }, func(session.ToolActivity) error { return nil })
	_ = log.start(models.ToolCallContent{ToolCallID: "start", ToolName: "exec_command", Input: `{"command":"sleep 30"}`})
	metadata, _ := json.Marshal(map[string]any{"clara_command": session.CommandOutcome{Running: true, SessionID: "exec_background"}})
	_ = log.complete(models.ToolResultContent{ToolCallID: "start", ToolName: "exec_command", ClientMetadata: string(metadata)})
	_ = log.start(models.ToolCallContent{ToolCallID: "poll", ToolName: "write_stdin", Input: `{"session_id":"exec_background"}`})
	log.finish(context.Canceled)
	if len(log.calls) != 0 || len(log.aliases) != 0 || len(log.background) != 0 {
		t.Fatalf("interrupted poll retained stale associations: %+v", log)
	}
}

func TestMultipleProgressMessagesPersistSeparately(t *testing.T) {
	model := fixtureModel{reply: func(models.Call) (*models.Response, error) {
		return &models.Response{Content: models.ResponseContent{models.TextContent{Text: "First update."}, models.TextContent{Text: "Second update."}}, FinishReason: models.FinishReasonStop}, nil
	}}
	c, _ := testHost(t, t.TempDir(), model)
	finish(t, c, prompt.Execute, "Show progress.")
	turns, err := c.Store.Turns()
	if err != nil {
		t.Fatal(err)
	}
	var texts []string
	for _, cell := range turns[0].Cells {
		if cell.Kind == "assistant" {
			texts = append(texts, cell.Text)
		}
	}
	if len(texts) != 2 || texts[0] != "First update." || texts[1] != "Second update." {
		body, _ := json.Marshal(turns)
		t.Fatalf("progress messages merged or duplicated: %s", body)
	}
}
