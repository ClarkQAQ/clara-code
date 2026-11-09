package coding

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"

	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing/filemode"
	"github.com/go-git/go-git/v5/plumbing/format/gitignore"
	"github.com/go-git/go-git/v5/plumbing/format/index"
	"github.com/go-git/go-git/v5/storage/filesystem"
)

func openGit(root string) (*git.Repository, error) {
	return git.PlainOpenWithOptions(root, &git.PlainOpenOptions{EnableDotGitCommonDir: true})
}

type gitIndex map[string]*index.Entry

func readGitIndex(repo *git.Repository) (gitIndex, error) {
	idx, err := repo.Storer.Index()
	if err != nil {
		return nil, err
	}
	entries := make(gitIndex, len(idx.Entries))
	for _, entry := range idx.Entries {
		if !filepath.IsLocal(filepath.FromSlash(entry.Name)) || path.Clean(entry.Name) != entry.Name || slices.Contains(strings.Split(entry.Name, "/"), ".git") || entry.Name == "." {
			return nil, fmt.Errorf("invalid Git index path %q", entry.Name)
		}
		old := entries[entry.Name]
		if old == nil || entry.Stage < old.Stage {
			entries[entry.Name] = entry
		}
	}
	return entries, nil
}

type gitIgnore struct {
	root  *os.Root
	rules []gitignore.Pattern
}

func readGitIgnore(repo *git.Repository, root *os.Root) (gitIgnore, error) {
	ignore := gitIgnore{root: root}
	global, err := config.LoadConfig(config.GlobalScope)
	if err != nil {
		return ignore, err
	}
	local, err := repo.Config()
	if err != nil {
		return ignore, err
	}
	excludes := local.Raw.Section("core").Option("excludesfile")
	if excludes == "" {
		excludes = global.Raw.Section("core").Option("excludesfile")
	}
	if excludes == "" {
		xdg := os.Getenv("XDG_CONFIG_HOME")
		if xdg == "" {
			if home, err := os.UserHomeDir(); err == nil {
				xdg = filepath.Join(home, ".config")
			}
		}
		if xdg != "" {
			excludes = filepath.Join(xdg, "git", "ignore")
		}
	}
	if relative, ok := strings.CutPrefix(excludes, "~/"); ok {
		home, err := os.UserHomeDir()
		if err != nil {
			return ignore, err
		}
		excludes = filepath.Join(home, relative)
	}
	if excludes != "" {
		if !filepath.IsAbs(excludes) {
			excludes = filepath.Join(root.Name(), excludes)
		}
		file, err := os.Open(excludes)
		ignore.rules, err = appendIgnoreRules(nil, nil, file, err)
		if err != nil {
			return ignore, err
		}
	}
	dotGit := repo.Storer.(*filesystem.Storage).Filesystem()
	file, err := dotGit.Open("info/exclude")
	ignore.rules, err = appendIgnoreRules(ignore.rules, nil, file, err)
	return ignore, err
}

func appendIgnoreRules(rules []gitignore.Pattern, dir []string, file io.ReadCloser, err error) ([]gitignore.Pattern, error) {
	if errors.Is(err, fs.ErrNotExist) {
		return rules, nil
	}
	if err != nil {
		return nil, err
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "#") || strings.TrimSpace(line) == "" {
			continue
		}
		if len(rules) >= 20000 {
			return nil, errors.New("git ignore rules exceeded 20000 patterns")
		}
		rules = append(rules, gitignore.ParsePattern(line, dir))
	}
	return rules, scanner.Err()
}

func (g gitIgnore) inDirectory(name string, rules []gitignore.Pattern) ([]gitignore.Pattern, error) {
	filename := filepath.FromSlash(path.Join(name, ".gitignore"))
	info, err := g.root.Lstat(filename)
	if errors.Is(err, fs.ErrNotExist) || (err == nil && !info.Mode().IsRegular()) {
		return rules, nil
	}
	if err != nil {
		return nil, err
	}
	var dir []string
	if name != "." {
		dir = strings.Split(name, "/")
	}
	file, err := g.root.Open(filename)
	return appendIgnoreRules(rules[:len(rules):len(rules)], dir, file, err)
}

// untracked walks only the requested subtree and prunes ignored directories
// before reading their own ignore files. Tracked files are enumerated separately.
func (g gitIgnore) untracked(ctx context.Context, entries gitIndex, start string, visit func(string) error) (bool, error) {
	rules := g.rules
	if start != "." {
		dir := "."
		for _, part := range strings.Split(start, "/") {
			var err error
			rules, err = g.inDirectory(dir, rules)
			if err != nil {
				return false, err
			}
			dir = path.Join(dir, part)
			if gitignore.NewMatcher(rules).Match(strings.Split(dir, "/"), true) {
				return true, nil
			}
		}
	}
	scopes := map[string][]gitignore.Pattern{path.Dir(start): rules}
	visited := 0
	err := fs.WalkDir(g.root.FS(), start, func(name string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		visited++
		if visited > 200000 {
			return errors.New("git worktree enumeration exceeded 200000 entries; narrow path")
		}
		parentRules := scopes[path.Dir(name)]
		if name != start {
			if entry.Name() == ".git" || (entries[name] != nil && entries[name].Mode == filemode.Submodule) ||
				gitignore.NewMatcher(parentRules).Match(strings.Split(name, "/"), entry.IsDir()) {
				if entry.IsDir() {
					return fs.SkipDir
				}
				return nil
			}
		}
		if !entry.IsDir() {
			if entries[name] == nil {
				return visit(name)
			}
			return nil
		}
		if name != "." {
			if _, err := g.root.Lstat(filepath.FromSlash(path.Join(name, ".git"))); err == nil {
				if err := visit(name); err != nil {
					return err
				}
				return fs.SkipDir
			} else if !errors.Is(err, fs.ErrNotExist) {
				return err
			}
		}
		scopes[name], err = g.inDirectory(name, parentRules)
		// WalkDir visits siblings in order, so only ancestor scopes are needed.
		for dir := range scopes {
			if dir != name && dir != "." && !strings.HasPrefix(name, dir+"/") {
				delete(scopes, dir)
			}
		}
		return err
	})
	return false, err
}
