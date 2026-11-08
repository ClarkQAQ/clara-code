package prompt_test

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"clara-code/pkg/core/prompt"
	"clara-code/pkg/model"
)

func windowText(messages []model.Message) string {
	var out strings.Builder
	for _, message := range messages {
		for _, part := range message.Content {
			if text, ok := model.AsMessagePart[model.TextPart](part); ok {
				out.WriteString(text.Text)
				out.WriteByte('\n')
			}
		}
	}
	return out.String()
}

func windowFragments(stamp string) []prompt.Fragment {
	return []prompt.Fragment{
		{Kind: "collaboration_mode", Source: "host", Role: model.MessageRoleDeveloper, Text: "Agent mode"},
		{Kind: "workspace_instructions", Source: "/repo/AGENTS.md", Scope: "/repo", Role: model.MessageRoleUser, Text: "Retain the complete directory rule."},
		{Kind: "environment", Source: "cwd", Role: model.MessageRoleUser, Text: "/repo"},
		{Kind: "environment", Source: "timestamp", Role: model.MessageRoleUser, Text: stamp},
		{Kind: "background_terminals", Source: "host", Role: model.MessageRoleUser},
	}
}

func TestWindowKeepsInitialPrefixAndOnlyAppendsChanges(t *testing.T) {
	history := []model.Message{submission("first task")}
	first, window, err := (prompt.Window{}).Prepare(history, windowFragments("first time"))
	if err != nil {
		t.Fatal(err)
	}
	if len(first) != 3 || first[0].Role != model.MessageRoleDeveloper || len(first[1].Content) != 2 {
		t.Fatal("initial blocks not grouped", len(first))
	}
	text := windowText(first)
	if strings.Contains(text, "context_fragment") || strings.Contains(text, `"kind"`) || strings.Contains(text, "background_terminals") || !strings.Contains(text, "# AGENTS.md instructions for /repo") {
		t.Fatal("management metadata or empty state entered prompt", text)
	}
	history = append(history, model.Message{Role: model.MessageRoleAssistant, Content: []model.MessagePart{model.TextPart{Text: "answer"}}}, submission("next task"))
	second, next, err := window.Prepare(history, windowFragments("second time"))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(first, second[:len(first)]) {
		t.Fatal("prior request prefix changed")
	}
	text = windowText(second)
	if strings.Count(text, "Retain the complete directory rule.") != 1 || strings.Count(text, "<cwd>") != 1 || strings.Count(text, "<environment_context>") != 2 {
		t.Fatal("unchanged context repeated", text)
	}
	if len(second[len(second)-2].Content) != 1 || !strings.Contains(windowText(second[len(second)-2:]), "<timestamp>second time</timestamp>") {
		t.Fatal("environment diff is not minimal")
	}
	raw, err := next.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	restored, err := prompt.LoadWindow(raw)
	if err != nil {
		t.Fatal(err)
	}
	again, _, err := restored.Prepare(history, windowFragments("second time"))
	if err != nil || !reflect.DeepEqual(second, again) {
		t.Fatal("restored window changed request", err)
	}
	if len(window.Entries) != 1 {
		t.Fatal("Prepare modified previous window")
	}
}

func TestWindowConsolidatesDeveloperTextWithoutLosingIndependentChanges(t *testing.T) {
	fragments := windowFragments("fixed")
	fragments = append([]prompt.Fragment{{Kind: "permissions", Source: "host", Role: model.MessageRoleDeveloper, Text: "Permission policy"}}, fragments...)
	history := []model.Message{submission("task")}
	first, window, err := (prompt.Window{}).Prepare(history, fragments)
	if err != nil {
		t.Fatal(err)
	}
	if len(first[0].Content) != 1 || !strings.Contains(windowText(first[:1]), "Permission policy\n\nAgent mode") {
		t.Fatal("developer sections not combined")
	}
	fragments[1].Text = "Plan mode"
	history = append(history, model.Message{Role: model.MessageRoleAssistant}, submission("change mode"))
	next, _, err := window.Prepare(history, fragments)
	if err != nil || !reflect.DeepEqual(first, next[:len(first)]) {
		t.Fatal("merge destabilized old prefix", err)
	}
	if strings.Count(windowText(next), "Permission policy") != 1 || strings.Count(windowText(next), "Plan mode") != 1 {
		t.Fatal("merge lost per-section diffing")
	}
}

func TestWindowRuleReplacementRemovalAndModeSwitch(t *testing.T) {
	history := []model.Message{submission("work")}
	fragments := windowFragments("fixed")
	_, window, err := (prompt.Window{}).Prepare(history, fragments)
	if err != nil {
		t.Fatal(err)
	}
	history = append(history, model.Message{Role: model.MessageRoleAssistant}, submission("plan instead"))
	fragments[0].Text = "Plan mode: read only"
	fragments[1].Text = "New scoped rule"
	request, next, err := window.Prepare(history, fragments)
	if err != nil {
		t.Fatal(err)
	}
	text := windowText(request)
	if !strings.Contains(text, "replaces the previous") || strings.Count(text, "New scoped rule") != 1 || !strings.Contains(text, "Plan mode: read only") {
		t.Fatal("replacement missing", text)
	}
	history = append(history, model.Message{Role: model.MessageRoleAssistant}, submission("continue"))
	fragments = append(fragments[:1:1], fragments[2:]...)
	request, _, err = next.Prepare(history, fragments)
	if err != nil || !strings.Contains(windowText(request), "no longer applies") {
		t.Fatal("removal not communicated", err)
	}
	request, _, err = (prompt.Window{}).Prepare(history, fragments)
	if err != nil || strings.Contains(windowText(request), "New scoped rule") || strings.Contains(windowText(request), "Retain the complete") {
		t.Fatal("rebased window kept old rules", err)
	}
}

func TestWindowDoesNotSplitToolsAndResetsForForkedHistory(t *testing.T) {
	history := []model.Message{submission("work")}
	_, window, err := (prompt.Window{}).Prepare(history, windowFragments("one"))
	if err != nil {
		t.Fatal(err)
	}
	history = append(history, model.Message{Role: model.MessageRoleAssistant, Content: []model.MessagePart{model.ToolCallPart{ToolCallID: "call", ToolName: "read"}}})
	if _, _, err := window.Prepare(history, windowFragments("two")); err == nil {
		t.Fatal("inserted context inside pending call")
	}
	history = append(history, model.Message{Role: model.MessageRoleTool, Content: []model.MessagePart{model.ToolResultPart{ToolCallID: "call", Output: model.ToolResultOutputContentText{Text: "done"}}}})
	request, next, err := window.Prepare(history, windowFragments("two"))
	if err != nil {
		t.Fatal(err)
	}
	if request[len(request)-2].Role != model.MessageRoleTool || request[len(request)-1].Metadata.Kind != "context" {
		t.Fatal("context split tool pair")
	}
	_, rebased, err := next.Prepare([]model.Message{submission("different branch")}, windowFragments("three"))
	if err != nil || len(rebased.Entries) != 1 || rebased.Entries[0].At != 0 {
		t.Fatal("fork did not rebuild prefix", err)
	}
	bad, _ := json.Marshal(prompt.Window{Version: 1, MessageCount: 1, Entries: []prompt.ContextEntry{{At: 2}}})
	if _, err := prompt.LoadWindow(bad); err == nil {
		t.Fatal("invalid anchor accepted")
	}
}

func TestWindowCompactionSeesHistoricalContextWithoutManagementMetadata(t *testing.T) {
	history := []model.Message{submission("yesterday's task")}
	_, window, err := (prompt.Window{}).Prepare(history, windowFragments("old clock"))
	if err != nil {
		t.Fatal(err)
	}
	history = append(history, model.Message{Role: model.MessageRoleAssistant}, submission("today's task"))
	_, window, err = window.Prepare(history, windowFragments("current clock"))
	if err != nil {
		t.Fatal(err)
	}
	var request model.Call
	m := summaryModel{input: &request, response: &model.Response{Content: model.ResponseContent{model.TextContent{Text: "Historical task completed; current work remains."}}, FinishReason: model.FinishReasonStop}}
	out, err := (prompt.Compactor{Force: true, Transcript: window.Prefix}).Fit(t.Context(), m, "base", history, nil)
	if err != nil || len(out) != 2 {
		t.Fatal("historical compaction failed", err)
	}
	text := windowText(request.Prompt[1:])
	if !strings.Contains(text, "old clock") || strings.Contains(text, "current clock") || strings.Contains(text, `\"kind\"`) || strings.Contains(text, `"metadata"`) {
		t.Fatal("summary got wrong clock or internal metadata", text)
	}
}
