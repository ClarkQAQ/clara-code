package terminal

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"strings"
	"testing"

	"clara-code/pkg/core/prompt"
	"clara-code/pkg/model"

	"clara-code/internal/coding"
	"clara-code/internal/session"
	"clara-code/internal/setting"
)

func TestStructuredQuestionAndLegacyOptions(t *testing.T) {
	for _, input := range []string{
		`{"questions":[{"id":"choice","header":"Display","question":"Is this working?","options":[{"label":"Yes","description":"Works as expected."},{"label":"No","description":"Needs changes."}]}]}`,
		`{"questions":[{"id":"choice","text":"Is this working?","options":["Yes","No"]}]}`,
	} {
		q := coding.NewQuestions()
		done := make(chan model.ToolResponse, 1)
		go func() {
			response, err := q.Tool().Run(t.Context(), model.ToolCall{Input: input})
			if err != nil {
				response = model.NewTextErrorResponse(err.Error())
			}
			done <- response
		}()
		var request coding.QuestionRequest
		select {
		case request = <-q.Requests:
		case response := <-done:
			t.Fatalf("question rejected: %+v", response)
		}
		form := newQuestionForm(request)
		p := painter{width: 100, height: 24, color: true}
		if rows := strings.Join(form.layout(&p).rows, "\n"); !strings.Contains(rows, "Question 1/1 (1 unanswered)") || !strings.Contains(rows, "1. Yes") || !strings.Contains(rows, "None of the above") {
			t.Fatalf("question layout %q", rows)
		}
		form.key(key{code: "down"})
		form.key(key{code: "tab"})
		form.key(key{code: "paste", text: "Please adjust the layout."})
		for _, width := range []int{100, 34, 18} {
			p.width = width
			for _, row := range form.layout(&p).rows {
				if widthOf(stripStyles(row)) > width {
					t.Fatalf("row exceeds %d cells: %q", width, row)
				}
			}
		}
		if !form.key(key{code: "enter"}) {
			t.Fatal("did not submit answer")
		}
		response := <-done
		if response.IsError {
			t.Fatal(response.Content)
		}
		var result struct {
			Answers map[string]coding.QuestionAnswer
		}
		if err := json.Unmarshal([]byte(response.Content), &result); err != nil {
			t.Fatal(err)
		}
		answer := result.Answers["choice"]
		if len(answer.Answers) != 1 || answer.Answers[0] != "No" || answer.Notes != "Please adjust the layout." {
			t.Fatalf("answer %+v", answer)
		}
	}
}

func TestMultipleQuestionsRetainAnswersAndNotes(t *testing.T) {
	request := coding.QuestionRequest{Items: []coding.Question{
		{ID: "one", Question: "First", Options: []coding.QuestionOption{{Label: "A"}, {Label: "B"}}},
		{ID: "two", Question: "Second"},
	}, Answer: make(chan map[string]string, 1)}
	q := newQuestionForm(request)
	q.key(key{code: "text", text: "2"})
	q.key(key{code: "tab"})
	q.key(key{code: "paste", text: "first note"})
	q.key(key{code: "tab"})
	q.key(key{code: "right"})
	q.key(key{code: "paste", text: "free answer"})
	q.key(key{code: "tab"})
	q.key(key{code: "left"})
	if q.drafts[0].selected != 1 || q.drafts[0].notes.text != "first note" {
		t.Fatal("lost first question draft")
	}
	if q.key(key{code: "enter"}) {
		t.Fatal("submitted with unanswered question")
	}
	if !q.key(key{code: "enter"}) {
		t.Fatal("did not submit all questions")
	}
	result := <-request.Answer
	if !strings.Contains(result["one"], "first note") || !strings.Contains(result["two"], "free answer") {
		t.Fatalf("answers %v", result)
	}
}

func TestResizeRebuildUsesSourcesAndRetainsQuestionState(t *testing.T) {
	var output bytes.Buffer
	a := &app{ctx: t.Context(), view: painter{output: &output, width: 100, height: 24}}
	a.view.welcome("test", "demo", "/workspace")
	a.view.message("user", "Original prompt with a long line that must reflow when the terminal narrows.")
	a.view.message("assistant", "## Result\nA **long reply** with `code` and [a link](https://example.test).")
	a.question = newQuestionForm(coding.QuestionRequest{Items: []coding.Question{{ID: "pick", Question: "Resize question", Options: []coding.QuestionOption{{Label: "First", Description: "A long option description that wraps."}, {Label: "Second"}}}}})
	a.question.key(key{code: "down"})
	for _, width := range []int{100, 44, 27, 80} {
		output.Reset()
		a.view.width = width
		a.reflow = true
		if err := a.draw(); err != nil {
			t.Fatal(err)
		}
		current := output.String()
		if strings.Count(current, "Resize question") != 1 {
			t.Fatalf("repeated transient question at width %d: %q", width, current)
		}
		if strings.Count(current, "Original prompt") != 1 {
			t.Fatal("history duplicated during rebuild")
		}
		if a.question.drafts[0].selected != 1 {
			t.Fatal("resize lost selection")
		}
		if len(a.view.history) != 2 {
			t.Fatal("transient question entered history")
		}
		for _, line := range a.question.layout(&a.view).rows {
			if widthOf(stripStyles(line)) > width {
				t.Fatalf("overflow at %d: %q", width, line)
			}
		}
	}
	a.question = nil
	a.reflow = true
	output.Reset()
	if err := a.draw(); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(output.String(), "Resize question") {
		t.Fatal("dismissed question entered rebuilt history")
	}
}

func testApp(t *testing.T) *app {
	t.Helper()
	space, err := coding.ResolveWorkspace(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	host := coding.Host{Config: setting.Config{Home: t.TempDir(), DefaultModel: "demo", Models: map[string]setting.Model{"demo": {ID: "demo", ContextWindow: 1_048_576}}},
		Models: func(context.Context, string) (model.LanguageModel, error) { return coding.DemoModel{}, nil }}
	store := session.New(host.Config.Home, session.Meta{Project: space.Root, Cwd: space.Cwd, Model: "demo"})
	conversation, err := host.Open(t.Context(), space, store, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	a := &app{ctx: t.Context(), host: host, space: space, conv: conversation, options: Options{Workspace: space, Version: "test"},
		mode: prompt.Execute, view: painter{output: io.Discard, width: 140, height: 24}, historyAt: -1}
	t.Cleanup(func() { a.conv.Close() })
	return a
}

func TestCommandMenuAndClearKeepSavedSession(t *testing.T) {
	a := testApp(t)
	a.editor.replace("/")
	if got := a.menuMatches(); len(got) == 0 || got[0].name != "/model" {
		t.Fatalf("menu %v", got)
	}
	a.handle(key{code: "down"})
	a.handle(key{code: "tab"})
	if a.editor.text != "/skills" {
		t.Fatalf("completion %q", a.editor.text)
	}
	a.editor.replace("/res")
	matches := a.menuMatches()
	if len(matches) != 1 || matches[0].name != "/resume" {
		t.Fatalf("filter %v", matches)
	}
	a.handle(key{code: "escape"})
	if len(a.menuMatches()) != 0 || a.editor.text != "/res" {
		t.Fatal("escape did not preserve command draft")
	}
	a.editor.replace("/clear")
	parent := a.conv.Session.ID()
	a.handle(key{code: "enter"})
	result := <-a.op
	a.stopOp()
	a.op = nil
	if result.err != nil {
		t.Fatal(result.err)
	}
	if err := a.install(result); err != nil {
		t.Fatal(err)
	}
	if a.conv.Session.ID() == parent || a.conv.Store.TurnCount() != 0 {
		t.Fatal("clear did not create a new session")
	}
	found, err := session.List(a.host.Config.Home, a.space.Root)
	if err != nil || len(found) != 2 {
		t.Fatalf("old session not retained: %+v %v", found, err)
	}
}

func TestContextAndRightAlignedMode(t *testing.T) {
	if got := (setting.Model{ContextWindow: 1_048_576}).EffectiveWindow(); got != 996147 {
		t.Fatalf("window %d", got)
	}
	for count, expected := range map[int64]string{996147: "996K", 11300: "11.3K", 1000000: "1M", 1048576: "1.05M"} {
		if got := tokenCount(count); got != expected {
			t.Fatalf("%d formatted as %q", count, got)
		}
	}
	if contextPercent(11300, 996147) != 0 || contextPercent(504073, 996147) != 50 {
		t.Fatal("baseline context percentage differs from Codex")
	}
	a := testApp(t)
	run, err := a.conv.Start(t.Context(), prompt.Execute, "hello", false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := run.Wait(t.Context()); err != nil {
		t.Fatal(err)
	}
	if label := a.contextLabel(); label != "15/996K (0%)" {
		t.Fatalf("context %q", label)
	}
	a.mode = prompt.Plan
	line := stripStyles(a.statusLine())
	if !strings.HasSuffix(line, "Plan mode (Shift+Tab to cycle)") || widthOf(line) != a.view.width {
		t.Fatalf("mode not right aligned: %q", line)
	}
	if err := a.conv.Store.InvalidateContext(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(a.contextLabel(), "unknown") {
		t.Fatal("invalidated context still reported as known")
	}
}

func TestLateInputStartsTheNextTurn(t *testing.T) {
	a := testApp(t)
	run, err := a.conv.Start(t.Context(), prompt.Execute, "initial", false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := run.Wait(t.Context()); err != nil {
		t.Fatal(err)
	}
	a.run = run
	a.stopRun = func() {}
	a.steer("late instruction", false)
	if len(a.pendingInput) != 1 {
		t.Fatal("late input was not retained")
	}
	a.onRunEvent(runEvent{done: true})
	if a.run == nil || a.run == run {
		t.Fatal("late input did not start a new run")
	}
	if _, err := a.run.Wait(t.Context()); err != nil {
		t.Fatal(err)
	}
	turns, err := a.conv.Store.Turns()
	if err != nil || len(turns) != 2 || turns[1].Prompt != "late instruction" {
		t.Fatalf("late turn %+v %v", turns, err)
	}
}

func TestMarkdownPreservesCodeAndFormatsLinks(t *testing.T) {
	p := painter{width: 50, height: 20, color: true}
	rows := p.markdownRows("## Heading\nA **bold** word and `code`.\n[label](https://example.test)\n```go\nfmt.Println(\"x\")\n```", 48)
	text := stripStyles(strings.Join(rows, "\n"))
	for _, expected := range []string{"Heading", "bold", "code", "label (https://example.test)", "fmt.Println(\"x\")"} {
		if !strings.Contains(text, expected) {
			t.Fatalf("missing %q from %q", expected, text)
		}
	}
	if strings.Contains(text, "**bold**") || strings.Contains(text, "## Heading") {
		t.Fatalf("markdown markers retained: %q", text)
	}
	for _, row := range rows {
		if widthOf(stripStyles(row)) > 48 {
			t.Fatalf("markdown overflow %q", row)
		}
	}
}

func TestMultilineCursorPreservesDisplayColumn(t *testing.T) {
	var input editor
	input.replace("你好abcd\nx\n12345678")
	input.at = len("你好abcd")
	input.vertical(1, 20)
	if input.at != len("你好abcd\nx") {
		t.Fatalf("short row cursor %d", input.at)
	}
	input.vertical(1, 20)
	if input.at != len(input.text) {
		t.Fatalf("display column lost: %d", input.at)
	}
	input.vertical(-1, 20)
	input.vertical(-1, 20)
	if input.at != len("你好abcd") {
		t.Fatalf("return column %d", input.at)
	}
	input.key(key{code: "left"})
	input.vertical(1, 20)
	input.vertical(1, 20)
	if got := input.text[input.at:]; got != "8" {
		t.Fatalf("horizontal move did not reset column: %q", got)
	}
}
