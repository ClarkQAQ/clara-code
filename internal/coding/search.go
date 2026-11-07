package coding

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"clara-code/pkg/tools"
	corefs "clara-code/pkg/tools/fs"
	"clara-code/pkg/util/errs"
)

func (w Workspace) files() *tools.Files {
	files := tools.NewFiles(w.toolFS(), nil)
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

type searchBuffer struct{ bytes.Buffer }

func (b *searchBuffer) Write(p []byte) (int, error) {
	const limit = 8 << 20
	if len(p) > limit-b.Len() {
		return 0, fmt.Errorf("git search enumeration exceeded 8 MiB; narrow path")
	}
	return b.Buffer.Write(p)
}

func (w Workspace) gitSearch(ctx context.Context, root corefs.Path, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	args = append([]string{"-C", w.Root, "-c", "core.fsmonitor=false", "ls-files"}, args...)
	if root.String() != filepath.ToSlash(w.Root) {
		relative, err := filepath.Rel(w.Root, filepath.FromSlash(root.String()))
		if err != nil {
			return nil, err
		}
		args = append(args, "--", ":(literal)"+filepath.ToSlash(relative))
	}
	command := searchGitCommand(ctx, args...)
	var output, stderr searchBuffer
	command.Stdout, command.Stderr = &output, &stderr
	if err := command.Run(); err != nil {
		return nil, fmt.Errorf("git search enumeration failed: %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	return output.Bytes(), nil
}

func (w Workspace) searchSource(ctx context.Context, root corefs.Path, includeIgnored bool) (corefs.FS, error) {
	repo, git, err := projectForPath(filepath.FromSlash(root.String()))
	if err != nil {
		return nil, err
	}
	w.Root, w.Git = repo, git
	storage := searchFS{FS: w.toolFS(), submodules: map[corefs.Path]bool{}, root: root}
	for _, part := range strings.Split(root.String(), "/") {
		if part == ".git" {
			return storage, nil
		}
	}
	if !w.Git {
		return storage, nil
	}
	stages, err := w.gitSearch(ctx, corefs.Path(filepath.ToSlash(w.Root)), "--stage", "-z")
	if err != nil {
		return nil, err
	}
	for _, record := range bytes.Split(stages, []byte{0}) {
		metadata, name, ok := strings.Cut(string(record), "\t")
		if ok && strings.HasPrefix(metadata, "160000 ") {
			storage.submodules[corefs.Path(filepath.ToSlash(filepath.Join(w.Root, name)))] = true
		}
	}
	for module := range storage.submodules {
		if root == module || strings.HasPrefix(root.String(), module.String()+"/") {
			return storage, nil
		}
	}
	if includeIgnored {
		return storage, nil
	}
	listing, err := w.gitSearch(ctx, root, "--cached", "--others", "--exclude-standard", "-z")
	if err != nil {
		return nil, err
	}
	storage.allowed = map[corefs.Path]bool{"/": true}
	count := 0
	for _, raw := range bytes.SplitN(listing, []byte{0}, 20002) {
		if len(raw) == 0 {
			continue
		}
		count++
		if count > 20000 {
			return nil, fmt.Errorf("git search enumeration exceeded 20000 files; narrow path")
		}
		name := filepath.ToSlash(filepath.Join(w.Root, string(raw)))
		for p := corefs.Path(name); p != "/"; p = p.Dir() {
			storage.allowed[p] = true
		}
	}
	// An empty, non-ignored directory still has a valid search with zero results.
	if !storage.allowed[root] {
		if _, err := w.toolFS().Stat(ctx, root); err != nil {
			return nil, err
		}
		ignored, err := w.gitIgnored(ctx, root)
		if err != nil {
			return nil, err
		}
		if !ignored {
			storage.allowed[root] = true
		}
	}
	return storage, nil
}

func (w Workspace) gitIgnored(ctx context.Context, root corefs.Path) (bool, error) {
	if root.String() == filepath.ToSlash(w.Root) {
		return false, nil
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	relative, err := filepath.Rel(w.Root, filepath.FromSlash(root.String()))
	if err != nil {
		return false, err
	}
	command := searchGitCommand(ctx, "-C", w.Root, "check-ignore", "-q", "--", filepath.ToSlash(relative))
	var stderr searchBuffer
	command.Stderr = &stderr
	err = command.Run()
	if err == nil {
		return true, nil
	}
	var exit *exec.ExitError
	if errors.As(err, &exit) && exit.ExitCode() == 1 {
		return false, nil
	}
	return false, fmt.Errorf("git ignore check failed: %w: %s", err, strings.TrimSpace(stderr.String()))
}

func searchGitCommand(ctx context.Context, args ...string) *exec.Cmd {
	command := exec.CommandContext(ctx, "git", args...)
	for _, value := range os.Environ() {
		name, _, _ := strings.Cut(value, "=")
		switch name {
		case "GIT_DIR", "GIT_WORK_TREE", "GIT_INDEX_FILE", "GIT_COMMON_DIR":
			continue
		}
		command.Env = append(command.Env, value)
	}
	return command
}
