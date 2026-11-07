package tool

import (
	"context"
	"fmt"
	"strings"

	clarafs "clara-code/pkg/tool/fs"
)

func (s *Files) searchService(ctx context.Context, root clarafs.Path, includeIgnored bool) (*clarafs.Service, error) {
	if s.SearchSource == nil {
		return s.service, nil
	}
	storage, err := s.SearchSource(ctx, root, includeIgnored)
	if err != nil {
		return nil, err
	}
	if storage == nil {
		return nil, fmt.Errorf("search source returned no filesystem")
	}
	return clarafs.New(storage), nil
}

func searchText(text, empty string, diagnostic clarafs.SearchDiagnostics) string {
	text = strings.TrimRight(text, "\n")
	if text == "" {
		if diagnostic.Truncated {
			text = "No matches in the searched portion."
		} else {
			text = empty
		}
	}
	if diagnostic.Truncated {
		text += "\n[Search incomplete. Do not infer absence from these results.]"
	}
	if diagnostic.Skipped > 0 {
		text += fmt.Sprintf("\n[Skipped %d special, binary, oversized or unreadable entries.]", diagnostic.Skipped)
	}
	for _, warning := range diagnostic.Warnings {
		text += "\nWarning: " + warning
	}
	return text
}
