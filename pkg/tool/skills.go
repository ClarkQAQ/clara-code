package tool

import (
	"context"
	"strings"

	"clara-code/pkg/tool/fs"
	"clara-code/pkg/util/errs"
)

// ReadSkill loads one skill from the catalog configured by the host.
func ReadSkill(storage fs.Reader, catalog map[string]fs.Path) Tool {
	paths := make(map[string]fs.Path, len(catalog))
	for name, path := range catalog {
		paths[name] = path
	}
	reader := fs.At(storage, "/")
	return Func(Definition{Name: "read_skill", Description: "Load the instructions for one available skill.", Effects: []Effect{EffectRead}, ParallelSafe: true},
		func(ctx context.Context, input struct {
			Name string `json:"name"`
		}) (Result, error) {
			path, ok := paths[strings.TrimSpace(input.Name)]
			if !ok {
				return Result{}, errs.ErrNotFound
			}
			data, err := reader.ReadFile(ctx, path, 64<<10+1)
			if err != nil {
				return Result{}, err
			}
			if len(data) > 64<<10 {
				return Result{}, errs.New(errs.CodeLimit, "skill is too large")
			}
			return TextResult(string(data)), nil
		})
}
