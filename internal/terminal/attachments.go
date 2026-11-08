package terminal

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"clara-code/internal/clipboard"
	"clara-code/internal/coding"
	"clara-code/internal/session"
	"clara-code/internal/setting"
)

type attachmentResult struct {
	items       []session.Attachment
	text, owner string
	fallback    string
	err         error
}

func (a *app) loadAttachments(paths []string, fromClipboard bool, original ...string) bool {
	if a.conv == nil {
		return false
	}
	if !fromClipboard && !a.conv.SupportsImages() {
		a.notice(errors.New("this model does not support image inputs; switch models before attaching images"))
		return false
	}
	if a.attachmentResults != nil {
		a.hint = "An image is already loading"
		return false
	}
	home, cwd, limits := a.host.Config.Home, a.space.Cwd, a.host.Config.Images.Defaults()
	owner := a.conv.Session.ID()
	out := make(chan attachmentResult, 1)
	a.attachmentResults = out
	ctx := a.ctx
	text := ""
	if len(original) > 0 {
		text = original[0]
	}
	if !a.editor.insertPending(text) {
		a.attachmentResults = nil
		a.notice(errors.New("not enough room in the input for an image"))
		return false
	}
	go func() {
		result := attachmentResult{owner: owner}
		if fromClipboard {
			content, err := clipboard.Read(ctx)
			if err != nil {
				result.err = err
				out <- result
				return
			}
			if len(content.Image) > 0 {
				item, err := coding.AttachBytes(ctx, home, "clipboard.png", content.Image, limits)
				result.err = err
				if err == nil {
					result.items = []session.Attachment{item}
				}
				out <- result
				return
			}
			result.fallback = content.Text
			if list, ok, err := coding.ImagePaths(content.Text, cwd); err != nil {
				result.err = err
			} else if ok {
				paths = list
			} else {
				result.text = content.Text
				out <- result
				return
			}
		}
		if len(paths) > limits.MaxAttachments {
			result.err = errors.New("too many image attachments")
			out <- result
			return
		}
		total := 0
		for _, path := range paths {
			item, err := coding.LoadAttachment(ctx, home, cwd, path, limits)
			if err != nil {
				result.err = err
				break
			}
			total += item.Size
			if total > limits.MaxTotalBytes {
				result.err = errors.New("images exceed the total attachment byte limit")
				break
			}
			result.items = append(result.items, item)
		}
		out <- result
	}()
	return true
}
func (a *app) acceptAttachments(result attachmentResult) {
	a.attachmentResults = nil
	if a.conv == nil || result.owner != a.conv.Session.ID() {
		return
	}
	pending := -1
	for i, span := range a.editor.images {
		if span.pending {
			pending = i
			break
		}
	}
	if pending < 0 {
		return
	}
	span := a.editor.images[pending]
	combined := append(append([]session.Attachment(nil), a.attachments...), result.items...)
	if result.err == nil && len(result.items) > 0 {
		_, result.err = coding.AttachmentFiles(a.host.Config.Home, combined, a.host.Config.Images)
		if result.err == nil && !a.conv.SupportsImages() {
			result.err = errors.New("this model does not support image inputs; switch models and attach again")
		}
	}
	if result.err == nil && len(result.items) > 0 {
		needed := len(a.editor.text) - (span.end - span.start)
		for _, current := range a.editor.images {
			if !current.pending {
				needed -= current.end - current.start
			}
		}
		for i := range len(combined) {
			needed += len(imageLabel(i))
		}
		if needed > inputLimit {
			result.err = errors.New("not enough room in the input for images")
		}
	}
	before := a.editor.at
	text := result.text
	if result.err != nil {
		text = span.original
		if text == "" {
			text = result.fallback
		}
	}
	a.editor.delete(span.start, span.end)
	a.editor.insert(text)
	if result.err == nil {
		for _, item := range result.items {
			a.editor.insertImage(item)
		}
	}
	end := a.editor.at
	if before <= span.start {
		a.editor.at = before
	} else if before >= span.end {
		a.editor.at = before + end - span.end
	}
	a.syncDraftImages()
	if result.err != nil {
		a.notice(result.err)
	}
}

func (a *app) manageAttachments() {
	p := &picker{kind: "attachments", title: "Image attachments · " + a.host.Config.TUI.Hint("picker", "delete_attachment") + " removes selected image"}
	for i, item := range a.attachments {
		p.items = append(p.items, choice{id: fmt.Sprint(i), label: fmt.Sprintf("[Image #%d] %s · %dx%d", i+1, item.Filename, item.Width, item.Height)})
	}
	if len(p.items) == 0 {
		a.hint = "No image attachments"
		return
	}
	a.picker = p
}
func (p *painter) attachmentHistory(items []session.Attachment) []string {
	var rows []string
	for i, item := range items {
		rows = append(rows, p.style(dim, truncate(fmt.Sprintf("  [Image #%d] %s · %dx%d", i+1, item.Filename, item.Width, item.Height), p.width)))
	}
	if len(rows) > 0 {
		rows = append(rows, "")
	}
	return rows
}
func (a *app) replayAttachments(items []session.Attachment) {
	if len(items) == 0 {
		return
	}
	copy := append([]session.Attachment(nil), items...)
	if a.view.cell(session.Cell{Kind: "attachments", Attachments: copy}) {
		a.reflow = true
	}
}
func (a *app) keyContext() string {
	if a.question != nil {
		return "question"
	}
	if a.picker != nil {
		if a.picker.kind == "rewind" {
			return "rewind"
		}
		return "picker"
	}
	return "composer"
}
func (a *app) configureUI() {
	t := a.host.Config.TUI
	a.view.markdown = t.Markdown
	a.view.previewLines = t.ToolOutputLines
	a.view.animate = t.Animations
	switch t.Color {
	case "", "auto":
		_, disabled := os.LookupEnv("NO_COLOR")
		a.view.color = !disabled
	case "always":
		a.view.color = true
	case "never":
		a.view.color = false
	}
}
func (a *app) hints(text string) string {
	if !setting.Enabled(a.host.Config.TUI.ShowTooltips) {
		return ""
	}
	context := a.keyContext()
	replacements := map[string]string{"Enter": a.host.Config.TUI.Hint(context, "confirm"), "Esc": a.host.Config.TUI.Hint(context, "cancel")}
	if context == "composer" {
		replacements["Enter"] = a.host.Config.TUI.Hint(context, "submit")
		replacements["Shift+Tab"] = a.host.Config.TUI.Hint(context, "cycle_mode")
		replacements["Alt+Enter"] = a.host.Config.TUI.Hint(context, "newline")
	}
	if context == "question" {
		replacements["Enter"] = a.host.Config.TUI.Hint(context, "submit")
		replacements["Tab"] = a.host.Config.TUI.Hint(context, "notes")
	}
	if context == "rewind" {
		replacements["Ctrl+T"] = a.host.Config.TUI.Hint(context, "details")
	}
	var pairs []string
	for _, old := range []string{"Shift+Tab", "Alt+Enter", "Ctrl+T", "Enter", "Esc", "Tab"} {
		if replacement := replacements[old]; replacement != "" {
			pairs = append(pairs, old, replacement)
		}
	}
	if context == "picker" {
		pairs = append(pairs, "↑/↓", a.host.Config.TUI.Hint(context, "previous")+"/"+a.host.Config.TUI.Hint(context, "next"))
	}
	if context == "rewind" {
		pairs = append(pairs, "←/→", a.host.Config.TUI.Hint(context, "previous")+"/"+a.host.Config.TUI.Hint(context, "next"), "↑/↓", a.host.Config.TUI.Hint(context, "scroll_up")+"/"+a.host.Config.TUI.Hint(context, "scroll_down"))
	}
	text = strings.NewReplacer(pairs...).Replace(text)
	return text
}
func (a *app) restoreDraft(text string, items []session.Attachment) {
	if a.editor.text == "" {
		a.setDraft(text, items)
		return
	}
	if len(a.attachments) == 0 {
		for _, item := range items {
			a.editor.insertImage(item)
		}
		a.syncDraftImages()
	}
}
func (a *app) bindKey(input key) key {
	if input.code != "text" && input.code != "paste" {
		input.code = a.host.Config.TUI.Dispatch(a.keyContext(), input.code)
	}
	return input
}
func (a *app) animationInterval() time.Duration {
	if !setting.Enabled(a.host.Config.TUI.Animations) {
		return time.Second
	}
	return 100 * time.Millisecond
}

func (a *app) questionHints(text string) string {
	if !setting.Enabled(a.host.Config.TUI.ShowTooltips) {
		return ""
	}
	for _, pair := range [][2]string{{"tab", a.host.Config.TUI.Hint("question", "notes")}, {"enter", a.host.Config.TUI.Hint("question", "submit")}, {"esc", a.host.Config.TUI.Hint("question", "cancel")}} {
		text = strings.ReplaceAll(text, pair[0], pair[1])
	}
	return text
}
