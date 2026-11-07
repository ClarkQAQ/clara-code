// Package hashline implements line-anchored, snapshot-checked text editing.
package hashline

import (
	clarafs "clara-code/pkg/tool/fs"
	"io/fs"
)

// Patch is one parsed hashline payload.
type Patch struct {
	Payload  string
	Sections []Section
}

// Section edits one file against one snapshot tag.
type Section struct {
	Path string
	Tag  string
	Line int
	Ops  []Operation
	File FileOperation
}

// FileOperation removes or moves a file.
type FileOperation struct {
	Remove bool
	MoveTo string
	Line   int
}

// OpKind identifies one line operation.
type OpKind int

const (
	OpReplaceRange OpKind = iota
	OpReplaceBlock
	OpInsertBefore
	OpInsertAfter
	OpAppend
	OpInsertBlock
	OpCutRange
	OpCutBlock
	OpPasteRange
	OpPasteBlock
	OpPasteGap
)

// Operation is one parsed edit operation.
type Operation struct {
	Kind     OpKind
	Start    int
	End      int
	Body     []string
	Register string
	Line     int
}

// Result reports prepared or applied edits.
type Result struct {
	Files     []Prepared
	Registers map[string][]string
	Warnings  []string
}

// Prepared is the final state of one edited file.
type Prepared struct {
	Mode     fs.FileMode
	Path     clarafs.Path
	MoveTo   clarafs.Path
	Original string
	Text     string
	OldTag   string
	NewTag   string
	Lines    int
	Remove   bool
	NoChange bool
	Warnings []string
}

// BlockResolver resolves an opening line to an inclusive syntax block.
type BlockResolver interface {
	Resolve(path, source string, line int) (start, end int, found bool, err error)
}
