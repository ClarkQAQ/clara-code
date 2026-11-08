package provider

import (
	"encoding/json"
	"strings"
	"testing"

	"clara-code/pkg/model"
)

func TestDeveloperRoleSurvivesBothProtocols(t *testing.T) {
	policy := model.NewDeveloperMessage("Plan mode; read only")
	policy.Metadata = &model.MessageMetadata{Kind: "context", TurnID: "local-only"}
	prompt := model.Prompt{model.NewSystemMessage("base"), policy, model.NewUserMessage("task")}
	chat, warnings := DefaultToPrompt(prompt, "", "")
	if len(warnings) != 0 {
		t.Fatal(warnings)
	}
	responses, warnings := toResponsesPrompt(prompt, responsesPromptOptions{systemMessageMode: "system"})
	if len(warnings) != 0 {
		t.Fatal(warnings)
	}
	for _, request := range []any{chat, responses} {
		body, err := json.Marshal(request)
		if err != nil {
			t.Fatal(err)
		}
		text := string(body)
		if !strings.Contains(text, `"role":"developer"`) || !strings.Contains(text, `"role":"system"`) || !strings.Contains(text, "Plan mode; read only") {
			t.Fatal(text)
		}
		if strings.Contains(text, "local-only") || strings.Contains(text, "metadata") {
			t.Fatal("host metadata leaked onto wire", text)
		}
	}
	removed, warnings := toResponsesPrompt(model.Prompt{policy}, responsesPromptOptions{systemMessageMode: "remove"})
	if len(removed) != 1 || len(warnings) != 0 {
		t.Fatal("explicit developer policy silently removed")
	}
	if err := validatePreviousResponseIDPrompt(prompt); err != nil {
		t.Fatal(err)
	}
}

func TestInvalidRolesFailBeforeProviderCall(t *testing.T) {
	for _, message := range []model.Message{
		{Role: "unknown"},
		{Role: model.MessageRoleDeveloper, Content: []model.MessagePart{model.FilePart{MediaType: "image/png"}}},
	} {
		if err := validatePromptRoles(model.Prompt{message}); err == nil {
			t.Fatal("invalid role or policy media accepted")
		}
	}
}
