package office

import (
	"bytes"
	"context"
	"regexp"
	"testing"
	"time"

	clarafs "clara-code/pkg/tools/fs"
	"clara-code/pkg/tools/fs/memory"
)

func TestDOCXRoundTrip(t *testing.T) {
	ctx := context.Background()
	storage := memory.New()
	data, err := BuildDOCX("Title", "Hello\nWorld")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := storage.WriteFile(ctx, "/doc.docx", data, clarafs.WriteOptions{CreateDirs: true}); err != nil {
		t.Fatal(err)
	}
	document, err := Read(ctx, storage, "/doc.docx")
	if err != nil {
		t.Fatal(err)
	}
	if document.Kind != KindDOCX || !bytes.Contains([]byte(document.Text), []byte("Hello")) {
		t.Fatalf("document = %+v", document)
	}
}

func TestPDFContainsHeaderAndText(t *testing.T) {
	before := time.Now().UTC().Truncate(time.Second)
	data, err := BuildPDF("Title", "Hello PDF")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(data, []byte("%PDF-1.4")) {
		t.Fatalf("not a PDF: %q", data[:min(len(data), 16)])
	}
	if !bytes.Contains(data, []byte("Hello PDF")) {
		t.Fatal("PDF has no text stream")
	}
	stamp := regexp.MustCompile(`/CreationDate \(D:([0-9]{14}Z)\)`).FindSubmatch(data)
	if len(stamp) != 2 {
		t.Fatal("PDF creation date has no explicit UTC marker")
	}
	created, err := time.Parse("20060102150405Z", string(stamp[1]))
	if err != nil || created.Before(before) || created.After(time.Now().UTC()) {
		t.Fatalf("invalid PDF creation time: %s %v", stamp[1], err)
	}
}
