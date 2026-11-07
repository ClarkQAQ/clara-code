// Package host provides native filesystem access with the current user's OS permissions.
// Unlike local, it does not confine access to a workspace root.
package host

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	corefs "clara-code/pkg/tool/fs"
	"clara-code/pkg/util/errs"
)

type FS struct{ cwd string }

func New(cwd string) (*FS, error) {
	value, err := filepath.Abs(cwd)
	if err != nil {
		return nil, err
	}
	value, err = filepath.EvalSymlinks(value)
	if err != nil {
		return nil, mapError(err)
	}
	info, err := os.Stat(value)
	if err != nil {
		return nil, mapError(err)
	}
	if !info.IsDir() {
		return nil, errs.New(errs.CodeInvalidArgument, "working directory is not a directory")
	}
	return &FS{cwd: value}, nil
}

func (f *FS) ResolvePath(value string) (corefs.Path, error) {
	if strings.IndexByte(value, 0) >= 0 {
		return "", errs.New(errs.CodeInvalidArgument, "invalid path")
	}
	if value == "~" || strings.HasPrefix(value, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		if value == "~" {
			value = home
		} else {
			value = filepath.Join(home, strings.TrimPrefix(value, "~/"))
		}
	}
	if !filepath.IsAbs(value) {
		value = filepath.Join(f.cwd, value)
	}
	return corefs.Path(filepath.ToSlash(filepath.Clean(value))), nil
}

func (f *FS) native(ctx context.Context, p corefs.Path) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	resolved, err := f.ResolvePath(p.String())
	return filepath.FromSlash(resolved.String()), err
}

// RealPath resolves existing parents as well as existing file symlinks.
func (f *FS) RealPath(ctx context.Context, p corefs.Path) (corefs.Path, error) {
	value, err := f.native(ctx, p)
	if err != nil {
		return "", err
	}
	var missing []string
	for {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		resolved, err := filepath.EvalSymlinks(value)
		if err == nil {
			for i := len(missing) - 1; i >= 0; i-- {
				resolved = filepath.Join(resolved, missing[i])
			}
			return corefs.Path(filepath.ToSlash(resolved)), nil
		}
		if !errors.Is(err, os.ErrNotExist) || filepath.Dir(value) == value {
			return "", mapError(err)
		}
		missing = append(missing, filepath.Base(value))
		value = filepath.Dir(value)
	}
}

func (f *FS) Stat(ctx context.Context, p corefs.Path) (corefs.Entry, error) {
	value, err := f.native(ctx, p)
	if err != nil {
		return corefs.Entry{}, err
	}
	info, err := os.Lstat(value)
	if err != nil {
		return corefs.Entry{}, mapError(err)
	}
	return corefs.Entry{Path: corefs.Path(filepath.ToSlash(value)), Name: info.Name(), Size: info.Size(), Mode: info.Mode(), ModTime: info.ModTime(), IsDir: info.IsDir(), IsLink: info.Mode()&os.ModeSymlink != 0}, nil
}

func (f *FS) ReadDir(ctx context.Context, p corefs.Path) ([]corefs.Entry, error) {
	value, err := f.native(ctx, p)
	if err != nil {
		return nil, err
	}
	dir, err := os.Open(value)
	if err != nil {
		return nil, mapError(err)
	}
	entries, readErr := dir.ReadDir(10001)
	if errors.Is(readErr, io.EOF) {
		readErr = nil
	}
	if err := errors.Join(readErr, dir.Close()); err != nil {
		return nil, mapError(err)
	}
	if len(entries) > 10000 {
		return nil, errs.New(errs.CodeLimit, "directory has more than 10000 entries")
	}
	out := make([]corefs.Entry, 0, len(entries))
	for _, entry := range entries {
		info, err := f.Stat(ctx, corefs.Path(filepath.Join(value, entry.Name())))
		if err != nil {
			return nil, err
		}
		out = append(out, info)
	}
	return out, nil
}

func (f *FS) Open(ctx context.Context, p corefs.Path) (io.ReadCloser, error) {
	value, err := f.native(ctx, p)
	if err != nil {
		return nil, err
	}
	file, err := os.Open(value)
	return file, mapError(err)
}

func (f *FS) ReadFile(ctx context.Context, p corefs.Path, limit int64) ([]byte, error) {
	file, err := f.Open(ctx, p)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	if limit <= 0 || limit > 64<<20 {
		limit = 64 << 20
	}
	data, err := io.ReadAll(io.LimitReader(file, limit))
	return data, mapError(err)
}

func (f *FS) ReadAt(ctx context.Context, p corefs.Path, offset, limit int64) ([]byte, error) {
	if offset < 0 || limit < 0 || limit > 64<<20 {
		return nil, errs.New(errs.CodeInvalidArgument, "invalid read range")
	}
	value, err := f.native(ctx, p)
	if err != nil {
		return nil, err
	}
	file, err := os.Open(value)
	if err != nil {
		return nil, mapError(err)
	}
	defer file.Close()
	data, err := io.ReadAll(io.NewSectionReader(file, offset, limit))
	return data, mapError(err)
}

func (f *FS) WriteFile(ctx context.Context, p corefs.Path, data []byte, options corefs.WriteOptions) (corefs.Entry, error) {
	value, err := f.native(ctx, p)
	if err != nil {
		return corefs.Entry{}, err
	}
	mode := options.Mode.Perm()
	if mode == 0 {
		mode = 0o644
	}
	if options.Overwrite {
		info, err := os.Lstat(value)
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return corefs.Entry{}, mapError(err)
		}
		if err == nil && info.Mode()&os.ModeSymlink != 0 {
			target, err := filepath.EvalSymlinks(value)
			if err != nil {
				return corefs.Entry{}, mapError(err)
			}
			actual, err := os.Stat(target)
			if err != nil {
				return corefs.Entry{}, mapError(err)
			}
			value, mode = target, actual.Mode().Perm()
		}
	}
	if options.CreateDirs {
		if err := os.MkdirAll(filepath.Dir(value), 0o755); err != nil {
			return corefs.Entry{}, mapError(err)
		}
	}
	file, err := os.CreateTemp(filepath.Dir(value), ".clara-core-write-")
	if err != nil {
		return corefs.Entry{}, mapError(err)
	}
	temp := file.Name()
	defer os.Remove(temp)
	writeErr := file.Chmod(mode)
	if writeErr == nil {
		_, writeErr = file.Write(data)
	}
	if writeErr == nil {
		writeErr = file.Sync()
	}
	if err := errors.Join(writeErr, file.Close(), ctx.Err()); err != nil {
		return corefs.Entry{}, mapError(err)
	}
	if options.Overwrite {
		err = os.Rename(temp, value)
	} else {
		err = os.Link(temp, value)
	}
	if err != nil {
		return corefs.Entry{}, mapError(err)
	}
	return f.Stat(ctx, p)
}

func (f *FS) MkdirAll(ctx context.Context, p corefs.Path, mode fs.FileMode) error {
	value, err := f.native(ctx, p)
	if err != nil {
		return err
	}
	if mode == 0 {
		mode = 0o755
	}
	return mapError(os.MkdirAll(value, mode))
}

func (f *FS) Remove(ctx context.Context, p corefs.Path, options corefs.RemoveOptions) error {
	value, err := f.native(ctx, p)
	if err != nil {
		return err
	}
	if err := corefs.EnsureNotRoot(corefs.Path(filepath.ToSlash(value))); err != nil {
		return err
	}
	if options.Recursive {
		return mapError(os.RemoveAll(value))
	}
	return mapError(os.Remove(value))
}

func (f *FS) Rename(ctx context.Context, from, to corefs.Path) error {
	source, err := f.native(ctx, from)
	if err != nil {
		return err
	}
	target, err := f.native(ctx, to)
	if err != nil {
		return err
	}
	for _, value := range []string{source, target} {
		if err := corefs.EnsureNotRoot(corefs.Path(filepath.ToSlash(value))); err != nil {
			return err
		}
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return mapError(err)
	}
	return mapError(os.Rename(source, target))
}

func mapError(err error) error {
	switch {
	case errors.Is(err, os.ErrNotExist):
		return errs.Wrap(errs.CodeNotFound, "path not found", err)
	case errors.Is(err, os.ErrPermission):
		return errs.Wrap(errs.CodeForbidden, "permission denied", err)
	case errors.Is(err, os.ErrExist):
		return errs.Wrap(errs.CodeConflict, "path already exists", err)
	default:
		return err
	}
}

var _ corefs.FS = (*FS)(nil)
