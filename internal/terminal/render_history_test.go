package terminal

import (
	"fmt"
	"strings"
	"testing"

	"clara-code/internal/session"
)

func TestPaintHistoryTruncatesWrappedMessage(t *testing.T) {
	p := painter{width: 24}
	p.rememberCell(session.Cell{Kind: "user", Text: "oldest prompt"})
	p.rememberCell(session.Cell{
		Kind: "assistant",
		Text: "start of long reply\n" + strings.Repeat("abcdefgh ", 4000) + "\nend of long reply",
	})
	p.rememberCell(session.Cell{Kind: "user", Text: "latest prompt"})
	if p.buf.Len() != 0 {
		t.Fatal("remembering cells wrote terminal output")
	}

	checkTail := func() {
		t.Helper()
		output := p.buf.String()
		if got := strings.Count(output, "\r\n"); got != maxHistoryRows {
			t.Fatalf("painted %d rows, want %d", got, maxHistoryRows)
		}
		if strings.Contains(output, "oldest prompt") || strings.Contains(output, "start of long reply") {
			t.Fatal("terminal output contains history before the retained tail")
		}
		notice := strings.Index(output, "Earlier display")
		end := strings.Index(output, "end of long reply")
		latest := strings.Index(output, "latest prompt")
		if notice < 0 || end <= notice || latest <= end || strings.Count(output, "Earlier display") != 1 {
			t.Fatal("terminal output must contain one notice and the newest rows in order")
		}
	}
	p.paintHistory()
	checkTail()
	p.rebuild()
	checkTail()

	p.width = 80
	p.rebuild()
	output := p.buf.String()
	if strings.Contains(output, "Earlier display history") || !strings.Contains(output, "oldest prompt") {
		t.Fatal("wider replay should recover rows that now fit")
	}
}

func TestPaintHistoryFitsExactlyAtRowLimit(t *testing.T) {
	p := painter{width: 80}
	for i := range maxHistoryRows / 2 {
		p.remember("user", fmt.Sprintf("prompt %03d", i))
	}
	p.paintHistory()
	output := p.buf.String()
	if got := strings.Count(output, "\r\n"); got != maxHistoryRows {
		t.Fatalf("painted %d rows, want %d", got, maxHistoryRows)
	}
	if strings.Contains(output, "Earlier display history") || !strings.Contains(output, "prompt 000") {
		t.Fatal("terminal output omitted source history at the exact limit")
	}
}
