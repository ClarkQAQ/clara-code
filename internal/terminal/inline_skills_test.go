package terminal

import (
	"strings"
	"testing"

	"clara-code/internal/session"
)

func TestSelectedSkillHighlightAndAtomicEditing(t *testing.T) {
	a := testApp(t)
	terminalSkill(t, a, "alpha", "First skill")
	a.view.color = true
	a.editor.replace("前面 $al 后面")
	a.editor.at = len("前面 $al")
	a.handle(key{code: "enter"})
	if len(a.editor.skills) != 1 {
		t.Fatal("selection did not create an atomic skill")
	}
	rows := strings.Join(a.pane().rows, "\n")
	if !strings.Contains(rows, accent+"$alpha"+reset) {
		t.Fatal("selected skill is not highlighted", rows)
	}
	a.editor.at = len("前面 $alpha")
	if a.skillMatches() {
		t.Fatal("selected skill reopened completion")
	}
	a.handle(key{code: "left"})
	if a.editor.at != len("前面 ") {
		t.Fatal("cursor entered skill token", a.editor.at)
	}
	a.handle(key{code: "right"})
	if a.editor.at != len("前面 $alpha") {
		t.Fatal("cursor did not jump past skill token", a.editor.at)
	}
	a.handle(key{code: "backspace"})
	if a.editor.text != "前面  后面" || len(a.editor.skills) != 0 {
		t.Fatal("backspace partially removed skill", a.editor.text)
	}
	a.editor.at = len("前面 ")
	a.editor.insertSkill("alpha")
	a.editor.at = len("前面 ")
	a.editor.key(key{code: "delete"})
	if strings.Contains(a.editor.text, "alpha") || len(a.editor.skills) != 0 {
		t.Fatal("forward deletion left skill fragments", a.editor.text)
	}
}

func TestSkillsMoveWithImagesAndRestoredDrafts(t *testing.T) {
	a := testApp(t)
	terminalSkill(t, a, "alpha", "First skill")
	a.editor.insertImage(session.Attachment{})
	a.editor.insert(" ")
	a.editor.insertSkill("alpha")
	for range 10 {
		a.editor.insertImage(session.Attachment{})
	}
	a.syncDraftImages()
	a.editor.at = 0
	a.editor.key(key{code: "delete"})
	a.syncDraftImages()
	span := a.editor.skills[0]
	if a.editor.text[span.start:span.end] != "$alpha" {
		t.Fatal("image renumbering displaced skill", a.editor.text, span)
	}
	a.editor.at = 0
	a.editor.insert("你好 ")
	span = a.editor.skills[0]
	if a.editor.text[span.start:span.end] != "$alpha" {
		t.Fatal("text insertion displaced skill")
	}
	text := a.editor.text
	items := a.editor.imageAttachments()
	a.setDraft(text, items)
	if len(a.editor.skills) != 1 {
		t.Fatal("restored draft lost skill token", a.editor.text)
	}
	var appended editor
	appended.insertSkill("alpha")
	if !a.editor.appendDraft(appended) || len(a.editor.skills) != 2 {
		t.Fatal("appending draft lost skill tokens")
	}
	for _, span := range a.editor.skills {
		if a.editor.text[span.start:span.end] != "$alpha" {
			t.Fatal("merged draft has wrong span", span)
		}
	}
	a.editor.replace("plain")
	if len(a.editor.skills) != 0 {
		t.Fatal("replace retained stale skills")
	}
}
