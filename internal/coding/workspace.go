package coding

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"clara-code/pkg/core/prompt"
	"clara-code/pkg/model"
	"clara-code/pkg/tool"
	corefs "clara-code/pkg/tool/fs"
	"clara-code/pkg/tool/fs/host"
	"clara-code/pkg/tool/fs/local"
	"clara-code/pkg/tool/fs/mount"
	"github.com/pelletier/go-toml/v2"
	"go.yaml.in/yaml/v3"
)

type Workspace struct {
	Root, Cwd string
	Git       bool
	FS        *local.FS
	Native    *host.FS
}

func ResolveWorkspace(path string) (Workspace, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return Workspace{}, err
	}
	cwd, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return Workspace{}, err
	}
	info, err := os.Stat(cwd)
	if err != nil {
		return Workspace{}, err
	}
	if !info.IsDir() {
		return Workspace{}, fmt.Errorf("%s is not a directory", cwd)
	}
	root := cwd
	git := false
	for dir := cwd; ; dir = filepath.Dir(dir) {
		if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
			root, git = dir, true
			break
		}
		if filepath.Dir(dir) == dir {
			break
		}
	}
	storage, err := local.New(root)
	if err != nil {
		return Workspace{}, err
	}
	native, err := host.New(cwd)
	return Workspace{Root: root, Cwd: cwd, Git: git, FS: storage, Native: native}, err
}

func (w Workspace) Branch(ctx context.Context) string {
	if !w.Git {
		return ""
	}
	limit, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	cmd := exec.CommandContext(limit, "git", "-C", w.Root, "branch", "--show-current")
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	if branch := strings.TrimSpace(string(out)); branch != "" {
		return branch
	}
	out, err = exec.CommandContext(limit, "git", "-C", w.Root, "rev-parse", "--short", "HEAD").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

type runner struct {
	workspace Workspace
	local     *tool.Local
}

func (r runner) Start(ctx context.Context, spec tool.Spec) (tool.Process, error) {
	resolved, err := r.workspace.Native.ResolvePath(spec.Workdir)
	if err != nil {
		return nil, err
	}
	cwd, err := filepath.EvalSymlinks(filepath.FromSlash(resolved.String()))
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(cwd)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("working directory is not a directory: %s", cwd)
	}
	spec.Workdir = cwd
	return r.local.Start(ctx, spec)
}

func (w Workspace) directories() []string {
	var dirs []string
	for dir := w.Cwd; ; dir = filepath.Dir(dir) {
		dirs = append(dirs, dir)
		if dir == w.Root || dir == filepath.Dir(dir) {
			break
		}
	}
	slices.Reverse(dirs)
	return dirs
}

func (w Workspace) instructionFragments(ctx context.Context, home string) ([]prompt.Fragment, error) {
	var fragments []prompt.Fragment
	remaining := int64(32 << 10)
	for index, dir := range append([]string{home}, w.directories()...) {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if index == 1 {
			remaining = 32 << 10
		}
		for _, name := range []string{"AGENTS.override.md", "AGENTS.md"} {
			path := filepath.Join(dir, name)
			body, err := boundedFile(path, remaining)
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			if err != nil {
				return nil, err
			}
			remaining -= int64(len(body))
			kind, scope := "workspace_instructions", dir
			if index == 0 {
				kind, scope = "global_instructions", "all workspaces"
			}
			fragments = append(fragments, prompt.Fragment{Kind: kind, Source: path, Scope: scope, Lifecycle: "step", Role: model.MessageRoleUser, Text: string(body)})
			break
		}
	}
	return fragments, nil
}

type Skill struct {
	Name, Description string
	Source            string
	Enabled           bool
}

type skillHeader struct {
	Name        string `yaml:"name" toml:"name"`
	Description string `yaml:"description" toml:"description"`
}

func (w Workspace) skills(ctx context.Context, home string) ([]Skill, []model.AgentTool, error) {
	return w.skillCatalog(ctx, home, false)
}

func (w Workspace) skillCatalog(ctx context.Context, home string, includeDisabled bool) ([]Skill, []model.AgentTool, error) {
	preferences, err := readSkillPreferences(home)
	if err != nil {
		return nil, nil, err
	}
	roots := []string{filepath.Join(home, "skills")}
	for _, dir := range w.directories() {
		roots = append(roots, filepath.Join(dir, ".agents", "skills"))
	}
	mounted := mount.New()
	paths := map[string]corefs.Path{}
	catalog := map[string]Skill{}
	resources := map[string]*local.FS{}
	for index, root := range roots {
		if err := ctx.Err(); err != nil {
			return nil, nil, err
		}
		entries, err := os.ReadDir(root)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, nil, err
		}
		storage, err := local.New(root)
		if err != nil {
			return nil, nil, err
		}
		prefix := fmt.Sprintf("/skills-%d", index)
		if err := mounted.Mount(corefs.Path(prefix), storage); err != nil {
			return nil, nil, err
		}
		for _, entry := range entries {
			if err := ctx.Err(); err != nil {
				return nil, nil, err
			}
			if !entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
				continue
			}
			rel := entry.Name() + "/SKILL.md"
			body, err := storage.ReadFile(ctx, corefs.Path("/"+rel), 64<<10)
			if err != nil {
				continue
			}
			header, ok := parseSkill(string(body))
			if !ok {
				continue
			}
			if len(catalog) >= 256 {
				if _, exists := catalog[header.Name]; !exists {
					continue
				}
			}
			resource, err := local.New(filepath.Join(root, entry.Name()))
			if err != nil {
				continue
			}
			file, err := skillFile(resource.Root())
			if err != nil {
				continue
			}
			catalog[header.Name] = Skill{Name: header.Name, Description: header.Description, Source: resource.Root(), Enabled: !slices.Contains(preferences.Disabled, file)}
			resources[header.Name] = resource
			paths[header.Name] = corefs.Path(prefix + "/" + rel)
		}
	}
	var skills []Skill
	for _, skill := range catalog {
		if includeDisabled || skill.Enabled {
			skills = append(skills, skill)
		}
		if !skill.Enabled {
			delete(paths, skill.Name)
			delete(resources, skill.Name)
		}
	}
	slices.SortFunc(skills, func(a, b Skill) int { return strings.Compare(a.Name, b.Name) })
	if len(paths) == 0 {
		return skills, nil, nil
	}
	resourceTool := tool.Func(tool.Definition{Name: "read_skill_resource", Description: "Read a text resource relative to an available skill directory. Use name and path from the skill instructions.", Effects: []tool.Effect{tool.EffectRead}, ParallelSafe: true},
		func(ctx context.Context, in struct {
			Name string `json:"name"`
			Path string `json:"path"`
		}) (tool.Result, error) {
			storage, ok := resources[in.Name]
			if !ok {
				return tool.ErrorResult("unknown skill"), nil
			}
			if filepath.IsAbs(in.Path) {
				return tool.ErrorResult("resource path must be relative to the skill directory"), nil
			}
			path, err := corefs.Clean(in.Path)
			if err != nil {
				return tool.ErrorResult(err.Error()), nil
			}
			info, err := storage.Stat(ctx, path)
			if err != nil {
				return tool.ErrorResult(err.Error()), nil
			}
			if !info.Mode.IsRegular() || info.Size > 256<<10 {
				return tool.ErrorResult("resource must be a text file smaller than 256 KiB"), nil
			}
			body, err := storage.ReadFile(ctx, path, 256<<10)
			if err != nil {
				return tool.ErrorResult(err.Error()), nil
			}
			if strings.ContainsRune(string(body), 0) {
				return tool.ErrorResult("resource is binary"), nil
			}
			return tool.TextResult(string(body)), nil
		})
	return skills, []model.AgentTool{tool.ReadSkill(mounted, paths), resourceTool}, nil
}

func parseSkill(body string) (skillHeader, bool) {
	lines := bufio.NewScanner(strings.NewReader(body))
	if !lines.Scan() {
		return skillHeader{}, false
	}
	delimiter := strings.TrimSpace(lines.Text())
	if delimiter != "---" && delimiter != "+++" {
		return skillHeader{}, false
	}
	var head strings.Builder
	for lines.Scan() {
		line := lines.Text()
		if strings.TrimSpace(line) == delimiter {
			var result skillHeader
			decode := yaml.Unmarshal
			if delimiter == "+++" {
				decode = toml.Unmarshal
			}
			if err := decode([]byte(head.String()), &result); err != nil {
				return skillHeader{}, false
			}
			result.Name, result.Description = strings.TrimSpace(result.Name), strings.TrimSpace(result.Description)
			return result, result.Name != "" && len(result.Name) <= 80 && result.Description != "" && len(result.Description) <= 1024
		}
		head.WriteString(line + "\n")
		if head.Len() > 8<<10 {
			break
		}
	}
	return skillHeader{}, false
}

func boundedFile(path string, limit int64) ([]byte, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("%s is not a regular file", path)
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	body, err := io.ReadAll(io.LimitReader(file, limit+1))
	if int64(len(body)) > limit {
		return nil, fmt.Errorf("%s exceeds its size limit", path)
	}
	return body, err
}

func (w Workspace) toolFS() *host.FS {
	return w.Native
}

func projectForPath(value string) (string, bool, error) {
	info, err := os.Stat(value)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return "", false, err
	}
	if err != nil || !info.IsDir() {
		value = filepath.Dir(value)
	}
	for dir := value; ; dir = filepath.Dir(dir) {
		if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
			return dir, true, nil
		} else if !errors.Is(err, os.ErrNotExist) {
			return "", false, err
		}
		if filepath.Dir(dir) == dir {
			break
		}
	}
	return string(filepath.Separator), false, nil
}

func (w Workspace) targetRules(ctx context.Context, target corefs.Path) (string, error) {
	real, err := w.Native.RealPath(ctx, target)
	if err != nil {
		return "", err
	}
	root, _, err := projectForPath(filepath.FromSlash(real.String()))
	if err != nil {
		return "", err
	}
	return corefs.InstructionsFrom(ctx, w.Native, corefs.Path(filepath.ToSlash(root)), real)
}
