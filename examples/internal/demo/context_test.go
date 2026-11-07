package demo

import (
	"context"
	"strings"
	"testing"
	"time"

	"clara-code/pkg/core"
	"clara-code/pkg/core/prompt"
	modelapi "clara-code/pkg/model"
)

func TestCompactionRetainsLatestToolPair(t *testing.T) {
	latest := []modelapi.Message{
		modelapi.NewUserMessage("Fix the current issue."),
		{Role: modelapi.MessageRoleAssistant, Content: []modelapi.MessagePart{modelapi.ToolCallPart{ToolCallID: "current", ToolName: "read", Input: "{}"}}},
		{Role: modelapi.MessageRoleTool, Content: []modelapi.MessagePart{modelapi.ToolResultPart{ToolCallID: "current", Output: modelapi.ToolResultOutputContentText{Text: "current result"}}}},
	}
	history := append([]modelapi.Message{modelapi.NewUserMessage(strings.Repeat("old history ", 300))}, latest...)
	model := Model{Reply: Static("Prior decisions and checks.")}
	replacement, err := (prompt.Compactor{BudgetBytes: 2000}).Fit(t.Context(), model, "system", history, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(replacement) != len(latest)+1 {
		t.Fatalf("wrong compacted transcript length: %d", len(replacement))
	}
	call, callOK := modelapi.AsMessagePart[modelapi.ToolCallPart](replacement[2].Content[0])
	result, resultOK := modelapi.AsMessagePart[modelapi.ToolResultPart](replacement[3].Content[0])
	if !callOK || !resultOK || call.ToolCallID != result.ToolCallID {
		t.Fatal("compaction separated a tool call from its result")
	}
}

func TestRunConsumesSteeringAndFollowUp(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	started, release := make(chan struct{}), make(chan struct{})
	requests := 0
	model := Model{Reply: func(ctx context.Context, call modelapi.Call) (modelapi.Response, error) {
		requests++
		if requests == 1 {
			close(started)
			select {
			case <-ctx.Done():
				return modelapi.Response{}, ctx.Err()
			case <-release:
				return Text("first response"), nil
			}
		}
		result, err := (Model{}).Generate(ctx, call)
		if err != nil {
			return modelapi.Response{}, err
		}
		return *result, nil
	}}
	conversation, err := core.NewSession(ctx, core.NewAgent(model), nil)
	if err != nil {
		t.Fatal(err)
	}
	run, err := conversation.Start(ctx, core.AgentStreamCall{Prompt: "start"})
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-started:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	if err := run.Steer(modelapi.NewUserMessage("steer")); err != nil {
		t.Fatal(err)
	}
	if err := run.FollowUp(modelapi.NewUserMessage("follow")); err != nil {
		t.Fatal(err)
	}
	close(release)
	result, err := run.Wait(ctx)
	if err != nil || result.Response.Content.Text() != "Demo response: follow" {
		t.Fatalf("unexpected run result %v %v", result, err)
	}
	history := conversation.Messages()
	if len(history) != 5 {
		t.Fatalf("lost or duplicated input in history: %+v", history)
	}
}
