package coding

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
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

func (f searchFS) ResolvePath(value string) (corefs.Path, error) {
	return corefs.ResolvePath(f.FS, value)
}

func gitPathKey(p corefs.Path) corefs.Path {
	if runtime.GOOS == "windows" {
		return corefs.Path(strings.ToLower(strings.TrimSuffix(p.String(), "/")))
	}
	return p
}

func nativeParent(p corefs.Path) corefs.Path {
	return corefs.Path(filepath.ToSlash(filepath.Dir(filepath.FromSlash(p.String()))))
}

func (f searchFS) excluded(p corefs.Path) bool {
	for current := p; gitPathKey(current) != gitPathKey(f.root); current = nativeParent(current) {
		if gitPathKey(corefs.Path(current.Base())) == ".git" || f.submodules[gitPathKey(current)] {
			return true
		}
		if nativeParent(current) == current {
			break
		}
	}
	return f.allowed != nil && !f.allowed[gitPathKey(p)]
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
		if gitPathKey(corefs.Path(part)) == ".git" {
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
	startKey := gitPathKey(corefs.Path(start)).String()
	for name, entry := range entries {
		if entry.Mode == filemode.Submodule {
			f.submodules[gitPathKey(corefs.Path(filepath.ToSlash(filepath.Join(repoRoot, name))))] = true
			key := gitPathKey(corefs.Path(name)).String()
			if startKey == key || strings.HasPrefix(startKey, key+"/") {
				return nil
			}
		}
	}
	if includeIgnored {
		return ctx.Err()
	}

	f.allowed = map[corefs.Path]bool{}
	count, size := 0, 0
	add := func(name string) error {
		count++
		size += len(name) + 1
		if count > 20000 || size > 8<<20 {
			return fmt.Errorf("git search enumeration exceeded 20000 files or 8 MiB; narrow path")
		}
		absolute := corefs.Path(filepath.ToSlash(filepath.Join(repoRoot, name)))
		for p := absolute; !f.allowed[gitPathKey(p)]; p = nativeParent(p) {
			f.allowed[gitPathKey(p)] = true
			if nativeParent(p) == p {
				break
			}
		}
		return nil
	}
	for name := range entries {
		if err := ctx.Err(); err != nil {
			return err
		}
		key := gitPathKey(corefs.Path(name)).String()
		if startKey == "." || key == startKey || strings.HasPrefix(key, startKey+"/") {
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
			f.submodules[gitPathKey(corefs.Path(filepath.ToSlash(filepath.Join(repoRoot, name))))] = true
		}
		return nil
	})
	if err == nil && !ignored {
		f.allowed[f.root] = true
	}
	return err
}
