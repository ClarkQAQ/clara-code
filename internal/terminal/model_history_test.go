package terminal

import (
	"strings"
	"testing"

	"clara-code/internal/session"
	"clara-code/internal/setting"
	"clara-code/pkg/core/prompt"
	"clara-code/pkg/model"
)

func TestAgentAndPlanEffortPickerDefaultsCurrentAndAtomicSave(t *testing.T) {
	a := testApp(t)
	a.host.Config.Models["demo"] = setting.Model{ID: "demo", Effort: "high", DefaultReasoningLevel: "high", SupportedReasoningLevels: []model.ReasoningEffort{"low", "high"}}
	if err := a.conv.Store.SetSelection("", "demo", "high", "low"); err != nil {
		t.Fatal(err)
	}
	a.picker = &picker{kind: "model", items: []choice{{id: "demo", label: "demo"}}}
	a.handle(key{code: "enter"})
	rows := strings.Join(a.pane().rows, "\n")
	if !strings.Contains(rows, "Agent (1/2)") || !strings.Contains(rows, "High (default) (current)") {
		t.Fatal("missing Agent markers", rows)
	}
	a.picker.selected = 1
	a.handle(key{code: "enter"})
	rows = strings.Join(a.pane().rows, "\n")
	if !strings.Contains(rows, "Plan (2/2)") || !strings.Contains(rows, "Inherit Agent (default)") || !strings.Contains(rows, "Low (current)") {
		t.Fatal("missing Plan markers", rows)
	}
	if a.conv.Store.Meta().Effort != "high" {
		t.Fatal("first stage committed before Plan confirmation")
	}
	a.handle(key{code: "escape"})
	if a.picker.items[a.picker.selected].id != "low" {
		t.Fatal("Back lost staged Agent choice")
	}
	a.handle(key{code: "enter"})
	a.picker.selected = 0
	a.handle(key{code: "enter"})
	meta := a.conv.Store.Meta()
	if a.picker != nil || meta.Effort != "low" || meta.PlanEffort != "" || a.conv.ModeEffort(prompt.Plan) != "low" {
		t.Fatal("Agent/Plan settings not saved together", meta)
	}
}

func TestCommandRecallIsIndependentOfConversationAndPersists(t *testing.T) {
	a := testApp(t)
	a.submit("/pwd")
	a.editor.replace("/sta")
	a.handle(key{code: "enter"})
	if a.run != nil || len(a.prompts) != 0 || a.conv.Store.TurnCount() != 0 {
		t.Fatal("local command entered model or rewind history")
	}
	a.handle(key{code: "up"})
	if a.editor.text != "/status" {
		t.Fatal("command menu saved partial text", a.editor.text)
	}
	a.handle(key{code: "up"})
	if a.editor.text != "/pwd" {
		t.Fatal("command popup stole history navigation", a.editor.text)
	}
	a.handle(key{code: "down"})
	if a.editor.text != "/status" {
		t.Fatal(a.editor.text)
	}
	a.handle(key{code: "down"})
	if a.editor.text != "" {
		t.Fatal("Down did not restore draft")
	}
	entries, err := session.ReadInputHistory(a.host.Config.Home)
	if err != nil || len(entries) != 2 || entries[1].Text != "/status" {
		t.Fatal("commands not persisted", entries, err)
	}
	a.historyLoaded = false
	a.inputHistory = nil
	a.handle(key{code: "up"})
	if a.editor.text != "/status" {
		t.Fatal("history did not reload", a.editor.text)
	}
}

func TestRecalledRichDraftAndClearExclusion(t *testing.T) {
	a := testApp(t)
	terminalSkill(t, a, "alpha", "First skill")
	a.editor.insertSkill("alpha")
	a.editor.insert(" ")
	a.editor.insertImage(session.Attachment{Filename: "fixture.png"})
	a.syncDraftImages()
	a.recordInput(a.editor.text)
	a.editor.replace("")
	a.attachments = nil
	a.recall("up")
	if len(a.editor.skills) != 1 || len(a.editor.images) != 1 || len(a.attachments) != 1 {
		t.Fatal("recall lost rich draft")
	}
	before := len(a.inputHistory)
	a.recordInput("/clear")
	if len(a.inputHistory) != before {
		t.Fatal("clear was added to recall")
	}
}

func TestEffortDefaultMarkerUsesModelMetadata(t *testing.T) {
	a := testApp(t)
	a.host.Config.Models["demo"] = setting.Model{ID: "demo", Effort: "high", DefaultReasoningLevel: "low", SupportedReasoningLevels: []model.ReasoningEffort{"low", "high"}}
	if err := a.conv.Store.SetSelection("", "demo", "high", ""); err != nil {
		t.Fatal(err)
	}
	a.showEfforts("", "demo", prompt.Execute, "", false)
	rows := strings.Join(a.pane().rows, "\n")
	if !strings.Contains(rows, "Low (default)") || !strings.Contains(rows, "High (current)") || strings.Contains(rows, "High (default)") {
		t.Fatal("configured current effort replaced model default marker", rows)
	}
}
