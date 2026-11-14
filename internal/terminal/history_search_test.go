package terminal

import (
	"strings"
	"testing"

	"clara-code/internal/session"
)

func TestHistorySearchUsesComposerBindings(t *testing.T) {
	a := testApp(t)
	a.host.Config.TUI.Keymap = map[string]map[string]string{"composer": {
		"history_search": "alt-r", "history_previous": "ctrl-p", "history_next": "ctrl-n",
		"submit": "alt-s", "cancel": "alt-c", "backspace": "ctrl-b",
	}}
	a.historyLoaded = true
	a.inputHistory = []session.InputHistoryEntry{{Text: "/status"}, {Text: "restarted service"}}
	a.submit("/help")
	if text := a.view.history[len(a.view.history)-1].Text; !strings.Contains(text, "Alt+R searches input history.") {
		t.Fatal("help did not show configured search key", text)
	}
	a.editor.replace("/st")
	a.handle(key{code: "ctrl-r"})
	if a.picker != nil {
		t.Fatal("default search binding remained active")
	}
	a.handle(key{code: "alt-r"})
	if a.picker == nil || a.picker.history == nil {
		t.Fatal("configured search binding did not open history")
	}
	if rows := strings.Join(a.pane().rows, "\n"); !strings.Contains(rows, "Ctrl+P/Ctrl+N Select   Alt+S Recall   Alt+C Cancel") {
		t.Fatal("history help did not show configured keys", rows)
	}
	a.handle(key{code: "text", text: "stz"})
	a.handle(key{code: "ctrl-b"})
	a.handle(key{code: "ctrl-n"})
	a.handle(key{code: "enter"})
	if a.picker == nil || a.editor.text != "/st" {
		t.Fatal("unbound Enter changed the composer")
	}
	a.handle(key{code: "alt-s"})
	if a.picker != nil || a.editor.text != "/status" || a.editor.at != len("/status") || a.historyAt != -1 || len(a.menuMatches()) != 0 {
		t.Fatal("configured confirm did not recall command", a.editor.text)
	}
	a.handle(key{code: "alt-r"})
	a.handle(key{code: "alt-c"})
	if a.picker != nil || a.editor.text != "/status" {
		t.Fatal("configured cancel did not keep recalled text", a.editor.text)
	}
}

func TestHistorySearchKeepsDraftAndRecallsRichEntries(t *testing.T) {
	a := testApp(t)
	a.view.color = true
	terminalSkill(t, a, "alpha", "First skill")
	image := session.Attachment{Filename: "recalled.png"}
	draftImage := session.Attachment{Filename: "draft.png"}
	const prompt = "检查 $alpha\n第二行 [Image #1]"
	a.historyLoaded = true
	a.inputHistory = []session.InputHistoryEntry{
		{Text: prompt, Attachments: []session.Attachment{image}},
		{Attachments: []session.Attachment{image}},
	}
	a.setDraft("当前 [Image #1] 草稿", []session.Attachment{draftImage})
	a.editor.at = len("当前")
	draft := a.editor.text
	a.handle(key{code: "ctrl-r"})
	const attachmentRow = "  Image attachment · recalled.png"
	const promptRow = "  检查 $alpha 第二行 [Image #1]"
	if rows := a.pane().rows; rows[1] != accent+attachmentRow+reset || rows[2] != promptRow {
		t.Fatal("history did not highlight the attachment row", rows)
	}
	a.handle(key{code: "down"})
	if rows := a.pane().rows; rows[1] != attachmentRow || rows[2] != accent+promptRow+reset {
		t.Fatal("history highlight did not follow selection", rows)
	}
	a.handle(key{code: "paste", text: "missing.png"})
	a.handle(key{code: "enter"})
	if a.picker == nil || a.attachmentResults != nil {
		t.Fatal("search paste started image loading or Enter closed an empty result list")
	}
	a.handle(key{code: "ctrl-c"})
	if a.picker != nil || a.editor.text != draft || a.editor.at != len("当前") || len(a.attachments) != 1 || a.attachments[0].Filename != draftImage.Filename {
		t.Fatal("cancelling search changed the draft", a.editor, a.attachments)
	}
	a.handle(key{code: "ctrl-r"})
	a.handle(key{code: "paste", text: "ALPHA\n第二行"})
	a.handle(key{code: "enter"})
	if a.picker != nil || a.editor.text != prompt || a.editor.at != len(prompt) || len(a.editor.skills) != 1 || len(a.attachments) != 1 || a.attachments[0].Filename != image.Filename {
		t.Fatal("rich history entry was not restored", a.editor, a.attachments)
	}
	if a.run != nil || a.conv.Store.TurnCount() != 0 {
		t.Fatal("recalling history submitted input")
	}
	a.handle(key{code: "ctrl-r"})
	a.handle(key{code: "enter"})
	if a.picker != nil || a.editor.text != "[Image #1]" || len(a.attachments) != 1 || a.attachments[0].Filename != image.Filename {
		t.Fatal("attachment-only entry was not restored", a.editor, a.attachments)
	}
}
