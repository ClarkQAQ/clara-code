package fs

import (
	"context"
	"errors"
	"io"
	"io/fs"

	"clara-code/pkg/util/errs"
)

// At adapts storage to a logical working directory.
// Mutations are available only when storage implements Writer.
func At(storage Reader, cwd Path) FS { return scoped{storage, cwd} }

type scoped struct {
	storage Reader
	cwd     Path
}

func (s scoped) Stat(ctx context.Context, value Path) (Entry, error) {
	p, err := Resolve(s.cwd, value.String())
	if err != nil {
		return Entry{}, err
	}
	return s.storage.Stat(ctx, p)
}

func (s scoped) ReadDir(ctx context.Context, value Path) ([]Entry, error) {
	p, err := Resolve(s.cwd, value.String())
	if err != nil {
		return nil, err
	}
	return s.storage.ReadDir(ctx, p)
}

func (s scoped) Open(ctx context.Context, value Path) (io.ReadCloser, error) {
	p, err := Resolve(s.cwd, value.String())
	if err != nil {
		return nil, err
	}
	return s.storage.Open(ctx, p)
}

func (s scoped) ReadFile(ctx context.Context, value Path, limit int64) ([]byte, error) {
	p, err := Resolve(s.cwd, value.String())
	if err != nil {
		return nil, err
	}
	if reader, ok := s.storage.(interface {
		ReadFile(context.Context, Path, int64) ([]byte, error)
	}); ok {
		return reader.ReadFile(ctx, p, limit)
	}
	file, err := s.storage.Open(ctx, p)
	if err != nil {
		return nil, err
	}
	if limit <= 0 || limit > 64<<20 {
		limit = 64 << 20
	}
	data, err := io.ReadAll(io.LimitReader(file, limit))
	return data, errors.Join(err, file.Close())
}

func (s scoped) ReadAt(ctx context.Context, value Path, offset, limit int64) ([]byte, error) {
	p, err := Resolve(s.cwd, value.String())
	if err != nil {
		return nil, err
	}
	if reader, ok := s.storage.(interface {
		ReadAt(context.Context, Path, int64, int64) ([]byte, error)
	}); ok {
		return reader.ReadAt(ctx, p, offset, limit)
	}
	file, err := s.storage.Open(ctx, p)
	if err != nil {
		return nil, err
	}
	if offset < 0 || limit < 0 || limit > 64<<20 {
		return nil, errors.Join(errs.ErrUnsupported, file.Close())
	}
	if seeker, ok := file.(io.Seeker); ok {
		_, err = seeker.Seek(offset, io.SeekStart)
	} else if offset <= 64<<20 {
		_, err = io.CopyN(io.Discard, file, offset)
		if errors.Is(err, io.EOF) {
			return nil, file.Close()
		}
	} else {
		err = errs.ErrUnsupported
	}
	if err != nil {
		return nil, errors.Join(err, file.Close())
	}
	data, err := io.ReadAll(io.LimitReader(file, limit))
	return data, errors.Join(err, file.Close())
}

func (s scoped) WriteFile(ctx context.Context, value Path, data []byte, options WriteOptions) (Entry, error) {
	p, err := Resolve(s.cwd, value.String())
	if err != nil {
		return Entry{}, err
	}
	writer, ok := s.storage.(Writer)
	if !ok {
		return Entry{}, errs.ErrUnsupported
	}
	return writer.WriteFile(ctx, p, data, options)
}

func (s scoped) MkdirAll(ctx context.Context, value Path, mode fs.FileMode) error {
	p, err := Resolve(s.cwd, value.String())
	if err != nil {
		return err
	}
	writer, ok := s.storage.(Writer)
	if !ok {
		return errs.ErrUnsupported
	}
	return writer.MkdirAll(ctx, p, mode)
}

func (s scoped) Remove(ctx context.Context, value Path, options RemoveOptions) error {
	p, err := Resolve(s.cwd, value.String())
	if err != nil {
		return err
	}
	writer, ok := s.storage.(Writer)
	if !ok {
		return errs.ErrUnsupported
	}
	return writer.Remove(ctx, p, options)
}

func (s scoped) Rename(ctx context.Context, value Path, to Path) error {
	p, err := Resolve(s.cwd, value.String())
	if err != nil {
		return err
	}
	writer, ok := s.storage.(Writer)
	if !ok {
		return errs.ErrUnsupported
	}
	target, err := Resolve(s.cwd, to.String())
	if err != nil {
		return err
	}
	return writer.Rename(ctx, p, target)
}

// ResolvePath returns the canonical logical path for a storage operation.
func ResolvePath(storage Reader, value string) (Path, error) {
	if resolver, ok := storage.(interface{ ResolvePath(string) (Path, error) }); ok {
		return resolver.ResolvePath(value)
	}
	return Clean(value)
}

func (s scoped) ResolvePath(value string) (Path, error) { return Resolve(s.cwd, value) }
