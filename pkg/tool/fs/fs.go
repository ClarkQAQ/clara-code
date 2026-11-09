// Package fs defines Clara Core's complete file storage and editing capability.
package fs

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"path"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"clara-code/pkg/util/errs"
)

// Path is a logical slash-separated path rooted at "/".
type Path string

// Clean normalizes a logical path and rejects escapes from the namespace root.
func Clean(value string) (Path, error) {
	return Resolve("/", value)
}

// Resolve resolves a logical path against base and rejects traversal above root.
func Resolve(base Path, value string) (Path, error) {
	if strings.IndexByte(value, 0) >= 0 {
		return "", errs.New(errs.CodeInvalidArgument, "invalid path")
	}
	value = strings.ReplaceAll(value, "\\", "/")
	if !strings.HasPrefix(value, "/") {
		value = strings.TrimSuffix(base.String(), "/") + "/" + value
	}
	var parts []string
	for _, component := range strings.Split(value, "/") {
		switch component {
		case "", ".":
		case "..":
			if len(parts) == 0 {
				return "", errs.New(errs.CodeForbidden, "path escapes the filesystem root")
			}
			parts = parts[:len(parts)-1]
		default:
			parts = append(parts, component)
		}
	}
	return Path("/" + strings.Join(parts, "/")), nil
}

// Join joins logical path elements below base.
func Join(base Path, elements ...string) (Path, error) {
	return Resolve(base, strings.Join(elements, "/"))
}

// Base returns the final logical path element.
func (p Path) Base() string { return path.Base(string(p)) }

// Dir returns the logical parent.
func (p Path) Dir() Path { return Path(path.Dir(string(p))) }

// String returns the logical path.
func (p Path) String() string { return string(p) }

// Entry describes one file or directory.
type Entry struct {
	Path    Path        `json:"path"`
	Name    string      `json:"name"`
	Size    int64       `json:"size"`
	Mode    fs.FileMode `json:"mode"`
	ModTime time.Time   `json:"modified_at"`
	IsDir   bool        `json:"is_dir"`
	IsLink  bool        `json:"is_link,omitempty"`
}

// WriteOptions controls a write.
type WriteOptions struct {
	Mode       fs.FileMode
	CreateDirs bool
	Overwrite  bool
}

// RemoveOptions controls deletion.
type RemoveOptions struct {
	Recursive bool
}

// WalkOptions bounds recursive traversal.
type WalkOptions struct {
	MaxDepth    int
	MaxEntries  int
	FollowLinks bool
}

// WalkEntry is one result from a bounded walk.
type WalkEntry struct {
	Entry
	Depth int
}

// WalkResult reports traversal output and truncation.
type WalkResult struct {
	Entries   []WalkEntry
	Truncated bool
}

// SearchOptions configures a text search.
type SearchOptions struct {
	Query       string
	MatchCase   bool
	MaxResults  int
	MaxFileSize int64
	Include     []string
	Walk        WalkOptions
}

// SearchHit is one matching line.
type SearchHit struct {
	Path Path
	Line int
	Text string
}

// Reader is the minimum contract for a readable workspace.
type Reader interface {
	Stat(context.Context, Path) (Entry, error)
	ReadDir(context.Context, Path) ([]Entry, error)
	Open(context.Context, Path) (io.ReadCloser, error)
}

// Writer is implemented by workspaces that support mutations.
type Writer interface {
	WriteFile(context.Context, Path, []byte, WriteOptions) (Entry, error)
	MkdirAll(context.Context, Path, fs.FileMode) error
	Remove(context.Context, Path, RemoveOptions) error
	Rename(context.Context, Path, Path) error
}

// FS combines storage operations with bounded and random access reads.
type FS interface {
	Reader
	Writer
	ReadFile(context.Context, Path, int64) ([]byte, error)
	ReadAt(context.Context, Path, int64, int64) ([]byte, error)
}

// Service exposes complete file operations over an FS.
type Service struct {
	FS FS
}

// New returns a file service.
func New(storage FS) *Service {
	return &Service{FS: storage}
}

// View reports metadata and a bounded textual preview.
type View struct {
	Entry     Entry
	Text      string
	Binary    bool
	Truncated bool
}

// View reads metadata and a text preview.
func (s *Service) View(ctx context.Context, value Path, max int64) (View, error) {
	if s == nil || s.FS == nil {
		return View{}, errs.New(errs.CodeInternal, "filesystem is not configured")
	}
	entry, err := s.FS.Stat(ctx, value)
	if err != nil {
		return View{}, err
	}
	if entry.IsDir {
		return View{Entry: entry}, nil
	}
	if max <= 0 {
		max = 256 * 1024
	}
	data, err := s.FS.ReadFile(ctx, value, max+1)
	if err != nil {
		return View{}, err
	}
	truncated := int64(len(data)) > max
	if truncated {
		data = data[:max]
	}
	binary := !utf8.Valid(data) || strings.IndexByte(string(data), 0) >= 0
	view := View{Entry: entry, Binary: binary, Truncated: truncated}
	if !binary {
		view.Text = string(data)
	}
	return view, nil
}

// Create creates a new file and refuses to replace an existing one.
func (s *Service) Create(ctx context.Context, value Path, data []byte, mode fs.FileMode) (Entry, error) {
	if mode == 0 {
		mode = 0o644
	}
	return s.FS.WriteFile(ctx, value, data, WriteOptions{Mode: mode, CreateDirs: true})
}

// Write replaces or creates a file.
func (s *Service) Write(ctx context.Context, value Path, data []byte, mode fs.FileMode) (Entry, error) {
	if mode == 0 {
		mode = 0o644
	}
	return s.FS.WriteFile(ctx, value, data, WriteOptions{Mode: mode, CreateDirs: true, Overwrite: true})
}

// Replace performs a bounded literal text replacement.
func (s *Service) Replace(ctx context.Context, value Path, oldText, newText string, replaceAll bool) (Entry, error) {
	if oldText == "" {
		return Entry{}, errs.New(errs.CodeInvalidArgument, "old text is empty")
	}
	data, err := s.FS.ReadFile(ctx, value, 16<<20)
	if err != nil {
		return Entry{}, err
	}
	text := string(data)
	count := strings.Count(text, oldText)
	if count == 0 {
		return Entry{}, errs.New(errs.CodeNotFound, "old text was not found")
	}
	if count > 1 && !replaceAll {
		return Entry{}, errs.New(errs.CodeConflict, "old text occurs more than once")
	}
	if replaceAll {
		text = strings.ReplaceAll(text, oldText, newText)
	} else {
		text = strings.Replace(text, oldText, newText, 1)
	}
	return s.Write(ctx, value, []byte(text), 0o644)
}

// Walk traverses a tree in lexical order.
func (s *Service) Walk(ctx context.Context, root Path, options WalkOptions) (WalkResult, error) {
	if options.MaxDepth <= 0 || options.MaxDepth > 64 {
		options.MaxDepth = 64
	}
	if options.MaxEntries <= 0 || options.MaxEntries > 20000 {
		options.MaxEntries = 10_000
	}
	entry, err := s.FS.Stat(ctx, root)
	if err != nil {
		return WalkResult{}, err
	}
	result := WalkResult{}
	queue := []WalkEntry{{Entry: entry, Depth: 0}}
	for len(queue) > 0 {
		if err := ctx.Err(); err != nil {
			return WalkResult{}, err
		}
		current := queue[0]
		queue = queue[1:]
		result.Entries = append(result.Entries, current)
		if current.IsLink && !options.FollowLinks {
			continue
		}
		if !current.IsDir {
			continue
		}
		if current.Depth >= options.MaxDepth {
			result.Truncated = true
			continue
		}
		children, err := s.FS.ReadDir(ctx, current.Path)
		if err != nil {
			return result, err
		}
		sort.SliceStable(children, func(i, j int) bool {
			if children[i].IsDir != children[j].IsDir {
				return children[i].IsDir
			}
			return children[i].Name < children[j].Name
		})
		for _, child := range children {
			if len(result.Entries)+len(queue) >= options.MaxEntries {
				result.Truncated = true
				break
			}
			queue = append(queue, WalkEntry{Entry: child, Depth: current.Depth + 1})
		}
	}
	return result, nil
}

// Glob is the compatibility view of GlobDetailed. Use GlobDetailed for diagnostics.
func (s *Service) Glob(ctx context.Context, root Path, pattern string, maxEntries int) ([]Entry, error) {
	result, err := s.GlobDetailed(ctx, root, GlobOptions{Pattern: pattern, MaxResults: maxEntries})
	return result.Entries, err
}

// Grep is the compatibility view of GrepDetailed. Use GrepDetailed for diagnostics.
func (s *Service) Grep(ctx context.Context, root Path, options SearchOptions) ([]SearchHit, error) {
	result, err := s.GrepDetailed(ctx, root, options)
	return result.Hits, err
}

// Copy copies one file or directory tree.
func (s *Service) Copy(ctx context.Context, source, destination Path) error {
	entry, err := s.FS.Stat(ctx, source)
	if err != nil {
		return err
	}
	if entry.IsLink {
		return errs.ErrForbidden
	}
	if !entry.IsDir {
		if entry.Size > 64<<20 {
			return errs.New(errs.CodeLimit, "file is too large to copy")
		}
		data, err := s.FS.ReadFile(ctx, source, 64<<20)
		if err != nil {
			return err
		}
		_, err = s.Create(ctx, destination, data, entry.Mode.Perm())
		return err
	}
	source = entry.Path
	destination, err = ResolvePath(s.FS, destination.String())
	if err != nil {
		return err
	}
	walk, err := s.Walk(ctx, source, WalkOptions{})
	if err != nil {
		return err
	}
	if walk.Truncated {
		return errs.New(errs.CodeLimit, "copy walk exceeds its limits")
	}
	for _, item := range walk.Entries {
		if item.IsLink || item.Size > 64<<20 {
			return errs.New(errs.CodeUnsupported, "cannot copy symlinks or oversized files")
		}
	}
	for _, item := range walk.Entries {
		relative := strings.TrimPrefix(item.Path.String(), source.String())
		relative = strings.TrimPrefix(relative, "/")
		target := destination
		if relative != "" {
			target = Path(strings.TrimSuffix(destination.String(), "/") + "/" + relative)
		}
		if item.IsDir {
			if err := s.FS.MkdirAll(ctx, target, item.Mode.Perm()); err != nil {
				return err
			}
			continue
		}
		data, err := s.FS.ReadFile(ctx, item.Path, 64<<20)
		if err != nil {
			return err
		}
		if _, err := s.Create(ctx, target, data, item.Mode.Perm()); err != nil {
			return err
		}
	}
	return nil
}

// EnsureNotRoot rejects destructive operations on the namespace root.
func EnsureNotRoot(value Path) error {
	clean, err := Clean(value.String())
	if err != nil {
		return err
	}
	if clean == "/" {
		return errs.New(errs.CodeForbidden, "the filesystem root cannot be removed or renamed")
	}
	return nil
}

// IsNotExist reports whether err wraps fs.ErrNotExist.
func IsNotExist(err error) bool { return errors.Is(err, fs.ErrNotExist) }
