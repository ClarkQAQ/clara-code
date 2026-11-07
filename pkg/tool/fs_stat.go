package tool

import (
	clarafs "clara-code/pkg/tool/fs"
	"context"
	"fmt"
	"time"
)

func (s *Files) Stat() Tool {
	type input struct {
		Path string `json:"path" description:"file or directory path"`
	}
	return Func(Definition{
		Name: "stat", Description: "Show metadata for one path. modified is an RFC3339 timestamp with a time-zone offset or Z, preserving fractional seconds.",
		Effects: []Effect{EffectRead}, ParallelSafe: true,
	}, func(ctx context.Context, in input) (Result, error) {
		target, err := s.cleanPath(in.Path)
		if err != nil {
			return ErrorResult(err.Error()), nil
		}
		entry, err := s.service.FS.Stat(ctx, target)
		if err != nil {
			return ErrorResult(err.Error()), nil
		}
		return TextResult(fmt.Sprintf("%s\nsize: %d\nmode: %s\nmodified: %s\nkind: %s",
			entry.Path, entry.Size, entry.Mode, entry.ModTime.Format(time.RFC3339Nano), kind(entry))), nil
	})
}

func kind(entry clarafs.Entry) string {
	if entry.IsDir {
		return "directory"
	}
	return "file"
}
