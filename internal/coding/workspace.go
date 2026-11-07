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

	"clara-core/models"
	"clara-core/tools"
	corefs "clara-core/tools/fs"
	"clara-core/tools/fs/local"
	"clara-core/tools/fs/mount"
	"github.com/pelletier/go-toml/v2"
	"go.yaml.in/yaml/v3"
)

type Workspace struct {
	Root, Cwd string
	Git       bool
	FS        *local.FS
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
	return Workspace{Root: root, Cwd: cwd, Git: git, FS: storage}, err
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
	local     *tools.Local
}

func (r runner) Start(ctx context.Context, spec tools.Spec) (tools.Process, error) {
	cwd := r.workspace.Cwd
	if spec.Workdir != "" {
		if filepath.IsAbs(spec.Workdir) {
			if spec.Workdir == r.workspace.Root || strings.HasPrefix(spec.Workdir, r.workspace.Root+string(filepath.Separator)) {
				cwd = spec.Workdir
			} else {
				// Canonical Core paths returned by file tools are rooted at the project.
				cwd = filepath.Join(r.workspace.Root, strings.TrimPrefix(spec.Workdir, "/"))
			}
		} else {
			cwd = filepath.Join(cwd, spec.Workdir)
		}
	}
	relative, err := filepath.Rel(r.workspace.Root, cwd)
	if err != nil {
		return nil, err
	}
	logical, err := corefs.Clean(filepath.ToSlash(relative))
	if err != nil {
		return nil, err
	}
	spec.Workdir, err = r.workspace.FS.ResolveDir(ctx, logical)
	if err != nil {
		return nil, err
	}
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

func (w Workspace) rules(ctx context.Context, home string) (string, error) {
	var global string
	for _, name := range []string{"AGENTS.override.md", "AGENTS.md"} {
		path := filepath.Join(home, name)
		body, err := boundedFile(path, 32<<10)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return "", err
		}
		global = path + "\n" + string(body)
		break
	}
	rel, err := filepath.Rel(w.Root, w.Cwd)
	if err != nil {
		return "", err
	}
	target := corefs.Path("/" + filepath.ToSlash(filepath.Join(rel, ".clara-code-context")))
	project, err := corefs.Instructions(ctx, w.FS, target)
	return strings.TrimSpace(strings.Join([]string{global, project}, "\n\n")), err
}

type Skill struct {
	Name, Description string
	Source            string
}

type skillHeader struct {
	Name        string `yaml:"name" toml:"name"`
	Description string `yaml:"description" toml:"description"`
}

func (w Workspace) skills(ctx context.Context, home string) ([]Skill, []models.AgentTool, error) {
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
			catalog[header.Name] = Skill{Name: header.Name, Description: header.Description, Source: resource.Root()}
			resources[header.Name] = resource
			paths[header.Name] = corefs.Path(prefix + "/" + rel)
		}
	}
	var skills []Skill
	for _, skill := range catalog {
		skills = append(skills, skill)
	}
	slices.SortFunc(skills, func(a, b Skill) int { return strings.Compare(a.Name, b.Name) })
	if len(skills) == 0 {
		return nil, nil, nil
	}
	resourceTool := tools.Func(tools.Definition{Name: "read_skill_resource", Description: "Read a text resource relative to an available skill directory. Use name and path from the skill instructions.", Effects: []tools.Effect{tools.EffectRead}, ParallelSafe: true},
		func(ctx context.Context, in struct {
			Name string `json:"name"`
			Path string `json:"path"`
		}) (tools.Result, error) {
			storage, ok := resources[in.Name]
			if !ok {
				return tools.ErrorResult("unknown skill"), nil
			}
			if filepath.IsAbs(in.Path) {
				return tools.ErrorResult("resource path must be relative to the skill directory"), nil
			}
			path, err := corefs.Clean(in.Path)
			if err != nil {
				return tools.ErrorResult(err.Error()), nil
			}
			info, err := storage.Stat(ctx, path)
			if err != nil {
				return tools.ErrorResult(err.Error()), nil
			}
			if !info.Mode.IsRegular() || info.Size > 256<<10 {
				return tools.ErrorResult("resource must be a text file smaller than 256 KiB"), nil
			}
			body, err := storage.ReadFile(ctx, path, 256<<10)
			if err != nil {
				return tools.ErrorResult(err.Error()), nil
			}
			if strings.ContainsRune(string(body), 0) {
				return tools.ErrorResult("resource is binary"), nil
			}
			return tools.TextResult(string(body)), nil
		})
	return skills, []models.AgentTool{tools.ReadSkill(mounted, paths), resourceTool}, nil
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

type workspaceFS struct{ *local.FS }

func (f workspaceFS) ResolvePath(value string) (corefs.Path, error) {
	if filepath.IsAbs(value) {
		root := f.Root()
		if value == root {
			return "/", nil
		}
		if strings.HasPrefix(value, root+string(filepath.Separator)) {
			relative, err := filepath.Rel(root, value)
			if err != nil {
				return "", err
			}
			return corefs.Clean(filepath.ToSlash(relative))
		}
	}
	return corefs.Clean(value)
}

func (w Workspace) toolFS() workspaceFS {
	return workspaceFS{w.FS}
}
