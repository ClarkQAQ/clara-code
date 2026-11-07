package coding

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"clara-code/pkg/core/prompt"
	"clara-code/pkg/models"
	"clara-code/pkg/tools"

	"clara-code/internal/session"
)

func TestYOLOToolsOperateOutsideTheProject(t *testing.T) {
	base := t.TempDir()
	project, other := filepath.Join(base, "project"), filepath.Join(base, "other")
	for _, dir := range []string{project, other} {
		if err := os.Mkdir(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	gitTest(t, other, "init", "-q")
	writeTestFile(t, other, "AGENTS.md", "TARGET RULES")
	writeTestFile(t, other, "AGENTS.override.md", "OVERRIDE TARGET RULES")
	writeTestFile(t, other, ".gitignore", "ignored.go\n")
	writeTestFile(t, other, "ignored.go", "needle")
	writeTestFile(t, other, ".git/fixture.txt", "metadata needle")
	c, _ := testHost(t, project, DemoModel{})
	invoke := func(name string, args any) models.ToolResponse {
		t.Helper()
		data, _ := json.Marshal(args)
		for _, tool := range c.items {
			if tool.Info().Name == name {
				return runSearchTool(t, tool, string(data))
			}
		}
		t.Fatal("missing tool", name)
		return models.ToolResponse{}
	}
	target := filepath.Join(other, "new.go")
	rules := invoke("fs_instructions", map[string]any{"path": target})
	if rules.IsError || !strings.Contains(rules.Content, "OVERRIDE TARGET RULES") || strings.Contains(rules.Content, "/AGENTS.md\n") {
		t.Fatal(rules.Content)
	}
	created := invoke("write", map[string]any{"path": target, "content": "package old\n"})
	if created.IsError {
		t.Fatal(created.Content)
	}
	read := invoke("read", map[string]any{"path": "../other/new.go", "anchored": true})
	if read.IsError || !strings.Contains(read.Content, "package old") {
		t.Fatal(read.Content)
	}
	header, _, _ := strings.Cut(read.Content, "\n")
	if !strings.Contains(header, "/other/new.go#") || !strings.HasPrefix(header, "[/") {
		t.Fatal(header)
	}
	edited := invoke("edit", map[string]any{"input": header + "\nPUT 1.=1:\n+package changed"})
	if edited.IsError {
		t.Fatal(edited.Content)
	}
	body, err := os.ReadFile(target)
	if err != nil || string(body) != "package changed\n" {
		t.Fatalf("edit %q %v", body, err)
	}
	for _, name := range []string{"list", "stat"} {
		result := invoke(name, map[string]any{"path": other})
		if result.IsError {
			t.Fatal(result.Content)
		}
	}
	result := invoke("glob", map[string]any{"path": other, "pattern": "*.go"})
	if result.IsError || !strings.Contains(result.Content, "new.go") || strings.Contains(result.Content, "ignored.go") {
		t.Fatal(result.Content)
	}
	result = invoke("grep", map[string]any{"path": other, "query": "needle", "include_ignored": true})
	if result.IsError || !strings.Contains(result.Content, "ignored.go") || strings.Contains(result.Content, "fixture.txt") {
		t.Fatal(result.Content)
	}
	result = invoke("grep", map[string]any{"path": filepath.Join(other, ".git"), "query": "metadata needle"})
	if result.IsError || !strings.Contains(result.Content, "fixture.txt") {
		t.Fatal(result.Content)
	}
	copied, moved := filepath.Join(other, "copied.go"), filepath.Join(other, "moved.go")
	for _, step := range []struct {
		name string
		args any
	}{
		{"copy", map[string]any{"from": target, "to": copied}},
		{"move", map[string]any{"from": copied, "to": moved}},
		{"remove", map[string]any{"path": moved}},
	} {
		result := invoke(step.name, step.args)
		if result.IsError {
			t.Fatal(result.Content)
		}
	}
	if _, err := os.Stat(moved); !os.IsNotExist(err) {
		t.Fatal("temporary moved file was not removed", err)
	}
	local := runner{workspace: c.Workspace, local: tools.NewLocal()}
	for _, workdir := range []string{other, "../other"} {
		process, err := local.Start(t.Context(), tools.Spec{Command: "pwd", Workdir: workdir})
		if err != nil {
			t.Fatal(err)
		}
		poll, err := process.Poll(t.Context(), time.Second)
		process.Close()
		real, _ := filepath.EvalSymlinks(other)
		if err != nil || !strings.Contains(poll.Output, real) {
			t.Fatalf("native workdir %s: %+v %v", workdir, poll, err)
		}
	}
	// Plan exposes the same path semantics while execution authorization stays read-only.
	setup, err := c.buildTask(t.Context(), tools.TaskSpec{Role: "explorer"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	var reader models.AgentTool
	for _, tool := range setup.Tools {
		if tool.Info().Name == "read" {
			reader = tool
		}
		if tool.Info().Name == "write" {
			t.Fatal("write exposed in explorer")
		}
	}
	result = runSearchTool(t, reader, `{"path":"../other/new.go"}`)
	if result.IsError || !strings.Contains(result.Content, "package changed") {
		t.Fatal(result.Content)
	}
}

func TestRelativeFilesStartAtCwdAndLegacySessionsMigrate(t *testing.T) {
	root := t.TempDir()
	gitTest(t, root, "init", "-q")
	writeTestFile(t, root, "file.txt", "root")
	writeTestFile(t, root, "sub/file.txt", "cwd")
	writeTestFile(t, root, "sub/AGENTS.md", "cwd rules")
	c, host := testHost(t, filepath.Join(root, "sub"), DemoModel{})
	if c.Store.Meta().PathMode != 2 || c.Store.Meta().LegacyPathContext {
		t.Fatal("new session marked legacy")
	}
	result := runSearchTool(t, c.Workspace.files().Read(), `{"path":"file.txt"}`)
	if result.IsError || !strings.HasPrefix(result.Content, "cwd") {
		t.Fatal(result.Content)
	}
	rules := runSearchTool(t, c.Workspace.files().Instructions(), `{"path":"."}`)
	if rules.IsError || !strings.Contains(rules.Content, "cwd rules") {
		t.Fatal(rules.Content)
	}
	finish(t, c, prompt.Execute, "legacy context")
	id := c.Session.ID()
	if err := c.Close(); err != nil {
		t.Fatal(err)
	}
	manifest := filepath.Join(host.Config.Home, "sessions", id+".json")
	raw, err := os.ReadFile(manifest)
	if err != nil {
		t.Fatal(err)
	}
	var document map[string]any
	if err := json.Unmarshal(raw, &document); err != nil {
		t.Fatal(err)
	}
	meta := document["Meta"].(map[string]any)
	delete(meta, "PathMode")
	delete(meta, "LegacyPathContext")
	raw, _ = json.Marshal(document)
	if err := os.WriteFile(manifest, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	store, err := session.Open(host.Config.Home, id, c.Workspace.Root)
	if err != nil {
		t.Fatal(err)
	}
	reopened, err := host.Open(t.Context(), c.Workspace, store, NewQuestions(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	if !store.Meta().LegacyPathContext || store.Meta().PathMode != 2 {
		t.Fatalf("migration lost %+v", store.Meta())
	}
	turn, err := store.ReadTurn(0)
	if err != nil || turn.Prompt != "legacy context" {
		t.Fatal("old transcript changed", err)
	}
	child, err := store.Fork(t.Context(), -1)
	if err != nil {
		t.Fatal(err)
	}
	defer child.Close()
	if !child.Meta().LegacyPathContext || child.Meta().PathMode != 2 {
		t.Fatal("fork discarded path migration metadata")
	}
}
