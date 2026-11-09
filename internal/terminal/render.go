package terminal

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"clara-code/internal/session"
	"github.com/rivo/uniseg"
)

type painter struct {
	output       io.Writer
	buf          bytes.Buffer
	width        int
	height       int
	rows         int
	cursorRow    int
	cursorHidden bool
	color        bool
	markdown     *bool
	previewLines *int
	animate      *bool
	history      []session.Cell
	header       *banner
	sourceBytes  int
	dropped      bool
	droppedCells int
}

type banner struct{ version, model, directory string }

func (p *painter) clearScreen() {
	p.buf.Reset()
	p.buf.WriteString("\x1b[0m\x1b[r\x1b[H\x1b[2J\x1b[3J")
	p.rows = 0
	p.cursorRow = 0
}

func (p *painter) rebuild() {
	p.clearScreen()
	if p.header != nil {
		p.paintWelcome(p.header.version, p.header.model, p.header.directory)
	}
	if p.dropped {
		p.paintMessage("notice", "Earlier display history omitted. Saved conversations remain available.")
	}
	for _, cell := range p.history {
		p.paintCell(cell)
	}
}

func (p *painter) resetHistory() {
	p.history = nil
	p.header = nil
	p.sourceBytes = 0
	p.dropped = false
	p.droppedCells = 0
	p.clearScreen()
}

func (p *painter) remember(kind, text string) {
	if strings.TrimSpace(text) == "" {
		return
	}
	p.history = append(p.history, session.Cell{Kind: kind, Text: text})
	p.sourceBytes += len(text)
	p.trimHistory()
}

func (p *painter) trimHistory() {
	for p.sourceBytes > 32<<20 && len(p.history) > 1 {
		p.sourceBytes -= len(p.history[0].Text)
		if p.history[0].Tool != nil {
			p.sourceBytes -= p.history[0].Tool.DisplayBytes()
		}
		p.history[0] = session.Cell{}
		p.history = p.history[1:]
		p.droppedCells++
		p.dropped = true
	}
}

const (
	reset  = "\x1b[0m"
	dim    = "\x1b[90m"
	bold   = "\x1b[1m"
	accent = "\x1b[36m"
	purple = "\x1b[95m"
)

func (p *painter) style(code, text string) string {
	if !p.color || code == "" || text == "" {
		return text
	}
	return code + text + reset
}

func (p *painter) erase() {
	if p.rows == 0 {
		return
	}
	p.buf.WriteByte('\r')
	if p.cursorRow > 0 {
		fmt.Fprintf(&p.buf, "\x1b[%dA", min(p.cursorRow, max(p.height-1, 0)))
	}
	p.buf.WriteString("\x1b[J")
	p.rows = 0
	p.cursorRow = 0
}

func (p *painter) commit(lines ...string) {
	p.erase()
	for _, line := range lines {
		p.buf.WriteString(line)
		p.buf.WriteString("\r\n")
	}
}

func (p *painter) setCursorVisible(visible bool) {
	if visible == !p.cursorHidden {
		return
	}
	if visible {
		p.buf.WriteString("\x1b[?25h")
	} else {
		p.buf.WriteString("\x1b[?25l")
	}
	p.cursorHidden = !visible
}

func (p *painter) contentHeight() int {
	return max(1, p.height-1)
}

func (p *painter) frame(view layout) error {
	view = view.slice(0, p.contentHeight())
	if p.height > 1 {
		minRows := 0
		if view.cursor != nil {
			minRows = view.cursor.row + 1
		}
		for len(view.rows) > minRows && view.rows[len(view.rows)-1] == "" {
			view.rows = view.rows[:len(view.rows)-1]
		}
		view.rows = append(view.rows, "")
	}
	p.erase()
	for index, line := range view.rows {
		if index > 0 {
			p.buf.WriteString("\r\n")
		}
		p.buf.WriteString(line)
		p.buf.WriteString("\x1b[K")
	}
	p.rows = len(view.rows)
	p.cursorRow = max(0, p.rows-1)
	visible := view.cursor != nil && view.cursor.column >= 0 && view.cursor.column < p.width
	if visible {
		p.buf.WriteByte('\r')
		if up := p.cursorRow - view.cursor.row; up > 0 {
			fmt.Fprintf(&p.buf, "\x1b[%dA", up)
		}
		fmt.Fprintf(&p.buf, "\x1b[%dG", view.cursor.column+1)
		p.cursorRow = view.cursor.row
	}
	p.setCursorVisible(visible)
	_, err := p.buf.WriteTo(p.output)
	return err
}

func (p *painter) finish() error {
	p.erase()
	p.buf.WriteString("\x1b[0m\x1b[?2004l\x1b[?25h")
	p.cursorHidden = false
	_, err := p.buf.WriteTo(p.output)
	return err
}

func (p *painter) welcome(version, model, directory string) {
	p.header = &banner{version, model, directory}
	p.paintWelcome(version, model, directory)
}

func (p *painter) paintWelcome(version, model, directory string) {
	if home, _ := os.UserHomeDir(); home != "" {
		if rel, ok := strings.CutPrefix(directory, home+"/"); ok {
			directory = "~/" + rel
		} else if directory == home {
			directory = "~"
		}
	}
	if p.width < 30 {
		p.commit(truncate("Clara Code "+version, p.width), "")
		return
	}
	directory = singleLine(directory)
	const labelWidth = 13
	const modelHint = "   /model to change"
	width := min(p.width, max(48, 4+labelWidth+widthOf(directory), 4+labelWidth+len(modelHint)+widthOf(model)))
	title := p.style(dim, ">_< ") + p.style(bold, "Clara Code ") + p.style(dim, "("+version+")")
	var lines []string
	lines = append(lines, p.style(dim, "╭"+strings.Repeat("─", width-2)+"╮"))
	inside := func(value string, cells int) {
		lines = append(lines, p.style(dim, "│")+" "+value+strings.Repeat(" ", max(0, width-4-cells))+" "+p.style(dim, "│"))
	}
	inside(title, widthOf(">_< Clara Code ("+version+")"))
	inside("", 0)
	label := func(name, value, color string) {
		label := fmt.Sprintf("%-*s", labelWidth, name)
		value = truncate(value, width-4-len(label))
		inside(p.style(dim, label)+p.style(color, value), len(label)+widthOf(value))
	}
	value := clean(model)
	label("model:", value, "")
	// The wide layout has room for the same model-switch hint as the reference.
	if widthOf(value)+labelWidth+len(modelHint) <= width-4 {
		lines = lines[:len(lines)-1]
		inside(p.style(dim, fmt.Sprintf("%-*s", labelWidth, "model:"))+value+"   "+p.style(accent, "/model")+p.style(dim, " to change"),
			labelWidth+widthOf(value)+len(modelHint))
	}
	label("directory:", clean(directory), "")
	label("permissions:", "YOLO mode", purple)
	lines = append(lines, p.style(dim, "╰"+strings.Repeat("─", width-2)+"╯"), "")
	p.commit(lines...)
}

func (p *painter) message(kind, text string) {
	text = strings.TrimRight(clean(text), "\n")
	if text == "" {
		return
	}
	p.remember(kind, text)
	p.paintMessage(kind, text)
}

func (p *painter) messageRows(kind, text string) []string {
	text = strings.TrimRight(clean(text), "\n")
	if text == "" {
		return nil
	}
	width := max(1, p.width-2)
	var body []string
	switch kind {
	case "assistant":
		if p.markdown != nil && !*p.markdown {
			for _, row := range wrap(text, width) {
				body = append(body, row.text)
			}
		} else {
			body = p.markdownRows(text, width)
		}
	case "diff":
		for _, row := range wrap(text, width) {
			style := ""
			if strings.HasPrefix(row.text, "+") {
				style = "\x1b[32m"
			}
			if strings.HasPrefix(row.text, "-") {
				style = "\x1b[31m"
			}
			body = append(body, p.style(style, truncate(row.text, width)))
		}
	default:
		for _, row := range wrap(text, width) {
			body = append(body, truncate(row.text, width))
		}
	}
	marker, style := markerFor(kind)
	for i := range body {
		prefix := "  "
		if i == 0 {
			prefix = p.style(style, marker) + " "
		}
		if p.width < 4 {
			prefix = ""
		}
		body[i] = prefix + body[i]
	}
	return append(body, "")
}

func (p *painter) paintMessage(kind, text string) {
	p.commit(p.messageRows(kind, text)...)
}

func markerFor(kind string) (string, string) {
	switch kind {
	case "user":
		return "❯", bold
	case "assistant":
		return "●", dim
	case "activity":
		return "↳", dim
	case "error":
		return "■", "\x1b[31m"
	default:
		return "·", accent
	}
}

func clean(text string) string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\t", "    ")
	return strings.Map(func(r rune) rune {
		if r == '\n' || !unicode.IsControl(r) {
			return r
		}
		return -1
	}, text)
}

func singleLine(text string) string {
	return strings.Join(strings.Fields(clean(text)), " ")
}

func widthOf(text string) int { return uniseg.StringWidth(text) }

func truncate(text string, width int) string {
	if width <= 0 {
		return ""
	}
	if widthOf(text) <= width {
		return text
	}
	if width == 1 {
		return "…"
	}
	it := uniseg.NewGraphemes(text)
	var result strings.Builder
	for it.Next() && it.Width() <= width-1 {
		result.WriteString(it.Str())
		width -= it.Width()
	}
	return result.String() + "…"
}

type row struct {
	text       string
	start, end int
}

func wrap(text string, width int) []row {
	width = max(1, width)
	var rows []row
	iterator := uniseg.NewGraphemes(text)
	start, cells := 0, 0
	for iterator.Next() {
		from, to := iterator.Positions()
		if iterator.Str() == "\n" {
			rows = append(rows, row{text[start:from], start, from})
			start, cells = to, 0
			continue
		}
		if cells > 0 && cells+iterator.Width() > width {
			rows = append(rows, row{text[start:from], start, from})
			start, cells = from, 0
		}
		cells += iterator.Width()
	}
	return append(rows, row{text[start:], start, len(text)})
}

type editor struct {
	images         []imageSpan
	skills         []inlineSpan
	text           string
	at             int
	verticalColumn int
	verticalSet    bool
}

func (e *editor) replace(text string) {
	e.text, e.at = "", 0
	e.images = nil
	e.skills = nil
	e.insert(text)
}

func (e *editor) insert(text string) {
	e.verticalSet = false
	text = clean(text)
	room := inputLimit - len(e.text)
	if len(text) > room {
		for room > 0 && !utf8.RuneStart(text[room]) {
			room--
		}
		text = text[:room]
	}
	for i := range e.images {
		if e.images[i].start >= e.at {
			e.images[i].start += len(text)
			e.images[i].end += len(text)
		}
	}
	e.shiftSkills(e.at, len(text))
	e.text = e.text[:e.at] + text + e.text[e.at:]
	e.at += len(text)
	e.at = e.boundary(e.at, 1)
}

func (e *editor) boundary(at, direction int) int {
	for _, span := range e.inlineSpans() {
		if span.start < at && at < span.end {
			if direction < 0 {
				return span.start
			}
			return span.end
		}
	}
	it := uniseg.NewGraphemes(e.text)
	for it.Next() {
		start, end := it.Positions()
		if start < at && at < end {
			if direction < 0 {
				return start
			}
			return end
		}
	}
	return at
}

func (e *editor) next(direction int) int {
	iterator := uniseg.NewGraphemes(e.text)
	for iterator.Next() {
		start, end := iterator.Positions()
		if direction < 0 && end >= e.at {
			return e.boundary(start, -1)
		}
		if direction > 0 && end > e.at {
			return e.boundary(end, 1)
		}
	}
	return e.at
}

func (e *editor) delete(start, end int) {
	e.verticalSet = false
	for _, span := range e.inlineSpans() {
		if start < span.end && end > span.start {
			start, end = min(start, span.start), max(end, span.end)
		}
	}
	images := e.images[:0]
	for _, span := range e.images {
		if start < span.end && end > span.start {
			continue
		}
		if span.start >= end {
			span.start -= end - start
			span.end -= end - start
		}
		images = append(images, span)
	}
	e.images = images
	skills := e.skills[:0]
	for _, span := range e.skills {
		if start < span.end && end > span.start {
			continue
		}
		if span.start >= end {
			span.start -= end - start
			span.end -= end - start
		}
		skills = append(skills, span)
	}
	e.skills = skills
	e.text = e.text[:start] + e.text[end:]
	e.at = e.boundary(start, -1)
}

func (e *editor) word(direction int) int {
	at := e.at
	space := func(r rune) bool { return unicode.IsSpace(r) }
	if direction < 0 {
		for at > 0 {
			r, size := utf8.DecodeLastRuneInString(e.text[:at])
			if !space(r) {
				break
			}
			at -= size
		}
		for at > 0 {
			r, size := utf8.DecodeLastRuneInString(e.text[:at])
			if space(r) {
				break
			}
			at -= size
		}
	} else {
		for at < len(e.text) {
			r, size := utf8.DecodeRuneInString(e.text[at:])
			if space(r) {
				break
			}
			at += size
		}
		for at < len(e.text) {
			r, size := utf8.DecodeRuneInString(e.text[at:])
			if !space(r) {
				break
			}
			at += size
		}
	}
	return e.boundary(at, direction)
}

func (e *editor) key(input key) bool {
	e.verticalSet = false
	switch input.code {
	case "text", "paste":
		e.insert(input.text)
	case "newline":
		e.insert("\n")
	case "left":
		e.at = e.next(-1)
	case "right":
		e.at = e.next(1)
	case "word-left":
		e.at = e.word(-1)
	case "word-right":
		e.at = e.word(1)
	case "home", "ctrl-a":
		e.at = e.boundary(strings.LastIndex(e.text[:e.at], "\n")+1, -1)
	case "end", "ctrl-e":
		offset := strings.Index(e.text[e.at:], "\n")
		if offset < 0 {
			e.at = len(e.text)
		} else {
			e.at += offset
		}
		e.at = e.boundary(e.at, 1)
	case "backspace":
		if e.at > 0 {
			e.delete(e.next(-1), e.at)
		}
	case "delete", "ctrl-d":
		e.delete(e.at, e.next(1))
	case "ctrl-w":
		e.delete(e.word(-1), e.at)
	case "ctrl-u":
		e.delete(strings.LastIndex(e.text[:e.at], "\n")+1, e.at)
	case "ctrl-k":
		end := strings.Index(e.text[e.at:], "\n")
		if end < 0 {
			end = len(e.text) - e.at
		}
		e.delete(e.at, e.at+end)
	default:
		return false
	}
	return true
}

func (e *editor) vertical(direction, width int) {
	rows := wrap(e.text, max(1, width))
	if last := rows[len(rows)-1]; e.at == last.end && widthOf(last.text) >= width {
		rows = append(rows, row{start: e.at, end: e.at})
	}
	current := 0
	for i, line := range rows {
		if line.start <= e.at {
			current = i
		}
	}
	if !e.verticalSet {
		e.verticalColumn = widthOf(e.text[rows[current].start:e.at])
		e.verticalSet = true
	}
	target := min(max(0, current+direction), len(rows)-1)
	line := rows[target]
	e.at = line.start
	column := 0
	it := uniseg.NewGraphemes(line.text)
	for it.Next() {
		if column+it.Width() > e.verticalColumn {
			break
		}
		_, end := it.Positions()
		e.at = line.start + end
		column += it.Width()
	}
	e.at = e.boundary(e.at, direction)
}

func (p *painter) cellRows(cell session.Cell) []string {
	if len(cell.Attachments) > 0 {
		return p.attachmentHistory(cell.Attachments)
	}
	if cell.Tool != nil {
		return p.toolRows(*cell.Tool, time.Now())
	}
	return p.messageRows(cell.Kind, cell.Text)
}

func (p *painter) paintCell(cell session.Cell) { p.commit(p.cellRows(cell)...) }

func (p *painter) cell(cell session.Cell) bool {
	if len(cell.Attachments) > 0 {
		p.history = append(p.history, cell)
		p.trimHistory()
		p.commit(p.attachmentHistory(cell.Attachments)...)
		return false
	}
	if cell.Tool == nil {
		p.message(cell.Kind, cell.Text)
		return false
	}
	activity := *cell.Tool
	for i, old := range p.history {
		if old.Tool != nil && old.Tool.Key() == activity.Key() {
			p.sourceBytes += activity.DisplayBytes() - old.Tool.DisplayBytes()
			p.history[i].Tool = &activity
			p.trimHistory()
			return true
		}
	}
	p.history = append(p.history, session.Cell{Kind: "tool", Tool: &activity})
	p.sourceBytes += activity.DisplayBytes()
	p.trimHistory()
	p.paintCell(cell)
	return false
}
