// Package office reads and creates PDF and DOCX documents.
package office

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"path"
	"strings"
	"time"

	clarafs "clara-code/pkg/tools/fs"
	"clara-code/pkg/util/errs"
	"github.com/ledongthuc/pdf"
)

// Kind identifies a supported document format.
type Kind string

const (
	KindPDF  Kind = "pdf"
	KindDOCX Kind = "docx"
)

// Document is extracted document content.
type Document struct {
	Kind     Kind
	Title    string
	Text     string
	Pages    int
	Metadata map[string]string
}

// Read extracts text from a PDF or DOCX file.
func Read(ctx context.Context, storage clarafs.FS, value clarafs.Path) (Document, error) {
	if err := ctx.Err(); err != nil {
		return Document{}, err
	}
	entry, err := storage.Stat(ctx, value)
	if err != nil {
		return Document{}, err
	}
	if entry.Size > 64<<20 {
		return Document{}, errs.New(errs.CodeLimit, "document is larger than 64 MiB")
	}
	data, err := storage.ReadFile(ctx, value, 64<<20)
	if err != nil {
		return Document{}, err
	}
	switch detectKind(value.String(), data) {
	case KindPDF:
		return readPDF(data)
	case KindDOCX:
		return readDOCX(data)
	default:
		return Document{}, errs.New(errs.CodeUnsupported, "document must be a PDF or DOCX file")
	}
}

// Create writes a new document from plain text or Markdown-like content.
func Create(ctx context.Context, storage clarafs.FS, value clarafs.Path, title, text string) (clarafs.Entry, error) {
	var (
		data []byte
		err  error
	)
	switch strings.ToLower(path.Ext(value.String())) {
	case ".pdf":
		data, err = BuildPDF(title, text)
	case ".docx":
		data, err = BuildDOCX(title, text)
	default:
		return clarafs.Entry{}, errs.New(errs.CodeUnsupported, "output must end in .pdf or .docx")
	}
	if err != nil {
		return clarafs.Entry{}, err
	}
	return storage.WriteFile(ctx, value, data, clarafs.WriteOptions{
		Mode: 0o644, CreateDirs: true, Overwrite: true,
	})
}

func detectKind(name string, data []byte) Kind {
	if bytes.HasPrefix(data, []byte("%PDF-")) {
		return KindPDF
	}
	if bytes.HasPrefix(data, []byte("PK\x03\x04")) && strings.EqualFold(path.Ext(name), ".docx") {
		return KindDOCX
	}
	switch strings.ToLower(path.Ext(name)) {
	case ".pdf":
		return KindPDF
	case ".docx":
		return KindDOCX
	default:
		return ""
	}
}

func readPDF(data []byte) (Document, error) {
	reader, err := pdf.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return Document{}, fmt.Errorf("read PDF: %w", err)
	}
	plain, err := reader.GetPlainText()
	if err != nil {
		return Document{}, fmt.Errorf("extract PDF text: %w", err)
	}
	text, err := io.ReadAll(io.LimitReader(plain, 16<<20))
	if err != nil {
		return Document{}, err
	}
	return Document{Kind: KindPDF, Text: string(text), Pages: reader.NumPage()}, nil
}

func readDOCX(data []byte) (Document, error) {
	archive, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return Document{}, fmt.Errorf("read DOCX: %w", err)
	}
	var documentXML []byte
	metadata := map[string]string{}
	for _, file := range archive.File {
		switch file.Name {
		case "word/document.xml":
			documentXML, err = readZipFile(file, 32<<20)
			if err != nil {
				return Document{}, err
			}
		case "docProps/core.xml":
			raw, readErr := readZipFile(file, 1<<20)
			if readErr == nil {
				metadata["core_xml"] = string(raw)
			}
		}
	}
	if len(documentXML) == 0 {
		return Document{}, errors.New("DOCX has no word/document.xml")
	}
	text, err := extractWordText(documentXML)
	if err != nil {
		return Document{}, err
	}
	title := metadata["title"]
	return Document{Kind: KindDOCX, Title: title, Text: text, Metadata: metadata}, nil
}

func readZipFile(file *zip.File, limit int64) ([]byte, error) {
	reader, err := file.Open()
	if err != nil {
		return nil, err
	}
	defer reader.Close()
	return io.ReadAll(io.LimitReader(reader, limit+1))
}

func extractWordText(data []byte) (string, error) {
	decoder := xml.NewDecoder(bytes.NewReader(data))
	var builder strings.Builder
	inText := false
	for {
		token, err := decoder.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return "", err
		}
		switch value := token.(type) {
		case xml.StartElement:
			if value.Name.Local == "t" {
				inText = true
			}
			if value.Name.Local == "tab" {
				builder.WriteByte('\t')
			}
		case xml.EndElement:
			if value.Name.Local == "t" {
				inText = false
			}
			if value.Name.Local == "p" {
				builder.WriteByte('\n')
			}
		case xml.CharData:
			if inText {
				builder.Write(value)
			}
		}
	}
	return builder.String(), nil
}

// BuildDOCX creates a minimal standards-compliant Word document.
func BuildDOCX(title, text string) ([]byte, error) {
	var buffer bytes.Buffer
	archive := zip.NewWriter(&buffer)
	files := map[string]string{
		"[Content_Types].xml": `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">
<Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/>
<Default Extension="xml" ContentType="application/xml"/>
<Override PartName="/word/document.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml"/>
</Types>`,
		"_rels/.rels": `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">
<Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="word/document.xml"/>
</Relationships>`,
		"word/document.xml": buildDocumentXML(title, text),
	}
	for name, value := range files {
		writer, err := archive.Create(name)
		if err != nil {
			return nil, err
		}
		if _, err := io.WriteString(writer, value); err != nil {
			return nil, err
		}
	}
	if err := archive.Close(); err != nil {
		return nil, err
	}
	return buffer.Bytes(), nil
}

func buildDocumentXML(title, text string) string {
	var body strings.Builder
	if strings.TrimSpace(title) != "" {
		body.WriteString(paragraphXML(title, true))
	}
	for _, line := range strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n") {
		body.WriteString(paragraphXML(line, false))
	}
	return `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body>` +
		body.String() + `<w:sectPr/></w:body></w:document>`
}

func paragraphXML(text string, heading bool) string {
	style := ""
	if heading {
		style = `<w:pPr><w:pStyle w:val="Heading1"/></w:pPr>`
	}
	return `<w:p>` + style + `<w:r><w:t xml:space="preserve">` + escapeXML(text) + `</w:t></w:r></w:p>`
}

func escapeXML(value string) string {
	var buffer bytes.Buffer
	_ = xml.EscapeText(&buffer, []byte(value))
	return buffer.String()
}

// BuildPDF creates a simple multi-page text PDF.
func BuildPDF(title, text string) ([]byte, error) {
	lines := []string{}
	if strings.TrimSpace(title) != "" {
		lines = append(lines, title, "")
	}
	lines = append(lines, strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")...)
	const linesPerPage = 48
	pages := chunkLines(lines, linesPerPage)
	if len(pages) == 0 {
		pages = [][]string{{}}
	}
	objects := make([][]byte, 0, 4+2*len(pages))
	objects = append(objects, []byte("<< /Type /Catalog /Pages 2 0 R >>"))
	kids := make([]string, 0, len(pages))
	firstPageObject := 5
	for index := range pages {
		kids = append(kids, fmt.Sprintf("%d 0 R", firstPageObject+index*2))
	}
	objects = append(objects, []byte(fmt.Sprintf("<< /Type /Pages /Kids [%s] /Count %d >>", strings.Join(kids, " "), len(pages))))
	objects = append(objects, []byte("<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>"))
	objects = append(objects, []byte(fmt.Sprintf("<< /CreationDate (D:%sZ) >>", time.Now().UTC().Format("20060102150405"))))
	for index, page := range pages {
		pageObject := firstPageObject + index*2
		contentObject := pageObject + 1
		objects = append(objects, []byte(fmt.Sprintf(
			"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << /Font << /F1 3 0 R >> >> /Contents %d 0 R >>",
			contentObject,
		)))
		stream := pdfTextStream(page)
		objects = append(objects, []byte(fmt.Sprintf("<< /Length %d >>\nstream\n%sendstream", len(stream), stream)))
	}
	var output bytes.Buffer
	output.WriteString("%PDF-1.4\n%\xe2\xe3\xcf\xd3\n")
	offsets := make([]int, len(objects)+1)
	for index, object := range objects {
		offsets[index+1] = output.Len()
		fmt.Fprintf(&output, "%d 0 obj\n", index+1)
		output.Write(object)
		output.WriteString("\nendobj\n")
	}
	xref := output.Len()
	fmt.Fprintf(&output, "xref\n0 %d\n0000000000 65535 f \n", len(objects)+1)
	for index := 1; index < len(offsets); index++ {
		fmt.Fprintf(&output, "%010d 00000 n \n", offsets[index])
	}
	fmt.Fprintf(&output, "trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", len(objects)+1, xref)
	return output.Bytes(), nil
}

func chunkLines(lines []string, size int) [][]string {
	var pages [][]string
	for len(lines) > 0 {
		count := min(size, len(lines))
		pages = append(pages, lines[:count])
		lines = lines[count:]
	}
	return pages
}

func pdfTextStream(lines []string) string {
	var builder strings.Builder
	builder.WriteString("BT\n/F1 11 Tf\n14 TL\n50 742 Td\n")
	for _, line := range lines {
		builder.WriteString("(" + escapePDF(line) + ") Tj\nT*\n")
	}
	builder.WriteString("ET\n")
	return builder.String()
}

func escapePDF(value string) string {
	value = strings.Map(func(r rune) rune {
		if r < 32 || r > 126 {
			return '?'
		}
		return r
	}, value)
	value = strings.ReplaceAll(value, "\\", "\\\\")
	value = strings.ReplaceAll(value, "(", "\\(")
	value = strings.ReplaceAll(value, ")", "\\)")
	return value
}

// PageCount estimates PDF pages from a document.
func PageCount(document Document) int {
	if document.Pages > 0 {
		return document.Pages
	}
	if document.Text == "" {
		return 0
	}
	return 1 + strings.Count(document.Text, "\f")
}
