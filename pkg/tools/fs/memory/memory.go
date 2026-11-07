// Package memory provides an ephemeral in-memory filesystem.
package memory

import (
	"bytes"
	"context"
	"io"
	"io/fs"
	"sort"
	"strings"
	"sync"
	"time"

	clarafs "clara-code/pkg/tools/fs"
	"clara-code/pkg/util/errs"
)

type node struct {
	data    []byte
	mode    fs.FileMode
	modTime time.Time
}

// FS is a concurrency-safe in-memory filesystem.
type FS struct {
	mu    sync.RWMutex
	files map[clarafs.Path]node
	dirs  map[clarafs.Path]node
}

// New creates an empty memory filesystem.
func New() *FS {
	return &FS{
		files: map[clarafs.Path]node{},
		dirs:  map[clarafs.Path]node{"/": {mode: fs.ModeDir | 0o755, modTime: time.Now()}},
	}
}

func (m *FS) Stat(ctx context.Context, value clarafs.Path) (clarafs.Entry, error) {
	if err := ctx.Err(); err != nil {
		return clarafs.Entry{}, err
	}
	clean, err := clarafs.Clean(value.String())
	if err != nil {
		return clarafs.Entry{}, err
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	if item, ok := m.dirs[clean]; ok {
		return entry(clean, item, true), nil
	}
	if item, ok := m.files[clean]; ok {
		return entry(clean, item, false), nil
	}
	return clarafs.Entry{}, errs.ErrNotFound
}

func entry(path clarafs.Path, item node, dir bool) clarafs.Entry {
	return clarafs.Entry{
		Path: path, Name: path.Base(), Size: int64(len(item.data)),
		Mode: item.mode, ModTime: item.modTime, IsDir: dir,
	}
}

func (m *FS) ReadDir(ctx context.Context, value clarafs.Path) ([]clarafs.Entry, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	clean, err := clarafs.Clean(value.String())
	if err != nil {
		return nil, err
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	if _, ok := m.dirs[clean]; !ok {
		return nil, errs.ErrNotFound
	}
	out := []clarafs.Entry{}
	for path, item := range m.dirs {
		if path != clean && path.Dir() == clean {
			out = append(out, entry(path, item, true))
		}
	}
	for path, item := range m.files {
		if path.Dir() == clean {
			out = append(out, entry(path, item, false))
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].IsDir != out[j].IsDir {
			return out[i].IsDir
		}
		return out[i].Name < out[j].Name
	})
	return out, nil
}

func (m *FS) Open(ctx context.Context, value clarafs.Path) (io.ReadCloser, error) {
	data, err := m.ReadFile(ctx, value, 64<<20)
	if err != nil {
		return nil, err
	}
	return io.NopCloser(bytes.NewReader(data)), nil
}

func (m *FS) ReadFile(ctx context.Context, value clarafs.Path, max int64) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	clean, err := clarafs.Clean(value.String())
	if err != nil {
		return nil, err
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	item, ok := m.files[clean]
	if !ok {
		return nil, errs.ErrNotFound
	}
	if max <= 0 || int64(len(item.data)) <= max {
		return append([]byte(nil), item.data...), nil
	}
	return append([]byte(nil), item.data[:max]...), nil
}

func (m *FS) ReadAt(ctx context.Context, value clarafs.Path, offset, limit int64) ([]byte, error) {
	data, err := m.ReadFile(ctx, value, 64<<20)
	if err != nil {
		return nil, err
	}
	if offset < 0 || limit < 0 || offset > int64(len(data)) {
		return nil, errs.New(errs.CodeInvalidArgument, "invalid read range")
	}
	end := offset + limit
	if end > int64(len(data)) {
		end = int64(len(data))
	}
	return append([]byte(nil), data[offset:end]...), nil
}

func (m *FS) WriteFile(ctx context.Context, value clarafs.Path, data []byte, options clarafs.WriteOptions) (clarafs.Entry, error) {
	if err := ctx.Err(); err != nil {
		return clarafs.Entry{}, err
	}
	clean, err := clarafs.Clean(value.String())
	if err != nil {
		return clarafs.Entry{}, err
	}
	if clean == "/" {
		return clarafs.Entry{}, errs.New(errs.CodeConflict, "root is a directory")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.dirs[clean]; ok {
		return clarafs.Entry{}, errs.New(errs.CodeConflict, "path is a directory")
	}
	if _, ok := m.files[clean]; ok && !options.Overwrite {
		return clarafs.Entry{}, errs.ErrConflict
	}
	if options.CreateDirs {
		if err := m.mkdirParents(clean.Dir()); err != nil {
			return clarafs.Entry{}, err
		}
	} else if _, ok := m.dirs[clean.Dir()]; !ok {
		return clarafs.Entry{}, errs.ErrNotFound
	}
	mode := options.Mode
	if mode == 0 {
		mode = 0o644
	}
	item := node{data: append([]byte(nil), data...), mode: mode, modTime: time.Now()}
	m.files[clean] = item
	return entry(clean, item, false), nil
}

func (m *FS) mkdirParents(value clarafs.Path) error {
	if value == "/" || value == "." {
		return nil
	}
	if _, ok := m.dirs[value]; ok {
		return nil
	}
	if err := m.mkdirParents(value.Dir()); err != nil {
		return err
	}
	m.dirs[value] = node{mode: fs.ModeDir | 0o755, modTime: time.Now()}
	return nil
}

func (m *FS) MkdirAll(ctx context.Context, value clarafs.Path, mode fs.FileMode) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	clean, err := clarafs.Clean(value.String())
	if err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.files[clean]; ok {
		return errs.ErrConflict
	}
	if mode == 0 {
		mode = 0o755
	}
	if err := m.mkdirParents(clean); err != nil {
		return err
	}
	m.dirs[clean] = node{mode: fs.ModeDir | mode, modTime: time.Now()}
	return nil
}

func (m *FS) Remove(ctx context.Context, value clarafs.Path, options clarafs.RemoveOptions) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := clarafs.EnsureNotRoot(value); err != nil {
		return err
	}
	clean, err := clarafs.Clean(value.String())
	if err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.files[clean]; ok {
		delete(m.files, clean)
		return nil
	}
	if _, ok := m.dirs[clean]; !ok {
		return errs.ErrNotFound
	}
	children, err := m.childrenLocked(clean)
	if err != nil {
		return err
	}
	if len(children) > 0 && !options.Recursive {
		return errs.New(errs.CodeConflict, "directory is not empty")
	}
	for _, child := range children {
		delete(m.dirs, child)
		delete(m.files, child)
	}
	delete(m.dirs, clean)
	return nil
}

func (m *FS) childrenLocked(root clarafs.Path) ([]clarafs.Path, error) {
	var out []clarafs.Path
	prefix := string(root)
	if prefix != "/" {
		prefix += "/"
	}
	for path := range m.dirs {
		if path != root && strings.HasPrefix(string(path), prefix) {
			out = append(out, path)
		}
	}
	for path := range m.files {
		if strings.HasPrefix(string(path), prefix) {
			out = append(out, path)
		}
	}
	return out, nil
}

func (m *FS) Rename(ctx context.Context, from, to clarafs.Path) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := clarafs.EnsureNotRoot(from); err != nil {
		return err
	}
	source, err := clarafs.Clean(from.String())
	if err != nil {
		return err
	}
	target, err := clarafs.Clean(to.String())
	if err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.files[target]; ok {
		return errs.ErrConflict
	}
	if _, ok := m.dirs[target]; ok {
		return errs.ErrConflict
	}
	if err := m.mkdirParents(target.Dir()); err != nil {
		return err
	}
	if item, ok := m.files[source]; ok {
		delete(m.files, source)
		m.files[target] = item
		return nil
	}
	if _, ok := m.dirs[source]; !ok {
		return errs.ErrNotFound
	}
	children, _ := m.childrenLocked(source)
	m.dirs[target] = m.dirs[source]
	delete(m.dirs, source)
	for _, child := range children {
		suffix := strings.TrimPrefix(string(child), string(source))
		next, _ := clarafs.Clean(string(target) + suffix)
		if item, ok := m.dirs[child]; ok {
			delete(m.dirs, child)
			m.dirs[next] = item
		}
		if item, ok := m.files[child]; ok {
			delete(m.files, child)
			m.files[next] = item
		}
	}
	return nil
}

var _ clarafs.FS = (*FS)(nil)
