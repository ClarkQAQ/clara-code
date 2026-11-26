package prompt_test

import (
	"encoding/json"
	"strings"
	"testing"

	"clara-code/pkg/core/prompt"
	"clara-code/pkg/model"
)

func TestMeasureUsesVisibleContentAndKeepsSerializedByteBudget(t *testing.T) {
	message := model.NewUserMessage("<\n>\"")
	message.Metadata = &model.MessageMetadata{Kind: "user", TurnID: strings.Repeat("id", 500)}
	tool := model.Message{Role: model.MessageRoleTool, Content: []model.MessagePart{model.ToolResultPart{
		ToolCallID: "a", ClientMetadata: strings.Repeat("private", 1000),
		Output: model.ToolResultOutputContentText{Text: "回复!"},
	}}}
	history := []model.Message{message, tool}
	raw, err := json.Marshal([]any{"system", history, []model.ToolInfo(nil)})
	if err != nil {
		t.Fatal(err)
	}
	got, err := prompt.Measure("system", history, nil)
	if err != nil || got.Tokens != 5 || got.Bytes != len(raw) {
		t.Fatalf("estimate = %+v, want 5 tokens and %d bytes: %v", got, len(raw), err)
	}
}

func TestMeasureImagesAndToolDefinitions(t *testing.T) {
	file := &model.FilePart{MediaType: "image/png", Data: []byte("encoded separately")}
	result := model.ToolResultPart{Output: &model.ToolResultOutputContentMedia{MediaType: "image/webp", Data: strings.Repeat("A", 64<<10)}}
	history := []model.Message{{Role: model.MessageRoleUser, Content: []model.MessagePart{file}}, {Role: model.MessageRoleTool, Content: []model.MessagePart{&result}}}
	got, err := prompt.Measure("", history, nil)
	if err != nil || got.Tokens != 2*1844 || got.Bytes > 4096 {
		t.Fatalf("image estimate = %+v: %v", got, err)
	}
	schema := map[string]any{"type": "object"}
	def := model.ToolInfo{Name: "read", Description: "file", InputSchema: schema, Parameters: map[string]any{"ignored": strings.Repeat("x", 8192)}}
	withTool, err := prompt.Measure("", history, []model.ToolInfo{def})
	if err != nil || withTool.Tokens-got.Tokens != 7 {
		t.Fatalf("tool estimate = %+v: %v", withTool, err)
	}
}
