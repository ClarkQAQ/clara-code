package coding

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"

	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
)

func initTestGit(t *testing.T, root string) *git.Repository {
	t.Helper()
	repo, err := git.PlainInitWithOptions(root, &git.PlainInitOptions{
		InitOptions: git.InitOptions{DefaultBranch: plumbing.NewBranchReferenceName("main")},
	})
	if err != nil {
		t.Fatal(err)
	}
	return repo
}

func stageTestFiles(t *testing.T, repo *git.Repository, names ...string) *git.Worktree {
	t.Helper()
	worktree, err := repo.Worktree()
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range names {
		if err := worktree.AddWithOptions(&git.AddOptions{Path: name, SkipStatus: true}); err != nil {
			t.Fatal(err)
		}
	}
	return worktree
}

func commitTestGit(t *testing.T, worktree *git.Worktree) plumbing.Hash {
	t.Helper()
	commit, err := worktree.Commit("fixture", &git.CommitOptions{
		Author: &object.Signature{Name: "Fixture", Email: "fixture@example.com"},
	})
	if err != nil {
		t.Fatal(err)
	}
	return commit
}

func reviewText(t *testing.T, space Workspace, kind string) string {
	t.Helper()
	var text strings.Builder
	for {
		page, more, err := space.DiffPage(t.Context(), kind, text.Len())
		if err != nil || len(page) > reviewPageSize || !utf8.ValidString(page) || (more && page == "") {
			t.Fatalf("%s page at offset %d: size %d, more %v, err %v", kind, text.Len(), len(page), more, err)
		}
		text.WriteString(page)
		if !more {
			return text.String()
		}
	}
}

func TestGitReviewWithoutGitExecutable(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	root := t.TempDir()
	repo := initTestGit(t, root)
	space, err := ResolveWorkspace(root)
	if err != nil {
		t.Fatal(err)
	}
	if branch := space.Branch(t.Context()); branch != "main" {
		t.Fatalf("unborn branch: %q", branch)
	}
	writeTestFile(t, root, "tracked.txt", "original\n")
	writeTestFile(t, root, "deleted.txt", "will be deleted\n")
	writeTestFile(t, root, "old.txt", "renamed without changing content\n")
	writeTestFile(t, root, "mode.sh", "echo hello\n")
	worktree := stageTestFiles(t, repo, "tracked.txt", "deleted.txt", "old.txt", "mode.sh")
	if staged := reviewText(t, space, "staged"); !strings.Contains(staged, "+original\n") {
		t.Fatal(staged)
	}
	commit := commitTestGit(t, worktree)
	if err := repo.Storer.SetReference(plumbing.NewHashReference(plumbing.HEAD, commit)); err != nil {
		t.Fatal(err)
	}
	if branch := space.Branch(t.Context()); branch != commit.String()[:7] {
		t.Fatalf("detached HEAD: %q", branch)
	}
	if staged := reviewText(t, space, "staged"); staged != "" {
		t.Fatal(staged)
	}
	writeTestFile(t, root, "tracked.txt", "in index\n")
	writeTestFile(t, root, "binary.dat", "\x00\x01")
	stageTestFiles(t, repo, "tracked.txt", "binary.dat")
	if _, err := worktree.Remove("deleted.txt"); err != nil {
		t.Fatal(err)
	}
	if _, err := worktree.Move("old.txt", "renamed.txt"); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(filepath.Join(root, "mode.sh"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, root, "tracked.txt", "in worktree\n")
	writeTestFile(t, root, ".gitignore", "ignored.txt\n")
	writeTestFile(t, root, "ignored.txt", "excluded\n")
	writeTestFile(t, root, "-[literal];$(echo hi).txt", strings.Repeat("中文\n", 7000))
	staged := reviewText(t, space, "staged")
	for _, want := range []string{"-original\n+in index\n", "-will be deleted\n", "Binary files /dev/null and b/binary.dat differ", "rename from old.txt\nrename to renamed.txt"} {
		if !strings.Contains(staged, want) {
			t.Fatalf("missing %q in staged diff: %s", want, staged)
		}
	}
	unstaged := reviewText(t, space, "unstaged")
	if !strings.Contains(unstaged, "-in index\n+in worktree\n") || strings.Contains(unstaged, "deleted.txt") || !strings.Contains(unstaged, "old mode 100644\nnew mode 100755") {
		t.Fatal(unstaged)
	}
	untracked := reviewText(t, space, "untracked")
	if !strings.Contains(untracked, "-[literal];$(echo hi).txt") || strings.Contains(untracked, "b/ignored.txt") ||
		strings.Count(untracked, "+中文\n") != 7000 {
		t.Fatalf("unexpected untracked diff, length %d", len(untracked))
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, _, err := space.DiffPage(ctx, "staged", 0); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled review: %v", err)
	}
}

func TestGitLinkedWorktree(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	root, linked := t.TempDir(), t.TempDir()
	repo := initTestGit(t, root)
	writeTestFile(t, root, "tracked.txt", "original\n")
	commitTestGit(t, stageTestFiles(t, repo, "tracked.txt"))
	writeTestFile(t, root, ".git/info/exclude", "ignored.txt\n")

	gitdir := filepath.Join(root, ".git", "worktrees", "linked")
	writeTestFile(t, linked, ".git", "gitdir: "+gitdir+"\n")
	writeTestFile(t, gitdir, "commondir", "../..\n")
	writeTestFile(t, gitdir, "HEAD", "ref: refs/heads/main\n")
	indexData, err := os.ReadFile(filepath.Join(root, ".git", "index"))
	if err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, gitdir, "index", string(indexData))
	writeTestFile(t, linked, "tracked.txt", "changed\n")
	writeTestFile(t, linked, "ignored.txt", "excluded\n")

	space, err := ResolveWorkspace(linked)
	if err != nil {
		t.Fatal(err)
	}
	if branch := space.Branch(t.Context()); branch != "main" {
		t.Fatalf("linked branch: %q", branch)
	}
	if diff := reviewText(t, space, "staged"); diff != "" {
		t.Fatal(diff)
	}
	if diff := reviewText(t, space, "unstaged"); !strings.Contains(diff, "-original\n+changed\n") {
		t.Fatal(diff)
	}
	if diff := reviewText(t, space, "untracked"); diff != "" {
		t.Fatal(diff)
	}
}

func TestGitReviewNormalizesConfiguredTextEndings(t *testing.T) {
	root := t.TempDir()
	repo := initTestGit(t, root)
	cfg, err := repo.Config()
	if err != nil {
		t.Fatal(err)
	}
	cfg.Raw.Section("core").SetOption("autocrlf", "true")
	if err := repo.SetConfig(cfg); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, root, ".gitattributes", "raw.txt -text\n*.ps1 text eol=crlf\n")
	writeTestFile(t, root, "tracked.txt", "one\ntwo\n")
	writeTestFile(t, root, "script.ps1", "old\nunchanged\n")
	writeTestFile(t, root, "raw.txt", "unchanged\n")
	writeTestFile(t, root, "large.txt", strings.Repeat("large line\n", reviewTextLimit/10))
	worktree := stageTestFiles(t, repo, ".gitattributes", "tracked.txt", "script.ps1", "raw.txt", "large.txt")
	commitTestGit(t, worktree)

	writeTestFile(t, root, "tracked.txt", "one\r\ntwo\r\n")
	writeTestFile(t, root, "script.ps1", "new\r\nunchanged\r\n")
	writeTestFile(t, root, "raw.txt", "unchanged\r\n")
	writeTestFile(t, root, "large.txt", strings.Repeat("large line\r\n", reviewTextLimit/10))
	space, err := ResolveWorkspace(root)
	if err != nil {
		t.Fatal(err)
	}
	diff := reviewText(t, space, "unstaged")
	if strings.Contains(diff, "tracked.txt") || strings.Contains(diff, "large.txt") || !strings.Contains(diff, "raw.txt") ||
		!strings.Contains(diff, "-old\n+new\n unchanged\n") {
		t.Fatalf("unexpected line ending diff:\n%s", diff)
	}
}
