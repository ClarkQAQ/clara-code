package tools

import (
	"context"
	"fmt"
)

func (s *Files) Move() Tool {
	type input struct {
		From string `json:"from" description:"source path"`
		To   string `json:"to" description:"destination path"`
	}
	return Func(Definition{
		Name: "move", Description: "Move or rename a file or directory.",
		Effects: []Effect{EffectWrite},
	}, func(ctx context.Context, in input) (Result, error) {
		from, err := s.cleanPath(in.From)
		if err != nil {
			return ErrorResult(err.Error()), nil
		}
		to, err := s.cleanPath(in.To)
		if err != nil {
			return ErrorResult(err.Error()), nil
		}
		if err := s.service.FS.Rename(ctx, from, to); err != nil {
			return ErrorResult(err.Error()), nil
		}
		return TextResult(fmt.Sprintf("Moved %s to %s.", from, to)), nil
	})
}
