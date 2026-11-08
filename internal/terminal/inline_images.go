package terminal

import (
	"fmt"
	"slices"
	"strings"

	"clara-code/internal/session"
)

type imageSpan struct {
	start, end int
	attachment session.Attachment
	pending    bool
	original   string
}

func imageLabel(index int) string { return fmt.Sprintf("[Image #%d]", index+1) }

func (e *editor) insertImage(item session.Attachment) {
	start := e.at
	label := imageLabel(len(e.images))
	e.insert(label)
	if e.at-start != len(label) {
		return
	}
	e.images = append(e.images, imageSpan{start: start, end: e.at, attachment: item})
}

func (e *editor) insertPending(original string) bool {
	text := original
	if text == "" {
		text = "[Loading image…]"
	}
	if len(e.text)+len(clean(text)) > inputLimit {
		return false
	}
	start := e.at
	e.insert(text)
	if e.at-start == len(clean(text)) {
		e.images = append(e.images, imageSpan{start: start, end: e.at, pending: true, original: original})
	}
	return true
}

func (e *editor) imageAttachments() []session.Attachment {
	slices.SortFunc(e.images, func(a, b imageSpan) int { return a.start - b.start })
	var items []session.Attachment
	for i := range e.images {
		span := &e.images[i]
		if span.pending {
			continue
		}
		label := imageLabel(len(items))
		oldEnd := span.end
		delta := len(label) - (oldEnd - span.start)
		e.text = e.text[:span.start] + label + e.text[oldEnd:]
		span.end += delta
		if e.at >= oldEnd {
			e.at += delta
		}
		for j := i + 1; j < len(e.images); j++ {
			e.images[j].start += delta
			e.images[j].end += delta
		}
		items = append(items, span.attachment)
	}
	return items
}

func (a *app) syncDraftImages() { a.attachments = a.editor.imageAttachments() }

func (a *app) setDraft(text string, items []session.Attachment) {
	a.editor.restoreImages(text, items)
	a.syncDraftImages()
}

func (e *editor) restoreImages(text string, items []session.Attachment) {
	e.replace(text)
	offset := 0
	for i, item := range items {
		label := imageLabel(i)
		at := strings.Index(e.text[offset:], label)
		if at >= 0 {
			start := offset + at
			e.images = append(e.images, imageSpan{start: start, end: start + len(label), attachment: item})
			offset = start + len(label)
		} else {
			e.at = len(e.text)
			e.insertImage(item)
			offset = len(e.text)
		}
	}
	e.imageAttachments()
}

func (e *editor) appendDraft(other editor) bool {
	separator := ""
	if e.text != "" && other.text != "" {
		separator = "\n"
	}
	if len(e.text)+len(separator)+len(other.text) > inputLimit {
		return false
	}
	offset := len(e.text) + len(separator)
	e.text += separator + other.text
	for _, span := range other.images {
		span.start += offset
		span.end += offset
		e.images = append(e.images, span)
	}
	e.at = len(e.text)
	e.imageAttachments()
	return true
}

func (p *painter) imageInputRow(e editor, r row) string {
	var out strings.Builder
	at := r.start
	for _, span := range e.images {
		start, end := max(r.start, span.start), min(r.end, span.end)
		if start >= end {
			continue
		}
		out.WriteString(e.text[at:start])
		out.WriteString(p.style(accent, e.text[start:end]))
		at = end
	}
	out.WriteString(e.text[at:r.end])
	return out.String()
}
