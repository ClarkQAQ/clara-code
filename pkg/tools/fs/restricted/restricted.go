// Package restricted adds path allowlists around an FS.
package restricted

import (
	"context"
	"io"
	"io/fs"
	"strings"

	clarafs "clara-code/pkg/tools/fs"
	"clara-code/pkg/util/errs"
)

// Policy limits readable and writable logical paths.
type Policy struct {
	Read  []string
	Write []string
	Deny  []string
}

// FS wraps an FS with a path policy.
type FS struct {
	base   clarafs.FS
	policy Policy
}

// New wraps base with policy.
func New(base clarafs.FS, policy Policy) *FS {
	return &FS{base: base, policy: policy}
}

func (r *FS) check(value clarafs.Path, write bool) error {
	path := value.String()
	for _, denied := range r.policy.Deny {
		if under(path, denied) {
			return errs.ErrForbidden
		}
	}
	allowed := r.policy.Read
	if write {
		allowed = r.policy.Write
	}
	if len(allowed) == 0 {
		if write {
			return errs.ErrForbidden
		}
		return nil
	}
	for _, prefix := range allowed {
		if under(path, prefix) {
			return nil
		}
	}
	return errs.ErrForbidden
}

func under(path, prefix string) bool {
	prefix = strings.TrimRight(prefix, "/")
	return prefix == "" || path == prefix || strings.HasPrefix(path, prefix+"/")
}

func (r *FS) Stat(ctx context.Context, path clarafs.Path) (clarafs.Entry, error) {
	if err := r.check(path, false); err != nil {
		return clarafs.Entry{}, err
	}
	return r.base.Stat(ctx, path)
}

func (r *FS) ReadDir(ctx context.Context, path clarafs.Path) ([]clarafs.Entry, error) {
	if err := r.check(path, false); err != nil {
		return nil, err
	}
	return r.base.ReadDir(ctx, path)
}

func (r *FS) Open(ctx context.Context, path clarafs.Path) (io.ReadCloser, error) {
	if err := r.check(path, false); err != nil {
		return nil, err
	}
	return r.base.Open(ctx, path)
}

func (r *FS) ReadFile(ctx context.Context, path clarafs.Path, max int64) ([]byte, error) {
	if err := r.check(path, false); err != nil {
		return nil, err
	}
	return r.base.ReadFile(ctx, path, max)
}

func (r *FS) ReadAt(ctx context.Context, path clarafs.Path, offset, limit int64) ([]byte, error) {
	if err := r.check(path, false); err != nil {
		return nil, err
	}
	return r.base.ReadAt(ctx, path, offset, limit)
}

func (r *FS) WriteFile(ctx context.Context, path clarafs.Path, data []byte, options clarafs.WriteOptions) (clarafs.Entry, error) {
	if err := r.check(path, true); err != nil {
		return clarafs.Entry{}, err
	}
	return r.base.WriteFile(ctx, path, data, options)
}

func (r *FS) MkdirAll(ctx context.Context, path clarafs.Path, mode fs.FileMode) error {
	if err := r.check(path, true); err != nil {
		return err
	}
	return r.base.MkdirAll(ctx, path, mode)
}

func (r *FS) Remove(ctx context.Context, path clarafs.Path, options clarafs.RemoveOptions) error {
	if err := r.check(path, true); err != nil {
		return err
	}
	return r.base.Remove(ctx, path, options)
}

func (r *FS) Rename(ctx context.Context, from, to clarafs.Path) error {
	if err := r.check(from, true); err != nil {
		return err
	}
	if err := r.check(to, true); err != nil {
		return err
	}
	return r.base.Rename(ctx, from, to)
}

var _ clarafs.FS = (*FS)(nil)
