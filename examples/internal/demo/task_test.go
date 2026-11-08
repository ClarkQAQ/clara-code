package demo

import (
	"context"
	"strings"
	"testing"
	"time"

	"clara-code/pkg/core"
	"clara-code/pkg/model"
	"clara-code/pkg/tool"
	"clara-code/pkg/tool/fs/memory"
	"clara-code/pkg/util/errs"
)

func TestTaskScopeClampsToolsAndVisibility(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	storage := memory.New()
	write := tool.NewFiles(storage, nil).Write()
	manager := tool.NewTaskManager(func(context.Context, tool.TaskSpec, *tool.TaskScope) (tool.TaskSetup, error) {
		return tool.TaskSetup{
			Model:            Model{Reply: ToolThenText("write", `{"path":"escaped","content":"data"}`)},
			Tools:            []tool.Tool{write},
			DynamicToolNames: []string{"write"},
			PrepareStep: func(ctx context.Context, _ core.PrepareStepFunctionOptions) (context.Context, core.PrepareStepResult, error) {
				return ctx, core.PrepareStepResult{Tools: []core.AgentTool{write}}, nil
			},
		}, nil
	}, 4, 3)
	scope := manager.Scope(ctx, "main", tool.ScopeOptions{AllowedTools: []string{"read"}})
	other := manager.Scope(ctx, "other", tool.ScopeOptions{})
	t.Cleanup(func() {
		if err := scope.Close(context.Background()); err != nil {
			t.Error(err)
		}
		if err := other.Close(context.Background()); err != nil {
			t.Error(err)
		}
	})
	task, err := scope.Spawn(ctx, tool.TaskSpec{Name: "check", Role: "reviewer", Message: "Try a write."})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := other.Wait(ctx, task.ID, time.Second); !errs.IsCode(err, errs.CodeNotFound) {
		t.Fatalf("unrelated task could access child: %v", err)
	}
	info, err := scope.Wait(ctx, task.ID, time.Second)
	if err != nil || info.Status != "completed" || !strings.Contains(info.Result, "tool") {
		t.Fatalf("unexpected task result %+v %v", info, err)
	}
	if _, err := storage.Stat(ctx, "/escaped"); !errs.IsCode(err, errs.CodeNotFound) {
		t.Fatalf("child exceeded parent tool permissions: %v", err)
	}
	if err := scope.Send(ctx, task.ID, "Follow up.", true); err != nil {
		t.Fatal(err)
	}
	if _, err := scope.Wait(ctx, task.ID, time.Second); err != nil {
		t.Fatal(err)
	}
}

func TestRecoveryDoesNotDispatchAnInterruptedTool(t *testing.T) {
	// Simulate the durable boundary between a model response and tool execution.
	ctx := t.Context()
	store := core.NewMemoryStore()
	call := model.ToolCallPart{ToolCallID: "pending", ToolName: "edit", Input: "{}"}
	staged := model.Message{Role: model.MessageRoleAssistant, Content: []model.MessagePart{
		model.TextPart{Text: "I found the problem."}, call,
	}}
	err := store.Save(ctx, core.State{
		ID: "recovery", Revision: 1, ActiveRun: "interrupted",
		Messages: []model.Message{model.NewUserMessage("Fix the issue.")},
		Staged:   []model.Message{staged}, Pending: []core.Pending{{Call: call}},
	})
	if err != nil {
		t.Fatal(err)
	}
	conversation, err := core.OpenSession(ctx, core.NewAgent(Model{}), store, "recovery")
	if err != nil {
		t.Fatal(err)
	}
	messages := conversation.Messages()
	if len(messages) != 3 || len(messages[1].Content) != 2 || messages[2].Role != model.MessageRoleTool {
		t.Fatalf("recovered transcript lost its tool pair or preceding text: %+v", messages)
	}
	result, ok := model.AsMessagePart[model.ToolResultPart](messages[2].Content[0])
	if !ok || result.ToolCallID != call.ToolCallID || result.Output.GetType() != model.ToolResultContentTypeError {
		t.Fatalf("interrupted call has no error result: %+v", result)
	}
}
