package coding

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"unicode/utf8"

	"clara-code/pkg/models"
	"clara-code/pkg/tools"
	corefs "clara-code/pkg/tools/fs"
)

func (w Workspace) reviewTool() models.AgentTool {
	type input struct {
		Kind   string `json:"kind" enum:"staged,unstaged,untracked"`
		Offset int    `json:"offset,omitempty" description:"byte offset from the previous result"`
	}
	return tools.Func(tools.Definition{Name: "review_changes", Description: "Read a page of staged, unstaged or untracked Git changes.", Effects: []tools.Effect{tools.EffectRead}},
		func(ctx context.Context, in input) (tools.Result, error) {
			if in.Offset < 0 || in.Offset > 16<<20 {
				return tools.ErrorResult("invalid diff offset"), nil
			}
			text, more, err := w.DiffPage(ctx, in.Kind, in.Offset)
			if err != nil {
				return tools.ErrorResult(err.Error()), nil
			}
			if more {
				text += fmt.Sprintf("\n[More changes. Request offset %d.]", in.Offset+len(text))
			}
			if text == "" {
				text = "No " + in.Kind + " changes."
			}
			return tools.TextResult(text), nil
		})
}

func gitPage(ctx context.Context, args []string, offset int) (string, bool, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	var stderr strings.Builder
	cmd.Stderr = &stderr
	out, err := cmd.StdoutPipe()
	if err != nil {
		return "", false, err
	}
	if err := cmd.Start(); err != nil {
		return "", false, err
	}
	_, skipErr := io.CopyN(io.Discard, out, int64(offset))
	page, readErr := io.ReadAll(io.LimitReader(out, 24<<10+1))
	if len(page) > 24<<10 && readErr == nil {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		end := 24 << 10
		for end > 0 && !utf8.RuneStart(page[end]) {
			end--
		}
		return string(page[:end]), true, nil
	}
	waitErr := cmd.Wait()
	if errors.Is(skipErr, io.EOF) {
		skipErr = nil
	}
	if err := errors.Join(skipErr, readErr, waitErr); err != nil {
		return "", false, fmt.Errorf("git: %w %s", err, strings.TrimSpace(stderr.String()))
	}
	return string(page), false, nil
}

func (w Workspace) DiffPage(ctx context.Context, kind string, offset int) (string, bool, error) {
	if !w.Git {
		return "", false, errors.New("/diff requires a Git working tree")
	}
	if offset < 0 || offset > 16<<20 {
		return "", false, errors.New("invalid diff offset")
	}
	args := []string{"--no-pager", "--no-optional-locks", "-c", "core.fsmonitor=false", "-C", w.Root}
	switch kind {
	case "staged":
		return gitPage(ctx, append(args, "diff", "--no-color", "--no-ext-diff", "--no-textconv", "--cached", "--"), offset)
	case "unstaged":
		return gitPage(ctx, append(args, "diff", "--no-color", "--no-ext-diff", "--no-textconv", "--"), offset)
	case "untracked":
		return w.untrackedDiff(ctx, args, offset)
	default:
		return "", false, errors.New("unknown diff section")
	}
}

func (w Workspace) untrackedDiff(ctx context.Context, args []string, offset int) (string, bool, error) {
	names, more, err := gitPage(ctx, append(args, "ls-files", "-z", "--others", "--exclude-standard", "--"), 0)
	if err != nil {
		return "", false, err
	}
	if more && !strings.HasSuffix(names, "\x00") {
		if at := strings.LastIndexByte(names, 0); at >= 0 {
			names = names[:at+1]
		} else {
			names = ""
		}
	}
	var out strings.Builder
	for _, name := range strings.Split(names, "\x00") {
		if ctx.Err() != nil {
			return "", false, ctx.Err()
		}
		if name == "" {
			continue
		}
		if out.Len() > 2<<20 {
			out.WriteString("\n[Additional untracked files omitted.]\n")
			break
		}
		path, err := corefs.Clean(name)
		if err != nil {
			continue
		}
		info, err := w.FS.Stat(ctx, path)
		out.WriteString("diff --git a/" + name + " b/" + name + "\nnew file\n")
		if err != nil {
			out.WriteString("[unavailable]\n")
			continue
		}
		if !info.Mode.IsRegular() || info.Size > 128<<10 {
			out.WriteString("[non-regular or larger than 128 KiB]\n")
			continue
		}
		body, err := w.FS.ReadFile(ctx, path, 128<<10)
		if err != nil {
			out.WriteString("[unavailable]\n")
			continue
		}
		if strings.ContainsRune(string(body), 0) {
			out.WriteString("[binary file]\n")
			continue
		}
		out.WriteString("--- /dev/null\n+++ b/" + name + "\n")
		for _, line := range strings.Split(strings.TrimSuffix(string(body), "\n"), "\n") {
			out.WriteString("+" + line + "\n")
		}
	}
	if more {
		out.WriteString("\n[More untracked paths omitted.]\n")
	}
	text := out.String()
	if offset >= len(text) {
		return "", false, nil
	}
	end := min(len(text), offset+24<<10)
	for end < len(text) && end > offset && !utf8.RuneStart(text[end]) {
		end--
	}
	return text[offset:end], end < len(text), nil
}
