// Package virtual provides read-only generated files over an FS.
package virtual

import (
	"context"
	"io/fs"

	clarafs "clara-code/pkg/tool/fs"
	"clara-code/pkg/util/errs"
)

// FS makes another filesystem read-only.
type FS struct {
	clarafs.FS
}

// New wraps storage as read-only.
func New(storage clarafs.FS) *FS { return &FS{FS: storage} }

func (v *FS) WriteFile(context.Context, clarafs.Path, []byte, clarafs.WriteOptions) (clarafs.Entry, error) {
	return clarafs.Entry{}, errs.ErrForbidden
}

func (v *FS) MkdirAll(context.Context, clarafs.Path, fs.FileMode) error {
	return errs.ErrForbidden
}

func (v *FS) Remove(context.Context, clarafs.Path, clarafs.RemoveOptions) error {
	return errs.ErrForbidden
}

func (v *FS) Rename(context.Context, clarafs.Path, clarafs.Path) error {
	return errs.ErrForbidden
}

var _ clarafs.FS = (*FS)(nil)
