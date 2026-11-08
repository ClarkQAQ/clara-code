package tool

import (
	"context"
	"fmt"
	"strings"

	clarafs "clara-code/pkg/tool/fs"
)

func (s *Files) Grep() Tool {
	type input struct {
		Path           string   `json:"path,omitempty" description:"search root; defaults to /"`
		Query          string   `json:"query" description:"literal text, not a regex; whitespace is significant"`
		MatchCase      bool     `json:"match_case,omitempty" description:"case-sensitive search; default false"`
		Limit          int      `json:"limit,omitempty" description:"maximum matching lines; default 200, maximum 1000"`
		Include        []string `json:"include,omitempty" description:"glob filters with the same semantics as glob; *.go matches Go filenames at any depth"`
		IncludeIgnored bool     `json:"include_ignored,omitempty" description:"include files excluded by the host search policy; default false"`
	}
	return Func(Definition{
		Name: "grep", Description: `Search ordinary text files and return path:line:text. Use query for the literal text, include for file globs. Example: {"path":"internal","query":"cursorPosition","include":["*.go"]}. Binary and special files are skipped. Check incomplete-search warnings before concluding text is absent.`,
		Effects: []Effect{EffectRead}, ParallelSafe: true,
	}, func(ctx context.Context, in input) (Result, error) {
		root, err := s.cleanPath(in.Path)
		if err != nil {
			return ErrorResult(err.Error()), nil
		}
		service, err := s.searchService(ctx, root, in.IncludeIgnored)
		if err != nil {
			return ErrorResult("grep search setup failed: " + err.Error()), nil
		}
		result, err := service.GrepDetailed(ctx, root, clarafs.SearchOptions{Query: in.Query, MatchCase: in.MatchCase, MaxResults: in.Limit, Include: in.Include})
		if err != nil {
			return ErrorResult("grep: " + err.Error()), nil
		}
		var builder strings.Builder
		for _, hit := range result.Hits {
			fmt.Fprintf(&builder, "%s:%d:%s\n", hit.Path, hit.Line, hit.Text)
		}
		return TextResult(searchText(builder.String(), "No matches.", result.SearchDiagnostics)), nil
	})
}
