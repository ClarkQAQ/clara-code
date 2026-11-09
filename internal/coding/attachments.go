package coding

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"unicode"

	"clara-code/internal/session"
	"clara-code/internal/setting"
	"clara-code/pkg/model"
	claraimage "clara-code/pkg/tool/fs/image"
)

func ImagePaths(text string, cwd ...string) ([]string, bool, error) {
	raw := strings.TrimSpace(text)
	if raw == "" {
		return nil, false, nil
	}
	// A single existing path can contain spaces without shell quoting.
	if path, err := imagePath(raw); err == nil {
		target := path
		if !filepath.IsAbs(target) && len(cwd) > 0 {
			target = filepath.Join(cwd[0], target)
		}
		if _, err := os.Stat(target); err == nil && imageExtension(path) {
			return []string{path}, true, nil
		}
	}
	var parts []string
	var current strings.Builder
	var quote rune
	escaped := false
	for _, r := range raw {
		if escaped {
			current.WriteRune(r)
			escaped = false
			continue
		}
		if r == '\\' && quote != '\'' {
			escaped = true
			continue
		}
		if quote != 0 {
			if r == quote {
				quote = 0
			} else {
				current.WriteRune(r)
			}
			continue
		}
		if r == '\'' || r == '"' {
			quote = r
			continue
		}
		if unicode.IsSpace(r) {
			if current.Len() > 0 {
				parts = append(parts, current.String())
				current.Reset()
			}
		} else {
			current.WriteRune(r)
		}
	}
	if escaped || quote != 0 {
		return nil, false, nil
	}
	if current.Len() > 0 {
		parts = append(parts, current.String())
	}
	if len(parts) == 0 {
		return nil, false, nil
	}
	var paths []string
	for _, part := range parts {
		path, err := imagePath(part)
		if err != nil {
			return nil, true, err
		}
		if !imageExtension(path) {
			return nil, false, nil
		}
		paths = append(paths, path)
	}
	return paths, true, nil
}
func imageExtension(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".png", ".jpg", ".jpeg", ".gif", ".webp", ".bmp", ".tif", ".tiff":
		return true
	}
	return false
}
func imagePath(raw string) (string, error) {
	path := strings.Trim(raw, "\"'")
	if strings.HasPrefix(path, "file://") {
		u, err := url.Parse(path)
		if err != nil || u.Host != "" && u.Host != "localhost" {
			return "", errors.New("only local file:// image paths are supported")
		}
		path = u.Path
	}
	if strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		path = filepath.Join(home, path[2:])
	}
	return path, nil
}
func LoadAttachment(ctx context.Context, home, cwd, path string, limits setting.Images) (session.Attachment, error) {
	path, err := imagePath(path)
	if err != nil {
		return session.Attachment{}, err
	}
	if !filepath.IsAbs(path) {
		path = filepath.Join(cwd, path)
	}
	limits = limits.Defaults()
	info, err := os.Stat(path)
	if err != nil {
		return session.Attachment{}, err
	}
	if !info.Mode().IsRegular() {
		return session.Attachment{}, errors.New("image must be a regular file")
	}
	file, err := os.Open(path)
	if err != nil {
		return session.Attachment{}, fmt.Errorf("cannot open image %s: %w", path, err)
	}
	defer file.Close()
	info, err = file.Stat()
	if err != nil {
		return session.Attachment{}, err
	}
	if !info.Mode().IsRegular() {
		return session.Attachment{}, errors.New("image must be a regular file")
	}
	if err := ctx.Err(); err != nil {
		return session.Attachment{}, err
	}
	data, err := io.ReadAll(io.LimitReader(file, limits.MaxSourceBytes+1))
	if err != nil {
		return session.Attachment{}, err
	}
	return AttachBytes(ctx, home, filepath.Base(path), data, limits)
}
func imageOptions(limits setting.Images) claraimage.Options {
	return claraimage.Options{
		MaxBytes: limits.MaxSourceBytes, MaxDimension: limits.MaxDimension, MaxOutputPixels: limits.MaxPixels,
	}
}

func AttachBytes(ctx context.Context, home, name string, data []byte, limits setting.Images) (session.Attachment, error) {
	if err := ctx.Err(); err != nil {
		return session.Attachment{}, err
	}
	limits = limits.Defaults()
	view, err := claraimage.Normalize(data, name, imageOptions(limits))
	if err != nil {
		return session.Attachment{}, err
	}
	if err := ctx.Err(); err != nil {
		return session.Attachment{}, err
	}
	if limits.ExceedsTotalBytes(len(view.Data)) {
		return session.Attachment{}, errors.New("normalized image exceeds the attachment byte limit")
	}
	return session.SaveAttachment(home, session.Attachment{Filename: name, MediaType: view.MIME, Width: view.Width, Height: view.Height}, view.Data)
}
func AttachmentFiles(home string, attachments []session.Attachment, limits setting.Images) ([]model.FilePart, error) {
	limits = limits.Defaults()
	if limits.ExceedsAttachmentCount(len(attachments)) {
		return nil, errors.New("too many image attachments")
	}
	var files []model.FilePart
	total := 0
	for _, a := range attachments {
		file, err := a.File(home)
		if err != nil {
			return nil, err
		}
		total += len(file.Data)
		if limits.ExceedsTotalBytes(total) {
			return nil, errors.New("images exceed the total attachment byte limit")
		}
		files = append(files, file)
	}
	return files, nil
}

func AttachmentsFromFiles(home string, files []model.FilePart) ([]session.Attachment, error) {
	var out []session.Attachment
	for _, f := range files {
		a, err := session.SaveAttachment(home, session.Attachment{Filename: f.Filename, MediaType: f.MediaType}, f.Data)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, nil
}
func (c *Conversation) PrepareImages(ctx context.Context, messages []model.Message) ([]model.Message, error) {
	out := make([]model.Message, len(messages))
	limits := c.Host.Config.Images.Defaults()
	count, total := 0, 0
	for i, m := range messages {
		out[i] = m
		out[i].Content = append([]model.MessagePart(nil), m.Content...)
		for j, part := range m.Content {
			file, ok := model.AsMessagePart[model.FilePart](part)
			if !ok || !strings.HasPrefix(file.MediaType, "image/") {
				continue
			}
			if !c.SupportsImages() {
				return nil, errors.New("this model does not support image inputs; remove images or switch models")
			}
			a, err := AttachBytes(ctx, c.Host.Config.Home, file.Filename, file.Data, limits)
			if err != nil {
				return nil, err
			}
			f, err := a.File(c.Host.Config.Home)
			if err != nil {
				return nil, err
			}
			f.ProviderOptions = file.ProviderOptions
			count++
			total += len(f.Data)
			if limits.ExceedsAttachmentCount(count) || limits.ExceedsTotalBytes(total) {
				return nil, errors.New("image attachment limits exceeded")
			}
			out[i].Content[j] = f
		}
	}
	return out, nil
}

func fileParts(files []model.FilePart) []model.MessagePart {
	parts := make([]model.MessagePart, len(files))
	for i, file := range files {
		parts[i] = file
	}
	return parts
}

func (c *Conversation) prepareFiles(ctx context.Context, files []model.FilePart) ([]model.FilePart, error) {
	if len(files) == 0 {
		return files, nil
	}
	prepared, err := c.PrepareImages(ctx, []model.Message{{Role: model.MessageRoleUser, Content: fileParts(files)}})
	if err != nil {
		return nil, err
	}
	out := make([]model.FilePart, 0, len(files))
	for _, part := range prepared[0].Content {
		if file, ok := model.AsMessagePart[model.FilePart](part); ok {
			out = append(out, file)
		}
	}
	return out, nil
}
