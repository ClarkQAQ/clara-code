package terminal

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"image"
	"image/png"
	"strings"
	"testing"
	"time"

	"clara-code/internal/session"
	"clara-code/pkg/core"
	"clara-code/pkg/model"
	"clara-code/pkg/tool"
	"clara-code/pkg/util/id"
)

func TestResumeHistoryReplaysOnlyRecentText(t *testing.T) {
	for _, entry := range []string{"startup", "slash command"} {
		t.Run(entry, func(t *testing.T) {
			a := testApp(t)
			var output bytes.Buffer
			a.view.output = &output
			sessionID, command := saveResumeHistory(t, a)
			if entry == "startup" {
				draft := a.conv
				if err := draft.Close(); err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() {
					if a.conv == nil {
						a.conv = draft
					}
				})
				a.conv = nil
				a.view.buf.WriteString("\x1b[?2004h")
				a.options.Resume = sessionID
				a.startup()
			} else {
				a.submit("/resume " + sessionID)
			}
			if a.op == nil {
				t.Fatal("resume did not start a conversation operation")
			}
			select {
			case result := <-a.op:
				a.stopOp()
				a.op, a.stopOp, a.opLabel = nil, nil, ""
				if result.err != nil {
					t.Fatal(result.err)
				}
				if err := a.install(result); err != nil {
					t.Fatal(err)
				}
			case <-time.After(5 * time.Second):
				a.stopOp()
				t.Fatal("resume operation timed out")
			}
			if err := a.draw(); err != nil {
				t.Fatal(err)
			}

			hidden := []string{
				"Earlier prompt sentinel", "Earlier reply sentinel", "historical command output", "saved background output",
				"stored questions output", "request_user_input failed", "resume-fixture.png",
			}
			assertTextHistory := func(output string) {
				t.Helper()
				for _, text := range []string{
					"Earlier display history omitted", "Long reply tail", "Saved prompt", "Extra direction", "Saved reply",
					"Proposed Plan", "Saved plan", "Context compacted", "· Goal paused",
				} {
					if !strings.Contains(output, text) {
						t.Errorf("resume lost %q", text)
					}
				}
				for _, text := range hidden {
					if strings.Contains(output, text) {
						t.Errorf("resume displayed %q", text)
					}
				}
			}
			assertTextHistory(output.String())
			if entry == "startup" {
				if !strings.Contains(output.String(), "\x1b[?2004h") {
					t.Fatal("resume lost the pending bracketed-paste initialization")
				}
				if len(a.inputHistory) != 2 || a.inputHistory[1].Text != "Saved prompt" || len(a.inputHistory[1].Attachments) != 1 {
					t.Fatalf("input recall lost saved prompt or attachment: %+v", a.inputHistory)
				}
			}

			// A new command may reuse a saved task command's key.
			command.Started = command.Started.Add(time.Second)
			command.Output = "new command output"
			if err := a.conv.Store.UpdateCommand(command); err != nil {
				t.Fatal(err)
			}
			a.syncBackground()
			a.view.width = 100
			a.reflow = true
			output.Reset()
			if err := a.draw(); err != nil {
				t.Fatal(err)
			}
			assertTextHistory(output.String())
			if !strings.Contains(output.String(), command.Output) {
				t.Fatalf("new tool activity was hidden: %q", output.String())
			}

			if len(a.prompts) != 2 || a.prompts[0].text != "Earlier prompt sentinel" || a.prompts[1].text != "Saved prompt" {
				t.Fatalf("rewind prompts lost saved prompts: %+v", a.prompts)
			}
			a.openRewind()
			a.picker.detail = true
			rows := strings.Join(a.rewindRows(), "\n")
			for _, text := range []string{"stored questions output", "request_user_input failed", "resume-fixture.png"} {
				if !strings.Contains(rows, text) {
					t.Errorf("rewind detail lost %q in %q", text, rows)
				}
			}
			a.handleTranscriptPicker(key{code: "left"})
			if rows := strings.Join(a.rewindRows(), "\n"); !strings.Contains(rows, "Earlier reply sentinel") {
				t.Errorf("rewind detail lost truncated reply: %q", rows)
			}
			messages := a.conv.Session.Messages()
			if len(messages) != 6 || messages[4].Role != model.MessageRoleTool || len(messages[4].Content) != 1 {
				t.Fatal("tool context was not restored")
			}
			result, ok := model.AsMessagePart[model.ToolResultPart](messages[4].Content[0])
			if !ok || result.ToolCallID != command.CallID {
				t.Fatalf("unexpected saved tool result: %+v", messages[4])
			}
		})
	}
}

func saveResumeHistory(t *testing.T, a *app) (string, session.ToolActivity) {
	t.Helper()
	store := session.New(a.host.Config.Home, session.Meta{Project: a.space.Root, Cwd: a.space.Cwd, Model: "demo"})
	t.Cleanup(func() { store.Close() })
	state := core.State{ID: id.New("session")}
	start := time.Now().UTC().Add(-time.Minute)
	addEvent := func(kind string, payload any) {
		t.Helper()
		data, err := json.Marshal(payload)
		if err != nil {
			t.Fatal(err)
		}
		state.Events = append(state.Events, core.Event{Sequence: uint64(len(state.Events) + 1), SessionID: state.ID, RunID: state.ActiveRun, Kind: kind, At: start, Data: data})
	}
	save := func(ctx context.Context) {
		t.Helper()
		state.Revision++
		if err := store.Save(ctx, state); err != nil {
			t.Fatal(err)
		}
	}
	startTurn := func(ctx context.Context, user model.Message) {
		t.Helper()
		state.ActiveRun = id.New("run")
		state.Messages = append(state.Messages, user)
		addEvent("run.started", []model.Message{user})
		save(ctx)
	}
	completeTurn := func(messages ...model.Message) {
		t.Helper()
		state.Staged = messages
		save(t.Context())
		addEvent("run.completed", nil)
		state.ActiveRun = ""
		state.Messages = append(state.Messages, state.Staged...)
		state.Staged = nil
		save(t.Context())
	}
	startTurn(t.Context(), model.NewUserMessage("Earlier prompt sentinel"))
	// This reply exceeds both the replay row cap and the old 64 KiB flush threshold.
	longReply := "Earlier reply sentinel\n" + strings.Repeat(strings.Repeat("x", 64)+"\n", 1_100) + "Long reply tail"
	completeTurn(model.Message{Role: model.MessageRoleAssistant, Content: []model.MessagePart{model.TextPart{Text: longReply}}})

	var imageData bytes.Buffer
	if err := png.Encode(&imageData, image.NewRGBA(image.Rect(0, 0, 2, 2))); err != nil {
		t.Fatal(err)
	}
	user := model.NewUserMessage("Saved prompt", model.FilePart{Filename: "resume-fixture.png", MediaType: "image/png", Data: imageData.Bytes()})
	startTurn(session.WithPlanning(t.Context()), user)

	command := session.ToolActivity{ID: "task:1:resume-command", CallID: "resume-command", Name: "exec_command", Detail: "saved command", Status: "succeeded", Started: start, Output: "historical command output"}
	addEvent("tool.activity", command)
	addEvent("tool.result", model.ToolResultContent{ToolName: "request_user_input", Result: model.ToolResultOutputContentText{
		Text: `{"Questions":{"id":"Question"},"Answers":{"id":{"Answers":["stored questions output"]}}}`,
	}})
	addEvent("tool.result", model.ToolResultContent{ToolName: "request_user_input", Result: model.ToolResultOutputContentError{Error: errors.New("stored question failure")}})
	addEvent(session.ContextCompactedKind, session.Compaction{DurationMS: new(int64(2000))})
	addEvent("input.steering", "Extra direction")
	call := model.Message{Role: model.MessageRoleAssistant, Content: []model.MessagePart{model.ToolCallPart{ToolCallID: command.CallID, ToolName: command.Name, Input: `{}`}}}
	result := model.Message{Role: model.MessageRoleTool, Content: []model.MessagePart{model.ToolResultPart{ToolCallID: command.CallID, Output: model.ToolResultOutputContentText{Text: command.Output}}}}
	reply := model.Message{Role: model.MessageRoleAssistant, Content: []model.MessagePart{model.TextPart{Text: "Saved reply\n\n<proposed_plan>\n# Saved plan\n</proposed_plan>"}}}
	completeTurn(call, result, reply)
	if err := store.UpdateCommand(command); err != nil {
		t.Fatal(err)
	}
	background := session.ToolActivity{ID: "task:2:saved-background", CallID: "saved-background", Name: "exec_command", Status: "background", Started: start, Output: "saved background output"}
	if err := store.UpdateCommand(background); err != nil {
		t.Fatal(err)
	}
	if _, err := store.SaveGoal(tool.GoalState{Status: tool.GoalPaused, Objective: "Saved objective"}); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	return state.ID, command
}
