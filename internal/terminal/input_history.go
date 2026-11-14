package terminal

import (
	"context"
	"reflect"
	"slices"
	"strings"
	"time"

	"clara-code/internal/session"
)

func (a *app) loadInputHistory() {
	if a.historyLoaded {
		return
	}
	a.historyLoaded = true
	entries, err := session.ReadInputHistory(a.host.Config.Home)
	a.inputHistory = entries
	if err != nil {
		a.notice(err)
	}
}

func (a *app) recordInput(text string) {
	if a.conv == nil || strings.TrimSpace(text) == "/clear" {
		return
	}
	a.loadInputHistory()
	entry := session.InputHistoryEntry{SessionID: a.conv.Session.ID(), Timestamp: time.Now().UTC(), Text: text, Attachments: slices.Clone(a.attachments), Kind: "prompt"}
	command, _ := splitCommand(text)
	if strings.HasPrefix(command, "/") && !strings.Contains(command[1:], "/") {
		entry.Kind = "command"
	}
	if text == "" && len(entry.Attachments) == 0 {
		return
	}
	a.historyAt = -1
	if len(a.inputHistory) > 0 {
		previous := a.inputHistory[len(a.inputHistory)-1]
		if previous.Text == entry.Text && reflect.DeepEqual(previous.Attachments, entry.Attachments) {
			return
		}
	}
	a.inputHistory = append(a.inputHistory, entry)
	if len(a.inputHistory) > 1000 {
		a.inputHistory = a.inputHistory[len(a.inputHistory)-1000:]
	}
	bytes, start := 0, len(a.inputHistory)
	for start > 0 {
		previous := a.inputHistory[start-1]
		size := len(previous.Text) + 256 + len(previous.Attachments)*1024
		if bytes+size > 4<<20 {
			break
		}
		bytes += size
		start--
	}
	a.inputHistory = slices.Clone(a.inputHistory[start:])
	ctx, cancel := context.WithTimeout(a.ctx, 2*time.Second)
	defer cancel()
	if err := session.AppendInputHistory(ctx, a.host.Config.Home, entry); err != nil {
		a.notice(err)
	}
}

func (a *app) recall(direction string) {
	a.loadInputHistory()
	if len(a.inputHistory) == 0 {
		return
	}
	if a.historyAt < 0 {
		if direction == "down" || a.editor.text != "" {
			return
		}
		a.historyDraft = a.editor
		a.historyAt = len(a.inputHistory) - 1
	} else if direction == "up" {
		a.historyAt = max(0, a.historyAt-1)
	} else {
		a.historyAt++
	}
	if a.historyAt >= len(a.inputHistory) {
		a.historyAt = -1
		a.editor = a.historyDraft
		a.syncDraftImages()
		a.dismissRecalledMenus()
		return
	}
	a.restoreHistoryEntry(a.inputHistory[a.historyAt])
}

func (a *app) restoreHistoryEntry(entry session.InputHistoryEntry) {
	a.setDraft(entry.Text, entry.Attachments)
	a.dismissRecalledMenus()
}

func (a *app) dismissRecalledMenus() {
	a.commandMenu = commandMenu{text: a.editor.text, hidden: true}
	a.skillMenu = skillMenu{}
	if a.skillMatches() {
		a.skillMenu.hidden = true
	}
}
