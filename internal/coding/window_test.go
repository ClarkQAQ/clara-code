package coding

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"clara-code/internal/session"
	"clara-code/pkg/core/prompt"
)

type requestWindow struct {
	Instructions string            `json:"instructions"`
	Input        []json.RawMessage `json:"input"`
	Messages     []json.RawMessage `json:"messages"`
}

func TestRequestWindowPrefixSurvivesTurnsRuleChangesAndResume(t *testing.T) {
	for _, protocol := range []string{"responses", "completions"} {
		t.Run(protocol, func(t *testing.T) {
			requests := make(chan requestWindow, 8)
			c, _ := remoteFixture(t, func(w http.ResponseWriter, r *http.Request) {
				var frame requestWindow
				if err := json.NewDecoder(r.Body).Decode(&frame); err != nil {
					t.Error(err)
				}
				if protocol == "completions" {
					frame.Input = frame.Messages
				}
				requests <- frame
				if protocol == "responses" {
					sendResponseText(w)
				} else {
					sendTextSSE(w, "Done.", true)
				}
			})
			c.Host.Config.Protocol = protocol
			factory, err := c.Host.Config.Factory()
			if err != nil {
				t.Fatal(err)
			}
			c.Host.Models = factory
			if err := c.ChangeModel(t.Context(), "fixture", "high"); err != nil {
				t.Fatal(err)
			}
			now := time.Date(2026, 11, 9, 1, 2, 3, 0, time.UTC)
			c.Host.now = func() time.Time { return now }
			writeTestFile(t, c.Workspace.Cwd, "AGENTS.md", "RULE-FULL-CONTENT-alpha")
			var previous requestWindow
			for index, input := range []string{"first task", "second task", "changed rule", "removed rule"} {
				if index == 2 {
					writeTestFile(t, c.Workspace.Cwd, "AGENTS.md", "RULE-FULL-CONTENT-beta")
				}
				if index == 3 {
					if err := os.Remove(filepath.Join(c.Workspace.Cwd, "AGENTS.md")); err != nil {
						t.Fatal(err)
					}
				}
				now = now.Add(time.Second)
				finish(t, c, prompt.Execute, input)
				frame := <-requests
				if index == 0 {
					policyIndex := 0
					if protocol == "completions" {
						policyIndex = 1
					}
					var policy struct {
						Role    string
						Content json.RawMessage
					}
					if err := json.Unmarshal(frame.Input[policyIndex], &policy); err != nil || policy.Role != "developer" {
						t.Fatal("developer policy missing", err)
					}
					var text string
					if protocol == "responses" {
						var blocks []struct{ Text string }
						if err := json.Unmarshal(policy.Content, &blocks); err != nil || len(blocks) != 1 {
							t.Fatal("developer policy not consolidated", err)
						}
						text = blocks[0].Text
					} else if err := json.Unmarshal(policy.Content, &text); err != nil {
						t.Fatal("developer policy not a single text", err)
					}
					for _, section := range []string{"<permissions_instructions>", "<collaboration_mode>", "# File search", "# Anchored file edits"} {
						if !strings.Contains(text, section) {
							t.Fatal("merged policy lost section", section)
						}
					}
				}
				if index > 0 && (len(frame.Input) < len(previous.Input) || !reflect.DeepEqual(frame.Input[:len(previous.Input)], previous.Input)) {
					t.Fatalf("request %d changed the old input prefix", index)
				}
				body, _ := json.Marshal(frame.Input)
				text := strings.ReplaceAll(strings.ReplaceAll(string(body), `\u003c`, "<"), `\u003e`, ">")
				if strings.Count(text, "environment_context>") != 2 || strings.Contains(text, "timestamp>") || strings.Contains(text, "local_datetime>") || strings.Contains(text, "message_context>") {
					t.Fatal("same-day requests added redundant environment context", text)
				}
				if strings.Contains(text, "context_fragment") || strings.Contains(text, `\"lifecycle\"`) || strings.Count(text, "RULE-FULL-CONTENT-alpha") != 1 {
					t.Fatal("metadata or repeated rules in wire input")
				}
				if index >= 2 && strings.Count(text, "RULE-FULL-CONTENT-beta") != 1 {
					t.Fatal("changed rule missing or repeated")
				}
				if index == 3 && !strings.Contains(text, "no longer applies") {
					t.Fatal("removed rule still appears active")
				}
				if protocol == "responses" {
					if !strings.HasPrefix(frame.Instructions, "You are Clara Code") || strings.Contains(text, "You are Clara Code") {
						t.Fatal("Responses base was duplicated or not moved to instructions")
					}
				} else if frame.Instructions != "" || !strings.Contains(text, "You are Clara Code") {
					t.Fatal("Completions lost its system instructions")
				}
				previous = frame
			}
			before := c.Session.ContextState()
			if len(before) == 0 {
				t.Fatal("model window not checkpointed")
			}
			host, workspace, id := c.Host, c.Workspace, c.Session.ID()
			if err := c.Close(); err != nil {
				t.Fatal(err)
			}
			store, err := session.Open(host.Config.Home, id, workspace.Root)
			if err != nil {
				t.Fatal(err)
			}
			resumed, err := host.Open(t.Context(), workspace, store, nil, nil)
			if err != nil {
				t.Fatal(err)
			}
			defer resumed.Close()
			now = now.Add(time.Second)
			finish(t, resumed, prompt.Execute, "after resume")
			frame := <-requests
			if len(frame.Input) < len(previous.Input) || !reflect.DeepEqual(frame.Input[:len(previous.Input)], previous.Input) {
				t.Fatal("resume moved the existing prefix")
			}
			for _, message := range resumed.Session.Messages() {
				if strings.Contains(session.Text(message), "<environment_context>") {
					t.Fatal("context entered raw history")
				}
			}
		})
	}
}
