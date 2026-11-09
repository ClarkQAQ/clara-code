// Package clipboard reads the local system clipboard without changing it.
package clipboard

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"time"
)

type Content struct {
	Image []byte
	Text  string
}
type Runner func(context.Context, string, ...string) ([]byte, error)

func command(ctx context.Context, name string, args ...string) ([]byte, error) {
	var output limitedBuffer
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Stdout = &output
	cmd.Stderr = io.Discard
	if err := cmd.Run(); err != nil {
		return nil, err
	}
	return output.Bytes(), nil
}

type limitedBuffer struct{ bytes.Buffer }

func (b *limitedBuffer) Write(p []byte) (int, error) {
	if b.Len()+len(p) > 24<<20 {
		return 0, errors.New("clipboard exceeds 24 MiB")
	}
	return b.Buffer.Write(p)
}
func Read(ctx context.Context) (Content, error) {
	ctx, cancel := context.WithTimeout(ctx, 4*time.Second)
	defer cancel()
	return readPlatform(ctx)
}
func ReadWith(ctx context.Context, platform string, wayland bool, run Runner) (Content, error) {
	switch platform {
	case "darwin":

		data, err := run(ctx, "/usr/bin/osascript", "-l", "JavaScript", "-e", darwinImageScript)
		if err == nil && len(bytes.TrimSpace(data)) > 0 {
			decoded, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(data)))
			if err != nil {
				return Content{}, errors.New("invalid clipboard image encoding")
			}
			return Content{Image: decoded}, nil
		}
		if ctx.Err() != nil {
			return Content{}, ctx.Err()
		}
		text, textErr := run(ctx, "/usr/bin/pbpaste")
		if textErr == nil {
			return Content{Text: string(text)}, nil
		}
		return Content{}, errors.New("system clipboard is unavailable; use /attach PATH")
	case "linux":
		name := "xclip"
		imageArgs := []string{"-selection", "clipboard", "-t", "image/png", "-o"}
		textArgs := []string{"-selection", "clipboard", "-t", "UTF8_STRING", "-o"}
		if wayland {
			name = "wl-paste"
			imageArgs = []string{"--no-newline", "--type", "image/png"}
			textArgs = []string{"--no-newline", "--type", "text"}
		}
		image, err := run(ctx, name, imageArgs...)
		if err == nil && len(image) > 0 {
			return Content{Image: image}, nil
		}
		if ctx.Err() != nil {
			return Content{}, ctx.Err()
		}
		text, err := run(ctx, name, textArgs...)
		if err == nil {
			return Content{Text: string(text)}, nil
		}
		return Content{}, fmt.Errorf("clipboard unavailable; install %s for your display or use /attach PATH", name)
	default:
		return Content{}, errors.New("system clipboard is unavailable; use /attach PATH")
	}
}

const darwinImageScript = `ObjC.import('AppKit'); var p=$.NSPasteboard.generalPasteboard; var d=p.dataForType($.NSPasteboardTypePNG); if(d.isNil()){d=p.dataForType($.NSPasteboardTypeTIFF);if(!d.isNil()){var b=$.NSBitmapImageRep.imageRepWithData(d);d=b.representationUsingTypeProperties($.NSBitmapImageFileTypePNG,$());}}; d.isNil() ? '' : ObjC.unwrap(d.base64EncodedStringWithOptions(0));`
