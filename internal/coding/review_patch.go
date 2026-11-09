package coding

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"maps"
	"os"
	"path"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unicode"

	git "github.com/go-git/go-git/v5"
	gitconfig "github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/filemode"
	"github.com/go-git/go-git/v5/plumbing/format/diff"
	"github.com/go-git/go-git/v5/plumbing/format/gitattributes"
	"github.com/go-git/go-git/v5/plumbing/object"
	gitdiff "github.com/go-git/go-git/v5/utils/diff"
	"github.com/sergi/go-diff/diffmatchpatch"
)

const reviewTextLimit = 1 << 20

type reviewFile struct {
	name string
	mode filemode.FileMode
	hash plumbing.Hash
}

func (f reviewFile) Path() string            { return reviewPath(f.name) }
func (f reviewFile) Mode() filemode.FileMode { return f.mode }
func (f reviewFile) Hash() plumbing.Hash     { return f.hash }

func reviewPath(name string) string {
	if strings.ContainsAny(name, `"\`) || strings.ContainsFunc(name, unicode.IsControl) {
		return strconv.Quote(name)
	}
	return name
}

type reviewChunk struct {
	text string
	op   diff.Operation
}

func (c reviewChunk) Content() string      { return c.text }
func (c reviewChunk) Type() diff.Operation { return c.op }

type reviewPatch struct {
	from, to reviewFile
	chunks   []diff.Chunk
	binary   bool
}

func (p reviewPatch) FilePatches() []diff.FilePatch { return []diff.FilePatch{p} }
func (p reviewPatch) Message() string               { return "" }
func (p reviewPatch) IsBinary() bool                { return p.binary }
func (p reviewPatch) Chunks() []diff.Chunk          { return p.chunks }
func (p reviewPatch) Files() (from, to diff.File) {
	if p.from.mode != filemode.Empty {
		from = p.from
	}
	if p.to.mode != filemode.Empty {
		to = p.to
	}
	return from, to
}

func writeReviewPatch(ctx context.Context, out io.Writer, patch reviewPatch, before, after []byte) error {
	if patch.from == patch.to {
		return nil
	}
	if len(before) > reviewTextLimit || len(after) > reviewTextLimit {
		name := patch.to.name
		if name == "" {
			name = patch.from.name
		}
		if err := diff.NewUnifiedEncoder(out, diff.DefaultContextLines).Encode(patch); err != nil {
			return err
		}
		_, err := fmt.Fprintf(out, "[Content omitted for %s: larger than 1 MiB.]\n", reviewPath(name))
		return err
	}
	patch.binary = bytes.IndexByte(before, 0) >= 0 || bytes.IndexByte(after, 0) >= 0
	if !patch.binary && !bytes.Equal(before, after) {
		timeout := time.Second
		if deadline, ok := ctx.Deadline(); ok {
			timeout = min(timeout, time.Until(deadline))
		}
		if timeout <= 0 {
			return context.DeadlineExceeded
		}
		for _, change := range gitdiff.DoWithTimeout(string(before), string(after), timeout) {
			op := diff.Equal
			switch change.Type {
			case diffmatchpatch.DiffInsert:
				op = diff.Add
			case diffmatchpatch.DiffDelete:
				op = diff.Delete
			}
			patch.chunks = append(patch.chunks, reviewChunk{text: change.Text, op: op})
		}
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return diff.NewUnifiedEncoder(out, diff.DefaultContextLines).Encode(patch)
}

func indexReviewFile(entries gitIndex, name string) reviewFile {
	entry := entries[name]
	if entry == nil || entry.Stage != 0 || entry.IntentToAdd {
		return reviewFile{}
	}
	return reviewFile{name: name, hash: entry.Hash, mode: entry.Mode}
}

func reviewBlob(repo *git.Repository, file reviewFile) ([]byte, error) {
	switch file.mode {
	case filemode.Empty:
		return nil, nil
	case filemode.Submodule:
		return fmt.Appendf(nil, "Subproject commit %s\n", file.hash), nil
	}
	blob, err := repo.BlobObject(file.hash)
	if err != nil {
		return nil, err
	}
	if blob.Size > reviewTextLimit {
		return oversizedReviewContent, nil
	}
	reader, err := blob.Reader()
	if err != nil {
		return nil, err
	}
	defer reader.Close()
	return io.ReadAll(io.LimitReader(reader, reviewTextLimit+1))
}

var oversizedReviewContent = make([]byte, reviewTextLimit+1)

func stagedReview(ctx context.Context, repo *git.Repository, entries gitIndex, out io.Writer) error {
	tree, err := reviewHeadTree(repo)
	if err != nil {
		return err
	}
	changes, err := stagedChanges(ctx, tree, entries)
	if err != nil {
		return err
	}
	config, err := repo.Config()
	if err != nil {
		return err
	}
	if !strings.EqualFold(config.Raw.Section("diff").Option("renames"), "false") {
		changes, err = object.DetectRenames(changes, &object.DiffTreeOptions{
			DetectRenames: true, RenameScore: 50, RenameLimit: 200,
		})
		if err != nil {
			return err
		}
	}
	for _, name := range slices.Sorted(maps.Keys(entries)) {
		if entries[name].Stage != 0 {
			if err := writeUnmerged(out, name); err != nil {
				return err
			}
		}
	}
	sort.Sort(changes)
	for _, change := range changes {
		if err := ctx.Err(); err != nil {
			return err
		}
		from := reviewFile{name: change.From.Name, hash: change.From.TreeEntry.Hash, mode: change.From.TreeEntry.Mode}
		to := reviewFile{name: change.To.Name, hash: change.To.TreeEntry.Hash, mode: change.To.TreeEntry.Mode}
		if from.hash == to.hash {
			if err := writeReviewPatch(ctx, out, reviewPatch{from: from, to: to}, nil, nil); err != nil {
				return err
			}
			continue
		}
		before, err := reviewBlob(repo, from)
		if err != nil {
			return err
		}
		after, err := reviewBlob(repo, to)
		if err != nil {
			return err
		}
		if err := writeReviewPatch(ctx, out, reviewPatch{from: from, to: to}, before, after); err != nil {
			return err
		}
	}
	return nil
}

func reviewHeadTree(repo *git.Repository) (*object.Tree, error) {
	head, err := repo.Head()
	if errors.Is(err, plumbing.ErrReferenceNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	commit, err := repo.CommitObject(head.Hash())
	if err != nil {
		return nil, err
	}
	return commit.Tree()
}

func stagedChanges(ctx context.Context, tree *object.Tree, entries gitIndex) (object.Changes, error) {
	var changes object.Changes
	seen := make(map[string]bool, len(entries))
	if tree != nil {
		walk := object.NewTreeWalker(tree, true, nil)
		defer walk.Close()
		for {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			name, entry, err := walk.Next()
			if errors.Is(err, io.EOF) {
				break
			}
			if err != nil {
				return nil, err
			}
			if entry.Mode == filemode.Dir {
				continue
			}
			seen[name] = true
			if indexEntry := entries[name]; indexEntry != nil && indexEntry.Stage != 0 {
				continue
			}
			to := indexReviewFile(entries, name)
			if entry.Hash != to.hash || entry.Mode != to.mode {
				changes = append(changes, &object.Change{
					From: object.ChangeEntry{Name: name, Tree: tree, TreeEntry: entry},
					To:   to.changeEntry(tree),
				})
			}
		}
	}
	for name := range entries {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if to := indexReviewFile(entries, name); !seen[name] && to.mode != filemode.Empty {
			changes = append(changes, &object.Change{To: to.changeEntry(tree)})
		}
	}
	return changes, nil
}

func (f reviewFile) changeEntry(tree *object.Tree) object.ChangeEntry {
	if f.mode == filemode.Empty {
		return object.ChangeEntry{}
	}
	return object.ChangeEntry{
		Name: f.name, Tree: tree,
		TreeEntry: object.TreeEntry{Name: path.Base(f.name), Hash: f.hash, Mode: f.mode},
	}
}

func writeUnmerged(out io.Writer, name string) error {
	_, err := fmt.Fprintf(out, "* Unmerged path %s\n", reviewPath(name))
	return err
}

func unstagedReview(ctx context.Context, repo *git.Repository, root *os.Root, entries gitIndex, out io.Writer) error {
	config, err := repo.Config()
	if err != nil {
		return err
	}
	core := config.Raw.Section("core")
	autocrlf := core.Option("autocrlf")
	if autocrlf == "" {
		if global, err := gitconfig.LoadConfig(gitconfig.GlobalScope); err == nil {
			autocrlf = global.Raw.Section("core").Option("autocrlf")
		}
	}
	endings := reviewEOL{root: root, autocrlf: strings.EqualFold(autocrlf, "true") || strings.EqualFold(autocrlf, "input"), dirs: make(map[string][]gitattributes.MatchAttribute)}
	fileMode := !strings.EqualFold(core.Option("filemode"), "false")
	symlinks := !strings.EqualFold(core.Option("symlinks"), "false")
	for _, name := range slices.Sorted(maps.Keys(entries)) {
		if err := ctx.Err(); err != nil {
			return err
		}
		entry := entries[name]
		if entry.SkipWorktree {
			continue
		}
		if entry.Stage != 0 {
			if err := writeUnmerged(out, name); err != nil {
				return err
			}
			continue
		}
		from := indexReviewFile(entries, name)
		to, after, err := worktreeReviewFile(ctx, root, entry.Name, entry.Mode)
		if errors.Is(err, errSubmoduleUninitialized) {
			continue
		}
		if err != nil {
			return err
		}
		if entry.IntentToAdd && to.mode == filemode.Empty {
			continue
		}
		if !fileMode && to.mode.IsFile() && to.mode != filemode.Symlink && from.mode.IsFile() && from.mode != filemode.Symlink {
			to.mode = from.mode
		}
		if !symlinks && from.mode == filemode.Symlink && to.mode.IsRegular() {
			to.mode = from.mode
		}
		if from == to {
			continue
		}
		if from.hash == to.hash {
			if err := writeReviewPatch(ctx, out, reviewPatch{from: from, to: to}, nil, nil); err != nil {
				return err
			}
			continue
		}
		before, err := reviewBlob(repo, from)
		if err != nil {
			return err
		}
		if to.mode.IsFile() && to.mode != filemode.Symlink {
			if len(after) > reviewTextLimit {
				to.hash, err = endings.normalizeLarge(ctx, repo, from, to)
			} else if bytes.Contains(after, []byte("\r\n")) {
				after, err = endings.normalize(name, before, after)
				to.hash = plumbing.ComputeHash(plumbing.BlobObject, after)
			}
			if err != nil {
				return err
			}
		}
		if err := writeReviewPatch(ctx, out, reviewPatch{from: from, to: to}, before, after); err != nil {
			return err
		}
	}
	return nil
}

func worktreeReviewFile(ctx context.Context, root *os.Root, name string, indexMode filemode.FileMode) (reviewFile, []byte, error) {
	for dir := path.Dir(name); dir != "."; dir = path.Dir(dir) {
		info, err := root.Lstat(filepath.FromSlash(dir))
		if errors.Is(err, fs.ErrNotExist) || errors.Is(err, syscall.ENOTDIR) || (err == nil && !info.IsDir()) {
			return reviewFile{}, nil, nil
		}
		if err != nil {
			return reviewFile{}, nil, err
		}
	}
	nameOnDisk := filepath.FromSlash(name)
	info, err := root.Lstat(nameOnDisk)
	if errors.Is(err, fs.ErrNotExist) || errors.Is(err, syscall.ENOTDIR) {
		return reviewFile{}, nil, nil
	}
	if err != nil {
		return reviewFile{}, nil, err
	}
	mode, _ := filemode.NewFromOSFileMode(info.Mode())
	file := reviewFile{name: name, mode: mode}
	var body []byte
	switch {
	case mode == filemode.Symlink:
		link, err := root.Readlink(nameOnDisk)
		if err != nil {
			return reviewFile{}, nil, err
		}
		body = []byte(link)
	case mode == filemode.Dir && indexMode == filemode.Submodule:
		if _, err := root.Lstat(filepath.Join(nameOnDisk, ".git")); errors.Is(err, fs.ErrNotExist) {
			// Git treats an uninitialized submodule directory as unchanged.
			return reviewFile{}, nil, errSubmoduleUninitialized
		} else if err != nil {
			return reviewFile{}, nil, err
		}
		sub, err := openGit(filepath.Join(root.Name(), nameOnDisk))
		if err != nil {
			return reviewFile{}, nil, err
		}
		head, err := sub.Head()
		if err != nil {
			return reviewFile{}, nil, err
		}
		file.mode, file.hash = filemode.Submodule, head.Hash()
		return file, fmt.Appendf(nil, "Subproject commit %s\n", file.hash), nil
	case mode.IsFile():
		if info.Size() > reviewTextLimit {
			hash, err := hashWorktreeFile(ctx, root, nameOnDisk, info.Size())
			file.hash = hash
			return file, oversizedReviewContent, err
		}
		body, err = readWorktreeFile(root, nameOnDisk, reviewTextLimit)
		if err != nil {
			return reviewFile{}, nil, err
		}
		if len(body) > reviewTextLimit {
			return reviewFile{}, nil, fmt.Errorf("worktree file %q changed during review", name)
		}
	default:
		return reviewFile{}, nil, nil
	}
	file.hash = plumbing.ComputeHash(plumbing.BlobObject, body)
	return file, body, nil
}

var errSubmoduleUninitialized = errors.New("uninitialized submodule")

func openWorktreeFile(root *os.Root, name string) (*os.File, error) {
	file, err := openReviewFile(root, name)
	if err != nil {
		return nil, err
	}
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() {
		file.Close()
		if err == nil {
			err = fmt.Errorf("worktree file %q is no longer regular", name)
		}
		return nil, err
	}
	return file, nil
}

func readWorktreeFile(root *os.Root, name string, limit int64) ([]byte, error) {
	file, err := openWorktreeFile(root, name)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	return io.ReadAll(io.LimitReader(file, limit+1))
}

func hashWorktreeFile(ctx context.Context, root *os.Root, name string, size int64) (plumbing.Hash, error) {
	file, err := openWorktreeFile(root, name)
	if err != nil {
		return plumbing.ZeroHash, err
	}
	defer file.Close()
	hash := plumbing.NewHasher(plumbing.BlobObject, size)
	for remaining := size; remaining > 0; {
		if err := ctx.Err(); err != nil {
			return plumbing.ZeroHash, err
		}
		n, err := io.CopyN(hash, file, min(remaining, 1<<20))
		if err != nil {
			return plumbing.ZeroHash, err
		}
		remaining -= n
	}
	return hash.Sum(), nil
}
