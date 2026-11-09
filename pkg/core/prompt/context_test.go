package prompt_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"clara-code/pkg/core/prompt"
	"clara-code/pkg/model"
)

type summaryModel struct {
	model.LanguageModel
	response *model.Response
	err      error
	input    *model.Call
}

func (m summaryModel) Generate(_ context.Context, call model.Call) (*model.Response, error) {
	if m.input != nil {
		*m.input = call
	}
	return m.response, m.err
}

func submission(text string) model.Message {
	m := model.NewUserMessage(text)
	m.Metadata = &model.MessageMetadata{Kind: "user", TurnID: text}
	return m
}

func TestAssemblePreservesSteeringAndToolOrder(t *testing.T) {
	steering := model.NewUserMessage("change direction")
	steering.Metadata = &model.MessageMetadata{Kind: "steering"}
	history := []model.Message{submission("old"), submission("current"), {Role: model.MessageRoleAssistant}, {Role: model.MessageRoleTool}, steering}
	fragments := []prompt.Fragment{{Kind: "environment", Source: "host", Scope: "/work", Lifecycle: "step", Role: model.MessageRoleUser, Text: "current clock"}}
	result := prompt.Assemble(history, fragments, model.NewDeveloperMessage("Plan only"))
	if result[0].Role != model.MessageRoleDeveloper || result[2].Metadata.Kind != "context" || prompt.ActiveTurn(history) != 1 {
		t.Fatal("incorrect request ordering", result)
	}
	for i := 1; i < len(history); i++ {
		if result[i+2].Role != history[i].Role {
			t.Fatal("history reordered")
		}
	}
	if history[0].Metadata.Kind != "user" {
		t.Fatal("history mutated")
	}
	body, err := json.Marshal(history)
	if err != nil {
		t.Fatal(err)
	}
	var restored []model.Message
	if err := json.Unmarshal(body, &restored); err != nil {
		t.Fatal(err)
	}
	if prompt.ActiveTurn(restored) != 1 || restored[4].Metadata.Kind != "steering" {
		t.Fatal("metadata not persisted")
	}
}

func TestCompactionKeepsActiveTurnAndMedia(t *testing.T) {
	var request model.Call
	m := summaryModel{input: &request, response: &model.Response{Content: model.ResponseContent{model.TextContent{Text: "Verified findings; unfinished task remains."}}, FinishReason: model.FinishReasonStop}}
	old := submission(strings.Repeat("old", 3000))
	old.Content = append(old.Content, model.FilePart{Filename: "old.png", MediaType: "image/png", Data: []byte("secret-image")})
	current := submission("continue")
	current.Content = append(current.Content, model.FilePart{Filename: "current.png", MediaType: "image/png", Data: []byte("keep-image")})
	steer := model.NewUserMessage("more context")
	steer.Metadata = &model.MessageMetadata{Kind: "steering"}
	history := []model.Message{old, current, steer}
	out, err := (prompt.Compactor{Force: true}).Fit(t.Context(), m, "stable", history, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 3 || out[0].Metadata.Kind != "handoff" || prompt.ActiveTurn(out) != 1 {
		t.Fatal("active turn lost", out)
	}
	image := out[1].Content[1].(model.FilePart)
	if string(image.Data) != "keep-image" {
		t.Fatal("active media changed")
	}
	encoded, _ := json.Marshal(request.Prompt)
	if strings.Contains(string(encoded), "c2VjcmV0LWltYWdl") || !strings.Contains(string(encoded), "old.png") {
		t.Fatal("summary did not describe media")
	}
	if old.Content[1].(model.FilePart).Data == nil {
		t.Fatal("original media mutated")
	}
}

func TestCompactionFailuresPreserveHistory(t *testing.T) {
	for _, tc := range []struct {
		name     string
		response *model.Response
		err      error
	}{
		{"empty", &model.Response{FinishReason: model.FinishReasonStop}, nil},
		{"truncated", &model.Response{Content: model.ResponseContent{model.TextContent{Text: "partial"}}, FinishReason: model.FinishReasonLength}, nil},
		{"network", nil, errors.New("offline")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			history := []model.Message{submission("old"), submission("active")}
			out, err := (prompt.Compactor{Force: true}).Fit(t.Context(), summaryModel{response: tc.response, err: tc.err}, "", history, nil)
			if err == nil || out != nil || history[0].Metadata.Kind != "user" {
				t.Fatal("failed compaction accepted")
			}
		})
	}
}

func TestCompactionRejectsUnpairedTools(t *testing.T) {
	call := model.Message{Role: model.MessageRoleAssistant, Content: []model.MessagePart{model.ToolCallPart{ToolCallID: "one", ToolName: "read"}, model.ToolCallPart{ToolCallID: "two", ToolName: "stat"}}}
	result := model.Message{Role: model.MessageRoleTool, Content: []model.MessagePart{model.ToolResultPart{ToolCallID: "one", Output: model.ToolResultOutputContentText{Text: "done"}}}}
	history := []model.Message{submission("old"), call, result, submission("current")}
	var request model.Call
	m := summaryModel{input: &request, response: &model.Response{Content: model.ResponseContent{model.TextContent{Text: "summary"}}, FinishReason: model.FinishReasonStop}}
	if _, err := (prompt.Compactor{Force: true}).Fit(t.Context(), m, "", history, nil); err == nil || request.Prompt != nil {
		t.Fatal("pending parallel call was split")
	}
	result.Content = append(result.Content, model.ToolResultPart{ToolCallID: "two", Output: model.ToolResultOutputContentText{Text: "done"}})
	history[2] = result
	if _, err := (prompt.Compactor{Force: true}).Fit(t.Context(), m, "", history, nil); err != nil {
		t.Fatal(err)
	}
}

func TestMediaBudgetDoesNotCountBase64(t *testing.T) {
	small := model.NewUserMessage("inspect", model.FilePart{Filename: "image.png", MediaType: "image/png", Data: make([]byte, 10)})
	large := model.NewUserMessage("inspect", model.FilePart{Filename: "image.png", MediaType: "image/png", Data: make([]byte, 1<<20)})
	a, err := prompt.Measure("policy", []model.Message{small}, nil)
	if err != nil {
		t.Fatal(err)
	}
	b, err := prompt.Measure("policy", []model.Message{large}, nil)
	if err != nil || a != b || a.Tokens < 8192 {
		t.Fatal("base64 inflated token estimate", a, b, err)
	}
}

func TestCompactionBudgetAndCancellation(t *testing.T) {
	history := []model.Message{submission(strings.Repeat("old context ", 1000)), submission("latest")}
	var request model.Call
	m := summaryModel{input: &request, response: &model.Response{Content: model.ResponseContent{model.TextContent{Text: "summary"}}, FinishReason: model.FinishReasonStop}}
	before, err := prompt.Measure("policy", history, nil)
	if err != nil {
		t.Fatal(err)
	}
	out, err := (prompt.Compactor{BudgetTokens: before.Tokens / 2}).Fit(t.Context(), m, "policy", history, nil)
	if err != nil || len(out) != 2 {
		t.Fatal("token window did not trigger compaction", err)
	}
	if _, err := (prompt.Compactor{Force: true, BudgetTokens: 1}).Fit(t.Context(), m, "policy", history, nil); err == nil {
		t.Fatal("over-budget summary accepted")
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if out, err := (prompt.Compactor{Force: true}).Fit(ctx, m, "policy", history, nil); !errors.Is(err, context.Canceled) || out != nil {
		t.Fatal("canceled summary accepted", err)
	}
	request = model.Call{}
	if out, err := (prompt.Compactor{BudgetTokens: before.Tokens + 100, UsageTokens: before.Tokens + 1000, UsageEstimate: before.Tokens}).Fit(t.Context(), m, "policy", history, nil); err != nil || out == nil || request.Prompt == nil {
		t.Fatal("actual usage did not calibrate estimate", err)
	}
}

func TestLegacyCompactionAndNoCompletedTurn(t *testing.T) {
	m := summaryModel{response: &model.Response{Content: model.ResponseContent{model.TextContent{Text: "summary"}}, FinishReason: model.FinishReasonStop}}
	legacy := []model.Message{model.NewUserMessage("Prior conversation handoff:\nold"), model.NewUserMessage("previous"), model.NewUserMessage("latest")}
	out, err := (prompt.Compactor{Force: true}).Fit(t.Context(), m, "", legacy, nil)
	if err != nil || len(out) != 2 {
		t.Fatal("legacy transcript not supported", err)
	}
	if out, err := (prompt.Compactor{Force: true}).Fit(t.Context(), m, "", []model.Message{submission("only turn")}, nil); err == nil || out != nil {
		t.Fatal("latest turn was summarized away")
	}
}

func TestEarlyCompactionTriggerIsSeparateFromHardBudget(t *testing.T) {
	history := []model.Message{submission(strings.Repeat("old context ", 1000)), submission("latest")}
	before, err := prompt.Measure("policy", history, nil)
	if err != nil {
		t.Fatal(err)
	}
	m := summaryModel{response: &model.Response{Content: model.ResponseContent{model.TextContent{Text: "summary"}}, FinishReason: model.FinishReasonStop}}
	out, err := (prompt.Compactor{BudgetTokens: before.Tokens + 100, TriggerTokens: 1}).Fit(t.Context(), m, "policy", history, nil)
	if err != nil || len(out) != 2 {
		t.Fatal("early trigger was incorrectly treated as the hard budget", err)
	}
	out, err = (prompt.Compactor{BudgetTokens: before.Tokens + 100, TriggerTokens: before.Tokens + 1}).Fit(t.Context(), m, "policy", history, nil)
	if err != nil || out != nil {
		t.Fatal("compacted below trigger", err)
	}
}
