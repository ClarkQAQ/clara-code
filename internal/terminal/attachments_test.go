package terminal

import (
	"bytes"
	"errors"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"clara-code/internal/session"
	"clara-code/internal/setting"
	"clara-code/pkg/model"
)

func TestImagePasteDraftDeleteAndCapabilityChange(t *testing.T) {
	a := testApp(t)
	var data bytes.Buffer
	if err := png.Encode(&data, image.NewRGBA(image.Rect(0, 0, 7, 5))); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(a.space.Cwd, "two words.png")
	if err := os.WriteFile(path, data.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
	a.editor.replace("Explain this: ")
	a.handle(key{code: "paste", text: "\"" + path + "\""})
	if a.attachmentResults == nil {
		t.Fatal("path paste did not start loading")
	}
	select {
	case result := <-a.attachmentResults:
		a.acceptAttachments(result)
	case <-t.Context().Done():
		t.Fatal(t.Context().Err())
	}
	if a.editor.text != "Explain this: " || len(a.attachments) != 1 {
		t.Fatal("image conversion lost draft", a.editor.text, a.attachments)
	}
	if rows := a.attachmentRows(); len(rows) != 2 || !strings.Contains(rows[0], "7x5") {
		t.Fatal(rows)
	}
	original := append([]session.Attachment(nil), a.attachments...)
	a.manageAttachments()
	a.handle(key{code: "delete"})
	if len(a.attachments) != 0 || a.picker != nil {
		t.Fatal("attachment deletion failed")
	}
	a.restoreDraft("retry text", original)
	if a.editor.text != "Explain this: " || len(a.attachments) != 1 {
		t.Fatal("retry replaced a newer draft")
	}
	a.host.Config.Models["text"] = setting.Model{ID: "text", InputModalities: []string{"text"}}
	a.conv.Host.Config = a.host.Config
	if err := a.conv.ChangeModel(t.Context(), "text", ""); err != nil {
		t.Fatal(err)
	}
	a.start(a.editor.text, false)
	if a.run != nil || len(a.attachments) != 1 || a.editor.text != "Explain this: " {
		t.Fatal("unsupported submit destroyed the draft")
	}
	a.acceptAttachments(attachmentResult{owner: a.conv.Session.ID(), err: errors.New("bad image")})
	if len(a.attachments) != 1 || a.editor.text != "Explain this: " {
		t.Fatal("failed loading destroyed draft")
	}
}

func TestConfiguredKeysKeepPasteAndAttachmentActionsDistinct(t *testing.T) {
	a := testApp(t)
	a.host.Config.TUI.Keymap = map[string]map[string]string{"composer": {"paste_image": "alt-v", "attachments": "alt-g"}, "picker": {"delete_attachment": "ctrl-x"}}
	if err := a.host.Config.TUI.ValidateKeys(); err != nil {
		t.Fatal(err)
	}
	if got := a.bindKey(key{code: "ctrl-v"}); got.code != "unbound" {
		t.Fatal(got)
	}
	if got := a.bindKey(key{code: "alt-v"}); got.code != "ctrl-v" {
		t.Fatal(got)
	}
	if got := a.bindKey(key{code: "paste", text: "你好"}); got.code != "paste" || got.text != "你好" {
		t.Fatal(got)
	}
	a.picker = &picker{kind: "attachments"}
	if got := a.bindKey(key{code: "ctrl-x"}); got.code != "delete" {
		t.Fatal(got)
	}
	if hint := a.hints("↑/↓ Select Enter Confirm Esc Back"); !strings.Contains(hint, "Enter Confirm") {
		t.Fatal(hint)
	}
}

func TestNoReasoningModelStillHasSelectableDefault(t *testing.T) {
	a := testApp(t)
	a.host.Config.Models["text"] = setting.Model{ID: "text", SupportedReasoningLevels: []model.ReasoningEffort{}}
	a.picker = &picker{kind: "model", items: []choice{{id: "text", label: "text"}}}
	a.handle(key{code: "enter"})
	if a.picker == nil || a.picker.kind != "effort" || len(a.picker.items) != 1 || a.picker.items[0].id != "" {
		t.Fatal("model with no reasoning could not be selected")
	}
}

func TestInvalidClipboardPathKeepsOriginalText(t *testing.T) {
	a := testApp(t)
	a.editor.replace("existing ")
	a.acceptAttachments(attachmentResult{owner: a.conv.Session.ID(), fallback: "/missing/image.png", err: errors.New("missing image")})
	if a.editor.text != "existing /missing/image.png" {
		t.Fatal("invalid clipboard path was discarded", a.editor.text)
	}
}
