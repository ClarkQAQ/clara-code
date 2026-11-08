package terminal

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func terminalSkill(t *testing.T, a *app, name, description string) {
	t.Helper()
	path := filepath.Join(a.space.Cwd, ".agents", "skills", name)
	if err := os.MkdirAll(path, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(path, "SKILL.md"), []byte("---\nname: "+name+"\ndescription: "+description+"\n---\nFollow this workflow."), 0600); err != nil {
		t.Fatal(err)
	}
}

func TestSkillMentionSelectionPreservesSurroundingText(t *testing.T) {
	a := testApp(t)
	terminalSkill(t, a, "alpha", "First skill")
	terminalSkill(t, a, "beta", "Second skill")
	a.editor.replace("前面 $al 后面")
	a.editor.at = len("前面 $al")
	if !a.skillMatches() || len(a.skillMenu.matches) != 1 || a.skillMenu.matches[0].Name != "alpha" {
		t.Fatal("mention not filtered")
	}
	pane := a.pane()
	if pane.cursor == nil || !strings.Contains(strings.Join(pane.rows, "\n"), "[Skill] First skill") {
		t.Fatal("skill menu lost cursor or description", pane)
	}
	a.handle(key{code: "enter"})
	if a.editor.text != "前面 $alpha 后面" || a.run != nil {
		t.Fatal("selection submitted or damaged surrounding text", a.editor.text)
	}
	a.editor.replace("$b")
	a.handle(key{code: "escape"})
	if a.skillMatches() {
		t.Fatal("Escape did not dismiss mention menu")
	}
	a.handle(key{code: "text", text: "e"})
	if !a.skillMatches() {
		t.Fatal("editing did not reopen menu")
	}
	a.handle(key{code: "tab"})
	if a.editor.text != "$beta " {
		t.Fatal(a.editor.text)
	}
	for _, text := range []string{"$HOME", "${HOME}", "$(pwd)", "cost$alpha", "$$"} {
		a.editor.replace(text)
		if a.skillMatches() {
			t.Fatal("shell or ordinary text opened skill menu", text)
		}
	}
}

func TestSkillsManagementMenuToggleAndDraft(t *testing.T) {
	a := testApp(t)
	terminalSkill(t, a, "alpha", "First skill")
	terminalSkill(t, a, "beta", "Second skill")
	a.editor.replace("keep draft")
	a.skillsMenu()
	rows := strings.Join(a.pane().rows, "\n")
	if !strings.Contains(rows, "List skills") || !strings.Contains(rows, "Enable/Disable Skills") || !strings.Contains(rows, "Tip: press $") || a.pane().cursor != nil {
		t.Fatal("management menu mismatch", rows)
	}
	a.handle(key{code: "text", text: "2"})
	if a.picker.kind != "skill-toggle" || a.pane().cursor == nil {
		t.Fatal("toggle search must have a real cursor")
	}
	a.handle(key{code: "text", text: "beta"})
	a.handle(key{code: "text", text: " "})
	skills, err := a.conv.Skills(t.Context())
	if err != nil || len(skills) != 1 || skills[0].Name != "alpha" {
		t.Fatal("toggle did not affect catalog", skills, err)
	}
	a.handle(key{code: "escape"})
	if a.picker.kind != "skills" {
		t.Fatal("toggle back did not return to management menu")
	}
	a.handle(key{code: "escape"})
	if a.editor.text != "keep draft" {
		t.Fatal("manager discarded draft", a.editor.text)
	}
	a.editor.replace("$")
	if !a.skillMatches() || len(a.skillMenu.matches) != 1 {
		t.Fatal("disabled skill is still offered")
	}
	a.handle(key{code: "escape"})
	a.editor.replace("")
	a.skillsMenu()
	a.handle(key{code: "enter"})
	if a.picker != nil || a.editor.text != "$" || !a.skillMatches() {
		t.Fatal("List skills did not open inline mention menu")
	}
	if len(a.view.history) != 0 {
		t.Fatal("transient skills UI leaked into history")
	}
}

func TestSkillLayoutsRespectResizeAndKeepInputCursor(t *testing.T) {
	a := testApp(t)
	terminalSkill(t, a, "long_skill_name", "A long description for narrow terminals")
	for _, width := range []int{8, 24, 60, 120} {
		for _, height := range []int{1, 3, 8, 24} {
			a.view.width, a.view.height = width, height
			a.picker = nil
			a.editor.replace("$")
			pane := a.pane().slice(0, height)
			for _, row := range pane.rows {
				if widthOf(row) > width {
					t.Fatalf("menu exceeds width %d: %q", width, row)
				}
			}
			if pane.cursor == nil {
				t.Fatal("mention cursor was clipped", width, height)
			}
			a.manageSkills()
			pane = a.pane().slice(0, height)
			for _, row := range pane.rows {
				if widthOf(row) > width {
					t.Fatalf("manager exceeds width %d: %q", width, row)
				}
			}
			if pane.cursor == nil {
				t.Fatal("search cursor was clipped", width, height)
			}
		}
	}
}
