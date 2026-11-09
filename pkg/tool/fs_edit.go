package tool

import (
	"clara-code/pkg/tool/fs/hashline"
	"context"
)

func (s *Files) Edit() Tool {
	type input struct {
		Input string `json:"input" description:"newline-separated Hashline sections, each starting with the exact [path#TAG] header from an anchored read"`
	}
	return Func(Definition{
		Name: "edit",
		Description: `Edit existing text with Hashline. First call read with anchored=true for the target lines, then copy its first line ([path#TAG]) verbatim into input. ` +
			`Use the numbers from the read without the trailing colon. Ranges are inclusive: PUT 12.=12: replaces line 12; PUT 12.=14: replaces lines 12 through 14. ` +
			`PUT <12: inserts before line 12, PUT >12: inserts after it, and PUT >$: appends. Follow each literal PUT with one + prefixed row per new line (a lone + inserts a blank line). ` +
			`Delete line 12 with CUT 12.=12 or lines 12 through 14 with CUT 12.=14. CUT has no trailing colon or body. REM deletes the file; MV destination moves it. ` +
			`After a successful edit, read again with anchored=true before editing that file again.`,
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
