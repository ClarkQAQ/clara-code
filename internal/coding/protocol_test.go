package coding

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"clara-code/pkg/core/prompt"
	modelapi "clara-code/pkg/model"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"clara-code/internal/session"
	"clara-code/internal/setting"
)

func TestMCPToolsFromHTTP(t *testing.T) {
	server := sdk.NewServer(&sdk.Implementation{Name: "fixture", Version: "1"}, nil)
	type echo struct {
		Text string `json:"text"`
	}
	sdk.AddTool(server, &sdk.Tool{Name: "echo", Description: "Echo input"},
		func(_ context.Context, _ *sdk.CallToolRequest, input echo) (*sdk.CallToolResult, echo, error) {
			return nil, input, nil
		})
	endpoint := httptest.NewServer(sdk.NewStreamableHTTPHandler(func(*http.Request) *sdk.Server { return server }, nil))
	defer endpoint.Close()

	model := fixtureModel{reply: func(call modelapi.Call) (*modelapi.Response, error) {
		for _, message := range call.Prompt {
			if message.Role == modelapi.MessageRoleTool {
				body, _ := json.Marshal(message)
				if !strings.Contains(string(body), "MCP probe") {
					return nil, fmt.Errorf("tool result %s", body)
				}
				return demoText("MCP worked."), nil
			}
		}
		return toolReply("mcp__fixture__echo", echo{Text: "MCP probe"}, 1), nil
	}}
	initial, host := testHost(t, t.TempDir(), model)
	host.Config.MCP = map[string]setting.MCP{
		"fixture": {URL: endpoint.URL, Required: true},
		"bad":     {URL: endpoint.URL, Command: "invalid-conflict"},
	}
	space := initial.Workspace
	store := session.New(host.Config.Home, session.Meta{Project: space.Root, Cwd: space.Cwd, Model: "fixture"})
	conversation, err := host.Open(t.Context(), space, store, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer conversation.Close()
	statuses := conversation.MCPStatuses()
	if len(statuses) != 2 || statuses[0].OK || !statuses[1].OK || statuses[1].ToolCount != 1 {
		t.Fatalf("MCP statuses %+v", statuses)
	}
	if result := finish(t, conversation, prompt.Execute, "Call MCP.").Response.Content.Text(); result != "MCP worked." {
		t.Fatalf("MCP response %q", result)
	}
}

func TestReviewChangesAreReadOnlyAndCoverWorktree(t *testing.T) {
	root := t.TempDir()
	repo := initTestGit(t, root)
	writeTestFile(t, root, "tracked.txt", "staged change\n")
	stageTestFiles(t, repo, "tracked.txt")
	writeTestFile(t, root, "tracked.txt", "unstaged change\n")
	writeTestFile(t, root, "untracked.txt", "new file\n")
	kinds := []string{"staged", "unstaged", "untracked"}
	expect := []string{"+staged change", "+unstaged change", "untracked.txt"}
	model := fixtureModel{reply: func(call modelapi.Call) (*modelapi.Response, error) {
		for _, tool := range call.Tools {
			if tool.GetName() == "exec_command" || tool.GetName() == "edit" {
				return nil, fmt.Errorf("review tool can mutate the workspace")
			}
		}
		var results []modelapi.Message
		for _, message := range call.Prompt {
			if message.Role == modelapi.MessageRoleTool {
				results = append(results, message)
			}
		}
		i := len(results)
		if i > 0 {
			body, _ := json.Marshal(results[i-1])
			if !strings.Contains(string(body), expect[i-1]) {
				return nil, fmt.Errorf("%s result %s", kinds[i-1], body)
			}
		}
		if i == len(kinds) {
			return demoText("Review completed."), nil
		}
		return toolReply("review_changes", map[string]string{"kind": kinds[i]}, i), nil
	}}
	c, _ := testHost(t, root, model)
	run, err := c.Start(t.Context(), prompt.Execute, "/review", true)
	if err != nil {
		t.Fatal(err)
	}
	result, err := run.Wait(t.Context())
	if err != nil || result.Response.Content.Text() != "Review completed." {
		t.Fatalf("review %v, %v", result, err)
	}
}

func TestSteeringFollowupAndQuestion(t *testing.T) {
	model := fixtureModel{reply: func(call modelapi.Call) (*modelapi.Response, error) {
		payload, _ := json.Marshal(call.Prompt)
		data := string(payload)
		if strings.Contains(data, "steer answer") && strings.Contains(data, "follow-up answer") {
			images := 0
			for _, message := range call.Prompt {
				for _, part := range message.Content {
					if _, ok := modelapi.AsMessagePart[modelapi.FilePart](part); ok {
						images++
					}
				}
			}
			if images != 2 {
				return nil, fmt.Errorf("queued image inputs missing: %d", images)
			}
			return demoText("Both inputs accepted."), nil
		}
		for _, message := range call.Prompt {
			if message.Role == modelapi.MessageRoleTool {
				if !strings.Contains(data, "option 1") {
					return nil, fmt.Errorf("missing the question response")
				}
				return demoText("Question answered."), nil
			}
		}
		return toolReply("request_user_input", map[string]any{"questions": []map[string]any{
			{"id": "first", "text": "Pick an option", "options": []string{"option 1", "option 2"}},
		}}, 1), nil
	}}
	c, _ := testHost(t, t.TempDir(), model)
	ctx, cancel := context.WithTimeout(t.Context(), 8*time.Second)
	defer cancel()
	run, err := c.Start(ctx, prompt.Execute, "Ask me something.", false)
	if err != nil {
		t.Fatal(err)
	}
	select {
	case question := <-c.Questions.Requests:
		if accepted, err := c.QueueFiles(ctx, run, "steer answer", false, []modelapi.FilePart{{Filename: "steer.png", MediaType: "image/png", Data: imageFixture(t)}}); err != nil || !accepted {
			t.Fatal(err)
		}
		if accepted, err := c.QueueFiles(ctx, run, "follow-up answer", true, []modelapi.FilePart{{Filename: "follow.png", MediaType: "image/png", Data: imageFixture(t)}}); err != nil || !accepted {
			t.Fatal(err)
		}
		question.Answer <- map[string]string{"first": "option 1"}
	case <-ctx.Done():
		t.Fatal("expected a user question")
	}
	result, err := run.Wait(ctx)
	if err != nil || result.Response.Content.Text() != "Both inputs accepted." {
		t.Fatalf("follow-up %v, %v", result, err)
	}
}

func TestQueuedInputDisplayRecordedOnce(t *testing.T) {
	c, _ := testHost(t, t.TempDir(), DemoModel{})
	run, err := c.Start(t.Context(), prompt.Execute, "initial", false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := run.Wait(t.Context()); err != nil {
		t.Fatal(err)
	}
	accepted, err := c.QueueInput(t.Context(), run, "late input", false)
	if err == nil || accepted {
		t.Fatal("finished run accepted new input")
	}
	turns, err := c.Store.Turns()
	if err != nil || len(turns) != 1 {
		t.Fatalf("turns %d %v", len(turns), err)
	}
	for _, cell := range turns[0].Cells {
		if cell.Kind == "user" && cell.Text == "late input" {
			t.Fatal("unaccepted input entered the transcript")
		}
	}
}
