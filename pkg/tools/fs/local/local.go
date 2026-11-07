// Package local provides a root-confined host filesystem.
package local

import (
	"context"
	"crypto/rand"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	clarafs "clara-code/pkg/tools/fs"
	"clara-code/pkg/util/errs"
)

// FS maps logical Clara Core paths below one host directory.
type FS struct {
	root string
}

// New creates a local filesystem rooted at root.
func New(root string) (*FS, error) {
	resolved, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(resolved, 0o755); err != nil {
		return nil, err
	}
	realRoot, err := filepath.EvalSymlinks(resolved)
	if err != nil {
		return nil, err
	}
	return &FS{root: realRoot}, nil
}

// Root returns the absolute host root.
func (l *FS) Root() string { return l.root }

func (l *FS) openRoot(ctx context.Context, value clarafs.Path) (*os.Root, string, error) {
	if err := ctx.Err(); err != nil {
		return nil, "", err
	}
	clean, err := clarafs.Clean(value.String())
	if err != nil {
		return nil, "", err
	}
	name := filepath.FromSlash(strings.TrimPrefix(clean.String(), "/"))
	if name == "" {
		name = "."
	}
	root, err := os.OpenRoot(l.root)
	return root, name, mapError(err)
}

// ResolveDir maps an existing logical directory to its host path.
func (l *FS) ResolveDir(ctx context.Context, value clarafs.Path) (string, error) {
	root, name, err := l.openRoot(ctx, value)
	if err != nil {
		return "", err
	}
	defer root.Close()
	info, err := root.Stat(name)
	if err != nil {
		return "", mapError(err)
	}
	if !info.IsDir() {
		return "", errs.New(errs.CodeInvalidArgument, "working directory is not a directory")
	}
	target, err := filepath.EvalSymlinks(filepath.Join(l.root, name))
	if err != nil {
		return "", mapError(err)
	}
	if !within(l.root, target) {
		return "", errs.ErrForbidden
	}
	return target, nil
}

func within(root, target string) bool {
	relative, err := filepath.Rel(root, target)
	if err != nil {
		return false
	}
	return relative == "." || (relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)))
}

func (l *FS) Stat(ctx context.Context, value clarafs.Path) (clarafs.Entry, error) {
	if err := ctx.Err(); err != nil {
		return clarafs.Entry{}, err
	}
	root, target, err := l.openRoot(ctx, value)
	if err != nil {
		return clarafs.Entry{}, err
	}
	defer root.Close()
	info, err := root.Lstat(target)
	if err != nil {
		return clarafs.Entry{}, mapError(err)
	}
	clean, _ := clarafs.Clean(value.String())
	return clarafs.Entry{
		Path: clean, Name: clean.Base(), Size: info.Size(), Mode: info.Mode(),
		ModTime: info.ModTime(), IsDir: info.IsDir(), IsLink: info.Mode()&os.ModeSymlink != 0,
	}, nil
}

func (l *FS) ReadDir(ctx context.Context, value clarafs.Path) ([]clarafs.Entry, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	root, target, err := l.openRoot(ctx, value)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	dir, err := root.Open(target)
	if err != nil {
		return nil, mapError(err)
	}
	entries, readErr := dir.ReadDir(10001)
	if errors.Is(readErr, io.EOF) {
		readErr = nil
	}
	err = errors.Join(readErr, dir.Close())
	if len(entries) > 10000 {
		return nil, errs.New(errs.CodeLimit, "directory has more than 10000 entries")
	}
	if err != nil {
		return nil, mapError(err)
	}
	out := make([]clarafs.Entry, 0, len(entries))
	for _, entry := range entries {
		info, err := root.Lstat(filepath.Join(target, entry.Name()))
		if err != nil {
			return nil, mapError(err)
		}
		child, err := clarafs.Join(value, entry.Name())
		if err != nil {
			continue
		}
		out = append(out, clarafs.Entry{
			Path: child, Name: entry.Name(), Size: info.Size(), Mode: info.Mode(),
			ModTime: info.ModTime(), IsDir: entry.IsDir(), IsLink: entry.Type()&os.ModeSymlink != 0,
		})
	}
	return out, nil
}

func (l *FS) Open(ctx context.Context, value clarafs.Path) (io.ReadCloser, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	root, target, err := l.openRoot(ctx, value)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	file, err := root.Open(target)
	if err != nil {
		return nil, mapError(err)
	}
	return file, nil
}

func (l *FS) ReadFile(ctx context.Context, value clarafs.Path, max int64) ([]byte, error) {
	file, err := l.Open(ctx, value)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	if max <= 0 || max > 64<<20 {
		max = 64 << 20
	}
	data, err := io.ReadAll(io.LimitReader(file, max+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > max {
		data = data[:max]
	}
	return data, nil
}

func (l *FS) ReadAt(ctx context.Context, value clarafs.Path, offset, limit int64) ([]byte, error) {
	if offset < 0 || limit < 0 || limit > 64<<20 {
		return nil, errs.New(errs.CodeInvalidArgument, "offset and limit must not be negative")
	}
	file, err := l.Open(ctx, value)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	reader := io.NewSectionReader(file.(io.ReaderAt), offset, limit)
	return io.ReadAll(reader)
}

func (l *FS) WriteFile(ctx context.Context, value clarafs.Path, data []byte, options clarafs.WriteOptions) (clarafs.Entry, error) {
	if err := ctx.Err(); err != nil {
		return clarafs.Entry{}, err
	}
	root, target, err := l.openRoot(ctx, value)
	if err != nil {
		return clarafs.Entry{}, err
	}
	defer root.Close()
	parent := filepath.Dir(target)
	if options.CreateDirs {
		if err := root.MkdirAll(parent, 0o755); err != nil {
			return clarafs.Entry{}, mapError(err)
		}
	}
	mode := options.Mode
	if mode == 0 {
		mode = 0o644
	}
	temp := filepath.Join(parent, ".clara-core-write-"+rand.Text())
	file, err := root.OpenFile(temp, os.O_CREATE|os.O_EXCL|os.O_WRONLY, mode)
	if err != nil {
		return clarafs.Entry{}, mapError(err)
	}
	defer root.Remove(temp)
	_, writeErr := file.Write(data)
	if writeErr == nil {
		writeErr = file.Sync()
	}
	if err := errors.Join(writeErr, file.Close()); err != nil {
		return clarafs.Entry{}, err
	}
	if options.Overwrite {
		err = root.Rename(temp, target)
	} else {
		err = root.Link(temp, target)
	}
	if err != nil {
		return clarafs.Entry{}, mapError(err)
	}
	return l.Stat(ctx, value)
}

func (l *FS) MkdirAll(ctx context.Context, value clarafs.Path, mode fs.FileMode) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	root, target, err := l.openRoot(ctx, value)
	if err != nil {
		return err
	}
	if mode == 0 {
		mode = 0o755
	}
	defer root.Close()
	return mapError(root.MkdirAll(target, mode))
}

func (l *FS) Remove(ctx context.Context, value clarafs.Path, options clarafs.RemoveOptions) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := clarafs.EnsureNotRoot(value); err != nil {
		return err
	}
	root, target, err := l.openRoot(ctx, value)
	if err != nil {
		return err
	}
	defer root.Close()
	if options.Recursive {
		return mapError(root.RemoveAll(target))
	}
	return mapError(root.Remove(target))
}

func (l *FS) Rename(ctx context.Context, from, to clarafs.Path) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := clarafs.EnsureNotRoot(from); err != nil {
		return err
	}
	root, source, err := l.openRoot(ctx, from)
	if err != nil {
		return err
	}
	defer root.Close()
	clean, err := clarafs.Clean(to.String())
	if err != nil {
		return err
	}
	if err := clarafs.EnsureNotRoot(clean); err != nil {
		return err
	}
	target := filepath.FromSlash(strings.TrimPrefix(clean.String(), "/"))
	if err := root.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return mapError(err)
	}
	return mapError(root.Rename(source, target))
}

func mapError(err error) error {
	if err != nil && strings.Contains(err.Error(), "escapes from parent") {
		return errs.Wrap(errs.CodeForbidden, "path escapes the filesystem root", err)
	}
	if errors.Is(err, os.ErrNotExist) {
		return errs.Wrap(errs.CodeNotFound, "path not found", err)
	}
	if errors.Is(err, os.ErrPermission) {
		return errs.Wrap(errs.CodeForbidden, "permission denied", err)
	}
	if errors.Is(err, fs.ErrExist) {
		return errs.Wrap(errs.CodeConflict, "path already exists", err)
	}
	return err
}

var _ clarafs.FS = (*FS)(nil)

// Touch updates a file's modification time.
func (l *FS) Touch(ctx context.Context, value clarafs.Path) error {
	root, target, err := l.openRoot(ctx, value)
	if err != nil {
		return err
	}
	defer root.Close()
	now := time.Now()
	return mapError(root.Chtimes(target, now, now))
}
