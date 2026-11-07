package tools

import (
	"context"
	"fmt"
	"strings"
)

func (s *Files) Read() Tool {
	type input struct {
		Path     string `json:"path" description:"file path"`
		Anchored bool   `json:"anchored,omitempty" description:"add a Hashline snapshot tag and line numbers"`
		Offset   int    `json:"offset,omitempty" description:"1-based first line to show"`
		Limit    int    `json:"limit,omitempty" description:"maximum lines to show"`
	}
	return Func(Definition{
		Name:        "read",
		Description: "Read a text file, up to 8 MiB. Set anchored to true before an edit. Use the returned [path#TAG] and line numbers. offset is a 1-based line number; limit is a line count. Ranges do not increase the 8 MiB snapshot limit.",
		Effects:     []Effect{EffectRead}, ParallelSafe: true, OutputLimit: 512 << 10,
	}, func(ctx context.Context, in input) (Result, error) {
		target, err := s.cleanPath(in.Path)
		if err != nil {
			return ErrorResult(err.Error()), nil
		}
		rules, err := s.rulesFor(ctx, target, true)
		if err != nil {
			return Result{}, err
		}
		view, err := s.service.View(ctx, target, readLimit)
		if err != nil {
			return ErrorResult(err.Error()), nil
		}
		if view.Entry.IsDir {
			return ErrorResult("path is a directory. Use list."), nil
		}
		if view.Binary {
			return ErrorResult("file is binary. Use image_view for images or document tools for office files."), nil
		}
		if in.Anchored && view.Truncated {
			return ErrorResult("file exceeds the 8 MiB anchored snapshot limit. offset and limit do not reduce snapshot size. Use anchored=false for inspection; editing requires a host capability that supports this file size."), nil
		}
		text := view.Text
		offset := in.Offset
		if offset <= 0 {
			offset = 1
		}
		lines := strings.Split(strings.TrimSuffix(text, "\n"), "\n")
		if text == "" {
			lines = nil
		}
		if offset > max(1, len(lines)) {
			return ErrorResult(fmt.Sprintf("offset %d is beyond the %d-line file", offset, len(lines))), nil
		}
		end := len(lines)
		if in.Limit > 0 && in.Limit < end-(offset-1) {
			end = offset - 1 + in.Limit
		}
		shown := lines[offset-1 : end]
		if !in.Anchored {
			result := strings.Join(shown, "\n")
			if view.Truncated {
				result += "\n[file truncated]"
			}
			if rules != "" {
				result += "\n\nApplicable project instructions:\n" + rules
			}
			return TextResult(result), nil
		}
		var body strings.Builder
		visibleEnd := offset - 1
		for index, line := range shown {
			row := fmt.Sprintf("\n%d:%s", offset+index, line)
			if body.Len()+len(row) > 450<<10 {
				body.WriteString("\n[range truncated]")
				break
			}
			body.WriteString(row)
			visibleEnd++
		}
		snapshot := s.store.Record(target.String(), text, visibleLines(offset, visibleEnd))
		var builder strings.Builder
		fmt.Fprintf(&builder, "[%s#%s]%s", target, snapshot.Tag, body.String())
		if rules != "" {
			builder.WriteString("\n\nApplicable project instructions:\n" + rules)
		}
		return TextResult(builder.String()), nil
	})
}

func visibleLines(start, end int) []int {
	out := make([]int, 0, max(0, end-start+1))
	for line := start; line <= end; line++ {
		out = append(out, line)
	}
	return out
}
