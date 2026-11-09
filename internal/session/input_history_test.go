package session

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestInputHistoryPersistenceConcurrencyAndRecovery(t *testing.T) {
	home := t.TempDir()
	var group sync.WaitGroup
	for i := range 12 {
		group.Go(func() {
			if err := AppendInputHistory(t.Context(), home, InputHistoryEntry{SessionID: fmt.Sprint(i), Text: fmt.Sprintf("prompt %d", i)}); err != nil {
				t.Error(err)
			}
		})
	}
	group.Wait()
	entries, err := ReadInputHistory(home)
	if err != nil || len(entries) != 12 {
		t.Fatal("concurrent writes lost history", len(entries), err)
	}
	path := filepath.Join(home, "history.jsonl")
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatal("history is not private", info, err)
	}
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range strings.Split(strings.TrimSpace(string(body)), "\n") {
		var record struct {
			Timestamp string `json:"ts"`
		}
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			t.Fatal(err)
		}
		if _, err := time.Parse(time.RFC3339Nano, record.Timestamp); err != nil {
			t.Fatal("timestamp lacks time zone", record, err)
		}
	}
	if err := os.WriteFile(path, append(body, []byte("{truncated")...), 0600); err != nil {
		t.Fatal(err)
	}
	entries, err = ReadInputHistory(home)
	if len(entries) != 12 || err == nil {
		t.Fatal("torn final line hid valid entries", len(entries), err)
	}
	if err := AppendInputHistory(t.Context(), home, InputHistoryEntry{Text: "/status", Kind: "command"}); err != nil {
		t.Fatal(err)
	}
	entries, err = ReadInputHistory(home)
	if err != nil || len(entries) != 13 || entries[12].Text != "/status" {
		t.Fatal("recovery failed", entries, err)
	}
}

func TestInputHistoryRetentionAndAttachmentReferences(t *testing.T) {
	home := t.TempDir()
	var body strings.Builder
	for i := range 1100 {
		line, _ := json.Marshal(InputHistoryEntry{Text: fmt.Sprint(i)})
		body.Write(line)
		body.WriteByte('\n')
	}
	if err := os.WriteFile(filepath.Join(home, "history.jsonl"), []byte(body.String()), 0600); err != nil {
		t.Fatal(err)
	}
	entries, err := ReadInputHistory(home)
	if err != nil || len(entries) != 1000 || entries[0].Text != "100" {
		t.Fatal("entry retention failed", len(entries), err)
	}
	image := Attachment{Filename: "fixture.png", Hash: strings.Repeat("a", 64)}
	if err := AppendInputHistory(t.Context(), home, InputHistoryEntry{Text: "[Image #1]", Attachments: []Attachment{image}}); err != nil {
		t.Fatal(err)
	}
	entries, err = ReadInputHistory(home)
	if err != nil || len(entries) != 1000 || entries[999].Attachments[0].Filename != "fixture.png" {
		t.Fatal("attachment reference lost", err)
	}
}
