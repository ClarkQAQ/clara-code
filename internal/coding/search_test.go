package coding

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"clara-code/pkg/model"
	toolapi "clara-code/pkg/tool"
)

func runSearchTool(t *testing.T, tool model.AgentTool, input string) model.ToolResponse {
	t.Helper()
	result, err := tool.Run(t.Context(), model.ToolCall{Input: input})
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func TestSearchUsesFilesystemWithUnsupportedGitMetadata(t *testing.T) {
	root := t.TempDir()
	for name, body := range map[string]string{
		".git/HEAD":             "ref: refs/heads/main\n",
		".git/config":           "[core]\n\trepositoryformatversion = 0\n[extensions]\n\tworktreeConfig = true\n",
		".git/index":            "invalid index",
		".git/fixture.txt":      "needle",
		".gitignore":            "ignored/\n# needle\n",
		"main.go":               "needle",
		"sub/a.go":              "needle",
		"ignored/a.go":          "needle",
		"linked/.git":           "gitdir: ../.git/worktrees/needle\n",
		"linked/worktree.go":    "needle",
		"nested/.git/secret.go": "needle",
		"nested/repo.go":        "needle",
		"-[literal]/a.go":       "needle",
	} {
		writeTestFile(t, root, name, body)
	}
	t.Setenv("PATH", t.TempDir())
	space, err := ResolveWorkspace(root)
	if err != nil {
		t.Fatal(err)
	}
	files := space.files()
	for _, search := range []struct {
		tool  model.AgentTool
		input string
	}{
		{files.Glob(), `{"pattern":"**/*"}`},
		{files.Grep(), `{"query":"needle"}`},
		{files.Glob(), `{"pattern":"**/*","include_ignored":true}`},
		{files.Grep(), `{"query":"needle","include_ignored":true}`},
	} {
		result := runSearchTool(t, search.tool, search.input)
		if result.IsError {
			t.Fatal(result.Content)
		}
		for _, want := range []string{"/.gitignore", "/main.go", "/sub/a.go", "/ignored/a.go", "/linked/worktree.go", "/nested/repo.go"} {
			if !strings.Contains(result.Content, want) {
				t.Fatalf("missing %s: %s", want, result.Content)
			}
		}
		for _, excluded := range []string{"/.git/", "/linked/.git"} {
			if strings.Contains(result.Content, excluded) {
				t.Fatalf("unexpected %s: %s", excluded, result.Content)
			}
		}
	}
	metadata := runSearchTool(t, files.Grep(), `{"path":".git","query":"needle"}`)
	if metadata.IsError || !strings.Contains(metadata.Content, "/.git/fixture.txt:1:needle") {
		t.Fatal(metadata.Content)
	}
	gitFile := runSearchTool(t, files.Glob(), `{"path":"linked/.git","pattern":".git"}`)
	if gitFile.IsError || !strings.HasSuffix(gitFile.Content, "/linked/.git") {
		t.Fatal(gitFile.Content)
	}
	sub := runSearchTool(t, files.Glob(), `{"path":"sub","pattern":"*.go"}`)
	if sub.IsError || !strings.Contains(sub.Content, "/sub/a.go") || strings.Contains(sub.Content, "main.go") {
		t.Fatal(sub.Content)
	}
	literal := runSearchTool(t, files.Glob(), `{"path":"-[literal]","pattern":"*.go"}`)
	if literal.IsError || !strings.Contains(literal.Content, "/-[literal]/a.go") {
		t.Fatal(literal.Content)
	}
	writeTestFile(t, root, ".git/config", "[core]\n\trepositoryformatversion = 0\n")
	badIndex := runSearchTool(t, files.Grep(), `{"query":"needle"}`)
	if badIndex.IsError || !strings.Contains(badIndex.Content, "/main.go:1:needle") {
		t.Fatal(badIndex.Content)
	}
}

func TestSearchWorksWithoutRepository(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, root, "ignored/a.go", "needle")
	space, err := ResolveWorkspace(root)
	if err != nil {
		t.Fatal(err)
	}
	result := runSearchTool(t, space.files().Glob(), `{"pattern":"**/*.go"}`)
	if result.IsError || !strings.Contains(result.Content, "ignored/a.go") {
		t.Fatal(result.Content)
	}
	if err := os.Mkdir(filepath.Join(root, "empty"), 0o755); err != nil {
		t.Fatal(err)
	}
	result = runSearchTool(t, space.files().Glob(), `{"path":"empty","pattern":"*.go"}`)
	if result.IsError || result.Content != "No matching files." {
		t.Fatal(result.Content)
	}
}

func TestMainAndTaskSearchAssemblyUsesSamePolicy(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, root, ".gitignore", "ignored/\n")
	writeTestFile(t, root, "ignored/a.go", "needle")
	writeTestFile(t, root, ".git/fixture.go", "needle")
	c, _ := testHost(t, root, DemoModel{})
	task, err := c.buildTask(t.Context(), toolapi.TaskSpec{Role: "explorer"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	inputs := map[string]string{"glob": `{"pattern":"*.go"}`, "grep": `{"query":"needle"}`}
	for _, items := range [][]model.AgentTool{c.items, task.Tools} {
		found := make(map[string]bool)
		for _, tool := range items {
			name := tool.Info().Name
			input, ok := inputs[name]
			if !ok {
				continue
			}
			found[name] = true
			schema, _ := json.Marshal(tool.Info())
			if !strings.Contains(string(schema), "include_ignored") {
				t.Fatalf("%s schema is missing include_ignored", name)
			}
			result := runSearchTool(t, tool, input)
			if result.IsError || !strings.Contains(result.Content, "/ignored/a.go") || strings.Contains(result.Content, "/.git/") {
				t.Fatalf("%s: %s", name, result.Content)
			}
		}
		for name := range inputs {
			if !found[name] {
				t.Fatalf("missing tool %s", name)
			}
		}
	}
}
