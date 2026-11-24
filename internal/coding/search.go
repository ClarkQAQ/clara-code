package coding

import (
	"context"
	"runtime"
	"slices"
	"strings"

	"clara-code/pkg/tool"
	corefs "clara-code/pkg/tool/fs"
	"clara-code/pkg/tool/fs/host"
)

func (w Workspace) files() *tool.Files {
	storage := w.toolFS()
	files := tool.NewFiles(storage, nil)
	files.SearchSource = func(context.Context, corefs.Path, bool) (corefs.FS, error) {
		return searchFS{FS: storage}, nil
	}
	files.InstructionsSource = w.targetRules
	return files
}

type searchFS struct {
	*host.FS
}

func (f searchFS) ReadDir(ctx context.Context, p corefs.Path) ([]corefs.Entry, error) {
	entries, err := f.FS.ReadDir(ctx, p)
	if err != nil {
		return nil, err
	}
	return slices.DeleteFunc(entries, func(entry corefs.Entry) bool {
		return entry.Name == ".git" || runtime.GOOS == "windows" && strings.EqualFold(entry.Name, ".git")
	}), nil
}
