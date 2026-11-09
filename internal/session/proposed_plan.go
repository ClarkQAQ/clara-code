package session

import (
	"context"
	"strings"
)

const ProposedPlanKind = "proposed_plan"

type planningKey struct{}

// WithPlanning marks a terminal run's display records without changing model history.
func WithPlanning(ctx context.Context) context.Context {
	return context.WithValue(ctx, planningKey{}, true)
}

// PlanCells separates standalone plan blocks from the surrounding reply. During
// streaming it also previews an open block and holds back a possible split tag.
func PlanCells(text string, streaming bool) []Cell {
	cells, _ := splitPlan(text, streaming)
	return cells
}

// CompletePlan returns the source of a single complete, nonempty proposal.
func CompletePlan(text string) string {
	cells, blocks := splitPlan(text, false)
	if blocks != 1 {
		return ""
	}
	for _, cell := range cells {
		if cell.Kind == ProposedPlanKind {
			return cell.Text
		}
	}
	return ""
}

func splitPlan(text string, streaming bool) ([]Cell, int) {
	const open, close = "<proposed_plan>", "</proposed_plan>"
	var cells []Cell
	appendCell := func(kind, text string) {
		if strings.TrimSpace(text) != "" {
			cells = append(cells, Cell{Kind: kind, Text: strings.Trim(text, "\r\n")})
		}
	}
	normal, start, body, end := 0, -1, 0, 0
	blocks := 0
	for line := range strings.SplitAfterSeq(text, "\n") {
		at := end
		end += len(line)
		tag := strings.TrimSpace(line)
		expected := open
		if start >= 0 {
			expected = close
		}
		if streaming && end == len(text) && !strings.HasSuffix(line, "\n") &&
			strings.HasPrefix(expected, tag) {
			end = at
			break
		}
		switch tag {
		case open:
			blocks++
			if start < 0 {
				appendCell("assistant", text[normal:at])
				start, body = at, end
			}
		case close:
			if start >= 0 {
				appendCell(ProposedPlanKind, text[body:at])
				normal, start = end, -1
			}
		}
	}
	switch {
	case start < 0:
		appendCell("assistant", text[normal:end])
	case streaming:
		appendCell(ProposedPlanKind, text[body:end])
	default:
		appendCell("assistant", text[start:end])
	}
	return cells, blocks
}
