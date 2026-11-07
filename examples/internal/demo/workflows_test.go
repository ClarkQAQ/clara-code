package demo

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"clara-code/examples/internal/host"
	"clara-code/pkg/core"
	"clara-code/pkg/core/prompt"
	modelapi "clara-code/pkg/model"
	"clara-code/pkg/tool"
	"clara-code/pkg/tool/fs/memory"
	"clara-code/pkg/util/errs"
)

func TestCodingWorkflow(t *testing.T) {
	if err := CodingLoop(t.Context(), io.Discard); err != nil {
		t.Fatal(err)
	}
}

func TestPlanAndGoalUseHostHooks(t *testing.T) {
	ctx := t.Context()
	storage := memory.New()
	asked := false
	model := Model{Reply: func(_ context.Context, call modelapi.Call) (modelapi.Response, error) {
		for _, message := range call.Prompt {
			for _, part := range message.Content {
				if text, ok := modelapi.AsMessagePart[modelapi.TextPart](part); ok && strings.Contains(text.Text, "Plan Mode") {
					for _, item := range call.Tools {
						if function, ok := item.(modelapi.FunctionTool); ok && (function.Name == "write" || function.Name == "update_plan") {
							return modelapi.Response{}, errors.New("side-effecting tool advertised in Plan Mode")
						}
					}
					return ToolThenText("request_user_input", `{"questions":[{"id":"choice","text":"Choose scope","options":["Small","Large"]}]}`)(ctx, call)
				}
			}
		}
		return Text("continue"), nil
	}}
	conversation, err := host.Open(ctx, host.Config{Model: model, FS: storage, Ask: func(_ context.Context, items []tool.Question) (map[string]string, error) {
		asked = len(items) == 1 && items[0].ID == "choice"
		return map[string]string{"choice": "Small"}, nil
	}}, "")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := conversation.Close(context.Background()); err != nil {
			t.Error(err)
		}
	})
	run, err := conversation.Start(ctx, prompt.Plan, "Plan an implementation.")
	if err != nil {
		t.Fatal(err)
	}
	result, err := run.Wait(ctx)
	if err != nil || !asked || !strings.Contains(result.Response.Content.Text(), "Small") {
		t.Fatalf("plan question was not answered. Result %v. Error %v", result, err)
	}
	guarded := tool.NewFiles(storage, nil).Write()
	result, err = core.NewAgent(Model{Reply: ToolThenText("write", `{"path":"blocked","content":"test"}`)}, core.WithTools(guarded)).
		Generate(ctx, core.AgentCall{Prompt: "try mutation", AuthorizeTool: tool.PlanOnly})
	if err != nil || !strings.Contains(result.Response.Content.Text(), "Plan Mode") {
		t.Fatalf("tool execution did not enforce Plan Mode. Result %v. Error %v", result, err)
	}
	if _, err := storage.Stat(ctx, "/blocked"); !errs.IsCode(err, errs.CodeNotFound) {
		t.Fatalf("Plan Mode created a file: %v", err)
	}
	if err := conversation.Goal.Start("finish this goal", 29, 5); err != nil {
		t.Fatal(err)
	}
	run, err = conversation.Start(ctx, prompt.Execute, "Work on my goal.")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := run.Wait(ctx); err != nil {
		t.Fatal(err)
	}
	state := conversation.Goal.State()
	if state.TokensUsed != 30 || state.Status != tool.GoalExhausted || state.TurnsUsed != 1 {
		t.Fatalf("unexpected goal state: %+v", state)
	}
}

func TestSessionCheckpointsAndReplays(t *testing.T) {
	ctx := t.Context()
	engine := core.NewAgent(Model{})
	store := &failingStore{Store: core.NewMemoryStore()}
	conversation, err := core.NewSession(ctx, engine, store)
	if err != nil {
		t.Fatal(err)
	}
	store.fail = true
	if _, err := conversation.Start(ctx, core.AgentStreamCall{Prompt: "first attempt"}); !errors.Is(err, errSave) {
		t.Fatalf("save failure: %v", err)
	}
	store.fail = false
	run, err := conversation.Start(ctx, core.AgentStreamCall{Prompt: "CANCEL_SLOW_TURN"})
	if err != nil {
		t.Fatal(err)
	}
	start, err := conversation.NextEvent(ctx, 0)
	if err != nil || start.Sequence != 1 || start.RunID != run.ID {
		t.Fatalf("start checkpoint left a replay gap: %+v %v", start, err)
	}
	run.Cancel()
	if _, err := run.Wait(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled run returned %v", err)
	}
	item, err := conversation.NextEvent(ctx, start.Sequence)
	if err != nil || item.Kind != "run.cancelled" {
		t.Fatalf("cancel event: %+v %v", item, err)
	}
	if err := conversation.Close(ctx); err != nil {
		t.Fatal(err)
	}
	restored, err := core.OpenSession(ctx, engine, store, conversation.ID())
	if err != nil {
		t.Fatal(err)
	}
	event, err := restored.NextEvent(ctx, start.Sequence)
	if err != nil || event.Sequence != item.Sequence {
		t.Fatalf("resume replay: %+v %v", event, err)
	}
}

func TestExecSurvivesToolReturn(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 8*time.Second)
	defer cancel()
	commands := tool.NewCommands(tool.NewLocal())
	t.Cleanup(func() {
		if err := commands.Close(); err != nil {
			t.Error(err)
		}
	})
	toolCtx, finishTool := context.WithCancel(ctx)
	result, err := commands.ExecCommand().Run(toolCtx, modelapi.ToolCall{
		Input: `{"command":"read line; printf '%s' \"$line\"","yield_seconds":1}`,
	})
	finishTool()
	if err != nil || result.IsError {
		t.Fatalf("start background command: %+v %v", result, err)
	}
	line, _, _ := strings.Cut(result.Content, "\n")
	id, ok := strings.CutPrefix(line, "session_id: ")
	if !ok {
		t.Fatalf("missing process ID: %s", result.Content)
	}
	result, err = commands.WriteStdin().Run(ctx, modelapi.ToolCall{Input: `{"session_id":"` + id + `","chars":"still alive\n","yield_seconds":2}`})
	if err != nil || result.IsError || !strings.Contains(result.Content, "still alive") {
		t.Fatalf("wait background command: %+v %v", result, err)
	}
}

var errSave = errors.New("store rejected the checkpoint")

type failingStore struct {
	core.Store
	fail bool
}

func (s *failingStore) Save(ctx context.Context, state core.State) error {
	if s.fail {
		return errSave
	}
	return s.Store.Save(ctx, state)
}
