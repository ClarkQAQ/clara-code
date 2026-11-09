package hashline

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

var (
	headerPattern       = regexp.MustCompile(`^\[(.+)#([0-9A-Fa-f]{4})\]$`)
	replaceRangePattern = regexp.MustCompile(`^PUT ([0-9]+)\.=([0-9]+)(?: @([A-Za-z0-9_-]+))?(:)?$`)
	replaceBlockPattern = regexp.MustCompile(`^PUT ([0-9]+)\*(?: @([A-Za-z0-9_-]+))?(:)?$`)
	beforePattern       = regexp.MustCompile(`^PUT <([0-9]+)(?: @([A-Za-z0-9_-]+))?(:)?$`)
	afterPattern        = regexp.MustCompile(`^PUT >([0-9]+)(?: @([A-Za-z0-9_-]+))?(:)?$`)
	appendPattern       = regexp.MustCompile(`^PUT >\$(?: @([A-Za-z0-9_-]+))?(:)?$`)
	insertBlockPattern  = regexp.MustCompile(`^PUT >([0-9]+)\*(?: @([A-Za-z0-9_-]+))?(:)?$`)
	cutRangePattern     = regexp.MustCompile(`^CUT ([0-9]+)\.=([0-9]+)(?: @([A-Za-z0-9_-]+))?$`)
	cutBlockPattern     = regexp.MustCompile(`^CUT ([0-9]+)\*(?: @([A-Za-z0-9_-]+))?$`)
	movePattern         = regexp.MustCompile(`^MV (.+)$`)
)

// Parse reads canonical Hashline syntax.
func Parse(input string) (Patch, error) {
	payload := strings.Trim(input, "\r\n")
	if strings.TrimSpace(payload) == "" {
		return Patch{}, fmt.Errorf("edit input is empty")
	}
	lines := strings.Split(Normalize(payload), "\n")
	if strings.TrimSpace(lines[0]) == "*** Begin Patch" {
		lines = lines[1:]
	}
	if len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) == "*** End Patch" {
		lines = lines[:len(lines)-1]
	}
	type inputSection struct {
		path string
		tag  string
		line int
		rows []bodyRow
	}
	var inputs []inputSection
	for index, row := range lines {
		trimmed := strings.TrimSpace(row)
		if trimmed == "" {
			continue
		}
		if match := headerPattern.FindStringSubmatch(trimmed); match != nil {
			inputs = append(inputs, inputSection{
				path: match[1], tag: strings.ToUpper(match[2]), line: index + 1,
			})
			continue
		}
		if len(inputs) == 0 {
			return Patch{}, fmt.Errorf("line %d appears before a [path#TAG] header", index+1)
		}
		inputs[len(inputs)-1].rows = append(inputs[len(inputs)-1].rows, bodyRow{line: index + 1, text: row})
	}
	if len(inputs) == 0 {
		return Patch{}, fmt.Errorf("no [path#TAG] section was found")
	}
	patch := Patch{Payload: payload}
	for _, input := range inputs {
		section, err := parseSection(input.path, input.tag, input.line, input.rows)
		if err != nil {
			return Patch{}, err
		}
		patch.Sections = append(patch.Sections, section)
	}
	return patch, nil
}

type bodyRow struct {
	line int
	text string
}

func parseSection(path, tag string, line int, rows []bodyRow) (Section, error) {
	section := Section{Path: path, Tag: tag, Line: line}
	var current *Operation
	for _, row := range rows {
		text := strings.TrimRight(row.text, " \t\r")
		if strings.TrimSpace(text) == "" {
			current = nil
			continue
		}
		if strings.HasPrefix(row.text, "+") {
			if current == nil || !bodyBearing(current.Kind) {
				return Section{}, fmt.Errorf("line %d: + rows must follow a PUT ending in ':'; CUT takes no body", row.line)
			}
			current.Body = append(current.Body, row.text[1:])
			continue
		}
		if strings.HasPrefix(strings.TrimSpace(text), "-") || strings.HasPrefix(strings.TrimSpace(text), "@@") {
			return Section{}, fmt.Errorf("line %d is unified-diff input, not hashline input", row.line)
		}
		if current != nil {
			section.Ops = append(section.Ops, *current)
			current = nil
		}
		trimmed := strings.TrimSpace(text)
		if trimmed == "REM" {
			if len(section.Ops) > 0 || section.File.Remove || section.File.MoveTo != "" {
				return Section{}, fmt.Errorf("line %d: REM cannot be combined with other operations", row.line)
			}
			section.File = FileOperation{Remove: true, Line: row.line}
			continue
		}
		if match := movePattern.FindStringSubmatch(trimmed); match != nil {
			if len(section.Ops) > 0 || section.File.Remove || section.File.MoveTo != "" {
				return Section{}, fmt.Errorf("line %d: MV cannot be combined with other operations", row.line)
			}
			section.File = FileOperation{MoveTo: strings.TrimSpace(match[1]), Line: row.line}
			continue
		}
		if section.File.Remove || section.File.MoveTo != "" {
			return Section{}, fmt.Errorf("line %d: line operations cannot follow REM or MV", row.line)
		}
		operation, err := parseOperation(trimmed, row.line)
		if err != nil {
			return Section{}, err
		}
		current = &operation
	}
	if current != nil {
		section.Ops = append(section.Ops, *current)
	}
	for _, operation := range section.Ops {
		if bodyBearing(operation.Kind) && len(operation.Body) == 0 && operation.Register == "" {
			return Section{}, fmt.Errorf("line %d: PUT needs at least one + row (use a lone + for a blank line); to delete, use CUT N.=M with no trailing colon or body", operation.Line)
		}
		if bodyBearing(operation.Kind) && len(operation.Body) > 0 && operation.Register != "" {
			return Section{}, fmt.Errorf("line %d: a register paste cannot also carry + rows", operation.Line)
		}
		if !bodyBearing(operation.Kind) && len(operation.Body) > 0 {
			return Section{}, fmt.Errorf("line %d: operation does not accept a body", operation.Line)
		}
	}
	if len(section.Ops) == 0 && !section.File.Remove && section.File.MoveTo == "" {
		return Section{}, fmt.Errorf("section for %s has no operation", path)
	}
	return section, nil
}

func bodyBearing(kind OpKind) bool {
	switch kind {
	case OpReplaceRange, OpReplaceBlock, OpInsertBefore, OpInsertAfter, OpAppend, OpInsertBlock:
		return true
	default:
		return false
	}
}

func parseOperation(text string, line int) (Operation, error) {
	operation := Operation{Line: line}
	if match := replaceRangePattern.FindStringSubmatch(text); match != nil {
		if err := validateBodyMarker(match[3], match[4], line); err != nil {
			return Operation{}, err
		}
		start, end, err := parseRange(match[1], match[2], line)
		if err != nil {
			return Operation{}, err
		}
		operation.Kind, operation.Start, operation.End, operation.Register = OpReplaceRange, start, end, match[3]
		return operation, nil
	}
	if match := replaceBlockPattern.FindStringSubmatch(text); match != nil {
		if err := validateBodyMarker(match[2], match[3], line); err != nil {
			return Operation{}, err
		}
		start, err := parseLine(match[1], line)
		if err != nil {
			return Operation{}, err
		}
		operation.Kind, operation.Start, operation.End, operation.Register = OpReplaceBlock, start, start, match[2]
		return operation, nil
	}
	if match := beforePattern.FindStringSubmatch(text); match != nil {
		if err := validateBodyMarker(match[2], match[3], line); err != nil {
			return Operation{}, err
		}
		start, err := parseLine(match[1], line)
		if err != nil {
			return Operation{}, err
		}
		operation.Kind, operation.Start, operation.End, operation.Register = OpInsertBefore, start, start, match[2]
		return operation, nil
	}
	if match := afterPattern.FindStringSubmatch(text); match != nil {
		if err := validateBodyMarker(match[2], match[3], line); err != nil {
			return Operation{}, err
		}
		start, err := parseLine(match[1], line)
		if err != nil {
			return Operation{}, err
		}
		operation.Kind, operation.Start, operation.End, operation.Register = OpInsertAfter, start, start, match[2]
		return operation, nil
	}
	if match := appendPattern.FindStringSubmatch(text); match != nil {
		if err := validateBodyMarker(match[1], match[2], line); err != nil {
			return Operation{}, err
		}
		operation.Kind, operation.Register = OpAppend, match[1]
		return operation, nil
	}
	if match := insertBlockPattern.FindStringSubmatch(text); match != nil {
		if err := validateBodyMarker(match[2], match[3], line); err != nil {
			return Operation{}, err
		}
		start, err := parseLine(match[1], line)
		if err != nil {
			return Operation{}, err
		}
		operation.Kind, operation.Start, operation.End, operation.Register = OpInsertBlock, start, start, match[2]
		return operation, nil
	}
	if match := cutRangePattern.FindStringSubmatch(text); match != nil {
		start, end, err := parseRange(match[1], match[2], line)
		if err != nil {
			return Operation{}, err
		}
		operation.Kind, operation.Start, operation.End, operation.Register = OpCutRange, start, end, match[3]
		return operation, nil
	}
	if match := cutBlockPattern.FindStringSubmatch(text); match != nil {
		start, err := parseLine(match[1], line)
		if err != nil {
			return Operation{}, err
		}
		operation.Kind, operation.Start, operation.End, operation.Register = OpCutBlock, start, start, match[2]
		return operation, nil
	}
	return Operation{}, fmt.Errorf("line %d is not a hashline operation: %s\nUse CUT 12.=12 to delete line 12 (no colon or body), or PUT 12.=12: followed by +new text to replace it", line, text)
}

func validateBodyMarker(register, colon string, line int) error {
	hasRegister := register != ""
	hasColon := colon == ":"
	if hasRegister == hasColon {
		return fmt.Errorf("line %d: use either ':' with + rows or '@name' for a register paste", line)
	}
	return nil
}

func parseLine(raw string, line int) (int, error) {
	value, err := strconv.Atoi(raw)
	if err != nil || value < 1 {
		return 0, fmt.Errorf("line %d: invalid line number %q", line, raw)
	}
	return value, nil
}

func parseRange(startRaw, endRaw string, line int) (int, int, error) {
	start, err := parseLine(startRaw, line)
	if err != nil {
		return 0, 0, err
	}
	end, err := parseLine(endRaw, line)
	if err != nil {
		return 0, 0, err
	}
	if end < start {
		return 0, 0, fmt.Errorf("line %d: range end is before its start", line)
	}
	return start, end, nil
}
