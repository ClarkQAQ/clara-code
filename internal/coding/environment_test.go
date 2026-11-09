package coding

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"

	"clara-code/internal/session"
	"clara-code/pkg/core"
	"clara-code/pkg/core/prompt"
	"clara-code/pkg/model"
)

func TestEnvironmentDoesNotUpdateForPassingSeconds(t *testing.T) {
	var calls int
	var prefix []model.Message
	m := fixtureModel{reply: func(call model.Call) (*model.Response, error) {
		calls++
		body := contextText(call.Prompt)
		if strings.Count(body, "<environment_context>") != 1 || strings.Contains(body, "<timestamp>") || strings.Contains(body, "<local_datetime>") || strings.Contains(body, "<message_context>") {
			t.Error("tool continuation generated a clock update", body)
		}
		if calls == 1 {
			prefix = call.Prompt
			return toolReply("stat", map[string]any{"path": "clock.txt"}, 1), nil
		}
		for i, message := range prefix {
			before, _ := json.Marshal(message)
			after, _ := json.Marshal(call.Prompt[i])
			if string(before) != string(after) {
				t.Errorf("tool continuation changed prefix message %d: before %s, after %s", i, before, after)
			}
		}
		return demoText("Done."), nil
	}}
	c, _ := testHost(t, t.TempDir(), m)
	now := time.Date(2026, 11, 9, 10, 2, 3, 0, time.UTC)
	c.Host.now = func() time.Time { now = now.Add(time.Second); return now }
	writeTestFile(t, c.Workspace.Cwd, "clock.txt", "fixture")
	finish(t, c, prompt.Execute, "Read clock.txt metadata.")
	if calls != 2 {
		t.Fatal("expected a tool continuation", calls)
	}
	for _, message := range c.Session.Messages() {
		if strings.Contains(session.Text(message), "<environment_context>") {
			t.Fatal("environment entered transcript")
		}
	}
}

func TestLegacyEnvironmentMigrationPreservesMixedChangesAndAnchors(t *testing.T) {
	c, _ := testHost(t, t.TempDir(), DemoModel{})
	history := []model.Message{model.NewUserMessage("first"), {Role: model.MessageRoleAssistant}, model.NewUserMessage("second")}
	fields := c.environmentFragments(false)
	legacy := func(name string) prompt.Fragment {
		return prompt.Fragment{Kind: "environment", Source: name, Role: model.MessageRoleUser, Text: "obsolete-" + name}
	}
	for _, name := range []string{"timestamp", "local_datetime", "project_root", "model", "image_input", "effective_context_window"} {
		fields = append(fields, legacy(name))
	}
	_, window, err := (prompt.Window{}).Prepare(history[:1], fields)
	if err != nil {
		t.Fatal(err)
	}
	fields = append(fields, prompt.Fragment{Kind: "workspace_instructions", Source: "/repo/AGENTS.md", Scope: "/repo", Role: model.MessageRoleUser, Text: "MIXED-RULE"})
	fields[len(fields)-2].Text = "updated-window"
	_, window, err = window.Prepare(history, fields)
	if err != nil {
		t.Fatal(err)
	}
	// A later clock-only entry must disappear, while the mixed entry stays at 2.
	window.Entries = append(window.Entries, prompt.ContextEntry{At: 3, Changes: []prompt.Change{{Fragment: legacy("timestamp")}}})
	original, err := window.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	migrated := migrateEnvironmentWindow(window)
	after, _ := window.Marshal()
	if string(after) != string(original) || !migrated.Matches(history) || len(migrated.Entries) != 2 || migrated.Entries[1].At != 2 {
		t.Fatal("migration mutated its input or changed transcript anchors")
	}
	for _, entry := range migrated.Entries {
		for _, change := range entry.Changes {
			if strings.HasPrefix(change.Fragment.Text, "obsolete-") || change.Fragment.Text == "updated-window" {
				t.Fatal("legacy field survived migration")
			}
		}
	}
	_, result, err := c.Call(prompt.Execute, false).PrepareStep(t.Context(), core.PrepareStepFunctionOptions{Messages: history, ContextState: original})
	if err != nil {
		t.Fatal(err)
	}
	body := contextText(result.Messages)
	if !strings.Contains(body, "MIXED-RULE") || strings.Contains(body, "obsolete-") || strings.Contains(body, "<timestamp>") || strings.Contains(body, "<local_datetime>") || strings.Contains(body, "<project_root>") || strings.Contains(body, "environment_context>\n</environment_context") {
		t.Fatal("legacy window not cleaned in model request", body)
	}
	var candidate prompt.Window
	if err := json.Unmarshal(result.ContextState, &candidate); err != nil || !candidate.Matches(history) {
		t.Fatal("migrated checkpoint invalid", err)
	}
	again := migrateEnvironmentWindow(migrated)
	if !reflect.DeepEqual(migrated, again) {
		t.Fatal("migration is not idempotent")
	}
}
