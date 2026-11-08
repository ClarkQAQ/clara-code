package coding

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"clara-code/pkg/core"
	"clara-code/pkg/core/prompt"
	modelapi "clara-code/pkg/model"
	"clara-code/pkg/tool"
	"clara-code/pkg/util/localtime"

	"clara-code/internal/session"
)

func TestEnvironmentDateRefreshesBetweenModelStepsWithoutEnteringHistory(t *testing.T) {
	times := []time.Time{
		time.Date(2026, 11, 7, 15, 59, 59, 123456789, time.UTC),
		time.Date(2026, 11, 8, 16, 0, 1, 987654321, time.UTC),
	}
	var requests, ticks atomic.Int32
	model := fixtureModel{reply: func(call modelapi.Call) (*modelapi.Response, error) {
		step := int(requests.Add(1)) - 1
		if step >= len(times) {
			return nil, fmt.Errorf("unexpected request %d", step)
		}
		var clock string
		for _, message := range call.Prompt {
			text := session.Text(message)
			if message.Role == modelapi.MessageRoleSystem {
				if strings.Contains(text, "## Current local time") || !strings.Contains(text, prompt.Default("base")) {
					return nil, fmt.Errorf("clock snapshot entered stable prompt or time guidance missing")
				}
			}
			if message.Metadata != nil && message.Metadata.Kind == "context" {
				clock += text
			}
		}
		stamp := "<current_date>" + times[step].Local().Format(time.DateOnly) + "</current_date>"
		if !strings.Contains(clock, stamp) || strings.Count(clock, stamp) != 1 || strings.Contains(clock, "<timestamp>") || strings.Contains(clock, "<local_datetime>") {
			return nil, fmt.Errorf("missing current step clock: %s", clock)
		}
		if step == 0 {
			return toolReply("stat", map[string]any{"path": "clock.txt"}, 1), nil
		}
		return demoText("Clock checked."), nil
	}}
	c, _ := testHost(t, t.TempDir(), model)
	c.Host.now = func() time.Time { return times[int(ticks.Add(1))-1] }
	writeTestFile(t, c.Workspace.Cwd, "clock.txt", "clock fixture")
	finish(t, c, prompt.Execute, "Read the file timestamp.")
	if ticks.Load() != 2 || requests.Load() != 2 {
		t.Fatalf("clock ticks %d, model requests %d", ticks.Load(), requests.Load())
	}
	for _, message := range c.Session.Messages() {
		if strings.Contains(session.Text(message), "## Current local time") {
			t.Fatal("ephemeral clock entered saved model history")
		}
	}
}

func TestClockSharedByModesAndTasks(t *testing.T) {
	c, _ := testHost(t, t.TempDir(), DemoModel{})
	now := time.Date(2026, 11, 8, 1, 2, 3, 0, time.UTC)
	c.Host.now = func() time.Time { return now }
	child, err := c.buildTask(t.Context(), tool.TaskSpec{Name: "planner", Role: "explorer"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	for index, prepare := range []core.PrepareStepFunction{
		c.Call(prompt.Execute, false).PrepareStep,
		c.Call(prompt.Plan, false).PrepareStep,
		c.Call(prompt.Execute, true).PrepareStep,
		child.PrepareStep,
	} {
		_, result, err := prepare(t.Context(), core.PrepareStepFunctionOptions{})
		if err != nil {
			t.Fatal(err)
		}
		body := contextText(result.Messages)
		access := "read_only"
		if index == 0 {
			access = "unrestricted"
		}
		if !strings.Contains(body, "<current_date>"+now.Local().Format(time.DateOnly)+"</current_date>") || !strings.Contains(body, "<access>"+access+"</access>") || !strings.Contains(body, "<timezone>"+localtime.Zone(now)+"</timezone>") {
			t.Fatal("mode or child agent lost date, zone or permissions", body)
		}
		for _, field := range []string{"project_root", "model", "image_input", "effective_context_window", "local_datetime", "timestamp", "message_context"} {
			if strings.Contains(body, "<"+field+">") {
				t.Fatal("removed environment field survived", field)
			}
		}
	}
}

func TestClockRefreshesAfterCompactionForkAndResume(t *testing.T) {
	c, _ := testHost(t, t.TempDir(), DemoModel{})
	old := time.Date(2026, 11, 7, 23, 59, 59, 0, time.UTC)
	c.Host.now = func() time.Time { return old }
	finish(t, c, prompt.Execute, "Historical appointment: 2026-11-07T23:00:00+08:00.")
	finish(t, c, prompt.Execute, "Continue the appointment discussion.")
	if err := c.Compact(t.Context()); err != nil {
		t.Fatal(err)
	}
	now := old.Add(25 * time.Hour)
	host := c.Host
	host.now = func() time.Time { return now }
	c.Host = host
	check := func(c *Conversation) {
		t.Helper()
		messages := c.Session.Messages()
		_, result, err := c.Call(prompt.Execute, false).PrepareStep(t.Context(), core.PrepareStepFunctionOptions{Messages: messages, ContextState: c.Session.ContextState()})
		if err != nil {
			t.Fatal(err)
		}
		clock := contextText(result.Messages)
		if !strings.Contains(clock, now.Local().Format(time.DateOnly)) || strings.LastIndex(clock, now.Local().Format(time.DateOnly)) < strings.LastIndex(clock, old.Local().Format(time.DateOnly)) || strings.Contains(clock, "<timestamp>") {
			t.Fatal("old clock survived a context transition", clock)
		}
	}
	check(c)
	fork, err := c.Store.Fork(t.Context(), -1)
	if err != nil {
		t.Fatal(err)
	}
	branch, err := host.Open(t.Context(), c.Workspace, fork, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer branch.Close()
	check(branch)
	id, root := c.Session.ID(), c.Workspace.Root
	if err := c.Close(); err != nil {
		t.Fatal(err)
	}
	store, err := session.Open(host.Config.Home, id, root)
	if err != nil {
		t.Fatal(err)
	}
	resumed, err := host.Open(t.Context(), c.Workspace, store, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer resumed.Close()
	check(resumed)
}

func TestCurrentClockReachesBothModelProtocols(t *testing.T) {
	for _, protocol := range []string{"completions", "responses"} {
		t.Run(protocol, func(t *testing.T) {
			now := time.Date(2026, 11, 8, 0, 1, 2, 123456789, time.UTC)
			var requests atomic.Int32
			c, _ := remoteFixture(t, func(w http.ResponseWriter, r *http.Request) {
				requests.Add(1)
				var payload any
				if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
					t.Error(err)
				}
				body := fmt.Sprint(payload)
				if !strings.Contains(body, "role:developer") {
					t.Error("wire payload lost developer policy")
				}
				if !strings.Contains(body, "<current_date>"+now.Local().Format(time.DateOnly)+"</current_date>") || !strings.Contains(body, prompt.Default("base")) {
					t.Error("wire payload lacks clock or time policy")
				}
				if protocol == "completions" {
					sendTextSSE(w, "Clock checked.", true)
				} else {
					sendResponseText(w)
				}
			})
			c.Host.now = func() time.Time { return now }
			if protocol == "responses" {
				c.Host.Config.Protocol = protocol
				factory, err := c.Host.Config.Factory()
				if err != nil {
					t.Fatal(err)
				}
				c.Host.Models = factory
				if err := c.ChangeModel(context.Background(), "fixture", ""); err != nil {
					t.Fatal(err)
				}
			}
			finish(t, c, prompt.Execute, "What is the local date?")
			if requests.Load() != 1 {
				t.Fatal("unexpected model request count", requests.Load())
			}
		})
	}
}
