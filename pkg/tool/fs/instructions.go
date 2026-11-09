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
	dir := ruleParent(root, target)
	if target != root {
		entry, err := storage.Stat(ctx, target)
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
		dir = ruleParent(root, dir)
		dirs = append(dirs, dir)
	}
	remaining := 32 << 10
	var text strings.Builder
	for i := len(dirs) - 1; i >= 0; i-- {
		for _, name := range []string{"AGENTS.override.md", "AGENTS.md"} {
			path := Path(strings.TrimSuffix(dirs[i].String(), "/") + "/" + name)
			data, err := readStorageFile(ctx, storage, path, int64(remaining+1))
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

// ruleParent preserves a native volume or share prefix while walking canonical paths.
func ruleParent(root, target Path) Path {
	if target == root {
		return root
	}
	parent := target.String()
	if at := strings.LastIndexByte(parent, '/'); at >= 0 {
		parent = parent[:at]
	}
	if parent == strings.TrimSuffix(root.String(), "/") {
		return root
	}
	return Path(parent)
}
