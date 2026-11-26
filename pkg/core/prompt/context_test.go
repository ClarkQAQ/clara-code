package prompt_test

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"slices"
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
	if len(request.Prompt) != 2 || request.Prompt[0].Role != model.MessageRoleDeveloper || strings.TrimSpace(windowText(request.Prompt[:1])) != prompt.Default("compact") || request.Prompt[1].Role != model.MessageRoleUser {
		t.Fatal("summarization instructions and transcript have incorrect roles")
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
	folded := []model.Message{model.NewUserMessage("Prior conversation handoff:\nold"), submission("latest")}
	out, err = (prompt.Compactor{Force: true}).Fit(t.Context(), m, "", folded, nil)
	if err != nil || len(out) != 2 || out[0].Metadata.Kind != "handoff" || !reflect.DeepEqual(out[1], folded[1]) {
		t.Fatalf("explicit compaction of folded history failed: %v", err)
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

func TestLongActiveTurnCompactionKeepsInstructionsAndToolBoundaries(t *testing.T) {
	oldResult := strings.Repeat("large historical tool result ", 100)
	task := submission("original task")
	task.Content = append(task.Content, model.FilePart{Filename: "task.png", MediaType: "image/png", Data: []byte("original-media")})
	steer := model.NewUserMessage("changed requirement")
	steer.Metadata = &model.MessageMetadata{Kind: "steering"}
	followup := model.NewUserMessage("also verify")
	followup.Metadata = &model.MessageMetadata{Kind: "followup"}
	call := func(ids ...string) model.Message {
		var parts []model.MessagePart
		for _, id := range ids {
			parts = append(parts, model.ToolCallPart{ToolCallID: id, ToolName: "read", Input: "{}"})
		}
		return model.Message{Role: model.MessageRoleAssistant, Content: parts}
	}
	result := func(id, text string) model.Message {
		return model.Message{Role: model.MessageRoleTool, Content: []model.MessagePart{
			model.ToolResultPart{ToolCallID: id, Output: model.ToolResultOutputContentText{Text: text}},
		}}
	}
	history := []model.Message{task, call("first", "second"), result("second", oldResult), result("first", "ok"), steer,
		call("next"), result("next", oldResult), followup, call("latest"), result("latest", "current"), call("pending", "also-pending"), result("pending", "done")}
	// A prior turn gives the compactor two candidates. Only the active-turn
	// candidate frees enough space, and the summary model is called once.
	historyWithOld := append([]model.Message{submission("short old turn")}, history...)
	measured := 0
	mixed := prompt.Compactor{BudgetBytes: 2000, AssembleCompacted: func(candidate []model.Message, at []int) ([]model.Message, error) {
		measured++
		if len(at) != len(historyWithOld)+1 || !slices.IsSorted(at) || at[len(historyWithOld)] != len(candidate) {
			t.Fatalf("invalid candidate boundaries: %v", at)
		}
		return candidate, nil
	}}
	mixedModel := summaryModel{response: &model.Response{FinishReason: model.FinishReasonStop, Content: model.ResponseContent{model.TextContent{Text: "summary"}}}}
	mixedOut, err := mixed.Fit(t.Context(), mixedModel, "", historyWithOld, nil)
	if err != nil || len(mixedOut) != 8 || measured != 2 || !reflect.DeepEqual(mixedOut[1], task) || !reflect.DeepEqual(mixedOut[4:], history[8:]) {
		t.Fatalf("combined old and active prefix compaction: measured %d replacements, error %v", measured, err)
	}
	before, err := prompt.Measure("", history, nil)
	if err != nil {
		t.Fatal(err)
	}
	var request model.Call
	m := summaryModel{input: &request, response: &model.Response{FinishReason: model.FinishReasonStop, Content: model.ResponseContent{model.TextContent{Text: "handoff one"}}}}
	var anchors []int
	var final []model.Message
	compactor := prompt.Compactor{BudgetBytes: before.Bytes / 2, AssembleCompacted: func(candidate []model.Message, at []int) ([]model.Message, error) {
		final, anchors = candidate, at
		return candidate, nil
	}}
	out, err := compactor.Fit(t.Context(), m, "", history, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 8 || &final[0] != &out[0] || out[1].Metadata.Kind != "handoff" || !reflect.DeepEqual(out[0], task) ||
		!reflect.DeepEqual(out[2], steer) || !reflect.DeepEqual(out[3], followup) || !reflect.DeepEqual(out[4:], history[8:]) {
		t.Fatalf("unexpected retained transcript: %+v", out)
	}
	if want := []int{0, 1, 1, 1, 2, 3, 3, 3, 4, 5, 6, 7, 8}; !slices.Equal(anchors, want) {
		t.Fatalf("anchors = %v, want %v", anchors, want)
	}
	body := windowText(request.Prompt[1:])
	if !strings.Contains(body, oldResult) || strings.Contains(body, "also-pending") || strings.Contains(body, "b3JpZ2luYWwtbWVkaWE=") {
		t.Fatal("summarizer did not receive just the safe prefix and media descriptions")
	}
	extended := append(slices.Clone(out), result("also-pending", oldResult), call("final"), result("final", "most recent"))
	m.response.Content = model.ResponseContent{model.TextContent{Text: "handoff two"}}
	out, err = (prompt.Compactor{Force: true}).Fit(t.Context(), m, "", extended, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 6 || !reflect.DeepEqual(out[0], task) || out[1].Metadata.Kind != "handoff" ||
		!reflect.DeepEqual(out[2], steer) || !reflect.DeepEqual(out[3], followup) || !reflect.DeepEqual(out[4:], extended[len(extended)-2:]) {
		t.Fatal("repeated compaction lost inputs or accumulated previous handoffs")
	}
	if body = windowText(request.Prompt[1:]); !strings.Contains(body, "handoff one") || !strings.Contains(body, oldResult) {
		t.Fatal("second summary missed prior progress")
	}
}
