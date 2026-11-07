package tools

import (
	"clara-code/pkg/tools/fs/hashline"
	"context"
)

func (s *Files) Edit() Tool {
	type input struct {
		Input string `json:"input" description:"one or more [path#TAG] Hashline sections"`
	}
	return Func(Definition{
		Name: "edit",
		Description: `Edit existing text with Hashline. Each section starts [path#TAG] using a tag from an anchored read. ` +
			`PUT N.=M: replaces lines. PUT <N: or PUT >N: inserts. PUT >$: appends. CUT removes. REM deletes. MV moves. ` +
			`Body rows start with + and are final content.`,
		Effects: []Effect{EffectWrite},
	}, func(ctx context.Context, in input) (Result, error) {
		patch, err := hashline.Parse(in.Input)
		if err != nil {
			return ErrorResult(err.Error()), nil
		}
		for _, section := range patch.Sections {
			for _, raw := range []string{section.Path, section.File.MoveTo} {
				if raw == "" {
					continue
				}
				target, err := s.cleanPath(raw)
				if err != nil {
					return Result{}, err
				}
				if _, err := s.rulesFor(ctx, target, false); err != nil {
					return ErrorResult(err.Error()), nil
				}
			}
		}
		result, err := s.editor.Apply(ctx, in.Input)
		if err != nil {
			return ErrorResult(err.Error() + "\n" + hashline.Render(result)), nil
		}
		return TextResult(hashline.Render(result)), nil
	})
}
