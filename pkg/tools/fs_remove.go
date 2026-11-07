package tools

import (
	clarafs "clara-code/pkg/tools/fs"
	"context"
)

func (s *Files) Remove() Tool {
	type input struct {
		Path      string `json:"path" description:"file or directory path"`
		Recursive bool   `json:"recursive,omitempty" description:"allow recursive directory removal"`
	}
	return Func(Definition{
		Name: "remove", Description: "Delete a file or directory.",
		Effects: []Effect{EffectDelete},
	}, func(ctx context.Context, in input) (Result, error) {
		target, err := s.cleanPath(in.Path)
		if err != nil {
			return ErrorResult(err.Error()), nil
		}
		if err := s.service.FS.Remove(ctx, target, clarafs.RemoveOptions{Recursive: in.Recursive}); err != nil {
			return ErrorResult(err.Error()), nil
		}
		return TextResult("Removed " + target.String()), nil
	})
}
