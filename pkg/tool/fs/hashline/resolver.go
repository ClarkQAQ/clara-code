package hashline

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path"
	"regexp"
	"strings"
)

// DefaultResolver resolves Go and Markdown syntax blocks.
type DefaultResolver struct{}

// Resolve returns the largest node starting on line.
func (DefaultResolver) Resolve(filePath, source string, line int) (int, int, bool, error) {
	switch strings.ToLower(path.Ext(filePath)) {
	case ".go":
		return resolveGo(source, line)
	case ".md", ".markdown":
		return resolveMarkdown(source, line)
	default:
		return 0, 0, false, nil
	}
}

func resolveGo(source string, line int) (int, int, bool, error) {
	files := token.NewFileSet()
	file, err := parser.ParseFile(files, "source.go", source, parser.ParseComments)
	if err != nil {
		return 0, 0, false, nil
	}
	startLine, endLine := 0, 0
	ast.Inspect(file, func(node ast.Node) bool {
		if node == nil {
			return true
		}
		start := files.Position(node.Pos()).Line
		end := files.Position(node.End()).Line
		if start == line && end > start && end > endLine {
			startLine, endLine = start, end
		}
		return true
	})
	if startLine == 0 {
		return 0, 0, false, nil
	}
	return startLine, endLine, true, nil
}

var (
	atxHeading = regexp.MustCompile(`^\s{0,3}(#{1,6})(?:[ \t]+|$)`)
	fenceLine  = regexp.MustCompile(`^\s{0,3}(` + "```" + `+|~~~+)`)
)

type heading struct {
	line  int
	level int
}

func resolveMarkdown(source string, line int) (int, int, bool, error) {
	text, err := ParseText(source)
	if err != nil {
		return 0, 0, false, err
	}
	if line < 1 || line > len(text.Lines) {
		return 0, 0, false, nil
	}
	var headings []heading
	inFence := false
	fence := ""
	for index, row := range text.Lines {
		if match := fenceLine.FindStringSubmatch(row); match != nil {
			marker := match[1]
			if !inFence {
				inFence = true
				fence = marker[:3]
			} else if strings.HasPrefix(marker, fence) {
				inFence = false
			}
			continue
		}
		if inFence {
			continue
		}
		if match := atxHeading.FindStringSubmatch(row); match != nil {
			headings = append(headings, heading{line: index + 1, level: len(match[1])})
		}
	}
	target := -1
	for index, item := range headings {
		if item.line == line {
			target = index
			break
		}
	}
	if target < 0 {
		return 0, 0, false, nil
	}
	end := len(text.Lines)
	for index := target + 1; index < len(headings); index++ {
		if headings[index].level <= headings[target].level {
			end = headings[index].line - 1
			break
		}
	}
	for end > line && strings.TrimSpace(text.Lines[end-1]) == "" {
		end--
	}
	return line, end, true, nil
}
