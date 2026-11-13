package terminal

import (
	"strings"
	"time"

	"clara-code/internal/session"
)

const (
	turnCompletedKind = "turn-completed"
	threadForkedKind  = "thread-forked"
)

func turnCompletion(turn session.Turn) string {
	if turn.Status != "completed" {
		return ""
	}
	var seconds int64
	if turn.DurationMS != nil {
		seconds = *turn.DurationMS / 1000
	} else if !turn.StartedAt.IsZero() && !turn.FinishedAt.IsZero() {
		seconds = int64(turn.FinishedAt.Sub(turn.StartedAt) / time.Second)
	}
	if seconds <= 60 {
		return ""
	}
	return "Worked for " + elapsedSeconds(seconds)
}

func compactionCell(compaction *session.Compaction) session.Cell {
	return session.Cell{Kind: session.ContextCompactedKind, Compaction: compaction}
}

func compactionSince(store *session.Store, count uint64) *session.Compaction {
	if store.CompactionCount() == count {
		return nil
	}
	markers := store.Compactions()
	if len(markers) == 0 {
		return nil
	}
	return &markers[len(markers)-1].Compaction
}

func transcriptCellText(cell session.Cell) string {
	if cell.Kind != session.ContextCompactedKind || (cell.Compaction == nil && cell.Text != "") {
		return cell.Text
	}
	label := "Context compacted"
	if cell.Compaction != nil && cell.Compaction.DurationMS != nil && *cell.Compaction.DurationMS >= 0 {
		label += " · " + elapsedSeconds(*cell.Compaction.DurationMS/1000)
	}
	return label
}

func transcriptBoundaries(meta session.Meta, compactions []session.CompactionMarker) func(int) []session.Cell {
	forkAt, source := 0, singleLine(meta.Parent)
	if source != "" && meta.ForkPoint != nil {
		forkAt = meta.ForkPoint.AfterTurn
		if title := singleLine(meta.ForkPoint.Title); title != "" {
			source = title
		}
	}
	return func(afterTurn int) []session.Cell {
		var cells []session.Cell
		for len(compactions) > 0 && compactions[0].AfterTurn == afterTurn {
			marker := compactions[0]
			if marker.AfterFork && source != "" && forkAt == afterTurn {
				cells = append(cells, session.Cell{Kind: threadForkedKind, Text: source})
				source = ""
			}
			cells = append(cells, compactionCell(&marker.Compaction))
			compactions = compactions[1:]
		}
		if source != "" && forkAt == afterTurn {
			cells = append(cells, session.Cell{Kind: threadForkedKind, Text: source})
			source = ""
		}
		return cells
	}
}

func (p *painter) transcriptNoteRows(prefix, text string) []string {
	margin := widthOf(prefix)
	if p.width <= margin {
		prefix, margin = "", 0
	}
	var rows []string
	for i, line := range wrap(text, p.width-margin) {
		if i > 0 {
			prefix = strings.Repeat(" ", margin)
		}
		rows = append(rows, p.style(dim, prefix+truncate(line.text, max(1, p.width-margin))))
	}
	return append(rows, "")
}

func (p *painter) forkRows(source string) []string {
	const label = "Forked from "
	text := label + singleLine(source)
	if p.width < len(label)+10 {
		return []string{p.style(dim, truncate(text, p.width)), ""}
	}
	text = truncate(text, p.width-6)
	rule := p.width - widthOf(text) - 2
	line := strings.Repeat("─", rule/2) + " " + text + " " + strings.Repeat("─", rule-rule/2)
	return []string{p.style(dim, line), ""}
}
