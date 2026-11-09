package tool

import (
	"context"
	"strings"

	clarafs "clara-code/pkg/tool/fs"
)

func (s *Files) Glob() Tool {
	type input struct {
		Path           string `json:"path,omitempty" description:"search root; defaults to /"`
		Pattern        string `json:"pattern" description:"glob: *.go matches filenames recursively; **/*.go includes root and nested files; /internal/*.go is rooted at the logical workspace"`
		Limit          int    `json:"limit,omitempty" description:"maximum file matches; default 500, maximum 5000"`
		IncludeIgnored bool   `json:"include_ignored,omitempty" description:"include files excluded by the host search policy; default false"`
	}
	return Func(Definition{
		Name: "glob", Description: `Find ordinary files. * and ? match within a component, [abc] matches characters, and a whole ** component matches zero or more directories. Relative patterns with / start at path. Example: {"path":"internal","pattern":"**/*.go"}. Check incomplete-search warnings before concluding a file does not exist.`,
		Effects: []Effect{EffectRead}, ParallelSafe: true,
	}, func(ctx context.Context, in input) (Result, error) {
		root, err := s.cleanPath(in.Path)
		if err != nil {
			return ErrorResult(err.Error()), nil
		}
		service, err := s.searchService(ctx, root, in.IncludeIgnored)
		if err != nil {
			return ErrorResult("glob search setup failed: " + err.Error()), nil
		}
		result, err := service.GlobDetailed(ctx, root, clarafs.GlobOptions{Pattern: in.Pattern, MaxResults: in.Limit})
		if err != nil {
			return ErrorResult("glob: " + err.Error()), nil
		}
		var builder strings.Builder
		for _, entry := range result.Entries {
			builder.WriteString(entry.Path.String())
			builder.WriteString("\n")
		}
		return TextResult(searchText(builder.String(), "No matching files.", result.SearchDiagnostics)), nil
	})
}
