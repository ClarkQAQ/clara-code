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

func (a *app) loadAttachments(paths []string, fromClipboard bool) {
	if a.conv == nil {
		return
	}
	if !fromClipboard && !a.conv.SupportsImages() {
		a.notice(errors.New("this model does not support image inputs; switch models before attaching images"))
		return
	}
	if a.attachmentResults != nil {
		a.hint = "An image is already loading"
		return
	}
	home, cwd, limits := a.host.Config.Home, a.space.Cwd, a.host.Config.Images.Defaults()
	owner := a.conv.Session.ID()
	out := make(chan attachmentResult, 1)
	a.attachmentResults = out
	ctx := a.ctx
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
}
func (a *app) acceptAttachments(result attachmentResult) {
	a.attachmentResults = nil
	pasteAfter, pasteBefore := a.attachmentPasteText, a.attachmentPasteBefore
	a.attachmentPasteText = ""
	a.attachmentPasteBefore = ""
	if a.conv == nil || result.owner != a.conv.Session.ID() {
		return
	}
	if result.err != nil {
		a.editor.insert(result.fallback)
		a.notice(result.err)
		return
	}
	if result.text != "" {
		a.editor.insert(result.text)
		return
	}
	combined := append(append([]session.Attachment(nil), a.attachments...), result.items...)
	if _, err := coding.AttachmentFiles(a.host.Config.Home, combined, a.host.Config.Images); err != nil {
		a.editor.insert(result.fallback)
		a.notice(err)
		return
	}
	if !a.conv.SupportsImages() {
		a.editor.insert(result.fallback)
		a.notice(errors.New("this model does not support image inputs; switch models and attach again"))
		return
	}
	a.attachments = combined
	if pasteAfter != "" && a.editor.text == pasteAfter {
		a.editor.replace(pasteBefore)
	}
	if len(result.items) > 0 {
		a.hint = fmt.Sprintf("%d image attachment(s) · %s to manage", len(a.attachments), a.host.Config.TUI.Hint("composer", "attachments"))
	}
}
func (a *app) attachmentRows() []string {
	var rows []string
	for i, item := range a.attachments {
		rows = append(rows, a.view.style(dim, truncate(fmt.Sprintf("  [Image #%d] %s · %dx%d", i+1, item.Filename, item.Width, item.Height), a.view.width)))
	}
	if a.attachmentResults != nil {
		rows = append(rows, a.view.style(dim, "  Loading image…"))
	}
	if len(rows) > 0 {
		rows = append(rows, "")
	}
	return rows
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
		a.editor.replace(text)
	}
	if len(a.attachments) == 0 {
		a.attachments = append([]session.Attachment(nil), items...)
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
