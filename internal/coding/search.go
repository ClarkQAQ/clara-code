package coding

import (
	"context"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"clara-code/pkg/tool"
	corefs "clara-code/pkg/tool/fs"
	"clara-code/pkg/util/errs"

	"github.com/go-git/go-git/v5/plumbing/filemode"
)

func (w Workspace) files() *tool.Files {
	files := tool.NewFiles(w.toolFS(), nil)
	files.SearchSource = w.searchSource
	files.InstructionsSource = w.targetRules
	return files
}

type searchFS struct {
	corefs.FS
	allowed    map[corefs.Path]bool
	submodules map[corefs.Path]bool
	root       corefs.Path
}

func (f searchFS) excluded(p corefs.Path) bool {
	for current := p; current != "/" && current != f.root; current = current.Dir() {
		if current.Base() == ".git" || f.submodules[current] {
			return true
		}
	}
	return f.allowed != nil && !f.allowed[p]
}

func (f searchFS) Stat(ctx context.Context, p corefs.Path) (corefs.Entry, error) {
	if f.excluded(p) {
		return corefs.Entry{}, errs.New(errs.CodeInvalidArgument, "path excluded by Git search filters; use include_ignored=true (direct file access is available)")
	}
	return f.FS.Stat(ctx, p)
}

func (f searchFS) ReadDir(ctx context.Context, p corefs.Path) ([]corefs.Entry, error) {
	entries, err := f.FS.ReadDir(ctx, p)
	if err != nil {
		return nil, err
	}
	filtered := entries[:0]
	for _, entry := range entries {
		if !f.excluded(entry.Path) {
			filtered = append(filtered, entry)
		}
	}
	return filtered, nil
}

func (w Workspace) searchSource(ctx context.Context, root corefs.Path, includeIgnored bool) (corefs.FS, error) {
	repoRoot, git, err := projectForPath(filepath.FromSlash(root.String()))
	if err != nil {
		return nil, err
	}
	storage := searchFS{FS: w.toolFS(), submodules: map[corefs.Path]bool{}, root: root}
	if !git {
		return storage, nil
	}
	for _, part := range strings.Split(root.String(), "/") {
		if part == ".git" {
			return storage, nil
		}
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if err := storage.gitPaths(ctx, repoRoot, includeIgnored); err != nil {
		return nil, fmt.Errorf("git search enumeration failed: %w", err)
	}
	return storage, nil
}

func (f *searchFS) gitPaths(ctx context.Context, repoRoot string, includeIgnored bool) error {
	repo, err := openGit(repoRoot)
	if err != nil {
		return err
	}
	entries, err := readGitIndex(repo)
	if err != nil {
		return err
	}
	start, err := filepath.Rel(repoRoot, filepath.FromSlash(f.root.String()))
	if err != nil {
		return err
	}
	start = filepath.ToSlash(start)
	for name, entry := range entries {
		if entry.Mode == filemode.Submodule {
			f.submodules[corefs.Path(filepath.ToSlash(filepath.Join(repoRoot, name)))] = true
			if start == name || strings.HasPrefix(start, name+"/") {
				return nil
			}
		}
	}
	if includeIgnored {
		return ctx.Err()
	}

	f.allowed = map[corefs.Path]bool{"/": true}
	count, size := 0, 0
	add := func(name string) error {
		count++
		size += len(name) + 1
		if count > 20000 || size > 8<<20 {
			return fmt.Errorf("git search enumeration exceeded 20000 files or 8 MiB; narrow path")
		}
		absolute := corefs.Path(filepath.ToSlash(filepath.Join(repoRoot, name)))
		for p := absolute; !f.allowed[p]; p = p.Dir() {
			f.allowed[p] = true
		}
		return nil
	}
	for name := range entries {
		if err := ctx.Err(); err != nil {
			return err
		}
		if start == "." || name == start || strings.HasPrefix(name, start+"/") {
			if err := add(name); err != nil {
				return err
			}
		}
	}
	root, err := os.OpenRoot(repoRoot)
	if err != nil {
		return err
	}
	defer root.Close()
	ignore, err := readGitIgnore(repo, root)
	if err != nil {
		return err
	}
	ignored, err := ignore.untracked(ctx, entries, start, func(name string) error {
		if err := add(name); err != nil {
			return err
		}
		// Like ls-files, a nested repository is listed as one entry.
		if info, err := root.Lstat(filepath.FromSlash(name)); err == nil && info.IsDir() {
			f.submodules[corefs.Path(path.Join(filepath.ToSlash(repoRoot), name))] = true
		}
		return nil
	})
	if err == nil && !ignored {
		f.allowed[f.root] = true
	}
	return err
}
