package terminal

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"clara-code/internal/coding"
	"clara-code/internal/session"
	"clara-code/pkg/core"
	"clara-code/pkg/model"
)

type delayedTurnModel struct {
	coding.DemoModel
	first bool
}

func (m *delayedTurnModel) Stream(ctx context.Context, call model.Call) (model.StreamResponse, error) {
	if m.first {
		m.first = false
		time.Sleep(61 * time.Second)
	}
	return m.DemoModel.Stream(ctx, call)
}

func TestLongTurnFooterPrecedesPendingInput(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		a := testApp(t)
		a.conv.Host.Models = func(context.Context, string) (model.LanguageModel, error) {
			return &delayedTurnModel{first: true}, nil
		}
		if err := a.conv.ChangeModel(t.Context(), "demo", ""); err != nil {
			t.Fatal(err)
		}
		a.start("Long task", false)
		first := a.run
		if first == nil {
			t.Fatalf("could not start the long task: %s", a.view.buf.String())
		}
		if _, err := first.Wait(t.Context()); err != nil {
			t.Fatal(err)
		}
		a.steer("Next task", false)
		for a.run != nil {
			a.onRunEvent(<-a.events)
		}
		text := a.view.buf.String()
		reply := strings.Index(text, "Demo response: Long task")
		footer := strings.Index(text, "Worked for 1m 01s")
		next := strings.Index(text, "❯ Next task")
		if reply < 0 || reply >= footer || footer >= next || strings.Count(text, "Worked for ") != 1 {
			t.Fatalf("long-turn footer did not appear once between the reply and next input: %q", text)
		}
	})
}

func TestTurnCompletionUsesSavedDuration(t *testing.T) {
	start := time.Date(2026, 11, 13, 12, 0, 0, 0, time.UTC)
	for _, fixture := range []struct {
		name, status, want string
		millis             *int64
		finish             time.Time
	}{
		{name: "short turn", status: "completed", millis: new(int64(60_999)), finish: start.Add(3 * time.Minute)},
		{name: "first visible second", status: "completed", millis: new(int64(61_000)), want: "Worked for 1m 01s"},
		{name: "hours", status: "completed", millis: new(int64(7_384_000)), want: "Worked for 2h 03m 04s"},
		{name: "legacy timestamps", status: "completed", finish: start.Add(61 * time.Second), want: "Worked for 1m 01s"},
		{name: "missing finish", status: "completed"},
		{name: "clock moved back", status: "completed", finish: start.Add(-2 * time.Hour)},
		{name: "cancelled", status: "cancelled", millis: new(int64(90_000))},
		{name: "failed", status: "failed", millis: new(int64(90_000))},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			turn := session.Turn{DurationMS: fixture.millis, Status: fixture.status, StartedAt: start, FinishedAt: fixture.finish}
			if got := turnCompletion(turn); got != fixture.want {
				t.Fatalf("completion = %q, want %q", got, fixture.want)
			}
		})
	}
}

func TestCompactionActivitySurvivesHistoryRebuild(t *testing.T) {
	a := testApp(t)
	a.view.color = false
	a.stream = "A response before compression."
	a.onRunEvent(runEvent{event: core.Event{Kind: session.ContextCompactedKind, Data: []byte(`{"DurationMS":2785}`)}})
	a.view.message("assistant", "A response after compression.")
	if len(a.view.history) != 3 || a.view.history[1].Kind != session.ContextCompactedKind {
		t.Fatalf("compaction was not inserted between replies: %+v", a.view.history)
	}
	a.view.width = 41
	a.view.rebuild()
	text := a.view.buf.String()
	if got := strings.Count(text, "Context compacted · 2s"); got != 1 {
		t.Fatalf("compaction appears %d times after rebuild: %q", got, text)
	}
	before, after := strings.Index(text, "before compression"), strings.Index(text, "after compression")
	if mark := strings.Index(text, "Context compacted"); before < 0 || before >= mark || mark >= after {
		t.Fatalf("compaction is outside the two replies: %q", text)
	}
	stored := session.Cell{Kind: session.ContextCompactedKind, Text: "Context compacted", Compaction: &session.Compaction{DurationMS: new(int64(2785))}}
	if got := a.view.cellRows(stored)[0]; got != "· Context compacted · 2s" {
		t.Fatalf("restored compaction row = %q", got)
	}
	a.onRunEvent(runEvent{snapshot: &session.Turn{Prompt: "Recover output", Cells: []session.Cell{
		{Kind: "assistant", Text: "A response before compression."}, stored,
	}}})
	if text := a.view.buf.String(); strings.Count(text, "Context compacted · 2s") != 1 || strings.Contains(text, "after compression") {
		t.Fatalf("gap recovery did not replace the live history with the saved marker: %q", text)
	}
	if got := a.view.cellRows(compactionCell(nil))[0]; got != "· Context compacted" {
		t.Fatalf("missing compaction duration rendered as %q", got)
	}
}

func TestForkBoundarySeparatesInheritedAndNewCompactions(t *testing.T) {
	marker := func(afterTurn int, afterFork bool, millis int64) session.CompactionMarker {
		return session.CompactionMarker{AfterTurn: afterTurn, AfterFork: afterFork, Compaction: session.Compaction{DurationMS: &millis}}
	}
	boundaries := transcriptBoundaries(session.Meta{
		Parent: "session_parent", ForkPoint: &session.ForkPoint{AfterTurn: 2, Title: " parent\n会话\x00  "},
	}, []session.CompactionMarker{marker(1, false, 1300), marker(2, false, 2300), marker(2, true, 3300), marker(3, false, 4300)})
	for i, want := range [][]string{
		nil,
		{"Context compacted · 1s"},
		{"Context compacted · 2s", "parent 会话", "Context compacted · 3s"},
		{"Context compacted · 4s"},
		nil,
	} {
		var labels []string
		for _, cell := range boundaries(i) {
			labels = append(labels, transcriptCellText(cell))
		}
		if !slices.Equal(labels, want) {
			t.Fatalf("boundary %d = %v, want %v", i, labels, want)
		}
	}
	legacy := transcriptBoundaries(session.Meta{Parent: "session_legacy"}, nil)
	if got := legacy(0); len(got) != 1 || got[0].Kind != threadForkedKind || got[0].Text != "session_legacy" {
		t.Fatalf("legacy branch lost source: %+v", got)
	}
	for _, width := range []int{1, 14, 26, 60} {
		t.Run(fmt.Sprint(width), func(t *testing.T) {
			p := painter{width: width}
			rows := p.messageRows(threadForkedKind, "会话来源\nwith some details")
			if widthOf(rows[0]) > width || strings.ContainsAny(rows[0], "\n\x1b") {
				t.Fatalf("fork row exceeds terminal or contains controls: %q", rows[0])
			}
			if width == 60 && (!strings.HasPrefix(rows[0], "──") || !strings.Contains(rows[0], "Forked from 会话来源 with some details") || !strings.HasSuffix(rows[0], "──")) {
				t.Fatalf("fork row lacks label or rules: %q", rows[0])
			}
		})
	}
}
