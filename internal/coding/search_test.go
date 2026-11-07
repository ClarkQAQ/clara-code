package coding

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"clara-core/models"
	"clara-core/tools"
	corefs "clara-core/tools/fs"
)

func searchGit(t *testing.T, root string, args ...string) {
	t.Helper()
	command := exec.Command("git", append([]string{"-C", root}, args...)...)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v %s", args, err, output)
	}
}

func runSearchTool(t *testing.T, tool models.AgentTool, input string) models.ToolResponse {
	t.Helper()
	result, err := tool.Run(t.Context(), models.ToolCall{Input: input})
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func TestSearchRespectsGitAndDirectReadsRemainAvailable(t *testing.T) {
	root := t.TempDir()
	searchGit(t, root, "init")
	for name, body := range map[string]string{
		".gitignore": "ignored/\n*.log\n!keep.log\n",
		"main.go":    "needle", "sub/a.go": "needle", "ignored/a.go": "needle", "ignored/new.go": "needle",
		"tracked.log": "needle", "drop.log": "needle", "keep.log": "needle", "module/hidden.go": "needle",
	} {
		writeTestFile(t, root, name, body)
	}
	searchGit(t, root, "add", "-f", "tracked.log", "ignored/a.go")
	searchGit(t, root, "update-index", "--add", "--cacheinfo", "160000,"+strings.Repeat("1", 40)+",module")
	space, err := ResolveWorkspace(root)
	if err != nil {
		t.Fatal(err)
	}
	files := space.files()
	for _, input := range []string{`{"pattern":"**/*"}`, `{"query":"needle"}`} {
		tool := files.Glob()
		if strings.Contains(input, "query") {
			tool = files.Grep()
		}
		result := runSearchTool(t, tool, input)
		if result.IsError {
			t.Fatal(result.Content)
		}
		for _, want := range []string{"main.go", "sub/a.go", "ignored/a.go", "tracked.log", "keep.log"} {
			if !strings.Contains(result.Content, want) {
				t.Fatalf("missing %s: %s", want, result.Content)
			}
		}
		for _, excluded := range []string{"ignored/new.go", "drop.log", "module/hidden.go", "/.git/"} {
			if strings.Contains(result.Content, excluded) {
				t.Fatalf("unexpected %s: %s", excluded, result.Content)
			}
		}
	}
	included := runSearchTool(t, files.Grep(), `{"query":"needle","include_ignored":true}`)
	if included.IsError || !strings.Contains(included.Content, "ignored/new.go") || !strings.Contains(included.Content, "drop.log") || strings.Contains(included.Content, "module/hidden.go") {
		t.Fatal(included.Content)
	}
	sub := runSearchTool(t, files.Glob(), `{"path":"sub","pattern":"*.go"}`)
	if sub.IsError || !strings.Contains(sub.Content, "/sub/a.go") || strings.Contains(sub.Content, "main.go") {
		t.Fatal(sub.Content)
	}
	module := runSearchTool(t, files.Grep(), `{"path":"module","query":"needle","include_ignored":true}`)
	if module.IsError || !strings.Contains(module.Content, "hidden.go") {
		t.Fatal("explicit submodule search failed: " + module.Content)
	}
	direct := runSearchTool(t, files.Read(), `{"path":"ignored/new.go"}`)
	if direct.IsError || direct.Content != "needle" {
		t.Fatal(direct.Content)
	}
	// Policy snapshots must refresh between calls after filesystem or ignore changes.
	writeTestFile(t, root, ".gitignore", "")
	refreshed := runSearchTool(t, files.Grep(), `{"query":"needle"}`)
	if refreshed.IsError || !strings.Contains(refreshed.Content, "ignored/new.go") {
		t.Fatal(refreshed.Content)
	}
}

func TestSearchPolicyEmptyNonGitAndGitFailure(t *testing.T) {
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
	searchGit(t, root, "init")
	space, err = ResolveWorkspace(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, "empty"), 0o755); err != nil {
		t.Fatal(err)
	}
	result = runSearchTool(t, space.files().Glob(), `{"path":"empty","pattern":"*.go"}`)
	if result.IsError || result.Content != "No matching files." {
		t.Fatal(result.Content)
	}
	writeTestFile(t, root, ".gitignore", "ignored/\n")
	result = runSearchTool(t, space.files().Grep(), `{"path":"ignored","query":"needle"}`)
	if !result.IsError || !strings.Contains(result.Content, "include_ignored") {
		t.Fatal(result.Content)
	}
	result = runSearchTool(t, space.files().Grep(), `{"path":"ignored","query":"needle","include_ignored":true}`)
	if result.IsError || !strings.Contains(result.Content, "needle") {
		t.Fatal(result.Content)
	}
	t.Setenv("PATH", t.TempDir())
	result = runSearchTool(t, space.files().Grep(), `{"query":"needle"}`)
	if !result.IsError || !strings.Contains(result.Content, "Git search enumeration failed") {
		t.Fatal("silently expanded Git scope: " + result.Content)
	}
}

func TestMainAndTaskSearchAssemblyUsesSamePolicy(t *testing.T) {
	root := t.TempDir()
	searchGit(t, root, "init")
	writeTestFile(t, root, ".gitignore", "ignored/\n")
	writeTestFile(t, root, "ignored/a.go", "secret")
	c, _ := testHost(t, root, DemoModel{})
	task, err := c.buildTask(t.Context(), tools.TaskSpec{Role: "explorer"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, items := range [][]models.AgentTool{c.items, task.Tools} {
		found := false
		for _, tool := range items {
			if tool.Info().Name != "glob" {
				continue
			}
			found = true
			schema, _ := json.Marshal(tool.Info())
			if !strings.Contains(string(schema), "include_ignored") {
				t.Fatal("model-visible policy parameter absent")
			}
			result := runSearchTool(t, tool, `{"pattern":"*.go"}`)
			if result.IsError || strings.Contains(result.Content, "ignored/a.go") {
				t.Fatal(result.Content)
			}
		}
		if !found {
			t.Fatal("glob missing")
		}
	}
	storage, err := c.Workspace.searchSource(t.Context(), corefs.Path(c.Workspace.Cwd), false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := storage.Stat(t.Context(), corefs.Path(filepath.ToSlash(filepath.Join(c.Workspace.Cwd, "ignored")))); err == nil {
		t.Fatal("ignored directory visible")
	}
}
