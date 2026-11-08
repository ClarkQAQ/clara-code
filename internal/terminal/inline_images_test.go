package terminal

import (
	"strings"
	"testing"

	"clara-code/internal/session"
)

func TestInlineImagesEditAtomicallyAndRenumberInDisplayOrder(t *testing.T) {
	var e editor
	e.replace("前后")
	e.at = len("前")
	e.insertImage(session.Attachment{Filename: "first.png"})
	if items := e.imageAttachments(); len(items) != 1 || e.text != "前[Image #1]后" {
		t.Fatal(e.text, items)
	}
	e.key(key{code: "text", text: "说明"})
	e.key(key{code: "home"})
	e.insertImage(session.Attachment{Filename: "second.png"})
	items := e.imageAttachments()
	if e.text != "[Image #1]前[Image #2]说明后" || items[0].Filename != "second.png" || items[1].Filename != "first.png" {
		t.Fatal(e.text, items)
	}
	e.key(key{code: "left"})
	if e.at != 0 {
		t.Fatal("left entered an image label", e.at)
	}
	e.key(key{code: "right"})
	if e.at != len("[Image #1]") {
		t.Fatal("right entered an image label", e.at)
	}
	e.key(key{code: "backspace"})
	items = e.imageAttachments()
	if len(items) != 1 || e.text != "前[Image #1]说明后" || items[0].Filename != "first.png" {
		t.Fatal(e.text, items)
	}
	e.key(key{code: "right"})
	e.key(key{code: "delete"})
	if len(e.imageAttachments()) != 0 || e.text != "前说明后" {
		t.Fatal("delete split the marker", e.text)
	}
}

func TestPendingImageKeepsItsInsertionAnchorWhileEditing(t *testing.T) {
	a := testApp(t)
	a.editor.replace("前后")
	a.editor.at = len("前")
	a.editor.insertPending("/tmp/image.png")
	a.editor.key(key{code: "text", text: "说明"})
	a.editor.key(key{code: "home"})
	a.editor.key(key{code: "text", text: "开始"})
	// Use an error to exercise anchored replacement without reading the system clipboard.
	a.acceptAttachments(attachmentResult{owner: a.conv.Session.ID(), text: "[replacement]"})
	if a.editor.text != "开始前[replacement]说明后" || a.editor.at != len("开始") {
		t.Fatal("asynchronous replacement moved the draft or cursor", a.editor.text, a.editor.at)
	}
	a.editor.insertPending("")
	a.editor.key(key{code: "backspace"})
	before := a.editor.text
	a.acceptAttachments(attachmentResult{owner: a.conv.Session.ID(), text: "must not reappear"})
	if a.editor.text != before {
		t.Fatal("deleted loading marker reappeared")
	}
}

func TestInlineImageLayoutPreservesRealCursorAndColorsOnlyAttachments(t *testing.T) {
	var e editor
	e.replace("你好")
	e.insertImage(session.Attachment{Filename: "image.png"})
	e.imageAttachments()
	e.key(key{code: "text", text: "👨‍👩‍👧‍👦"})
	p := painter{width: 60, height: 10, color: true}
	view := p.inputLayout(e, "placeholder", 7)
	if view.cursor == nil || view.cursor.column != 2+widthOf(e.text) || !strings.Contains(strings.Join(view.rows, "\n"), accent+"[Image #1]") {
		t.Fatal(view)
	}
	for _, width := range []int{1, 3, 8, 14} {
		p.width = width
		e.vertical(-1, max(1, width-2))
		for _, span := range e.images {
			if e.at > span.start && e.at < span.end {
				t.Fatal("vertical movement entered marker", width, e.at)
			}
		}
		view = p.inputLayout(e, "placeholder", 3)
		if view.cursor == nil || len(view.rows) > 3 {
			t.Fatal(view)
		}
		for _, line := range view.rows {
			if widthOf(stripStyles(line)) > width {
				t.Fatal("wrapped marker overflow", width, line)
			}
		}
	}
	literal := editor{text: "[Image #1]", at: 10}
	literal.key(key{code: "backspace"})
	if literal.text != "[Image #1" {
		t.Fatal("ordinary text was treated as an attachment")
	}
}
