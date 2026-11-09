package hashline

import (
	"clara-code/pkg/util/errs"
	"context"
	"fmt"
	"io/fs"
	"sort"
	"strings"

	clarafs "clara-code/pkg/tool/fs"
)

type changeKind int

const (
	changeReplace changeKind = iota
	changeInsert
)

type change struct {
	kind  changeKind
	start int
	end   int
	gap   int
	lines []string
	seq   int
	line  int
}

type fileState struct {
	path     clarafs.Path
	expected Snapshot
	current  string
	mode     fs.FileMode
	changes  []change
	remove   bool
	moveTo   clarafs.Path
	line     int
	warnings []string
}

// Editor applies Hashline patches through an FS and session snapshot store.
type Editor struct {
	FS       clarafs.FS
	Store    *Store
	Resolver BlockResolver
}

// Apply parses and applies input atomically at the validation boundary.
func (e *Editor) Apply(ctx context.Context, input string) (Result, error) {
	patch, err := Parse(input)
	if err != nil {
		return Result{}, err
	}
	prepared, err := e.prepare(ctx, patch)
	if err != nil {
		return Result{}, err
	}
	applied := make([]Prepared, 0, len(prepared.Files))
	for _, file := range prepared.Files {
		if file.NoChange && file.MoveTo == "" && !file.Remove {
			continue
		}
		current, err := e.FS.ReadFile(ctx, file.Path, MaxFileBytes+1)
		if err != nil {
			return Result{Files: applied}, err
		}
		if Normalize(string(current)) != file.Original {
			return Result{Files: applied}, errs.New(errs.CodeConflict, fmt.Sprintf("file changed before applying edits: %s; read it with anchored=true before retrying", file.Path))
		}
		final := file.Path
		if file.MoveTo != "" {
			final = file.MoveTo
		}
		if file.Remove {
			if err := e.FS.Remove(ctx, file.Path, clarafs.RemoveOptions{}); err != nil {
				return Result{Files: applied, Registers: prepared.Registers, Warnings: prepared.Warnings},
					fmt.Errorf("%w; %d earlier file(s) landed", err, len(applied))
			}
		} else if file.MoveTo != "" && file.Text != file.Original {
			if _, err := e.FS.WriteFile(ctx, file.MoveTo, []byte(file.Text), clarafs.WriteOptions{
				Mode: file.Mode, CreateDirs: true, Overwrite: false,
			}); err != nil {
				return Result{Files: applied}, fmt.Errorf("write moved file: %w", err)
			}
			if err := e.FS.Remove(ctx, file.Path, clarafs.RemoveOptions{}); err != nil {
				return Result{Files: applied}, fmt.Errorf("source remained after move: %w", err)
			}
		} else if file.MoveTo != "" {
			if err := e.FS.Rename(ctx, file.Path, file.MoveTo); err != nil {
				return Result{Files: applied}, fmt.Errorf("rename moved file: %w", err)
			}
		} else if file.Text != file.Original {
			if _, err := e.FS.WriteFile(ctx, file.Path, []byte(file.Text), clarafs.WriteOptions{
				Mode: file.Mode, CreateDirs: true, Overwrite: true,
			}); err != nil {
				return Result{Files: applied}, fmt.Errorf("%w; %d earlier file(s) landed", err, len(applied))
			}
		}
		if !file.Remove {
			e.Store.Record(string(final), file.Text, nil)
		}
		e.Store.Success(string(file.Path))
		applied = append(applied, file)
	}
	e.Store.CommitRegisters(prepared.Registers)
	return Result{Files: applied, Registers: prepared.Registers, Warnings: prepared.Warnings}, nil
}

// Prepare validates every operation and stages new file contents.
func (e *Editor) Prepare(input string) (Result, error) {
	patch, err := Parse(input)
	if err != nil {
		return Result{}, err
	}
	return e.prepare(context.Background(), patch)
}

func (e *Editor) prepare(ctx context.Context, patch Patch) (Result, error) {
	if e.FS == nil {
		return Result{}, fmt.Errorf("hashline: filesystem is not configured")
	}
	if e.Store == nil {
		e.Store = NewStore()
	}
	states := map[clarafs.Path]*fileState{}
	order := make([]clarafs.Path, 0, len(patch.Sections))
	registers := map[string][]string{}
	var anonymous []string
	anonymousSet := false
	sequence := 0
	seenTags := map[clarafs.Path]string{}
	for _, section := range patch.Sections {
		filePath, err := clarafs.ResolvePath(e.FS, section.Path)
		if err != nil {
			return Result{}, fmt.Errorf("line %d: %w", section.Line, err)
		}
		if previous, ok := seenTags[filePath]; ok && previous != section.Tag {
			return Result{}, fmt.Errorf("line %d: sections for %s use different snapshot tags", section.Line, filePath)
		}
		seenTags[filePath] = section.Tag
		expected, ok := e.Store.Lookup(string(filePath), section.Tag)
		if !ok {
			return Result{}, fmt.Errorf("line %d: snapshot [%s#%s] is unknown; call read with anchored=true for this file and the target lines, then copy the returned header", section.Line, filePath, section.Tag)
		}
		current, ok := e.Store.Current(string(filePath))
		if !ok {
			return Result{}, fmt.Errorf("line %d: %s is not in this session", section.Line, filePath)
		}
		state := states[filePath]
		if state == nil {
			text, err := e.FS.ReadFile(ctx, filePath, MaxFileBytes+1)
			if err != nil {
				return Result{}, fmt.Errorf("read %s: %w", filePath, err)
			}
			if len(text) > MaxFileBytes {
				return Result{}, fmt.Errorf("%s is too large to edit", filePath)
			}
			entry, err := e.FS.Stat(ctx, filePath)
			if err != nil {
				return Result{}, err
			}
			state = &fileState{path: filePath, expected: expected, current: Normalize(string(text)), line: section.Line, mode: entry.Mode.Perm()}
			if state.current != expected.Text {
				return Result{}, errs.New(errs.CodeConflict, fmt.Sprintf("%s changed after the tagged read; call read with anchored=true for the target lines and rebuild the edit", filePath))
			}
			states[filePath] = state
			order = append(order, filePath)
		}
		if current.Tag != section.Tag && current.Tag != "" {
			return Result{}, fmt.Errorf("%s has a newer snapshot; call read with anchored=true for the target lines and use the returned header and line numbers", filePath)
		}
		if state.remove || state.moveTo != "" {
			return Result{}, fmt.Errorf("line %d: %s already has REM or MV", section.Line, filePath)
		}
		if section.File.Remove {
			if len(section.Ops) > 0 || len(state.changes) > 0 {
				return Result{}, fmt.Errorf("line %d: REM cannot share a file with edits", section.Line)
			}
			state.remove = true
			continue
		}
		if section.File.MoveTo != "" {
			target, err := clarafs.ResolvePath(e.FS, section.File.MoveTo)
			if err != nil {
				return Result{}, fmt.Errorf("line %d: %w", section.File.Line, err)
			}
			if target == filePath {
				return Result{}, fmt.Errorf("line %d: MV target must differ from source", section.File.Line)
			}
			state.moveTo = target
		}
		text, err := ParseText(state.expected.Text)
		if err != nil {
			return Result{}, fmt.Errorf("%s: %w", filePath, err)
		}
		for _, operation := range section.Ops {
			sequence++
			if err := e.stageOperation(
				state, text, operation, sequence, registers, &anonymous, &anonymousSet,
			); err != nil {
				return Result{}, fmt.Errorf("%s: %w", filePath, err)
			}
		}
	}
	destinations := map[clarafs.Path]bool{}
	for _, filePath := range order {
		target := states[filePath].moveTo
		if target == "" {
			continue
		}
		if target == "/" || destinations[target] || states[target] != nil {
			return Result{}, errs.New(errs.CodeConflict, "conflicting Hashline move target")
		}
		if _, err := e.FS.Stat(ctx, target); !errs.IsCode(err, errs.CodeNotFound) {
			if err == nil {
				err = errs.New(errs.CodeConflict, "move target already exists")
			}
			return Result{}, err
		}
		destinations[target] = true
	}
	result := Result{Registers: registers}
	for _, filePath := range order {
		state := states[filePath]
		prepared, err := prepareFile(state)
		if err != nil {
			return Result{}, err
		}
		prepared.Mode = state.mode
		prepared.Warnings = append(prepared.Warnings, state.warnings...)
		result.Files = append(result.Files, prepared)
	}
	return result, nil
}

func (e *Editor) stageOperation(
	state *fileState,
	text Text,
	operation Operation,
	sequence int,
	registers map[string][]string,
	anonymous *[]string,
	anonymousSet *bool,
) error {
	lineCount := len(text.Lines)
	resolve := func() (int, int, bool, error) {
		if e.Resolver == nil {
			return 0, 0, false, fmt.Errorf("line %d: block operations need a block resolver; use an explicit range", operation.Line)
		}
		start, end, found, err := e.Resolver.Resolve(state.path.String(), state.expected.Text, operation.Start)
		if err != nil {
			return 0, 0, false, fmt.Errorf("line %d: resolve block: %w", operation.Line, err)
		}
		if !found {
			return 0, 0, false, fmt.Errorf("line %d: no syntax block starts at line %d", operation.Line, operation.Start)
		}
		return start, end, true, nil
	}
	requireVisible := func(line int) error {
		if !state.expected.Seen[line] {
			return fmt.Errorf("line %d: file line %d was not visible in the tagged read; call read with anchored=true and offset/limit covering the target lines", operation.Line, line)
		}
		return nil
	}
	requireRange := func(start, end int) error {
		if start < 1 || end < start || end > lineCount {
			return fmt.Errorf("line %d: range %d..%d is outside the %d-line file", operation.Line, start, end, lineCount)
		}
		for line := start; line <= end; line++ {
			if err := requireVisible(line); err != nil {
				return err
			}
		}
		return nil
	}
	operationBody := func(kind OpKind) ([]string, error) {
		if operation.Register == "" {
			return append([]string(nil), operation.Body...), nil
		}
		if lines, ok := registers[operation.Register]; ok {
			return append([]string(nil), lines...), nil
		}
		lines := e.Store.Register(operation.Register)
		if lines == nil {
			return nil, fmt.Errorf("line %d: register @%s is unknown", operation.Line, operation.Register)
		}
		if kind == OpReplaceRange || kind == OpReplaceBlock || kind == OpInsertBlock {
			registers[operation.Register] = append([]string(nil), lines...)
		}
		return lines, nil
	}
	capture := func(name string, lines []string) error {
		if name == "" {
			*anonymous = append([]string(nil), lines...)
			*anonymousSet = true
			return nil
		}
		registers[name] = append([]string(nil), lines...)
		return nil
	}
	appendChange := func(change change) error {
		for _, existing := range state.changes {
			if existing.kind == changeReplace && change.kind == changeReplace &&
				change.start <= existing.end && change.end >= existing.start {
				return fmt.Errorf("line %d: overlapping replacement ranges", operation.Line)
			}
			if existing.kind == changeReplace && change.kind == changeInsert &&
				change.gap > existing.start-1 && change.gap < existing.end {
				return fmt.Errorf("line %d: insertion falls inside a replaced range", operation.Line)
			}
			if existing.kind == changeInsert && change.kind == changeReplace &&
				existing.gap > change.start-1 && existing.gap < change.end {
				return fmt.Errorf("line %d: replacement covers an earlier insertion", operation.Line)
			}
		}
		state.changes = append(state.changes, change)
		return nil
	}

	switch operation.Kind {
	case OpReplaceRange:
		if err := requireRange(operation.Start, operation.End); err != nil {
			return err
		}
		body, err := operationBody(operation.Kind)
		if err != nil {
			return err
		}
		return appendChange(change{
			kind: changeReplace, start: operation.Start, end: operation.End,
			lines: body, seq: sequence, line: operation.Line,
		})
	case OpReplaceBlock:
		start, end, _, err := resolve()
		if err != nil {
			return err
		}
		if start == end {
			return fmt.Errorf("line %d: block at line %d is single-line. Use the explicit range form with PUT N.=N", operation.Line, start)
		}
		if err := requireRange(start, end); err != nil {
			return err
		}
		body, err := operationBody(operation.Kind)
		if err != nil {
			return err
		}
		return appendChange(change{
			kind: changeReplace, start: start, end: end,
			lines: body, seq: sequence, line: operation.Line,
		})
	case OpInsertBefore:
		if operation.Start < 1 || operation.Start > lineCount+1 {
			return fmt.Errorf("line %d: insertion anchor is outside the file", operation.Line)
		}
		if operation.Register == "" && operation.Start <= lineCount {
			if err := requireVisible(operation.Start); err != nil {
				return err
			}
		}
		body, err := operationBody(operation.Kind)
		if err != nil {
			return err
		}
		return appendChange(change{
			kind: changeInsert, gap: operation.Start - 1, lines: body, seq: sequence, line: operation.Line,
		})
	case OpInsertAfter:
		if operation.Start < 0 || operation.Start > lineCount {
			return fmt.Errorf("line %d: insertion anchor is outside the file", operation.Line)
		}
		if operation.Start > 0 {
			if err := requireVisible(operation.Start); err != nil {
				return err
			}
		}
		body, err := operationBody(operation.Kind)
		if err != nil {
			return err
		}
		return appendChange(change{
			kind: changeInsert, gap: operation.Start, lines: body, seq: sequence, line: operation.Line,
		})
	case OpAppend:
		body, err := operationBody(operation.Kind)
		if err != nil {
			return err
		}
		return appendChange(change{
			kind: changeInsert, gap: lineCount, lines: body, seq: sequence, line: operation.Line,
		})
	case OpInsertBlock:
		_, end, _, err := resolve()
		if err != nil {
			return err
		}
		if err := requireRange(operation.Start, end); err != nil {
			return err
		}
		body, err := operationBody(operation.Kind)
		if err != nil {
			return err
		}
		return appendChange(change{
			kind: changeInsert, gap: end, lines: body, seq: sequence, line: operation.Line,
		})
	case OpCutRange:
		if err := requireRange(operation.Start, operation.End); err != nil {
			return err
		}
		lines := append([]string(nil), text.Lines[operation.Start-1:operation.End]...)
		if err := capture(operation.Register, lines); err != nil {
			return err
		}
		return appendChange(change{
			kind: changeReplace, start: operation.Start, end: operation.End,
			seq: sequence, line: operation.Line,
		})
	case OpCutBlock:
		start, end, _, err := resolve()
		if err != nil {
			return err
		}
		if err := requireRange(start, end); err != nil {
			return err
		}
		lines := append([]string(nil), text.Lines[start-1:end]...)
		if err := capture(operation.Register, lines); err != nil {
			return err
		}
		return appendChange(change{
			kind: changeReplace, start: start, end: end, seq: sequence, line: operation.Line,
		})
	default:
		return fmt.Errorf("line %d: unsupported operation", operation.Line)
	}
}

func prepareFile(state *fileState) (Prepared, error) {
	original := state.expected.Text
	if state.remove {
		return Prepared{
			Path: state.path, Original: original, OldTag: state.expected.Tag,
			Remove: true, Lines: len(strings.Split(strings.TrimSuffix(original, "\n"), "\n")),
		}, nil
	}
	text, err := ParseText(original)
	if err != nil {
		return Prepared{}, err
	}
	next, err := applyChanges(text.Lines, state.changes)
	if err != nil {
		return Prepared{}, fmt.Errorf("%s: %w", state.path, err)
	}
	rendered := Text{Lines: next, TrailingNewline: text.TrailingNewline || len(text.Lines) == 0}.String()
	if len(next) == 0 {
		rendered = ""
	}
	if len(text.Lines) > 0 && next == nil {
		rendered = ""
	}
	noChange := Normalize(rendered) == Normalize(original)
	return Prepared{
		Path: state.path, MoveTo: state.moveTo, Original: original, Text: rendered,
		OldTag: state.expected.Tag, NewTag: Tag(rendered), Lines: len(next),
		NoChange: noChange,
	}, nil
}

func applyChanges(lines []string, changes []change) ([]string, error) {
	if len(changes) == 0 {
		return append([]string(nil), lines...), nil
	}
	inserts := map[int][]change{}
	replaces := map[int]change{}
	for _, item := range changes {
		switch item.kind {
		case changeInsert:
			inserts[item.gap] = append(inserts[item.gap], item)
		case changeReplace:
			if _, exists := replaces[item.start]; exists {
				return nil, fmt.Errorf("two replacements begin at line %d", item.start)
			}
			replaces[item.start] = item
		}
	}
	for gap := range inserts {
		sort.SliceStable(inserts[gap], func(i, j int) bool {
			return inserts[gap][i].seq < inserts[gap][j].seq
		})
	}
	// An empty file has one conceptual head gap.
	var out []string
	line := 1
	for line <= len(lines) {
		out = append(out, insertLines(inserts[line-1])...)
		if replace, ok := replaces[line]; ok {
			out = append(out, replace.lines...)
			line = replace.end + 1
			continue
		}
		out = append(out, lines[line-1])
		line++
	}
	out = append(out, insertLines(inserts[len(lines)])...)
	return out, nil
}

func insertLines(items []change) []string {
	var out []string
	for _, item := range items {
		out = append(out, item.lines...)
	}
	return out
}

// Render returns a compact result description.
func Render(result Result) string {
	var builder strings.Builder
	for index, file := range result.Files {
		if index > 0 {
			builder.WriteByte('\n')
		}
		switch {
		case file.Remove:
			fmt.Fprintf(&builder, "[%s#%s]\nREM", file.Path, file.OldTag)
		case file.NoChange:
			fmt.Fprintf(&builder, "[%s#%s]\nno change", file.Path, file.NewTag)
		default:
			target := file.Path
			if file.MoveTo != "" {
				target = file.MoveTo
			}
			fmt.Fprintf(&builder, "[%s#%s]\n%d lines", target, file.NewTag, file.Lines)
			if file.MoveTo != "" {
				fmt.Fprintf(&builder, "\nmoved from %s", file.Path)
			}
		}
	}
	return strings.TrimSpace(builder.String())
}
