package tool

import (
	"context"
	"fmt"
	"strings"
)

func (s *Files) List() Tool {
	type input struct {
		Path  string `json:"path,omitempty" description:"directory path inside the workspace. / is the workspace root"`
		Limit int    `json:"limit,omitempty" description:"maximum entries, default 200"`
	}
	return Func(Definition{
		Name: "list", Description: "List one directory with names, types, and sizes.",
		Effects: []Effect{EffectRead}, ParallelSafe: true,
	}, func(ctx context.Context, in input) (Result, error) {
		target, err := s.cleanPath(in.Path)
		if err != nil {
			return ErrorResult(err.Error()), nil
		}
		entries, err := s.service.FS.ReadDir(ctx, target)
		if err != nil {
			return ErrorResult(err.Error()), nil
		}
		limit := in.Limit
		if limit <= 0 {
			limit = 200
		}
		var builder strings.Builder
		for index, entry := range entries {
			if index >= limit {
				fmt.Fprintf(&builder, "… %d more entries\n", len(entries)-limit)
				break
			}
			kind := "file"
			if entry.IsDir {
				kind = "dir"
			}
			fmt.Fprintf(&builder, "%s\t%s\t%d\n", entry.Path, kind, entry.Size)
		}
		if builder.Len() == 0 {
			return TextResult("Directory is empty."), nil
		}
		return TextResult(strings.TrimSpace(builder.String())), nil
	})
}
