package fs

import (
	"context"
	"strings"

	"clara-code/pkg/util/errs"
)

// Instructions returns the root-to-target project rules within the workspace.
func Instructions(ctx context.Context, storage Reader, target Path) (string, error) {
	return InstructionsFrom(ctx, storage, "/", target)
}

// InstructionsFrom loads rules between a host-selected root and target.
func InstructionsFrom(ctx context.Context, storage Reader, root, target Path) (string, error) {
	target, err := ResolvePath(storage, target.String())
	if err != nil {
		return "", err
	}
	root, err = ResolvePath(storage, root.String())
	if err != nil {
		return "", err
	}
	if root != "/" && target != root && !strings.HasPrefix(target.String(), strings.TrimRight(root.String(), "/")+"/") {
		return "", errs.ErrForbidden
	}
	scoped := At(storage, "/")
	dir := target.Dir()
	if target == root {
		dir = root
	}
	if target != root {
		entry, err := scoped.Stat(ctx, target)
		if err != nil && !errs.IsCode(err, errs.CodeNotFound) {
			return "", err
		}
		if err == nil && entry.IsDir {
			dir = target
		}
	}
	dirs := []Path{dir}
	for dir != root {
		if len(dirs) >= 64 {
			return "", errs.New(errs.CodeLimit, "project path is too deep")
		}
		dir = dir.Dir()
		dirs = append(dirs, dir)
	}
	remaining := 32 << 10
	var text strings.Builder
	for i := len(dirs) - 1; i >= 0; i-- {
		for _, name := range []string{"AGENTS.override.md", "AGENTS.md"} {
			path, err := Join(dirs[i], name)
			if err != nil {
				return "", err
			}
			data, err := scoped.ReadFile(ctx, path, int64(remaining+1))
			if errs.IsCode(err, errs.CodeNotFound) {
				continue
			}
			if err != nil {
				return "", err
			}
			if len(data) > remaining {
				return "", errs.New(errs.CodeLimit, "project instructions exceed 32 KiB")
			}
			remaining -= len(data)
			if len(data) > 0 {
				text.WriteString("\n" + path.String() + "\n" + string(data) + "\n")
			}
			break
		}
	}
	return strings.TrimSpace(text.String()), nil
}
