package terminal

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"clara-code/pkg/core"
	"clara-code/pkg/model"

	"clara-code/internal/session"
)

func TestToolGroupUpdatesInPlaceAndCommitsOnce(t *testing.T) {
	a := testApp(t)
	started := time.Now()
	first := session.NewToolActivity(model.ToolCallContent{ToolCallID: "first", ToolName: "exec_command", Input: `{"command":"printf first"}`}, started)
	second := session.NewToolActivity(model.ToolCallContent{ToolCallID: "second", ToolName: "read", Input: `{"path":"file.go"}`}, started)
	a.onToolActivity(first)
	a.onToolActivity(second)
	if len(a.view.history) != 0 || len(a.toolGroup) != 2 {
		t.Fatal("running tools entered scrollback")
	}
	second.Status = "succeeded"
	second.Output = "file contents"
	a.onToolActivity(second)
	if len(a.view.history) != 0 {
		t.Fatal("parallel group committed before all results")
	}
	exit := 3
	first.Status = "failed"
	first.ExitCode = &exit
	first.Output = "command output"
	a.onToolActivity(first)
	if len(a.toolGroup) != 0 || len(a.view.history) != 2 {
		t.Fatalf("tool group %+v history %+v", a.toolGroup, a.view.history)
	}
	rows := strings.Join(a.view.cellRows(a.view.history[0]), "\n")
	if !strings.Contains(stripStyles(rows), "Bash(printf first) · exit 3") || !strings.Contains(rows, "└ command output") {
		t.Fatalf("tool layout %q", rows)
	}
	// A later update of a retained command changes that cell instead of adding a result message.
	first.Output += "\nmore output"
	a.onToolActivity(first)
	if len(a.view.history) != 2 || !a.reflow {
		t.Fatal("tool update duplicated its source cell")
	}
	for _, width := range []int{100, 32, 12, 2} {
		a.view.width = width
		for _, row := range a.view.toolRows(first, time.Now()) {
			if widthOf(stripStyles(row)) > width {
				t.Fatalf("tool row exceeds width %d: %q", width, row)
			}
		}
	}
}

func TestSkillToolActivity(t *testing.T) {
	a := testApp(t)
	a.run = &core.Run{ID: "run"}
	now := time.Now()
	activity := session.NewToolActivity(model.ToolCallContent{
		ToolCallID: "skill",
		ToolName:   "read_skill",
		Input:      `{"name":"git-commit"}`,
	}, now)
	a.onToolActivity(activity)
	rows := stripStyles(strings.Join(a.view.toolRows(a.toolGroup[0], now), "\n"))
	if !strings.Contains(rows, "Skill(git-commit)") {
		t.Fatalf("running tool rows %q", rows)
	}
	summary := stripStyles(strings.Join(a.activityRows(now), "\n"))
	if !strings.Contains(summary, "Running Skill (") {
		t.Fatalf("running tool summary %q", summary)
	}
	activity.Status = "succeeded"
	activity.Output = "Skill instructions"
	a.onToolActivity(activity)
	rows = stripStyles(strings.Join(a.view.cellRows(a.view.history[0]), "\n"))
	if !strings.Contains(rows, "Skill(git-commit)") || !strings.Contains(rows, "└ Skill instructions") {
		t.Fatalf("completed tool rows %q", rows)
	}

	var restored session.Cell
	if err := json.Unmarshal([]byte(`{
		"Kind":"tool",
		"Tool":{
			"name":"read_skill_resource",
			"title":"read_skill_resource",
			"detail":"git-commit references/style.md",
			"arguments":"{\"name\":\"git-commit\",\"path\":\"references/style.md\"}",
			"status":"failed",
			"error":"resource missing"
		}
	}`), &restored); err != nil {
		t.Fatal(err)
	}
	rows = stripStyles(strings.Join(a.view.cellRows(restored), "\n"))
	if !strings.Contains(rows, "Skill(git-commit/references/style.md) · failed") || !strings.Contains(rows, "└ resource missing") {
		t.Fatalf("restored tool rows %q", rows)
	}
}

func TestSkillToolActivityArgumentFallbacks(t *testing.T) {
	a := testApp(t)
	now := time.Now()
	for _, test := range []struct {
		name, tool, input, want string
	}{
		{"missing name", "read_skill", `{}`, "Skill"},
		{"missing resource name", "read_skill_resource", `{"path":"style.md"}`, "Skill"},
		{"missing resource path", "read_skill_resource", `{"name":"git-commit"}`, "Skill(git-commit)"},
		{"no arguments", "read_skill", "", "Skill"},
		{"malformed arguments", "read_skill_resource", `{"name":"git-commit"`, "Skill"},
		{"invalid argument type", "read_skill_resource", `{"name":"git-commit","path":2}`, "Skill"},
	} {
		t.Run(test.name, func(t *testing.T) {
			activity := session.NewToolActivity(model.ToolCallContent{ToolName: test.tool, Input: test.input}, now)
			_, heading, _ := strings.Cut(stripStyles(a.view.toolRows(activity, now)[0]), " ")
			if heading != test.want {
				t.Fatalf("tool heading %q, want %q", heading, test.want)
			}
		})
	}
}

func TestStandardToolActivityLabels(t *testing.T) {
	a := testApp(t)
	a.run = &core.Run{ID: "run"}
	now := time.Now()
	for _, test := range []struct {
		tool, input, title, detail string
	}{
		{"stat", `{"path":"src/main.go"}`, "Stat", "src/main.go"},
		{"fs_instructions", `{"path":"src"}`, "Instructions", "src"},
		{"review_changes", `{"kind":"staged","offset":20}`, "ReviewChanges", "staged"},
		{"get_plan", `{}`, "Plan", ""},
		{"update_plan", `{"plan":[{"step":"Write code","status":"pending"}]}`, "UpdatePlan", ""},
		{"get_goal", `{}`, "Goal", ""},
		{"update_goal", `{"status":"completed","summary":"Verified the changes"}`, "UpdateGoal", "completed"},
		{"spawn_agent", `{"task_name":"repo_survey","message":"Inspect the repo"}`, "SpawnAgent", "repo_survey"},
		{"subagent_task", `{"task_name":"repo_survey"}`, "SubagentTask", "repo_survey"},
		{"list_agents", `{}`, "ListAgents", ""},
		{"send_message", `{"target":"task_1","message":"Inspect the repo"}`, "SendMessage", "task_1"},
		{"followup_task", `{"target":"task_1"}`, "FollowupTask", "task_1"},
		{"wait_agent", `{"target":"task_1","timeout_ms":1000}`, "WaitAgent", "task_1"},
		{"interrupt_agent", `{"target":"task_1"}`, "InterruptAgent", "task_1"},
		{"close_agent", `{"target":"task_1"}`, "CloseAgent", "task_1"},
		{"list_acp_sessions", `{"role":"external","cursor":"page-2"}`, "ListACPSessions", "external"},
		{"stat", `{}`, "Stat", ""},
		{"wait_agent", `{"target":42}`, "WaitAgent", ""},
		{"spawn_agent", `{"task_name":"repo_survey"`, "SpawnAgent", ""},
	} {
		t.Run(test.tool, func(t *testing.T) {
			activity := session.NewToolActivity(model.ToolCallContent{ToolName: test.tool, Input: test.input}, now)
			a.toolGroup = []session.ToolActivity{activity}
			summary := stripStyles(strings.Join(a.activityRows(now), "\n"))
			if !strings.Contains(summary, "Running "+test.title+" (") {
				t.Fatalf("running tool summary %q", summary)
			}
			activity.Status = "succeeded"
			rows := a.view.cellRows(session.Cell{Kind: "tool", Tool: &activity})
			_, heading, _ := strings.Cut(stripStyles(rows[0]), " ")
			want := test.title + "(" + test.detail + ")"
			if heading != want {
				t.Fatalf("tool heading %q, want %q", heading, want)
			}
		})
	}
}

func TestIndependentActivityClockRetryAndPublicSummary(t *testing.T) {
	a := testApp(t)
	a.run = &core.Run{ID: "run"}
	now := time.Now()
	a.activity.start(now.Add(-65 * time.Second))
	a.activity.header = "Proofing"
	rows := strings.Join(a.activityRows(now), "\n")
	if !strings.Contains(stripStyles(rows), "Proofing (1m 05s · esc to interrupt)") {
		t.Fatalf("activity row %q", rows)
	}
	retry := session.RetryStatus{Attempt: 2, Limit: 3, DelayMS: 3000, Kind: "server", Message: "HTTP 503"}
	body, _ := json.Marshal(retry)
	a.onRunEvent(runEvent{event: core.Event{Kind: "model.retry", Data: body}})
	rows = strings.Join(a.activityRows(now), "\n")
	if !strings.Contains(rows, "Reconnecting… 2/3") || !strings.Contains(rows, "HTTP 503") {
		t.Fatalf("retry activity %q", rows)
	}
	summary := model.StreamPart{ID: "reasoning", ProviderMetadata: model.ProviderMetadata{ReasoningSummary: []string{"**Inspecting the workspace**\nCheck the project layout."}}}
	body, _ = json.Marshal(summary)
	a.onRunEvent(runEvent{event: core.Event{Kind: "reasoning_delta", Data: body}})
	rows = strings.Join(a.activityRows(now), "\n")
	if a.activity.retry != nil || !strings.Contains(rows, "Inspecting the workspace") {
		t.Fatalf("summary did not restore activity: %q", rows)
	}
	if elapsedCompact(2*time.Hour+3*time.Minute+4*time.Second) != "2h 03m 04s" {
		t.Fatal("elapsed time format differs")
	}
	a.activity.pause(now)
	a.activity.resume(now.Add(30 * time.Second))
	rows = strings.Join(a.activityRows(now.Add(30*time.Second)), "\n")
	if !strings.Contains(rows, "1m 05s") {
		t.Fatal("answering a question advanced the activity timer")
	}
	a.run = nil
	if a.activityRows(now) != nil {
		t.Fatal("completed activity row remained visible")
	}
}

func TestRetryClearsOnResponseAndFinalFailurePreservesDraft(t *testing.T) {
	for _, draft := range []string{"", "My next instruction"} {
		t.Run(draft, func(t *testing.T) {
			a := testApp(t)
			a.run = &core.Run{ID: "failed"}
			a.stopRun = func() {}
			a.submitted = "Retry this prompt"
			a.editor.replace(draft)
			a.activity.start(time.Now())
			a.activity.retry = &session.RetryStatus{Attempt: 1, Limit: 3}
			a.onRunEvent(runEvent{event: core.Event{Kind: "model.responding"}})
			if a.activity.retry != nil {
				t.Fatal("reconnect message remained after a response")
			}
			a.onRunEvent(runEvent{event: core.Event{Kind: "run.failed"}, done: true, err: errTest("request failed")})
			want := draft
			if want == "" {
				want = "Retry this prompt"
			}
			if a.editor.text != want || a.activityRows(time.Now()) != nil || a.run != nil {
				t.Fatalf("failure left stale state or discarded draft: %q %+v", a.editor.text, a.activity)
			}
		})
	}
}

func TestModelMessageBoundariesAndInterruptedText(t *testing.T) {
	a := testApp(t)
	send := func(kind string, part model.StreamPart) {
		body, _ := json.Marshal(part)
		a.onRunEvent(runEvent{event: core.Event{Kind: kind, Data: body}})
	}
	for _, text := range []string{"I will inspect the project.", "I found the failing command."} {
		send("text_start", model.StreamPart{ID: text})
		send("text_delta", model.StreamPart{ID: text, Delta: text})
		send("text_end", model.StreamPart{ID: text})
	}
	if len(a.view.history) != 2 || a.view.history[0].Text == a.view.history[1].Text {
		t.Fatalf("progress messages merged: %+v", a.view.history)
	}
	send("text_delta", model.StreamPart{ID: "partial", Delta: "Received before the stream disconnected."})
	a.flushStream()
	failure := session.Failure{Kind: "stream", Message: "Connection interrupted"}
	a.activity.failure = &failure
	a.runError(errTest("unexpected EOF"))
	if len(a.view.history) != 4 || a.view.history[2].Text != "Received before the stream disconnected." || !strings.Contains(a.view.history[3].Text, "stream") {
		t.Fatalf("interrupted text or error lost: %+v", a.view.history)
	}
}

type errTest string

func (e errTest) Error() string { return string(e) }
