package terminal

import (
	"regexp"
	"strings"

	"github.com/rivo/uniseg"
)

type span struct{ text, style string }

var ansiStyle = regexp.MustCompile("\\x1b\\[[0-9;]*m")

var inlineMarkdown = regexp.MustCompile("(`[^`\n]+`|\\*\\*[^*\n]+\\*\\*|\\*[^*\n]+\\*|\\[[^]\\n]+\\]\\([^ )\\n]+\\))")

func inlineSpans(text string) []span {
	var spans []span
	at := 0
	for _, match := range inlineMarkdown.FindAllStringIndex(text, -1) {
		spans = append(spans, span{text[at:match[0]], ""})
		part := text[match[0]:match[1]]
		switch {
		case strings.HasPrefix(part, "`"):
			spans = append(spans, span{part[1 : len(part)-1], accent})
		case strings.HasPrefix(part, "**"):
			spans = append(spans, span{part[2 : len(part)-2], bold})
		case strings.HasPrefix(part, "*"):
			spans = append(spans, span{part[1 : len(part)-1], "\x1b[3m"})
		case strings.HasPrefix(part, "["):
			end := strings.Index(part, "](")
			spans = append(spans, span{part[1:end], accent + "\x1b[4m"}, span{" (" + part[end+2:len(part)-1] + ")", dim})
		}
		at = match[1]
	}
	return append(spans, span{text[at:], ""})
}

func (p *painter) spanRows(spans []span, width int) []string {
	width = max(1, width)
	var rows []string
	var line strings.Builder
	cells := 0
	for _, item := range spans {
		var segment strings.Builder
		flush := func() {
			line.WriteString(p.style(item.style, segment.String()))
			segment.Reset()
		}
		it := uniseg.NewGraphemes(item.text)
		for it.Next() {
			if cells > 0 && cells+it.Width() > width {
				flush()
				rows = append(rows, line.String())
				line.Reset()
				cells = 0
			}
			if it.Width() > width {
				segment.WriteString("…")
				cells++
				continue
			}
			segment.WriteString(it.Str())
			cells += it.Width()
		}
		flush()
	}
	return append(rows, line.String())
}

func (p *painter) markdownRows(text string, width int) []string {
	var rows []string
	fenced := false
	for _, line := range strings.Split(text, "\n") {
		trim := strings.TrimSpace(line)
		if strings.HasPrefix(trim, "~~~") || strings.HasPrefix(trim, "```") {
			if !fenced {
				language := strings.TrimSpace(trim[3:])
				if language == "" {
					language = "code"
				}
				rows = append(rows, p.style(dim, truncate("┌ "+language, width)))
			} else {
				rows = append(rows, p.style(dim, truncate("└", width)))
			}
			fenced = !fenced
			continue
		}
		if fenced {
			for _, row := range wrap(line, max(1, width-2)) {
				rows = append(rows, p.style(dim, "│ ")+p.style(accent, truncate(row.text, max(1, width-2))))
			}
			continue
		}
		switch {
		case strings.HasPrefix(trim, "#"):
			heading := strings.TrimLeft(trim, "#")
			if strings.HasPrefix(heading, " ") {
				rows = append(rows, p.spanRows([]span{{strings.TrimSpace(heading), bold}}, width)...)
				continue
			}
		case strings.HasPrefix(trim, "- "), strings.HasPrefix(trim, "* "):
			indent := len(line) - len(strings.TrimLeft(line, " "))
			line = strings.Repeat(" ", indent) + "• " + trim[2:]
		case strings.HasPrefix(trim, "> "):
			line = "│ " + trim[2:]
		}
		rows = append(rows, p.spanRows(inlineSpans(line), width)...)
	}
	return rows
}
