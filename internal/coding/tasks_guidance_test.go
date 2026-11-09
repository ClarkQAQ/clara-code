package coding

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"

	"clara-code/pkg/core/prompt"
	"clara-code/pkg/model"
)

func TestDelegationContractReachesModelAndInvalidNameCanBeCorrected(t *testing.T) {
	var children atomic.Int32
	m := fixtureModel{reply: func(call model.Call) (*model.Response, error) {
		body := contextText(call.Prompt)
		if strings.Contains(body, "<task_assignment>") {
			children.Add(1)
			if !strings.Contains(body, "Current role: explorer") || !strings.Contains(body, "<access>read_only</access>") {
				return nil, fmt.Errorf("child lost role or read-only boundary")
			}
			for _, tool := range call.Tools {
				switch tool.GetName() {
				case "exec_command", "write_stdin":
					return nil, fmt.Errorf("read-only child exposed %s", tool.GetName())
				}
			}
			return demoText("Inspection complete."), nil
		}
		for _, expected := range []string{"^[a-z0-9_]{1,100}$", "repo_survey", "fork_history defaults to false", "timeout_ms", "explorer and reviewer are read-only", "Inspect list_agents"} {
			if !strings.Contains(body, expected) {
				return nil, fmt.Errorf("missing delegation guidance: %s", expected)
			}
		}
		var outputs []string
		for _, message := range call.Prompt {
			for _, part := range message.Content {
				if result, ok := model.AsMessagePart[model.ToolResultPart](part); ok {
					text, ok := model.AsToolResultOutputType[model.ToolResultOutputContentText](result.Output)
					if ok {
						outputs = append(outputs, text.Text)
					} else if failure, ok := model.AsToolResultOutputType[model.ToolResultOutputContentError](result.Output); ok {
						outputs = append(outputs, failure.Error.Error())
					}
				}
			}
		}
		switch len(outputs) {
		case 0:
			for _, item := range call.Tools {
				if item.GetName() == "spawn_agent" {
					schema := item.(model.FunctionTool).InputSchema
					property := schema["properties"].(map[string]any)["task_name"].(map[string]any)
					if property["pattern"] != "^[a-z0-9_]{1,100}$" {
						return nil, fmt.Errorf("model did not receive task_name schema")
					}
				}
			}
			return toolReply("spawn_agent", map[string]any{"task_name": "repo-survey", "role": "explorer", "message": "Inspect the project."}, 1), nil
		case 1:
			if children.Load() != 0 || !strings.Contains(outputs[0], "task_name") || !strings.Contains(outputs[0], "^[a-z0-9_]{1,100}$") {
				return nil, fmt.Errorf("bad name started a child or returned unclear error: %s", outputs[0])
			}
			return toolReply("subagent_task", map[string]any{"task_name": "repo_survey", "role": "explorer", "message": "Inspect go.mod and report the module name. Do not modify files."}, 2), nil
		case 2:
			if !strings.Contains(outputs[1], "Inspection complete.") || children.Load() != 1 {
				return nil, fmt.Errorf("corrected task failed: %s", outputs[1])
			}
			return demoText("Delegation verified."), nil
		default:
			return nil, fmt.Errorf("unexpected step")
		}
	}}
	c, _ := testHost(t, t.TempDir(), m)
	if got := finish(t, c, prompt.Execute, "Use a read-only subagent to inspect the project.").Response.Content.Text(); got != "Delegation verified." {
		t.Fatal(got)
	}
}

func TestDelegationSchemaReachesBothProtocols(t *testing.T) {
	for _, protocol := range []string{"responses", "completions"} {
		t.Run(protocol, func(t *testing.T) {
			c, _ := remoteFixture(t, func(w http.ResponseWriter, r *http.Request) {
				var payload struct{ Tools []map[string]any }
				if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
					t.Error(err)
				}
				found := 0
				for _, item := range payload.Tools {
					if protocol == "completions" {
						item, _ = item["function"].(map[string]any)
					}
					if item["name"] != "spawn_agent" && item["name"] != "subagent_task" {
						continue
					}
					found++
					parameters, _ := item["parameters"].(map[string]any)
					properties, _ := parameters["properties"].(map[string]any)
					name, _ := properties["task_name"].(map[string]any)
					if name["pattern"] != "^[a-z0-9_]{1,100}$" || !strings.Contains(fmt.Sprint(name["description"]), "repo_survey") {
						t.Error("provider lost task naming contract", name)
					}
				}
				if found != 2 {
					t.Error("delegation tools missing from wire request", found)
				}
				if protocol == "responses" {
					sendResponseText(w)
				} else {
					sendTextSSE(w, "Contract checked.", true)
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
			finish(t, c, prompt.Execute, "Describe the available delegation contract.")
		})
	}
}
