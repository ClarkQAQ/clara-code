package terminal

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"clara-core/agent"
	"clara-core/models"

	"clara-code/internal/coding"
	"clara-code/internal/session"
)

func TestInputCursorUsesDisplayColumns(t *testing.T) {
	for _, test := range []struct {
		name, text string
		width, at  int
		limit      int
		want       cursorPosition
	}{
		{name: "placeholder", width: 20, want: cursorPosition{column: 2}},
		{name: "pinyin", text: "ni hao", width: 20, at: 6, want: cursorPosition{column: 8}},
		{name: "middle", text: "abcd", width: 20, at: 2, want: cursorPosition{column: 4}},
		{name: "chinese", text: "你好世界", width: 20, at: len("你好"), want: cursorPosition{column: 6}},
		{name: "emoji", text: "A👨‍👩‍👧‍👦B", width: 20, at: len("A👨‍👩‍👧‍👦"), want: cursorPosition{column: 5}},
		{name: "combining", text: "e\u0301x", width: 20, at: len("e\u0301"), want: cursorPosition{column: 3}},
		{name: "soft wrap", text: "abcdefghi", width: 8, at: 8, want: cursorPosition{row: 1, column: 4}},
		{name: "full row", text: "abcdef", width: 8, at: 6, want: cursorPosition{row: 1, column: 2}},
		{name: "full row before newline", text: "abcdef\nghi", width: 8, at: 6, want: cursorPosition{row: 1, column: 2}},
		{name: "newline", text: "你好\nabc", width: 20, at: len("你好\nab"), want: cursorPosition{row: 1, column: 4}},
		{name: "scroll", text: strings.Repeat("line\n", 20) + "tail", width: 20, at: 104, limit: 3, want: cursorPosition{row: 2, column: 6}},
		{name: "narrow", text: "你好", width: 1, at: len("你好"), want: cursorPosition{row: 2}},
	} {
		t.Run(test.name, func(t *testing.T) {
			p := painter{width: test.width, height: 24, color: true}
			limit := test.limit
			if limit == 0 {
				limit = 7
			}
			view := p.inputLayout(editor{text: test.text, at: test.at}, "Type here", limit)
			if view.cursor == nil || *view.cursor != test.want {
				t.Fatalf("cursor %+v, want %+v; rows %q", view.cursor, test.want, view.rows)
			}
			if len(view.rows) > limit || view.cursor.row >= len(view.rows) {
				t.Fatalf("cursor outside input viewport: %+v", view)
			}
			for _, row := range view.rows {
				if strings.Contains(row, "\x1b[7m") || widthOf(stripStyles(row)) > test.width {
					t.Fatalf("simulated cursor or overflowing row: %q", row)
				}
			}
		})
	}
}

func TestLayoutOffsetsAndClippingPreserveCursor(t *testing.T) {
	input := layout{rows: []string{"input"}, cursor: &cursorPosition{column: 5}}
	view := layout{rows: []string{"activity", "border"}}
	view.append(input)
	view.rows = append(view.rows, "footer")
	view = view.prepend([]string{"stream", "tail"})
	clipped := view.slice(2, 6)
	if *clipped.cursor != (cursorPosition{row: 2, column: 5}) || input.cursor.row != 0 || view.cursor.row != 4 {
		t.Fatalf("cursor offsets or source changed: %+v %+v %+v", input, view, clipped)
	}
	if view.slice(0, 3).cursor != nil {
		t.Fatal("clipped input retained a cursor in an unrelated row")
	}
}

func TestQuestionCursorFollowsNotesAndFreeAnswer(t *testing.T) {
	request := coding.QuestionRequest{Items: []coding.Question{{ID: "one", Question: "Choose an option", Options: []coding.QuestionOption{{Label: "Yes"}, {Label: "No"}}}}}
	q := newQuestionForm(request)
	p := painter{width: 40, height: 20}
	if q.layout(&p).cursor != nil {
		t.Fatal("selection-only question showed a text cursor")
	}
	q.notesMode = true
	q.drafts[0].notes.replace("你好\nni hao")
	for _, height := range []int{20, 8, 4, 2, 1} {
		p.height = height
		view := q.layout(&p)
		if view.cursor == nil || view.cursor.row >= len(view.rows) || !strings.Contains(view.rows[view.cursor.row], "ni hao") || view.cursor.column != 8 {
			t.Fatalf("notes cursor lost when clipped to %d rows: %+v", height, view)
		}
	}
	q.notesMode = false
	q.request.Items[0].Options = nil
	if q.layout(&p).cursor == nil {
		t.Fatal("free answer did not receive the real cursor")
	}
}

func TestComposerCursorSurvivesActivityMenuAndStreaming(t *testing.T) {
	a := testApp(t)
	var output bytes.Buffer
	a.view.output = &output
	a.run = &agent.Run{ID: "cursor-test"}
	a.activity.start(time.Now())
	a.editor.replace("ni hao")
	tool := session.NewToolActivity(models.ToolCallContent{ToolCallID: "call", ToolName: "read", Input: `{"path":"file.go"}`}, time.Now())
	a.toolGroup = []session.ToolActivity{tool}
	for _, height := range []int{24, 8, 4, 2, 1} {
		a.view.height = height
		view := a.pane()
		if view.cursor == nil || view.cursor.row >= len(view.rows) || !strings.Contains(view.rows[view.cursor.row], "ni hao") || len(view.rows) > height {
			t.Fatalf("composer cursor lost at height %d: %+v", height, view)
		}
	}
	a.view.height = 24
	a.editor.replace("/mo")
	view := a.pane()
	if !strings.Contains(strings.Join(view.rows, "\n"), "/model") || !strings.Contains(view.rows[view.cursor.row], "/mo") {
		t.Fatal("command menu moved the composer cursor")
	}
	a.editor.replace("ni hao")
	a.toolGroup = nil
	a.stream = "Streaming reply."
	if err := a.draw(); err != nil {
		t.Fatal(err)
	}
	view = a.pane()
	if a.view.cursorRow != view.cursor.row+2 {
		t.Fatalf("stream tail did not offset real cursor: %d, composer %+v", a.view.cursorRow, view.cursor)
	}
	a.picker = &picker{kind: "model", title: "Select model", items: []choice{{label: "demo"}}}
	if a.pane().cursor != nil {
		t.Fatal("read-only selector retained the composer cursor")
	}
}
