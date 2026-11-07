package terminal

import "strings"

type cursorPosition struct {
	row, column int
}

type layout struct {
	rows   []string
	cursor *cursorPosition
}

func (v *layout) append(other layout) {
	if other.cursor != nil {
		v.cursor = &cursorPosition{row: len(v.rows) + other.cursor.row, column: other.cursor.column}
	}
	v.rows = append(v.rows, other.rows...)
}

func (v layout) prepend(rows []string) layout {
	if v.cursor != nil {
		v.cursor = &cursorPosition{row: len(rows) + v.cursor.row, column: v.cursor.column}
	}
	v.rows = append(rows, v.rows...)
	return v
}

func (v layout) slice(start, end int) layout {
	start = min(max(0, start), len(v.rows))
	end = min(max(start, end), len(v.rows))
	if v.cursor != nil {
		if v.cursor.row >= start && v.cursor.row < end {
			v.cursor = &cursorPosition{row: v.cursor.row - start, column: v.cursor.column}
		} else {
			v.cursor = nil
		}
	}
	v.rows = v.rows[start:end]
	return v
}

func (p *painter) inputLayout(e editor, placeholder string, limit int) layout {
	prefix := "❯ "
	if p.width < 4 {
		prefix = ""
	}
	width := max(1, p.width-widthOf(prefix))
	if e.text == "" {
		return layout{
			rows:   []string{p.style(bold, prefix) + p.style(dim, truncate(placeholder, width))},
			cursor: &cursorPosition{column: widthOf(prefix)},
		}
	}
	rows := wrap(e.text, width)
	cursorRow := 0
	for i, row := range rows {
		if row.start <= e.at {
			cursorRow = i
		}
	}
	column := widthOf(e.text[rows[cursorRow].start:e.at])
	if column >= width {
		blank := row{start: e.at, end: e.at}
		rows = append(rows[:cursorRow+1], append([]row{blank}, rows[cursorRow+1:]...)...)
		cursorRow++
		column = 0
	}
	limit = max(1, limit)
	start := min(max(cursorRow-limit+1, 0), max(0, len(rows)-limit))
	end := min(len(rows), start+limit)
	view := layout{cursor: &cursorPosition{row: cursorRow - start, column: widthOf(prefix) + column}}
	for i := start; i < end; i++ {
		lead := strings.Repeat(" ", widthOf(prefix))
		if i == start {
			lead = p.style(bold, prefix)
		}
		view.rows = append(view.rows, lead+truncate(rows[i].text, width))
	}
	return view
}
