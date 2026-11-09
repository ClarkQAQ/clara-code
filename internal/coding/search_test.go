package coding

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"clara-code/pkg/model"
	toolapi "clara-code/pkg/tool"
	corefs "clara-code/pkg/tool/fs"

	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/filemode"
)

func runSearchTool(t *testing.T, tool model.AgentTool, input string) model.ToolResponse {
	t.Helper()
	result, err := tool.Run(t.Context(), model.ToolCall{Input: input})
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func TestSearchRespectsGitAndDirectReadsRemainAvailable(t *testing.T) {
	root := t.TempDir()
	repo := initTestGit(t, root)
	for name, body := range map[string]string{
		".gitignore": "ignored/\n*.log\n!keep.log\n",
		"main.go":    "needle", "sub/a.go": "needle", "ignored/a.go": "needle", "ignored/new.go": "needle",
		"tracked.log": "needle", "drop.log": "needle", "keep.log": "needle", "module/hidden.go": "needle",
		"sub/.gitignore":    "*.skip\n",
		"sub/drop.skip":     "needle",
		".git/info/exclude": "local-only.go\n",
		"local-only.go":     "needle",
		"-[literal]/a.go":   "needle",
	} {
		writeTestFile(t, root, name, body)
	}
	stageTestFiles(t, repo, "tracked.log", "ignored/a.go")
	idx, err := repo.Storer.Index()
	if err != nil {
		t.Fatal(err)
	}
	moduleEntry := idx.Add("module")
	moduleEntry.Mode = filemode.Submodule
	moduleEntry.Hash = plumbing.NewHash(strings.Repeat("1", 40))
	if err := repo.Storer.SetIndex(idx); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", t.TempDir())
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
		for _, excluded := range []string{"ignored/new.go", "drop.log", "drop.skip", "local-only.go", "module/hidden.go", "/.git/"} {
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
	literal := runSearchTool(t, files.Glob(), `{"path":"-[literal]","pattern":"*.go"}`)
	if literal.IsError || !strings.Contains(literal.Content, "/-[literal]/a.go") {
		t.Fatal(literal.Content)
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
	initTestGit(t, root)
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
	result = runSearchTool(t, space.files().Grep(), `{"query":"needle","include_ignored":true}`)
	if result.IsError || !strings.Contains(result.Content, "needle") {
		t.Fatal("search requires a Git executable: " + result.Content)
	}
	writeTestFile(t, root, ".git/index", "invalid index")
	result = runSearchTool(t, space.files().Grep(), `{"query":"needle"}`)
	if !result.IsError || !strings.Contains(result.Content, "git search enumeration failed") {
		t.Fatal("silently expanded Git scope: " + result.Content)
	}
}

func TestMainAndTaskSearchAssemblyUsesSamePolicy(t *testing.T) {
	root := t.TempDir()
	initTestGit(t, root)
	writeTestFile(t, root, ".gitignore", "ignored/\n")
	writeTestFile(t, root, "ignored/a.go", "secret")
	c, _ := testHost(t, root, DemoModel{})
	task, err := c.buildTask(t.Context(), toolapi.TaskSpec{Role: "explorer"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, items := range [][]model.AgentTool{c.items, task.Tools} {
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
