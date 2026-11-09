package host

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	corefs "clara-code/pkg/tool/fs"
	"clara-code/pkg/util/errs"
)

func TestWindowsNativePaths(t *testing.T) {
	storage := &FS{cwd: `D:\项目\work`}
	t.Setenv("USERPROFILE", `C:\Users\Clara User`)
	for input, expected := range map[string]corefs.Path{
		`../文件.txt`:                   `D:/项目/文件.txt`,
		`/`:                           `D:/`,
		`\other\path`:                 `D:/other/path`,
		`e:\新目录\file.txt`:             `E:/新目录/file.txt`,
		`//server/share/dir\file.txt`: `//server/share/dir/file.txt`,
		`~\draft.txt`:                 `C:/Users/Clara User/draft.txt`,
	} {
		resolved, err := storage.ResolvePath(input)
		if err != nil || resolved != expected {
			t.Errorf("%q: %q, %v, want %q", input, resolved, err, expected)
		}
	}
	for _, input := range []string{`C:relative.txt`, `\\.\pipe\device`, `C:\NUL.txt`, `file.txt:stream`, `\\server`} {
		if _, err := storage.ResolvePath(input); !errs.IsCode(err, errs.CodeInvalidArgument) {
			t.Errorf("%q: expected invalid path, got %v", input, err)
		}
	}
	for _, input := range []string{`/`, `D:/`, `\\server\share`, `\\server\share\`} {
		if err := storage.Remove(t.Context(), corefs.Path(input), corefs.RemoveOptions{Recursive: true}); !errors.Is(err, errs.ErrForbidden) {
			t.Errorf("%q: expected forbidden, got %v", input, err)
		}
	}
}

func TestWindowsNativeSearchInstructionsAndCopy(t *testing.T) {
	cwd := t.TempDir()
	storage, err := New(cwd)
	if err != nil {
		t.Fatal(err)
	}
	service := corefs.New(storage)
	if err := os.Mkdir(filepath.Join(cwd, "项目"), 0o755); err != nil {
		t.Fatal(err)
	}
	for name, content := range map[string]string{
		"AGENTS.md": "Windows rules",
		"项目/文件.txt": "search needle",
	} {
		if err := os.WriteFile(filepath.Join(cwd, filepath.FromSlash(name)), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	root, err := storage.ResolvePath(".")
	if err != nil {
		t.Fatal(err)
	}
	results, err := service.GlobDetailed(t.Context(), root, corefs.GlobOptions{
		Pattern: root.String() + "/**/*.txt",
	})
	if err != nil || len(results.Entries) != 1 || results.Entries[0].Name != "文件.txt" {
		t.Fatalf("glob: %+v, %v", results, err)
	}
	target := results.Entries[0].Path
	rules, err := corefs.InstructionsFrom(t.Context(), storage, root, target)
	if err != nil || !strings.Contains(rules, "Windows rules") {
		t.Fatalf("rules: %q, %v", rules, err)
	}
	if err := service.Copy(t.Context(), corefs.Path(`项目`), corefs.Path(`副本`)); err != nil {
		t.Fatal(err)
	}
	if data, err := os.ReadFile(filepath.Join(cwd, "副本", "文件.txt")); err != nil || string(data) != "search needle" {
		t.Fatalf("copied file: %q, %v", data, err)
	}
}
