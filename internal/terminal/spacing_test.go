package terminal

import (
	"clara-code/internal/coding"
	"clara-code/internal/session"
	"context"
	"encoding/json"
	"strconv"
	"strings"
	"testing"
	"time"

	"clara-code/pkg/core"
	"clara-code/pkg/model"
	"github.com/rivo/uniseg"
)

// screenState models cursor movement, erase and scrolling independently of layout.
type screenState struct {
	width, height, x, y int
	cells               [][]string
	pending, visible    bool
}

func newScreen(width, height int) *screenState {
	s := &screenState{width: width, height: height, visible: true}
	for range height {
		s.cells = append(s.cells, make([]string, width))
	}
	return s
}

func (s *screenState) newline() {
	s.y++
	if s.y >= s.height {
		s.cells = append(s.cells[1:], make([]string, s.width))
		s.y = s.height - 1
	}
	s.pending = false
}

func (s *screenState) Write(data []byte) (int, error) {
	text := string(data)
	for len(text) > 0 {
		switch text[0] {
		case '\r':
			s.x = 0
			s.pending = false
			text = text[1:]
		case '\n':
			s.newline()
			text = text[1:]
		case '\x1b':
			if !strings.HasPrefix(text, "\x1b[") {
				panic("unsupported terminal escape")
			}
			end := 2
			for end < len(text) && (text[end] < 0x40 || text[end] > 0x7e) {
				end++
			}
			argument, code := text[2:end], text[end]
			n, _ := strconv.Atoi(argument)
			if n == 0 {
				n = 1
			}
			switch code {
			case 'A':
				s.y = max(0, s.y-n)
				s.pending = false
			case 'B':
				s.y = min(s.height-1, s.y+n)
				s.pending = false
			case 'G':
				s.x = min(s.width-1, n-1)
				s.pending = false
			case 'H':
				s.x, s.y = 0, 0
				s.pending = false
			case 'J':
				if argument != "3" {
					first := s.y
					if argument == "2" {
						first = 0
					}
					for row := first; row < s.height; row++ {
						start := 0
						if row == s.y && argument != "2" {
							start = s.x
						}
						clear(s.cells[row][start:])
					}
				}
			case 'K':
				clear(s.cells[s.y][s.x:])
			case 'h', 'l':
				if argument == "?25" {
					s.visible = code == 'h'
				}
			}
			text = text[end+1:]
		default:
			end := strings.IndexAny(text, "\r\n\x1b")
			if end < 0 {
				end = len(text)
			}
			graphemes := uniseg.NewGraphemes(text[:end])
			for graphemes.Next() {
				size := graphemes.Width()
				if size == 0 {
					continue
				}
				if s.pending || s.x+size > s.width {
					s.x = 0
					s.newline()
				}
				s.cells[s.y][s.x] = graphemes.Str()
				for k := 1; k < size; k++ {
					s.cells[s.y][s.x+k] = ""
				}
				s.x += size
				if s.x >= s.width {
					s.x = s.width - 1
					s.pending = true
				}
			}
			text = text[end:]
		}
	}
	return len(data), nil
}

func (s *screenState) lines() []string {
	var lines []string
	for _, cells := range s.cells {
		var line strings.Builder
		for _, cell := range cells {
			if cell == "" {
				cell = " "
			}
			line.WriteString(cell)
		}
		lines = append(lines, strings.TrimRight(line.String(), " "))
	}
	return lines
}

func TestStreamingFramesKeepExactlyOneComposerGap(t *testing.T) {
	a := testApp(t)
	screen := newScreen(70, 24)
	a.view.output = screen
	a.view.width, a.view.height = 70, 24
	a.run = &core.Run{ID: "spacing"}
	a.stopRun = func() {}
	a.activity.start(time.Now())
	emit := func(kind string, value any) {
		data, _ := json.Marshal(value)
		a.onRunEvent(runEvent{event: core.Event{Kind: kind, Data: data}})
	}
	frame := func() {
		t.Helper()
		if err := a.draw(); err != nil {
			t.Fatal(err)
		}
		lines := screen.lines()
		if !screen.visible || screen.x != 2 || !strings.HasPrefix(lines[screen.y], "❯ ") {
			t.Fatalf("cursor displaced (%d,%d):\n%s", screen.x, screen.y, strings.Join(lines, "\n"))
		}
		border := screen.y - 1
		if border >= 2 && lines[border] == strings.Repeat("─", screen.width) {
			if lines[border-1] != "" || lines[border-2] == "" {
				t.Fatalf("composer gap changed:\n%s", strings.Join(lines, "\n"))
			}
		}
	}
	emit("text_start", model.StreamPart{ID: "text"})
	for _, delta := range []string{"Hello", " world", "\n\n", "## Finding", "\n", "- first", "\n- second", "\n\n```go\n", "package main", "\n```", "\n喵~"} {
		emit("text_delta", model.StreamPart{ID: "text", Delta: delta})
		frame()
	}
	before := screen.y
	emit("text_end", model.StreamPart{ID: "text"})
	frame()
	if screen.y != before {
		t.Fatal("text_end shifted composer")
	}
	emit("step.completed", model.Response{Content: model.ResponseContent{model.TextContent{Text: "final"}}})
	frame()
	if screen.y != before {
		t.Fatal("step completion temporarily showed an activity row")
	}
	a.onRunEvent(runEvent{done: true, event: core.Event{Kind: "run.completed"}})
	frame()
	if screen.y != before {
		t.Fatal("turn completion shifted composer")
	}
}

func TestLongStreamGapSurvivesResizeToolAndInterruption(t *testing.T) {
	a := testApp(t)
	a.run = &core.Run{ID: "long"}
	a.stopRun = func() {}
	a.activity.start(time.Now())
	a.activity.quiet = true
	for _, size := range [][2]int{{60, 24}, {22, 12}, {60, 24}, {10, 6}, {3, 2}} {
		screen := newScreen(size[0], size[1])
		a.view.output = screen
		a.view.width, a.view.height = size[0], size[1]
		a.reflow = true
		a.stream = strings.Repeat("Long streamed message.\n", 30) + "last"
		if err := a.draw(); err != nil {
			t.Fatal(err)
		}
		lines := screen.lines()
		if size[1] >= 6 && lines[screen.y-2] != "" {
			t.Fatalf("lost gap after resize: %v\n%s", size, strings.Join(lines, "\n"))
		}
		if !screen.visible {
			t.Fatal("stream hid native composer cursor")
		}
	}
	screen := newScreen(60, 24)
	a.view.output = screen
	a.view.width, a.view.height = 60, 24
	a.reflow = true
	tool := session.NewToolActivity(model.ToolCallContent{ToolCallID: "read", ToolName: "read", Input: `{"path":"file.txt"}`}, time.Now())
	a.onToolActivity(tool)
	if err := a.draw(); err != nil {
		t.Fatal(err)
	}
	if screen.lines()[screen.y-2] != "" {
		t.Fatal("tool activity lost separator")
	}
	a.onRunEvent(runEvent{done: true, event: core.Event{Kind: "run.cancelled"}, err: context.Canceled})
	if err := a.draw(); err != nil {
		t.Fatal(err)
	}
	if screen.lines()[screen.y-2] != "" || !screen.visible {
		t.Fatal("interruption lost separator or cursor")
	}

}

func TestTransientViewsShareOneBottomBlankLine(t *testing.T) {
	a := testApp(t)
	screen := newScreen(100, 12)
	screen.y = screen.height - 1
	a.view.output = screen
	a.view.width, a.view.height = screen.width, screen.height

	assertBottom := func(lastText string) {
		t.Helper()
		if err := a.draw(); err != nil {
			t.Fatal(err)
		}
		last := screen.y - a.view.cursorRow + a.view.rows - 1
		lines := screen.lines()
		if last < 1 || last >= len(lines) || lines[last] != "" || !strings.Contains(lines[last-1], lastText) {
			t.Fatalf("expected one blank after %q:\n%s", lastText, strings.Join(lines, "\n"))
		}
	}
	assertBottom("Ready")
	a.question = newQuestionForm(coding.QuestionRequest{Items: []coding.Question{
		{Question: "Pick an option", Options: []coding.QuestionOption{{Label: "First"}}},
	}})
	a.handle(key{code: "tab"})
	assertBottom("enter to submit answer")
	a.handle(key{code: "escape"})
	assertBottom("enter to submit answer")
	a.question = nil
	a.picker = &picker{kind: "model", title: "Select model", items: []choice{{label: "demo"}}}
	assertBottom("Enter Confirm")
	tooltips := false
	a.host.Config.TUI.ShowTooltips = &tooltips
	assertBottom("demo")
	a.skillsMenu()
	assertBottom("Enable/Disable Skills")
	a.picker = nil
	assertBottom("Ready")
}
