package tool

import (
	"context"
	"fmt"

	clarafs "clara-code/pkg/tool/fs"
	"clara-code/pkg/tool/fs/office"
)

// Documents provides document tools over one filesystem.
type Documents struct {
	FS clarafs.FS
}

// New creates a document tool set.
func NewDocuments(storage clarafs.FS) *Documents { return &Documents{FS: storage} }

// All returns PDF and DOCX
func (s *Documents) All() []Tool {
	return []Tool{s.Read(), s.Create()}
}

func (s *Documents) Read() Tool {
	type input struct {
		Path string `json:"path" description:"path to a PDF or DOCX file"`
	}
	return Func(Definition{
		Name: "document_read",
		Description: "Read text from a PDF or DOCX file. PDF extraction is best effort. " +
			"DOCX paragraphs and tables are returned as text.",
		Effects: []Effect{EffectRead}, ParallelSafe: true, OutputLimit: 512 << 10,
	}, func(ctx context.Context, in input) (Result, error) {
		target, err := clarafs.ResolvePath(s.FS, in.Path)
		if err != nil {
			return ErrorResult(err.Error()), nil
		}
		document, err := office.Read(ctx, s.FS, target)
		if err != nil {
			return ErrorResult(err.Error()), nil
		}
		header := fmt.Sprintf("%s (%s", target, document.Kind)
		if document.Pages > 0 {
			header += fmt.Sprintf(", %d pages", document.Pages)
		}
		header += ")\n\n"
		return TextResult(header + document.Text), nil
	})
}

func (s *Documents) Create() Tool {
	type input struct {
		Path              string `json:"path" description:"output path ending in .pdf or .docx"`
		Title             string `json:"title,omitempty" description:"document title"`
		Content           string `json:"content" description:"document text"`
		ReadExistingFirst bool   `json:"read_existing_first,omitempty" description:"must be true to replace an existing document"`
	}
	return Func(Definition{
		Name: "document_create",
		Description: "Create a PDF or DOCX document from plain text. " +
			"Read an existing document before replacing it and set read_existing_first.",
		Effects: []Effect{EffectWrite},
	}, func(ctx context.Context, in input) (Result, error) {
		target, err := clarafs.ResolvePath(s.FS, in.Path)
		if err != nil {
			return ErrorResult(err.Error()), nil
		}
		if _, err := s.FS.Stat(ctx, target); err == nil && !in.ReadExistingFirst {
			return ErrorResult("document exists. Read it first and set read_existing_first=true."), nil
		}
		entry, err := office.Create(ctx, s.FS, target, in.Title, in.Content)
		if err != nil {
			return ErrorResult(err.Error()), nil
		}
		return TextResult(fmt.Sprintf("Created %s (%d bytes).", entry.Path, entry.Size)), nil
	})
}
