package coding

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"clara-code/pkg/core"
	"clara-code/pkg/core/prompt"
	modelapi "clara-code/pkg/model"
	toolapi "clara-code/pkg/tool"

	"clara-code/internal/session"
	"clara-code/internal/setting"
)

func testHost(t *testing.T, cwd string, model modelapi.LanguageModel) (*Conversation, Host) {
	t.Helper()
	workspace, err := ResolveWorkspace(cwd)
	if err != nil {
		t.Fatal(err)
	}
	host := Host{Config: setting.Config{Home: t.TempDir(), DefaultModel: "fixture",
		Models: map[string]setting.Model{"fixture": {ID: "fixture"}, "second": {ID: "second"}}},
		Models: func(context.Context, string) (modelapi.LanguageModel, error) { return model, nil }}
	store := session.New(host.Config.Home, session.Meta{Project: workspace.Root, Cwd: workspace.Cwd, Model: "fixture"})
	c, err := host.Open(t.Context(), workspace, store, NewQuestions(), nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := c.Close(); err != nil {
			t.Error(err)
		}
	})
	return c, host
}

func finish(t *testing.T, c *Conversation, mode prompt.Mode, input string) *core.AgentResult {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 8*time.Second)
	defer cancel()
	run, err := c.Start(ctx, mode, input, false)
	if err != nil {
		t.Fatal(err)
	}
	result, err := run.Wait(ctx)
	if err != nil {
		t.Fatal(err)
	}
	return result
}

type fixtureModel struct {
	DemoModel
	reply func(modelapi.Call) (*modelapi.Response, error)
}

func (m fixtureModel) Stream(_ context.Context, call modelapi.Call) (modelapi.StreamResponse, error) {
	response, err := m.reply(call)
	if err != nil {
		return nil, err
	}
	return func(yield func(modelapi.StreamPart) bool) {
		for _, item := range response.Content {
			switch content := item.(type) {
			case modelapi.ToolCallContent:
				if !yield(modelapi.StreamPart{Type: modelapi.StreamPartTypeToolCall, ID: content.ToolCallID,
					ToolCallName: content.ToolName, ToolCallInput: content.Input}) {
					return
				}
			case modelapi.TextContent:
				if !yield(modelapi.StreamPart{Type: modelapi.StreamPartTypeTextStart, ID: "text"}) ||
					!yield(modelapi.StreamPart{Type: modelapi.StreamPartTypeTextDelta, ID: "text", Delta: content.Text}) ||
					!yield(modelapi.StreamPart{Type: modelapi.StreamPartTypeTextEnd, ID: "text"}) {
					return
				}
			}
		}
		yield(modelapi.StreamPart{Type: modelapi.StreamPartTypeFinish, FinishReason: response.FinishReason, Usage: response.Usage})
	}, nil
}

func toolReply(name string, args any, step int) *modelapi.Response {
	body, _ := json.Marshal(args)
	return &modelapi.Response{Content: modelapi.ResponseContent{modelapi.ToolCallContent{
		ToolCallID: fmt.Sprintf("call_%d", step), ToolName: name, Input: string(body),
	}}, FinishReason: modelapi.FinishReasonToolCalls}
}

func TestCodingLoopUsesRealHashlineAndShell(t *testing.T) {
	cwd := t.TempDir()
	writeTestFile(t, cwd, "test.txt", "old\n")
	model := fixtureModel{reply: func(call modelapi.Call) (*modelapi.Response, error) {
		var outputs []string
		for _, message := range call.Prompt {
			for _, part := range message.Content {
				if result, ok := modelapi.AsMessagePart[modelapi.ToolResultPart](part); ok {
					text, _ := modelapi.AsToolResultOutputType[modelapi.ToolResultOutputContentText](result.Output)
					outputs = append(outputs, text.Text)
				}
			}
		}
		switch len(outputs) {
		case 0:
			return toolReply("read", map[string]any{"path": "test.txt", "anchored": true}, 1), nil
		case 1:
			header, _, _ := strings.Cut(outputs[0], "\n")
			if !strings.HasPrefix(header, "[") || !strings.Contains(header, "/test.txt#") {
				return nil, fmt.Errorf("no anchored snapshot: %s", outputs[0])
			}
			return toolReply("edit", map[string]any{"input": header + "\nPUT 1.=1:\n+new"}, 2), nil
		case 2:
			return toolReply("exec_command", map[string]any{"command": "grep -x new test.txt"}, 3), nil
		case 3:
			if !strings.Contains(outputs[2], "exit_code: 0") || !strings.Contains(outputs[2], "new") {
				return nil, fmt.Errorf("shell did not see edited file: %s", outputs[2])
			}
			return demoText("Verified edit."), nil
		default:
			return nil, fmt.Errorf("unexpected step %d", len(outputs))
		}
	}}
	c, _ := testHost(t, cwd, model)
	if got := finish(t, c, prompt.Execute, "Edit then verify.").Response.Content.Text(); got != "Verified edit." {
		t.Fatalf("response %q", got)
	}
	turns, err := c.Store.Turns()
	if err != nil || len(turns) != 1 || turns[0].Cells[len(turns[0].Cells)-1].Text != "Verified edit." {
		t.Fatalf("turns %+v, %v", turns, err)
	}
}

func TestPlanOnlyAndProjectInstructions(t *testing.T) {
	root := t.TempDir()
	initTestGit(t, root)
	writeTestFile(t, root, "AGENTS.md", "project instructions")
	writeTestFile(t, root, "sub/AGENTS.md", "overridden")
	writeTestFile(t, root, "sub/AGENTS.override.md", "nearby instructions")
	writeTestFile(t, root, ".agents/skills/example/SKILL.md", "---\nname: example\ndescription: project skill\n---\nroot secret")
	writeTestFile(t, root, "sub/.agents/skills/example/SKILL.md", "---\nname: example\ndescription: nearer skill\n---\nonly load on request")
	model := fixtureModel{reply: func(call modelapi.Call) (*modelapi.Response, error) {
		if !strings.Contains(contextText(call.Prompt), "global instructions") {
			return nil, fmt.Errorf("missing global instructions")
		}
		dynamic := contextText(call.Prompt)
		for _, expected := range []string{"project instructions", "nearby instructions", "nearer skill"} {
			if !strings.Contains(dynamic, expected) {
				return nil, fmt.Errorf("missing %s", expected)
			}
		}
		for _, hidden := range []string{"overridden", "root secret", "only load on request", "project skill"} {
			if strings.Contains(dynamic, hidden) {
				return nil, fmt.Errorf("unexpected context %s", hidden)
			}
		}
		if slices.ContainsFunc(call.Tools, func(tool modelapi.Tool) bool {
			return tool.GetName() == "exec_command" || tool.GetName() == "edit"
		}) {
			return nil, fmt.Errorf("write tools are visible in Plan mode")
		}
		for _, message := range call.Prompt {
			if message.Role == modelapi.MessageRoleTool {
				if !strings.Contains(fmt.Sprint(message.Content), "only load on request") {
					body, _ := json.Marshal(message)
					if !strings.Contains(string(body), "only load on request") {
						return nil, fmt.Errorf("skill tool returned %s", body)
					}
				}
				return demoText("Read-only plan."), nil
			}
		}
		return toolReply("read_skill", map[string]any{"name": "example"}, 1), nil
	}}
	c, host := testHost(t, filepath.Join(root, "sub"), model)
	writeTestFile(t, host.Config.Home, "AGENTS.md", "global instructions")
	writeTestFile(t, host.Config.Home, "skills/example/SKILL.md", "---\nname: example\ndescription: global skill\n---\nglobal secret")
	call := c.Call(prompt.Plan, false)
	if err := call.AuthorizeTool(t.Context(), modelapi.ToolInfo{Effect: "write"}, modelapi.ToolCall{}); err == nil {
		t.Fatal("plan authorized a write")
	}
	finish(t, c, prompt.Plan, "Plan this.")
}

func TestResumeLockAndForkAfterCompaction(t *testing.T) {
	c, host := testHost(t, t.TempDir(), DemoModel{})
	finish(t, c, prompt.Execute, "first prompt")
	finish(t, c, prompt.Execute, "second prompt")
	parent := c.Store.Meta()
	if _, err := session.Open(host.Config.Home, parent.ID, parent.Project); err == nil {
		t.Fatal("opened a locked session")
	}
	if err := c.ChangeModel(t.Context(), "second", "high"); err != nil {
		t.Fatal(err)
	}
	if err := c.Compact(t.Context()); err != nil {
		t.Fatal(err)
	}
	if history := c.Session.Messages(); len(history) != 3 || !strings.Contains(session.Text(history[0]), "summarized") || session.Text(history[1]) != "second prompt" {
		t.Fatalf("unexpected compacted history: %v", history)
	}
	child, err := c.Store.Fork(t.Context(), 1)
	if err != nil {
		t.Fatal(err)
	}
	fork, err := host.Open(t.Context(), c.Workspace, child, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer fork.Close()
	if meta := child.Meta(); meta.Parent != parent.ID || meta.Model != "second" || meta.Effort != "high" {
		t.Fatalf("unexpected fork metadata %+v", meta)
	}
	if got := fork.Session.Messages(); len(got) != 2 || session.Text(got[0]) != "first prompt" {
		t.Fatalf("fork did not restore the pre-turn checkpoint: %v", got)
	}
	if archive, err := c.Store.Turns(); err != nil || len(archive) != 2 || archive[1].Prompt != "second prompt" {
		t.Fatalf("parent transcript changed: %+v, %v", archive, err)
	}
	if items, err := session.List(host.Config.Home, "/other"); err != nil || len(items) != 0 {
		t.Fatalf("unrelated project sessions %+v, %v", items, err)
	}
	finish(t, fork, prompt.Execute, "new branch prompt")
	if err := c.Close(); err != nil {
		t.Fatal(err)
	}
	store, err := session.Open(host.Config.Home, parent.ID, parent.Project)
	if err != nil {
		t.Fatal(err)
	}
	resumed, err := host.Open(t.Context(), c.Workspace, store, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer resumed.Close()
	finish(t, resumed, prompt.Execute, "continue the parent")
}

func TestOpenAICompatibleEndpoint(t *testing.T) {
	requests := make(chan string, 2)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Model, ReasoningEffort string
			Stream                 bool
		}
		var payload map[string]json.RawMessage
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Error(err)
		}
		_ = json.Unmarshal(payload["model"], &req.Model)
		_ = json.Unmarshal(payload["reasoning_effort"], &req.ReasoningEffort)
		_ = json.Unmarshal(payload["stream"], &req.Stream)
		requests <- fmt.Sprintf("%s %s %s %t", r.URL.Path, req.Model, req.ReasoningEffort, req.Stream)
		w.Header().Set("Content-Type", "text/event-stream")
		for _, data := range []string{
			`{"id":"x","object":"chat.completion.chunk","created":1,"model":"model-b","choices":[{"index":0,"delta":{"role":"assistant","content":"hello endpoint"},"finish_reason":null}]}`,
			`{"id":"x","object":"chat.completion.chunk","created":1,"model":"model-b","choices":[{"index":0,"delta":{},"finish_reason":"stop"}],"usage":{"prompt_tokens":12,"completion_tokens":4,"total_tokens":16}}`,
			"[DONE]",
		} {
			fmt.Fprintf(w, "data: %s\n\n", data)
		}
	}))
	defer server.Close()
	c, _ := testHost(t, t.TempDir(), DemoModel{})
	c.Host.Config.Endpoint = server.URL + "/v1"
	c.Host.Config.Protocol = "completions"
	c.Host.Config.Models["second"] = setting.Model{ID: "model-b"}
	factory, err := c.Host.Config.Factory()
	if err != nil {
		t.Fatal(err)
	}
	c.Host.Models = factory
	if err := c.ChangeModel(t.Context(), "second", "high"); err != nil {
		t.Fatal(err)
	}
	result := finish(t, c, prompt.Execute, "hello")
	if got := result.Response.Content.Text(); got != "hello endpoint" {
		t.Fatalf("response: %q", got)
	}
	if got := <-requests; got != "/v1/chat/completions model-b high true" {
		t.Fatalf("request: %s", got)
	}
}

func writeTestFile(t *testing.T, root, name, text string) {
	t.Helper()
	path := filepath.Join(root, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestAutomaticCompactionKeepsVisibleTurns(t *testing.T) {
	model := fixtureModel{reply: func(modelapi.Call) (*modelapi.Response, error) {
		return demoText("Short response."), nil
	}}
	c, _ := testHost(t, t.TempDir(), model)
	prepare := c.Call(prompt.Execute, false).PrepareStep
	_, prepared, err := prepare(t.Context(), core.PrepareStepFunctionOptions{})
	if err != nil {
		t.Fatal(err)
	}
	definitions := make([]modelapi.ToolInfo, 0, len(prepared.Tools))
	for _, item := range prepared.Tools {
		definitions = append(definitions, item.Info())
	}
	estimate, err := prompt.Measure(*prepared.System, prepared.Messages, definitions)
	if err != nil {
		t.Fatal(err)
	}
	profile := c.Host.Config.Models["fixture"]
	profile.ContextWindow = 2 * (estimate.Tokens + 4500)
	profile.EffectiveContextWindowPercent = 100
	profile.MaxOutputTokens = 1
	profile.AutoCompactContextPercent = new(50)
	c.Host.Config.Models["fixture"] = profile
	if err := c.ChangeModel(t.Context(), "fixture", ""); err != nil {
		t.Fatal(err)
	}
	first, second := strings.Repeat("a", 7000), strings.Repeat("b", 6000)
	finish(t, c, prompt.Execute, first)
	finish(t, c, prompt.Execute, second)
	if got := session.Text(c.Session.Messages()[0]); !strings.Contains(got, "Earlier conversation summarized.") {
		t.Fatalf("automatic compaction did not replace the old context, first message has %d bytes", len(got))
	}
	turns, err := c.Store.Turns()
	if err != nil || len(turns) != 2 || turns[0].Prompt != first || turns[1].Prompt != second {
		t.Fatalf("original prompts were not archived: %d turns, %v", len(turns), err)
	}
	fork, err := c.Store.Fork(t.Context(), 1)
	if err != nil {
		t.Fatal(err)
	}
	defer fork.Close()
	checkpoint, err := fork.Load(t.Context(), fork.Meta().ID)
	if err != nil || len(checkpoint.Messages) != 2 || session.Text(checkpoint.Messages[0]) != first {
		t.Fatalf("original context missing from fork: %d messages, %v", len(checkpoint.Messages), err)
	}
}

func TestWorkspacePathsAndSkillResources(t *testing.T) {
	root := t.TempDir()
	initTestGit(t, root)
	writeTestFile(t, root, "target.txt", "root content")
	writeTestFile(t, root, "sub/child/keep.txt", "child content")
	writeTestFile(t, root, ".agents/skills/demo/SKILL.md", "---\nname: demo\ndescription: resource test\n---\nRead refs/guide.txt")
	writeTestFile(t, root, ".agents/skills/demo/refs/guide.txt", "resource content")
	space, err := ResolveWorkspace(filepath.Join(root, "sub"))
	if err != nil {
		t.Fatal(err)
	}
	files := toolapi.NewFiles(space.toolFS(), nil)
	body, _ := json.Marshal(map[string]string{"path": filepath.Join(space.Root, "target.txt")})
	reply, err := files.Read().Run(t.Context(), modelapi.ToolCall{Input: string(body)})
	if err != nil || reply.IsError || reply.Content != "root content" {
		t.Fatalf("absolute file path %+v %v", reply, err)
	}
	localRunner := runner{workspace: space, local: toolapi.NewLocal()}
	process, err := localRunner.Start(t.Context(), toolapi.Spec{Command: "pwd", Workdir: "child"})
	if err != nil {
		t.Fatal(err)
	}
	defer process.Close()
	poll, err := process.Poll(t.Context(), time.Second)
	if err != nil || !strings.Contains(poll.Output, filepath.Join(space.Cwd, "child")) {
		t.Fatalf("relative shell workdir %+v %v", poll, err)
	}
	if process, err := localRunner.Start(t.Context(), toolapi.Spec{Command: "pwd", Workdir: "../../outside"}); err == nil {
		process.Close()
		t.Fatal("workdir escaped the project")
	}
	catalog, items, err := space.skills(t.Context(), t.TempDir())
	if err != nil || len(catalog) != 1 || catalog[0].Source == "" {
		t.Fatalf("skills %+v %v", catalog, err)
	}
	var resource modelapi.AgentTool
	for _, item := range items {
		if item.Info().Name == "read_skill_resource" {
			resource = item
		}
	}
	if resource == nil {
		t.Fatal("resource reader missing")
	}
	for path, expected := range map[string]string{"refs/guide.txt": "resource content", "../SKILL.md": ""} {
		body, _ := json.Marshal(map[string]string{"name": "demo", "path": path})
		result, err := resource.Run(t.Context(), modelapi.ToolCall{Input: string(body)})
		if err != nil {
			t.Fatal(err)
		}
		if expected == "" && !result.IsError {
			t.Fatal("skill resource escaped its directory")
		}
		if expected != "" && (result.IsError || result.Content != expected) {
			t.Fatalf("resource %+v", result)
		}
	}
}

func TestUsageRestoresWithoutDoubleCounting(t *testing.T) {
	c, host := testHost(t, t.TempDir(), DemoModel{})
	finish(t, c, prompt.Execute, "first")
	finish(t, c, prompt.Execute, "second")
	meta := c.Store.Meta()
	if meta.LastUsage == nil || meta.LastUsage.TotalTokens != 15 || meta.TotalUsage.TotalTokens != 30 {
		t.Fatalf("usage %+v", meta)
	}
	if err := c.Close(); err != nil {
		t.Fatal(err)
	}
	store, err := session.Open(host.Config.Home, meta.ID, meta.Project)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if got := store.Meta(); got.LastUsage == nil || got.LastUsage.TotalTokens != 15 || got.TotalUsage.TotalTokens != 30 {
		t.Fatalf("restored usage %+v", got)
	}
	if err := store.InvalidateContext(); err != nil {
		t.Fatal(err)
	}
	if store.Meta().LastUsage != nil {
		t.Fatal("context was not invalidated")
	}
}
