package coding

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"unicode/utf8"

	"clara-code/pkg/model"
	"clara-code/pkg/tool"
)

func (w Workspace) reviewTool() model.AgentTool {
	type input struct {
		Kind   string `json:"kind" enum:"staged,unstaged,untracked"`
		Offset int    `json:"offset,omitempty" description:"byte offset from the previous result"`
	}
	return tool.Func(tool.Definition{Name: "review_changes", Description: "Read a page of staged, unstaged or untracked Git changes.", Effects: []tool.Effect{tool.EffectRead}},
		func(ctx context.Context, in input) (tool.Result, error) {
			if in.Offset < 0 || in.Offset > 16<<20 {
				return tool.ErrorResult("invalid diff offset"), nil
			}
			text, more, err := w.DiffPage(ctx, in.Kind, in.Offset)
			if err != nil {
				return tool.ErrorResult(err.Error()), nil
			}
			if more {
				text += fmt.Sprintf("\n[More changes. Request offset %d.]", in.Offset+len(text))
			}
			if text == "" {
				text = "No " + in.Kind + " changes."
			}
			return tool.TextResult(text), nil
		})
}

const reviewPageSize = 24 << 10

var errReviewPageFull = errors.New("review page full")

type reviewPage struct {
	skip int
	body []byte
}

func (p *reviewPage) Write(data []byte) (int, error) {
	skip := min(p.skip, len(data))
	p.skip -= skip
	data = data[skip:]
	n := min(len(data), reviewPageSize+1-len(p.body))
	p.body = append(p.body, data[:n]...)
	if len(p.body) > reviewPageSize {
		return skip + n, errReviewPageFull
	}
	return skip + n, nil
}

func (w Workspace) DiffPage(ctx context.Context, kind string, offset int) (string, bool, error) {
	if !w.Git {
		return "", false, errors.New("/diff requires a Git working tree")
	}
	if offset < 0 || offset > 16<<20 {
		return "", false, errors.New("invalid diff offset")
	}
	switch kind {
	case "staged", "unstaged", "untracked":
	default:
		return "", false, errors.New("unknown diff section")
	}
	if err := ctx.Err(); err != nil {
		return "", false, err
	}
	repo, err := openGit(w.Root)
	if err != nil {
		return "", false, err
	}
	entries, err := readGitIndex(repo)
	if err != nil {
		return "", false, err
	}
	page := reviewPage{skip: offset}
	if kind == "staged" {
		err = stagedReview(ctx, repo, entries, &page)
	} else {
		var root *os.Root
		root, err = os.OpenRoot(w.Root)
		if err == nil {
			defer root.Close()
			if kind == "unstaged" {
				err = unstagedReview(ctx, repo, root, entries, &page)
			} else {
				var ignore gitIgnore
				ignore, err = readGitIgnore(repo, root)
				if err == nil {
					err = untrackedReview(ctx, ignore, entries, &page)
				}
			}
		}
	}
	if errors.Is(err, errReviewPageFull) {
		end := reviewPageSize
		for end > reviewPageSize-utf8.UTFMax && !utf8.RuneStart(page.body[end]) {
			end--
		}
		if !utf8.RuneStart(page.body[end]) {
			end = reviewPageSize
		}
		return string(page.body[:end]), true, ctx.Err()
	}
	if err != nil {
		return "", false, err
	}
	return string(page.body), false, ctx.Err()
}

func untrackedReview(ctx context.Context, ignore gitIgnore, entries gitIndex, out io.Writer) error {
	size := 0
	_, err := ignore.untracked(ctx, entries, ".", func(name string) error {
		var file strings.Builder
		fmt.Fprintf(&file, "diff --git a/%s b/%s\nnew file\n", reviewPath(name), reviewPath(name))
		info, err := ignore.root.Lstat(name)
		switch {
		case err != nil:
			file.WriteString("[unavailable]\n")
		case !info.Mode().IsRegular() || info.Size() > 128<<10:
			file.WriteString("[non-regular or larger than 128 KiB]\n")
		default:
			body, err := readWorktreeFile(ignore.root, name, 128<<10)
			switch {
			case err != nil || len(body) > 128<<10:
				file.WriteString("[unavailable]\n")
			case strings.ContainsRune(string(body), 0):
				file.WriteString("[binary file]\n")
			default:
				fmt.Fprintf(&file, "--- /dev/null\n+++ b/%s\n", reviewPath(name))
				for line := range strings.SplitSeq(strings.TrimSuffix(string(body), "\n"), "\n") {
					fmt.Fprintf(&file, "+%s\n", line)
				}
			}
		}
		size += file.Len()
		if size > 2<<20 {
			return errUntrackedReviewLimit
		}
		_, err = io.WriteString(out, file.String())
		return err
	})
	if errors.Is(err, errUntrackedReviewLimit) {
		_, err = io.WriteString(out, "\n[Additional untracked files omitted.]\n")
	}
	return err
}

var errUntrackedReviewLimit = errors.New("untracked review size limit")
