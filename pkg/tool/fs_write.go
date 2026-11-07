package tool

import (
	"clara-code/pkg/util/errs"
	"context"
	"fmt"
	"io/fs"
)

func (s *Files) Write() Tool {
	type input struct {
		Path    string `json:"path" description:"file path"`
		Content string `json:"content" description:"complete file content"`
	}
	return Func(Definition{
		Name: "write", Description: "Create a new file. Use edit to modify an existing text file.",
		Effects: []Effect{EffectWrite},
	}, func(ctx context.Context, in input) (Result, error) {
		target, err := s.cleanPath(in.Path)
		if err != nil {
			return ErrorResult(err.Error()), nil
		}
		if _, err := s.rulesFor(ctx, target, false); err != nil {
			return ErrorResult(err.Error()), nil
		}
		entry, err := s.service.Create(ctx, target, []byte(in.Content), fs.FileMode(0o644))
		if errs.IsCode(err, errs.CodeConflict) {
			return ErrorResult("The file exists. Read it with anchored=true, then use edit."), nil
		}
		if err != nil {
			return Result{}, err
		}
		return TextResult(fmt.Sprintf("Wrote %s (%d bytes).", entry.Path, entry.Size)), nil
	})
}
