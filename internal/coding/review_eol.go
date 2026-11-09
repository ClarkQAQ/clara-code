package coding

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/format/gitattributes"
)

type reviewEOL struct {
	root     *os.Root
	autocrlf bool
	dirs     map[string][]gitattributes.MatchAttribute
}

func (e *reviewEOL) patterns(dir string, domain []string) ([]gitattributes.MatchAttribute, error) {
	if cached, ok := e.dirs[dir]; ok {
		return cached, nil
	}
	name := filepath.Join(dir, ".gitattributes")
	body, err := readWorktreeFile(e.root, name, 64<<10)
	if errors.Is(err, fs.ErrNotExist) {
		err = nil
	}
	if err != nil {
		// Git ignores symlinked .gitattributes files.
		if info, statErr := e.root.Lstat(name); statErr == nil && !info.Mode().IsRegular() {
			return nil, nil
		}
		return nil, err
	}
	if len(body) > 64<<10 {
		return nil, fmt.Errorf("%s exceeds 64 KiB", name)
	}
	patterns, err := gitattributes.ReadAttributes(bytes.NewReader(body), domain, dir == "")
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", name, err)
	}
	if len(e.dirs) < 128 {
		e.dirs[dir] = patterns
	}
	return patterns, nil
}

type reviewTextMode uint8

const (
	reviewRaw reviewTextMode = iota
	reviewForceText
	reviewAuto
)

func (e *reviewEOL) mode(name string) (reviewTextMode, error) {
	parts := strings.Split(name, "/")
	var text, eol gitattributes.Attribute
	var binary bool
	for depth := range parts {
		patterns, err := e.patterns(filepath.Join(parts[:depth]...), parts[:depth])
		if err != nil {
			return reviewRaw, err
		}
		for _, pattern := range patterns {
			if pattern.Pattern == nil || !pattern.Pattern.Match(parts) {
				continue
			}
			for _, attr := range pattern.Attributes {
				switch attr.Name() {
				case "binary":
					if attr.IsSet() {
						binary, text = true, nil
					}
				case "text":
					text, binary = attr, false
				case "eol":
					eol = attr
				}
			}
		}
	}
	if binary || text != nil && text.IsUnset() {
		return reviewRaw, nil
	}
	explicitEOL := eol != nil && (eol.Value() == "lf" || eol.Value() == "crlf")
	switch {
	case text != nil && text.Value() == "auto":
		return reviewAuto, nil
	case text != nil && text.IsSet(), explicitEOL:
		return reviewForceText, nil
	case e.autocrlf:
		return reviewAuto, nil
	default:
		return reviewRaw, nil
	}
}

func (e *reviewEOL) normalize(name string, before, after []byte) ([]byte, error) {
	if !bytes.Contains(after, []byte("\r\n")) {
		return after, nil
	}
	mode, err := e.mode(name)
	if err != nil || mode == reviewRaw {
		return after, err
	}
	if mode == reviewAuto && (bytes.IndexByte(after, 0) >= 0 || bytes.Contains(before, []byte("\r\n")) || hasLoneCR(after)) {
		return after, nil
	}
	return bytes.ReplaceAll(after, []byte("\r\n"), []byte("\n")), nil
}

// normalizeLarge computes a filtered blob hash without keeping the worktree
// file in memory. The patch still uses the existing large-content notice.
func (e *reviewEOL) normalizeLarge(ctx context.Context, repo *git.Repository, from, to reviewFile) (plumbing.Hash, error) {
	mode, err := e.mode(to.name)
	if err != nil || mode == reviewRaw {
		return to.hash, err
	}
	file, err := openWorktreeFile(e.root, filepath.FromSlash(to.name))
	if err != nil {
		return to.hash, err
	}
	defer file.Close()
	before, err := file.Stat()
	if err != nil {
		return to.hash, err
	}
	scan, err := streamReviewEOL(ctx, file, nil)
	if err != nil || !scan.crlf || (mode == reviewAuto && scan.binary) {
		return to.hash, err
	}
	if mode == reviewAuto && from.mode != 0 && !from.hash.IsZero() {
		blob, err := repo.BlobObject(from.hash)
		if err != nil {
			return to.hash, err
		}
		reader, err := blob.Reader()
		if err != nil {
			return to.hash, err
		}
		base, readErr := streamReviewEOL(ctx, reader, nil)
		if err := errors.Join(readErr, reader.Close()); err != nil {
			return to.hash, err
		}
		if base.crlf {
			return to.hash, nil
		}
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return to.hash, err
	}
	hash := plumbing.NewHasher(plumbing.BlobObject, scan.size)
	written, err := streamReviewEOL(ctx, file, hash)
	if err != nil {
		return to.hash, err
	}
	after, err := file.Stat()
	if err != nil {
		return to.hash, err
	}
	if written != scan || before.Size() != after.Size() || !before.ModTime().Equal(after.ModTime()) {
		return to.hash, fmt.Errorf("worktree file %q changed during review", to.name)
	}
	return hash.Sum(), nil
}

type reviewEOLScan struct {
	size         int64
	crlf, binary bool
}

func streamReviewEOL(ctx context.Context, input io.Reader, output io.Writer) (reviewEOLScan, error) {
	var result reviewEOLScan
	var buf [64<<10 + 1]byte
	pending := 0
	for {
		if err := ctx.Err(); err != nil {
			return result, err
		}
		n, err := input.Read(buf[pending:])
		chunk := buf[:n+pending]
		pending = 0
		if err == nil && len(chunk) > 0 && chunk[len(chunk)-1] == '\r' {
			pending = 1
			chunk = chunk[:len(chunk)-1]
		}
		pairs := bytes.Count(chunk, []byte("\r\n"))
		result.crlf = result.crlf || pairs > 0
		result.binary = result.binary || bytes.IndexByte(chunk, 0) >= 0 || hasLoneCR(chunk)
		result.size += int64(len(chunk) - pairs)
		if output != nil {
			if _, writeErr := output.Write(bytes.ReplaceAll(chunk, []byte("\r\n"), []byte("\n"))); writeErr != nil {
				return result, writeErr
			}
		}
		if pending != 0 {
			buf[0] = '\r'
		}
		if err != nil {
			if errors.Is(err, io.EOF) {
				return result, nil
			}
			return result, err
		}
	}
}

func hasLoneCR(body []byte) bool {
	for i, char := range body {
		if char == '\r' && (i+1 == len(body) || body[i+1] != '\n') {
			return true
		}
	}
	return false
}
