package coding

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"clara-code/pkg/core/prompt"
	modelapi "clara-code/pkg/model"
	toolapi "clara-code/pkg/tool"

	"clara-code/internal/session"
)

func TestActualExitCodeDrivesActivity(t *testing.T) {
	commands := toolapi.NewCommands(observedRunner{runnerForTest(t)})
	defer commands.Close()
	result, err := observeCommand(commands.ExecCommand()).Run(t.Context(), modelapi.ToolCall{Input: `{"command":"printf 'literal exit_code: 0\\n'; exit 7"}`})
	if err != nil {
		t.Fatal(err)
	}
	tool := session.NewToolActivity(modelapi.ToolCallContent{ToolCallID: "failed", ToolName: "exec_command"}, time.Now())
	tool = tool.WithResult(modelapi.ToolResultContent{ToolCallID: "failed", Result: modelapi.ToolResultOutputContentError{Error: errors.New(result.Content)}, ClientMetadata: result.Metadata}, time.Now())
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
	return runner{workspace, toolapi.NewLocal()}
}

func TestOnDemandBackgroundUpdatesWithoutSnapshotSupport(t *testing.T) {
	log := newActivityLog(func() string { return "run" }, func(session.ToolActivity) error { return nil })
	if err := log.start(modelapi.ToolCallContent{ToolCallID: "original", ToolName: "exec_command", Input: `{"command":"fixture"}`}); err != nil {
		t.Fatal(err)
	}
	metadata, _ := json.Marshal(map[string]any{"clara_command": session.CommandOutcome{SessionID: "terminal", Output: "first", Running: true}})
	if err := log.complete(modelapi.ToolResultContent{ToolCallID: "original", ToolName: "exec_command", ClientMetadata: string(metadata)}); err != nil {
		t.Fatal(err)
	}
	info := toolapi.TerminalInfo{ID: "terminal", Status: "running", Revision: 1, Poll: toolapi.Poll{Running: true, Output: "last"}}
	activity, changed := log.terminal(info)
	if !changed || activity.Output != "firstlast" {
		t.Fatalf("on-demand polling lost output: %+v %v", activity, changed)
	}
	if _, changed := log.terminal(info); changed {
		t.Fatal("repeated state replay duplicated unread output")
	}
}

func TestPlanShellBackgroundPollingUpdatesTheOriginalRecordAcrossTurns(t *testing.T) {
	step := 0
	model := fixtureModel{reply: func(call modelapi.Call) (*modelapi.Response, error) {
		step++
		if step == 1 {
			return toolReply("exec_command", map[string]any{"command": "printf first; sleep 1.2; printf last", "yield_seconds": 1}, 1), nil
		}
		var output string
		for _, message := range call.Prompt {
			for _, part := range message.Content {
				if result, ok := modelapi.AsMessagePart[modelapi.ToolResultPart](part); ok {
					if text, ok := modelapi.AsToolResultOutputType[modelapi.ToolResultOutputContentText](result.Output); ok {
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
	finish(t, c, prompt.Plan, "Start a background diagnostic.")
	finish(t, c, prompt.Plan, "Wait for it.")
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
	if first == nil || last == nil || first.Status != "succeeded" || last.Status != "succeeded" || first.SessionID != last.SessionID || first.Key() == last.Key() {
		t.Fatalf("background record not associated: %+v %+v", first, last)
	}
	if !strings.Contains(first.Output, "first") || !strings.Contains(first.Output, "last") {
		t.Fatalf("background output lost: %q", first.Output)
	}
}

func TestParallelActivityResultsAndRepeatedCallIDs(t *testing.T) {
	currentRun := "run1"
	var records []session.ToolActivity
	log := newActivityLog(func() string { return currentRun }, func(activity session.ToolActivity) error { records = append(records, activity); return nil })
	for _, id := range []string{"a", "b"} {
		if err := log.start(modelapi.ToolCallContent{ToolCallID: id, ToolName: "read", Input: `{"path":"file.go"}`}); err != nil {
			t.Fatal(err)
		}
	}
	for _, id := range []string{"b", "a"} {
		if err := log.complete(modelapi.ToolResultContent{ToolCallID: id, ToolName: "read", Result: modelapi.ToolResultOutputContentText{Text: id}}); err != nil {
			t.Fatal(err)
		}
	}
	if len(records) != 4 || records[2].CallID != "b" || records[3].CallID != "a" || records[2].Status != "succeeded" {
		t.Fatalf("out-of-order results mixed: %+v", records)
	}
	currentRun = "run2"
	_ = log.start(modelapi.ToolCallContent{ToolCallID: "a", ToolName: "read"})
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
	_ = log.start(modelapi.ToolCallContent{ToolCallID: "start", ToolName: "exec_command", Input: `{"command":"sleep 30"}`})
	metadata, _ := json.Marshal(map[string]any{"clara_command": session.CommandOutcome{Running: true, SessionID: "exec_background"}})
	_ = log.complete(modelapi.ToolResultContent{ToolCallID: "start", ToolName: "exec_command", ClientMetadata: string(metadata)})
	_ = log.start(modelapi.ToolCallContent{ToolCallID: "poll", ToolName: "write_stdin", Input: `{"session_id":"exec_background"}`})
	log.finish(context.Canceled)
	if len(log.calls) != 1 || len(log.background) != 1 {
		t.Fatalf("interrupted poll discarded the live background record: %+v", log)
	}
}

func TestMultipleProgressMessagesPersistSeparately(t *testing.T) {
	model := fixtureModel{reply: func(modelapi.Call) (*modelapi.Response, error) {
		return &modelapi.Response{Content: modelapi.ResponseContent{modelapi.TextContent{Text: "First update."}, modelapi.TextContent{Text: "Second update."}}, FinishReason: modelapi.FinishReasonStop}, nil
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
