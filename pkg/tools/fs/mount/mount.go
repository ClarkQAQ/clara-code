// Package mount composes several filesystems under one logical namespace.
package mount

import (
	"context"
	"io"
	"io/fs"
	"sort"
	"strings"

	clarafs "clara-code/pkg/tools/fs"
	"clara-code/pkg/util/errs"
)

type mounted struct {
	prefix clarafs.Path
	fs     clarafs.FS
}

// FS dispatches each operation to the longest matching mount.
type FS struct {
	mounts []mounted
}

// New creates a composite filesystem.
func New() *FS { return &FS{} }

// Mount registers storage at prefix.
func (m *FS) Mount(prefix clarafs.Path, storage clarafs.FS) error {
	clean, err := clarafs.Clean(prefix.String())
	if err != nil {
		return err
	}
	for _, existing := range m.mounts {
		if existing.prefix == clean {
			return errs.ErrConflict
		}
	}
	m.mounts = append(m.mounts, mounted{prefix: clean, fs: storage})
	sort.SliceStable(m.mounts, func(i, j int) bool {
		return len(m.mounts[i].prefix) > len(m.mounts[j].prefix)
	})
	return nil
}

func (m *FS) resolve(value clarafs.Path) (clarafs.FS, clarafs.Path, error) {
	clean, err := clarafs.Clean(value.String())
	if err != nil {
		return nil, "", err
	}
	for _, item := range m.mounts {
		if clean == item.prefix || strings.HasPrefix(clean.String(), item.prefix.String()+"/") {
			relative := strings.TrimPrefix(clean.String(), item.prefix.String())
			if relative == "" {
				relative = "/"
			}
			target, err := clarafs.Clean(relative)
			return item.fs, target, err
		}
	}
	return nil, "", errs.ErrNotFound
}

func (m *FS) Stat(ctx context.Context, path clarafs.Path) (clarafs.Entry, error) {
	storage, relative, err := m.resolve(path)
	if err != nil {
		return clarafs.Entry{}, err
	}
	entry, err := storage.Stat(ctx, relative)
	if err != nil {
		return clarafs.Entry{}, err
	}
	entry.Path, _ = clarafs.Clean(path.String())
	return entry, nil
}

func (m *FS) ReadDir(ctx context.Context, path clarafs.Path) ([]clarafs.Entry, error) {
	storage, relative, err := m.resolve(path)
	if err != nil {
		return nil, err
	}
	entries, err := storage.ReadDir(ctx, relative)
	if err != nil {
		return nil, err
	}
	for index := range entries {
		entries[index].Path, _ = clarafs.Join(path, entries[index].Name)
	}
	return entries, nil
}

func (m *FS) Open(ctx context.Context, path clarafs.Path) (io.ReadCloser, error) {
	storage, relative, err := m.resolve(path)
	if err != nil {
		return nil, err
	}
	return storage.Open(ctx, relative)
}

func (m *FS) ReadFile(ctx context.Context, path clarafs.Path, max int64) ([]byte, error) {
	storage, relative, err := m.resolve(path)
	if err != nil {
		return nil, err
	}
	return storage.ReadFile(ctx, relative, max)
}

func (m *FS) ReadAt(ctx context.Context, path clarafs.Path, offset, limit int64) ([]byte, error) {
	storage, relative, err := m.resolve(path)
	if err != nil {
		return nil, err
	}
	return storage.ReadAt(ctx, relative, offset, limit)
}

func (m *FS) WriteFile(ctx context.Context, path clarafs.Path, data []byte, options clarafs.WriteOptions) (clarafs.Entry, error) {
	storage, relative, err := m.resolve(path)
	if err != nil {
		return clarafs.Entry{}, err
	}
	entry, err := storage.WriteFile(ctx, relative, data, options)
	if err != nil {
		return clarafs.Entry{}, err
	}
	entry.Path, _ = clarafs.Clean(path.String())
	return entry, nil
}

func (m *FS) MkdirAll(ctx context.Context, path clarafs.Path, mode fs.FileMode) error {
	storage, relative, err := m.resolve(path)
	if err != nil {
		return err
	}
	return storage.MkdirAll(ctx, relative, mode)
}

func (m *FS) Remove(ctx context.Context, path clarafs.Path, options clarafs.RemoveOptions) error {
	storage, relative, err := m.resolve(path)
	if err != nil {
		return err
	}
	return storage.Remove(ctx, relative, options)
}

func (m *FS) Rename(ctx context.Context, from, to clarafs.Path) error {
	fromFS, fromPath, err := m.resolve(from)
	if err != nil {
		return err
	}
	toFS, toPath, err := m.resolve(to)
	if err != nil {
		return err
	}
	if fromFS != toFS {
		return errs.New(errs.CodeUnsupported, "cross-mount rename is not supported")
	}
	return fromFS.Rename(ctx, fromPath, toPath)
}

var _ clarafs.FS = (*FS)(nil)
