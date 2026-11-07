package fs_test

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	clarafs "clara-code/pkg/tool/fs"
	"clara-code/pkg/tool/fs/memory"
)

func searchFixture(t *testing.T) *memory.FS {
	t.Helper()
	storage := memory.New()
	for _, name := range []string{"/root.go", "/internal/a.go", "/internal/sub/b.go", "/internal/sub/a.txt"} {
		if _, err := storage.WriteFile(t.Context(), clarafs.Path(name), []byte("prefix cursorPosition suffix\ncursorPosition\n"), clarafs.WriteOptions{CreateDirs: true}); err != nil {
			t.Fatal(err)
		}
	}
	return storage
}

func TestSearchPatternsShareSemantics(t *testing.T) {
	service := clarafs.New(searchFixture(t))
	for _, test := range []struct {
		root    clarafs.Path
		pattern string
		want    []string
	}{
		{"/", "*.go", []string{"/root.go", "/internal/a.go", "/internal/sub/b.go"}},
		{"/", "**/*.go", []string{"/root.go", "/internal/a.go", "/internal/sub/b.go"}},
		{"/internal", "*.go", []string{"/internal/a.go", "/internal/sub/b.go"}},
		{"/internal", "**/*.go", []string{"/internal/a.go", "/internal/sub/b.go"}},
		{"/internal", "sub/?.[gt][ox]", []string{"/internal/sub/b.go"}},
		{"/internal", "/internal/*.go", []string{"/internal/a.go"}},
		{"/internal", "*.missing", nil},
	} {
		result, err := service.GlobDetailed(t.Context(), test.root, clarafs.GlobOptions{Pattern: test.pattern})
		if err != nil {
			t.Fatal(err)
		}
		var names []string
		for _, entry := range result.Entries {
			names = append(names, entry.Path.String())
		}
		if !reflect.DeepEqual(names, test.want) {
			t.Fatalf("%s %s: %v want %v", test.root, test.pattern, names, test.want)
		}
		grep, err := service.GrepDetailed(t.Context(), test.root, clarafs.SearchOptions{Query: "prefix ", Include: []string{test.pattern}})
		if err != nil || len(grep.Hits) != len(test.want) {
			t.Fatalf("grep %s %s: %+v %v", test.root, test.pattern, grep, err)
		}
	}
	for _, pattern := range []string{"[", "x/[abc", "", "../*.go"} {
		if _, err := service.GlobDetailed(t.Context(), "/", clarafs.GlobOptions{Pattern: pattern}); err == nil {
			t.Fatalf("accepted %q", pattern)
		}
		if _, err := service.GrepDetailed(t.Context(), "/", clarafs.SearchOptions{Query: "x", Include: []string{pattern}}); err == nil {
			t.Fatalf("grep accepted %q", pattern)
		}
	}
	literal, err := service.GrepDetailed(t.Context(), "/", clarafs.SearchOptions{Query: " cursorPosition "})
	if err != nil || len(literal.Hits) != 4 {
		t.Fatalf("literal whitespace lost: %+v %v", literal, err)
	}
	for _, hit := range literal.Hits {
		if hit.Line != 1 {
			t.Fatal("query was trimmed")
		}
	}
}

type failingSearchFS struct {
	clarafs.FS
	rootFailure bool
}

func (f failingSearchFS) ReadFile(ctx context.Context, p clarafs.Path, limit int64) ([]byte, error) {
	if p == "/root.go" {
		return nil, errors.New("fixture read failure")
	}
	return f.FS.ReadFile(ctx, p, limit)
}

func (f failingSearchFS) ReadDir(ctx context.Context, p clarafs.Path) ([]clarafs.Entry, error) {
	if f.rootFailure && p == "/" {
		return nil, errors.New("fixture root failure")
	}
	return f.FS.ReadDir(ctx, p)
}

func TestSearchDiagnosticsAndCancellation(t *testing.T) {
	service := clarafs.New(failingSearchFS{FS: searchFixture(t)})
	result, err := service.GrepDetailed(t.Context(), "/", clarafs.SearchOptions{Query: "cursorPosition", MaxResults: 100})
	if err != nil || len(result.Hits) != 6 || !result.Truncated || result.Skipped != 1 || !strings.Contains(strings.Join(result.Warnings, " "), "/root.go: fixture read failure") {
		t.Fatalf("lost partial results: %+v %v", result, err)
	}

	service = clarafs.New(searchFixture(t))
	cases := []struct {
		options clarafs.SearchOptions
		warning string
	}{
		{clarafs.SearchOptions{Query: "cursorPosition", MaxResults: 1}, "hit limit"},
		{clarafs.SearchOptions{Query: "cursorPosition", MaxFileSize: 1}, "file size limit"},
		{clarafs.SearchOptions{Query: "cursorPosition", Walk: clarafs.WalkOptions{MaxEntries: 2}}, "traversal entry limit"},
		{clarafs.SearchOptions{Query: "cursorPosition", Walk: clarafs.WalkOptions{MaxDepth: 1}}, "depth limit"},
	}
	for _, test := range cases {
		result, err := service.GrepDetailed(t.Context(), "/", test.options)
		if err != nil || !result.Truncated || !strings.Contains(strings.Join(result.Warnings, " "), test.warning) {
			t.Fatalf("missing %s diagnosis: %+v %v", test.warning, result, err)
		}
	}
	glob, err := service.GlobDetailed(t.Context(), "/", clarafs.GlobOptions{Pattern: "**", MaxResults: 1})
	if err != nil || len(glob.Entries) != 1 || !glob.Truncated {
		t.Fatalf("glob limit: %+v %v", glob, err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := service.GrepDetailed(ctx, "/", clarafs.SearchOptions{Query: "x"}); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancel: %v", err)
	}
	service = clarafs.New(failingSearchFS{FS: searchFixture(t), rootFailure: true})
	if _, err := service.GrepDetailed(t.Context(), "/", clarafs.SearchOptions{Query: "x"}); err == nil {
		t.Fatal("root error hidden")
	}
}
