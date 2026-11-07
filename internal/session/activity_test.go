package session

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"clara-code/pkg/core"
	"clara-code/pkg/models"
)

func TestToolSnapshotReplacesCallAndSurvivesSerialization(t *testing.T) {
	tool := NewToolActivity(models.ToolCallContent{ToolCallID: "call", ToolName: "exec_command", Input: `{"command":"printf hello"}`}, time.Now())
	state := Turn{ID: "run"}
	emit := func(sequence uint64, activity ToolActivity) {
		body, _ := json.Marshal(activity)
		state.collect([]core.Event{{Sequence: sequence, RunID: "run", Kind: "tool.activity", Data: body}})
	}
	emit(1, tool)
	exit := 7
	metadata, _ := json.Marshal(map[string]any{"clara_command": CommandOutcome{Output: "hello\nfailed command", ExitCode: &exit, DurationMS: 240}})
	result := models.ToolResultContent{ToolCallID: "call", ToolName: "exec_command", Result: models.ToolResultOutputContentError{Error: errFixture("exit failed")}, ClientMetadata: string(metadata)}
	tool = tool.WithResult(result, time.Now())
	emit(2, tool)
	if len(state.Cells) != 1 || state.Cells[0].Tool == nil {
		t.Fatalf("tool split into multiple cells: %+v", state.Cells)
	}
	got := state.Cells[0].Tool
	if got.Status != "failed" || got.ExitCode == nil || *got.ExitCode != 7 || got.Output != "hello\nfailed command" {
		t.Fatalf("tool %+v", got)
	}
	body, _ := json.Marshal(state)
	var restored Turn
	if err := json.Unmarshal(body, &restored); err != nil {
		t.Fatal(err)
	}
	if restored.Cells[0].Tool.ExitCode == nil || *restored.Cells[0].Tool.ExitCode != 7 {
		t.Fatal("structured tool data lost")
	}
	var legacy Turn
	if err := json.Unmarshal([]byte(`{"Cells":[{"Kind":"activity","Text":"exec_command old"}]}`), &legacy); err != nil || legacy.Cells[0].Text != "exec_command old" {
		t.Fatal("old text records no longer readable")
	}
}

type errFixture string

func (e errFixture) Error() string { return string(e) }

func TestToolPreviewKeepsHeadAndTailAndEmptyPollKeepsOutput(t *testing.T) {
	text := "first\n" + strings.Repeat("middle line\n", 4000) + "last"
	preview, omitted := PreviewOutput(text, 1000)
	if !strings.HasPrefix(preview, "first") || !strings.HasSuffix(preview, "last") || omitted == 0 {
		t.Fatalf("preview omitted=%d %.100s", omitted, preview)
	}
	tool := ToolActivity{CallID: "background", Name: "exec_command", Status: "background", Started: time.Now(), SessionID: "exec_test", Output: "earlier output"}
	body, _ := json.Marshal(map[string]any{"clara_command": CommandOutcome{Running: true, SessionID: "exec_test"}})
	tool = tool.WithResult(models.ToolResultContent{ClientMetadata: string(body)}, time.Now())
	if tool.Output != "earlier output" || tool.Status != "background" {
		t.Fatalf("empty poll lost previous output: %+v", tool)
	}
}

func TestTextBoundariesDoNotDuplicateAtCheckpoint(t *testing.T) {
	turn := Turn{ID: "run"}
	data := func(value any) json.RawMessage { body, _ := json.Marshal(value); return body }
	turn.collect([]core.Event{
		{Sequence: 1, RunID: "run", Kind: "text_delta", Data: data(models.StreamPart{Delta: "First progress."})},
		{Sequence: 2, RunID: "run", Kind: "text_end", Data: data(models.StreamPart{})},
		{Sequence: 3, RunID: "run", Kind: "text_delta", Data: data(models.StreamPart{Delta: "Second progress."})},
		{Sequence: 4, RunID: "run", Kind: "text_end", Data: data(models.StreamPart{})},
	})
	if len(turn.Cells) != 2 || turn.StreamedText != "First progress.Second progress." {
		t.Fatalf("message boundaries %+v", turn)
	}
}
