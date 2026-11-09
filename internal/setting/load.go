package setting

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/pelletier/go-toml/v2"
)

type LoadOptions struct {
	Cwd, Root, Model string
	Overrides        []string
	Reload           bool
}

func Load() (Config, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return Config{}, err
	}
	return LoadWith(context.Background(), LoadOptions{Cwd: cwd, Root: cwd})
}
func LoadWith(ctx context.Context, o LoadOptions) (Config, error) {
	home, err := Home()
	if err != nil {
		return Config{}, err
	}
	return loadWith(ctx, o, home, nil)
}

func loadWith(ctx context.Context, o LoadOptions, home string, initial map[string]any) (Config, error) {
	merged := map[string]any{"auto_compact_context_percent": int64(90)}
	origins := map[string]string{}
	var sources []string
	apply := func(layer map[string]any, source string) {
		merge(merged, layer)
		recordOrigins(layer, "", source, origins)
		sources = append(sources, source)
	}
	paths := []string{filepath.Join(home, "config.toml")}
	if initial != nil {
		apply(initial, paths[0])
	}
	cwd, err := canonical(o.Cwd)
	if err != nil {
		return Config{}, err
	}
	root, err := canonical(o.Root)
	if err != nil {
		return Config{}, err
	}
	relative, err := filepath.Rel(root, cwd)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return Config{}, errors.New("configuration cwd must be inside project root")
	}
	var dirs []string
	for dir := cwd; ; dir = filepath.Dir(dir) {
		dirs = append(dirs, dir)
		if dir == root {
			break
		}
	}
	slices.Reverse(dirs)
	for _, dir := range dirs {
		paths = append(paths, filepath.Join(dir, ".agents", "clara-code.toml"))
	}
	hasConfigFile := false
	for _, path := range paths {
		if err := ctx.Err(); err != nil {
			return Config{}, err
		}
		file, err := os.Open(path)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return Config{}, fmt.Errorf("configuration %s: cannot open file", path)
		}
		if initial != nil {
			_ = file.Close()
			return Config{}, configAlreadyExists(path)
		}
		hasConfigFile = true
		body, err := io.ReadAll(io.LimitReader(file, (1<<20)+1))
		closeErr := file.Close()
		if err != nil || closeErr != nil || len(body) > 1<<20 {
			return Config{}, fmt.Errorf("configuration %s: unreadable or exceeds 1 MiB", path)
		}
		var layer map[string]any
		if err := toml.Unmarshal(body, &layer); err != nil {
			var parse *toml.DecodeError
			if errors.As(err, &parse) {
				row, column := parse.Position()
				return Config{}, fmt.Errorf("configuration %s:%d:%d: invalid TOML syntax", path, row, column)
			}
			return Config{}, fmt.Errorf("configuration %s: invalid TOML", path)
		}
		if _, exists := layer["context_bytes"]; exists {
			return Config{}, fmt.Errorf("configuration %s: context_bytes was removed; use auto_compact_context_percent and model context_window", path)
		}
		apply(layer, path)
	}
	env := map[string]any{}
	for key, name := range map[string]string{"CLARA_CODE_MODEL": "model", "CLARA_CODE_MODEL_PROVIDER": "model_provider", "CLARA_CODE_EFFORT": "model_reasoning_effort", "CLARA_CODE_MODEL_REASONING_EFFORT": "model_reasoning_effort", "CLARA_CODE_PLAN_MODE_REASONING_EFFORT": "plan_mode_reasoning_effort"} {
		if value := os.Getenv(key); value != "" {
			env[name] = value
		}
	}
	if os.Getenv("CLARA_CODE_CONTEXT_BYTES") != "" {
		return Config{}, errors.New("CLARA_CODE_CONTEXT_BYTES was removed; use CLARA_CODE_AUTO_COMPACT_CONTEXT_PERCENT and configure the model context_window")
	}
	if v := os.Getenv("CLARA_CODE_AUTO_COMPACT_CONTEXT_PERCENT"); v != "" {
		n, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			return Config{}, errors.New("CLARA_CODE_AUTO_COMPACT_CONTEXT_PERCENT must be an integer between 1 and 100")
		}
		env["auto_compact_context_percent"] = n
	}
	selected, _ := merged["model_provider"].(string)
	if v, ok := env["model_provider"].(string); ok {
		selected = v
	}
	if selected == "" {
		if ps, ok := merged["model_providers"].(map[string]any); ok && len(ps) == 1 {
			for id := range ps {
				selected = id
			}
		}
	}
	connection := map[string]any{}
	for key, field := range map[string]string{"CLARA_CODE_ENDPOINT": "base_url", "CLARA_CODE_PROTOCOL": "wire_api", "CLARA_CODE_API_KEY": "api_key"} {
		if v := os.Getenv(key); v != "" {
			connection[field] = v
		}
	}
	if len(connection) > 0 {
		if selected == "" {
			selected = "environment"
		}
		env["model_provider"] = selected
		env["model_providers"] = map[string]any{selected: connection}
	}
	if value := os.Getenv("CLARA_CODE_MODEL_REASONING_EFFORT"); value != "" {
		env["model_reasoning_effort"] = value
	}
	if len(env) > 0 {
		apply(env, "environment")
	}
	for _, raw := range o.Overrides {
		key, value, ok := strings.Cut(raw, "=")
		key = strings.TrimSpace(key)
		if !ok || key == "" || len(raw) > 1<<20 {
			return Config{}, errors.New("-c requires a nonempty key=value")
		}
		var layer map[string]any
		if err := toml.Unmarshal([]byte(key+"="+value), &layer); err != nil {
			if err := toml.Unmarshal([]byte(key+"="+strconv.Quote(strings.Trim(strings.TrimSpace(value), "\"'"))), &layer); err != nil {
				return Config{}, errors.New("-c: invalid TOML key path")
			}
		}
		apply(layer, "command line: "+key)
	}
	if o.Model != "" {
		apply(map[string]any{"model": o.Model}, "--model")
	}
	body, err := toml.Marshal(merged)
	if err != nil {
		return Config{}, errors.New("configuration merge failed")
	}
	var c Config
	decoder := toml.NewDecoder(bytes.NewReader(body)).DisallowUnknownFields()
	if err := decoder.Decode(&c); err != nil {
		var strict *toml.StrictMissingError
		if errors.As(err, &strict) && len(strict.Errors) > 0 {
			key := strings.Join(strict.Errors[0].Key(), ".")
			return Config{}, fmt.Errorf("configuration %s: unknown key %s", originFor(origins, key), key)
		}
		var decode *toml.DecodeError
		if errors.As(err, &decode) {
			key := strings.Join(decode.Key(), ".")
			return Config{}, fmt.Errorf("configuration %s: invalid value at %s", originFor(origins, key), key)
		}
		return Config{}, errors.New("configuration contains an invalid field or type")
	}
	c.Home = home
	c.hasConfigFile = hasConfigFile || initial != nil
	c.Sources = sources
	c.Origins = origins
	c.Options = o
	c.Options.Reload = true
	if c.Providers == nil {
		c.Providers = map[string]Provider{}
	}
	if c.ModelProvider == "" && len(c.Providers) == 1 {
		for id := range c.Providers {
			c.ModelProvider = id
		}
	}
	if c.ModelProvider != "" {
		p, ok := c.Providers[c.ModelProvider]
		if !ok {
			return Config{}, c.invalid("model_provider", "provider is not configured")
		}
		if p.Models == nil {
			p.Models = map[string]Model{}
		}
		if c.DefaultModel != "" {
			if _, ok := p.Models[c.DefaultModel]; !ok {
				p.Models[c.DefaultModel] = Model{}
			}
		}
		c.Providers[c.ModelProvider] = p
	}
	if err := c.validate(); err != nil {
		return Config{}, err
	}
	if c.ModelProvider != "" {
		c, err = c.SelectProvider(c.ModelProvider)
		if err != nil {
			return Config{}, err
		}
	}
	if c.Models == nil {
		c.Models = map[string]Model{}
	}
	return c, nil
}
func canonical(path string) (string, error) {
	if path == "" {
		path = "."
	}
	path, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	return filepath.EvalSymlinks(path)
}
func merge(dst, src map[string]any) {
	for k, v := range src {
		if child, ok := v.(map[string]any); ok {
			old, ok := dst[k].(map[string]any)
			if !ok {
				old = map[string]any{}
				dst[k] = old
			}
			merge(old, child)
		} else {
			dst[k] = v
		}
	}
}
func recordOrigins(m map[string]any, prefix, source string, out map[string]string) {
	for k, v := range m {
		key := prefix + k
		out[key] = source
		if child, ok := v.(map[string]any); ok {
			recordOrigins(child, key+".", source, out)
		}
	}
}
func originFor(origins map[string]string, key string) string {
	for key != "" {
		if source := origins[key]; source != "" {
			return source
		}
		i := strings.LastIndexByte(key, '.')
		if i < 0 {
			break
		}
		key = key[:i]
	}
	return "defaults"
}
