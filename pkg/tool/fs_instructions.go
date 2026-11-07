package tool

import (
	"clara-code/pkg/tool/fs"
	"context"
	"errors"
)

func (s *Files) rulesFor(ctx context.Context, path fs.Path, seen bool) (string, error) {
	source := s.InstructionsSource
	if source == nil {
		source = func(ctx context.Context, target fs.Path) (string, error) {
			return fs.Instructions(ctx, s.service.FS, target)
		}
	}
	text, err := source(ctx, path)
	if err != nil {
		return "", err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if !seen && text != "" && s.rules[path] != text {
		return "", errors.New("Read the applicable project instructions before changing " + path.String() + ".")
	}
	if seen {
		if len(s.rules) >= 256 {
			clear(s.rules)
		}
		s.rules[path] = text
	}
	return text, nil
}

// Instructions shows the project rules that apply to a target path.
func (s *Files) Instructions() Tool {
	return Func(Definition{Name: "fs_instructions", Description: "Read project AGENTS.md or AGENTS.override.md rules for a file or directory. This does not query global rules supplied by the host. Before creating a new file, query its destination path.", Effects: []Effect{EffectRead}, ParallelSafe: true},
		func(ctx context.Context, input struct {
			Path string `json:"path"`
		}) (Result, error) {
			path, err := s.cleanPath(input.Path)
			if err != nil {
				return Result{}, err
			}
			text, err := s.rulesFor(ctx, path, true)
			if err != nil {
				return Result{}, err
			}
			if text == "" {
				text = "No project instructions apply to this path."
			}
			return TextResult(text), nil
		})
}
