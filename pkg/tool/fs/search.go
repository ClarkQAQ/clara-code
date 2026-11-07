package fs

import (
	"bufio"
	"context"
	"fmt"
	"path"
	"sort"
	"strings"
	"unicode/utf8"

	"clara-code/pkg/util/errs"
)

// SearchDiagnostics distinguishes complete searches from partial results.
type SearchDiagnostics struct {
	Truncated bool
	Skipped   int
	Warnings  []string
}

func (d *SearchDiagnostics) warn(message string) {
	d.Truncated = true
	if len(d.Warnings) < 100 {
		d.Warnings = append(d.Warnings, message)
	}
}

// GlobOptions controls file discovery. Walk bounds traversal independently of results.
type GlobOptions struct {
	Pattern    string
	MaxResults int
	Walk       WalkOptions
}

type GlobResult struct {
	Entries []Entry
	SearchDiagnostics
}

type SearchResult struct {
	Hits []SearchHit
	SearchDiagnostics
}

type filePattern struct {
	parts              []string
	absolute, basename bool
}

func compilePattern(pattern string) (filePattern, error) {
	if pattern == "" || len(pattern) > 4096 || strings.IndexByte(pattern, 0) >= 0 {
		return filePattern{}, errs.New(errs.CodeInvalidArgument, "glob pattern must contain 1 to 4096 bytes")
	}
	p := filePattern{absolute: strings.HasPrefix(pattern, "/"), basename: !strings.Contains(pattern, "/")}
	p.parts = strings.Split(strings.TrimPrefix(pattern, "/"), "/")
	if len(p.parts) > 64 {
		return filePattern{}, errs.New(errs.CodeInvalidArgument, "glob pattern has more than 64 components")
	}
	for _, part := range p.parts {
		if part == "" || part == "." || part == ".." {
			return filePattern{}, errs.New(errs.CodeInvalidArgument, "glob pattern contains an empty, . or .. component")
		}
		if _, err := path.Match(part, ""); err != nil {
			return filePattern{}, errs.Wrap(errs.CodeInvalidArgument, "invalid glob pattern "+pattern, err)
		}
	}
	return p, nil
}

func (p filePattern) matches(root Path, entry Entry) bool {
	value := strings.TrimPrefix(entry.Path.String(), "/")
	if p.basename {
		value = entry.Name
	} else if !p.absolute && root != "/" {
		value = strings.TrimPrefix(entry.Path.String(), strings.TrimRight(root.String(), "/")+"/")
		if entry.Path == root {
			value = entry.Name
		}
	}
	names := strings.Split(value, "/")
	// Dynamic programming keeps repeated ** components bounded.
	previous := make([]bool, len(names)+1)
	previous[0] = true
	for _, part := range p.parts {
		current := make([]bool, len(names)+1)
		if part == "**" {
			current[0] = previous[0]
			for j := 1; j <= len(names); j++ {
				current[j] = previous[j] || current[j-1]
			}
		} else {
			for j, name := range names {
				matched, _ := path.Match(part, name)
				current[j+1] = previous[j] && matched
			}
		}
		previous = current
	}
	return previous[len(names)]
}

func (s *Service) searchFiles(ctx context.Context, root Path, options WalkOptions) ([]Entry, SearchDiagnostics, error) {
	var diagnostic SearchDiagnostics
	root, err := Clean(root.String())
	if err != nil {
		return nil, diagnostic, err
	}
	entry, err := s.FS.Stat(ctx, root)
	if err != nil {
		return nil, diagnostic, err
	}
	depthLimit := options.MaxDepth
	if depthLimit <= 0 || depthLimit > 64 {
		depthLimit = 64
	}
	entryLimit := options.MaxEntries
	if entryLimit <= 0 || entryLimit > 20000 {
		entryLimit = 20000
	}
	queue := []WalkEntry{{Entry: entry}}
	var files []Entry
	visited := 0
	for len(queue) > 0 {
		if err := ctx.Err(); err != nil {
			return nil, diagnostic, err
		}
		item := queue[0]
		queue = queue[1:]
		visited++
		if item.IsLink || (!item.IsDir && !item.Mode.IsRegular()) {
			diagnostic.Skipped++
			continue
		}
		if !item.IsDir {
			files = append(files, item.Entry)
			continue
		}
		if item.Depth >= depthLimit {
			diagnostic.warn("depth limit reached at " + item.Path.String())
			continue
		}
		children, err := s.FS.ReadDir(ctx, item.Path)
		if err != nil {
			if ctx.Err() != nil {
				return nil, diagnostic, ctx.Err()
			}
			if item.Path == root {
				return nil, diagnostic, err
			}
			diagnostic.warn(item.Path.String() + ": " + err.Error())
			continue
		}
		sort.SliceStable(children, func(i, j int) bool { return children[i].Name < children[j].Name })
		for _, child := range children {
			if visited+len(queue) >= entryLimit {
				diagnostic.warn("traversal entry limit reached")
				break
			}
			queue = append(queue, WalkEntry{Entry: child, Depth: item.Depth + 1})
		}
	}
	return files, diagnostic, nil
}

// GlobDetailed finds ordinary files and reports incomplete traversal.
func (s *Service) GlobDetailed(ctx context.Context, root Path, options GlobOptions) (GlobResult, error) {
	var result GlobResult
	pattern, err := compilePattern(options.Pattern)
	if err != nil {
		return result, err
	}
	root, err = Clean(root.String())
	if err != nil {
		return result, err
	}
	limit := options.MaxResults
	if limit <= 0 || limit > 5000 {
		limit = 500
	}
	if options.Walk.MaxEntries <= 0 {
		options.Walk.MaxEntries = limit * 4
	}
	files, diagnostic, err := s.searchFiles(ctx, root, options.Walk)
	result.SearchDiagnostics = diagnostic
	if err != nil {
		return result, err
	}
	for _, file := range files {
		if err := ctx.Err(); err != nil {
			return result, err
		}
		if pattern.matches(root, file) {
			if len(result.Entries) == limit {
				result.warn("match limit reached")
				break
			}
			result.Entries = append(result.Entries, file)
		}
	}
	return result, nil
}

// GrepDetailed searches literal text and reports skipped or incomplete reads.
func (s *Service) GrepDetailed(ctx context.Context, root Path, options SearchOptions) (SearchResult, error) {
	var result SearchResult
	if options.Query == "" {
		return result, errs.New(errs.CodeInvalidArgument, "query must not be empty")
	}
	var patterns []filePattern
	for _, raw := range options.Include {
		pattern, err := compilePattern(raw)
		if err != nil {
			return result, err
		}
		patterns = append(patterns, pattern)
	}
	root, err := Clean(root.String())
	if err != nil {
		return result, err
	}
	if options.MaxResults <= 0 || options.MaxResults > 1000 {
		options.MaxResults = 200
	}
	if options.MaxFileSize <= 0 || options.MaxFileSize > 8<<20 {
		options.MaxFileSize = 4 << 20
	}
	files, diagnostic, err := s.searchFiles(ctx, root, options.Walk)
	result.SearchDiagnostics = diagnostic
	if err != nil {
		return result, err
	}
	needle := options.Query
	if !options.MatchCase {
		needle = strings.ToLower(needle)
	}
	for _, file := range files {
		if err := ctx.Err(); err != nil {
			return result, err
		}
		included := len(patterns) == 0
		for _, pattern := range patterns {
			included = included || pattern.matches(root, file)
		}
		if !included {
			continue
		}
		if file.Size > options.MaxFileSize {
			result.Skipped++
			result.warn(file.Path.String() + ": file size limit reached")
			continue
		}
		data, err := s.FS.ReadFile(ctx, file.Path, options.MaxFileSize+1)
		if err != nil {
			if ctx.Err() != nil {
				return result, ctx.Err()
			}
			result.Skipped++
			result.warn(file.Path.String() + ": " + err.Error())
			continue
		}
		if int64(len(data)) > options.MaxFileSize {
			result.Skipped++
			result.warn(file.Path.String() + ": file grew beyond the size limit")
			continue
		}
		if !utf8.Valid(data) || strings.IndexByte(string(data), 0) >= 0 {
			result.Skipped++
			continue
		}
		scanner := bufio.NewScanner(strings.NewReader(string(data)))
		scanner.Buffer(make([]byte, 64*1024), 1<<20)
		line := 0
		for scanner.Scan() {
			if err := ctx.Err(); err != nil {
				return result, err
			}
			line++
			text := scanner.Text()
			haystack := text
			if !options.MatchCase {
				haystack = strings.ToLower(text)
			}
			if !strings.Contains(haystack, needle) {
				continue
			}
			if len(result.Hits) == options.MaxResults {
				result.warn("hit limit reached")
				return result, nil
			}
			if len(text) > 8192 {
				text = strings.ToValidUTF8(text[:8192], "") + " [line truncated]"
				result.warn(fmt.Sprintf("%s:%d: displayed line truncated", file.Path, line))
			}
			result.Hits = append(result.Hits, SearchHit{Path: file.Path, Line: line, Text: text})
		}
		if err := scanner.Err(); err != nil {
			result.warn(file.Path.String() + ": " + err.Error())
		}
	}
	return result, nil
}
