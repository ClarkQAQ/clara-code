package settings

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"clara-core/models"
	"clara-core/models/provider"
	"clara-core/tools/mcp"
	"github.com/pelletier/go-toml/v2"
)

type Model struct {
	ID                            string                 `toml:"id"`
	Effort                        models.ReasoningEffort `toml:"effort"`
	ContextWindow                 int64                  `toml:"context_window"`
	EffectiveContextWindowPercent int                    `toml:"effective_context_window_percent"`
}

func (m Model) EffectiveWindow() int64 {
	percent := m.EffectiveContextWindowPercent
	if percent == 0 {
		percent = 95
	}
	return m.ContextWindow/100*int64(percent) + m.ContextWindow%100*int64(percent)/100
}

type MCP struct {
	Command  string            `toml:"command"`
	Args     []string          `toml:"args"`
	Env      map[string]string `toml:"env"`
	URL      string            `toml:"url"`
	Headers  map[string]string `toml:"headers"`
	Required bool              `toml:"required"`
}

func (m MCP) Server(name, cwd string) mcp.ServerConfig {
	return mcp.ServerConfig{Name: name, Command: m.Command, Args: m.Args, Env: m.Env,
		URL: m.URL, Headers: m.Headers, Cwd: cwd, Required: m.Required}
}

type ACP struct {
	Command string            `toml:"command"`
	Args    []string          `toml:"args"`
	Env     map[string]string `toml:"env"`
}

type Config struct {
	Home         string           `toml:"-"`
	Endpoint     string           `toml:"endpoint"`
	Protocol     string           `toml:"protocol"`
	APIKeyEnv    string           `toml:"api_key_env"`
	APIKeyFile   string           `toml:"api_key_file"`
	DefaultModel string           `toml:"model"`
	ContextBytes int              `toml:"context_bytes"`
	Models       map[string]Model `toml:"models"`
	MCP          map[string]MCP   `toml:"mcp_servers"`
	ACP          map[string]ACP   `toml:"acp_agents"`
	apiKey       string
}

func Home() (string, error) {
	if path := os.Getenv("CLARA_CODE_HOME"); path != "" {
		return filepath.Abs(path)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	if runtime.GOOS == "darwin" {
		return filepath.Join(home, "Library", "Application Support", "clara-code"), nil
	}
	data := os.Getenv("XDG_DATA_HOME")
	if !filepath.IsAbs(data) {
		data = filepath.Join(home, ".local", "share")
	}
	return filepath.Join(data, "clara-code"), nil
}

func Load() (Config, error) {
	home, err := Home()
	if err != nil {
		return Config{}, err
	}
	cfg := Config{Home: home, ContextBytes: 512 << 10, Models: map[string]Model{}}
	file, err := os.Open(filepath.Join(home, "config.toml"))
	switch {
	case err == nil:
		defer file.Close()
		body, err := io.ReadAll(io.LimitReader(file, 1<<20+1))
		if err != nil {
			return Config{}, err
		}
		if len(body) > 1<<20 {
			return Config{}, errors.New("config.toml exceeds 1 MiB")
		}
		if err := toml.Unmarshal(body, &cfg); err != nil {
			return Config{}, fmt.Errorf("config.toml: %w", err)
		}
	case !errors.Is(err, os.ErrNotExist):
		return Config{}, err
	}
	cfg.Endpoint = first(os.Getenv("CLARA_CODE_ENDPOINT"), cfg.Endpoint, os.Getenv("CLARA_CORE_ENDPOINT"))
	cfg.Protocol = first(os.Getenv("CLARA_CODE_PROTOCOL"), cfg.Protocol, os.Getenv("CLARA_CORE_PROTOCOL"), "responses")
	cfg.DefaultModel = first(os.Getenv("CLARA_CODE_MODEL"), cfg.DefaultModel, os.Getenv("CLARA_CORE_MODEL"))
	cfg.apiKey = first(os.Getenv("CLARA_CODE_API_KEY"), os.Getenv(cfg.APIKeyEnv), os.Getenv("CLARA_CORE_API_KEY"))
	if value := os.Getenv("CLARA_CODE_CONTEXT_BYTES"); value != "" {
		budget, err := strconv.Atoi(value)
		if err != nil {
			return Config{}, fmt.Errorf("CLARA_CODE_CONTEXT_BYTES: %w", err)
		}
		cfg.ContextBytes = budget
	}
	if cfg.DefaultModel != "" {
		if _, ok := cfg.Models[cfg.DefaultModel]; !ok {
			cfg.Models[cfg.DefaultModel] = Model{ID: cfg.DefaultModel}
		}
	}
	for name, model := range cfg.Models {
		model.ID = first(model.ID, name)
		if name == cfg.DefaultModel {
			model.Effort = models.ReasoningEffort(first(os.Getenv("CLARA_CODE_EFFORT"), string(model.Effort)))
		}
		if !ValidEffort(model.Effort) || model.ContextWindow < 0 || model.EffectiveContextWindowPercent < 0 || model.EffectiveContextWindowPercent > 100 || strings.TrimSpace(model.ID) == "" {
			return Config{}, fmt.Errorf("invalid settings for model %q", name)
		}
		cfg.Models[name] = model
	}
	if cfg.ContextBytes < 0 || cfg.ContextBytes > 16<<20 {
		return Config{}, errors.New("context_bytes must be between 0 and 16 MiB")
	}
	return cfg, nil
}

func ValidEffort(effort models.ReasoningEffort) bool {
	switch effort {
	case "", "none", "minimal", "low", "medium", "high", "xhigh", "max":
		return true
	default:
		return false
	}
}

func (c Config) Factory() (func(context.Context, string) (models.LanguageModel, error), error) {
	if c.Endpoint == "" || c.DefaultModel == "" {
		return nil, errors.New("configure an endpoint and model in config.toml or CLARA_CODE_ENDPOINT and CLARA_CODE_MODEL, or use --demo")
	}
	var api provider.API
	switch c.Protocol {
	case "responses":
		api = provider.APIResponses
	case "completions":
		api = provider.APIChatCompletions
	default:
		return nil, errors.New("protocol must be responses or completions")
	}
	key := c.apiKey
	if key == "" && c.APIKeyFile != "" {
		path := c.APIKeyFile
		if !filepath.IsAbs(path) {
			path = filepath.Join(c.Home, path)
		}
		var err error
		key, err = readAPIKey(path)
		if err != nil {
			return nil, err
		}
	}
	client, err := provider.New(provider.WithBaseURL(c.Endpoint), provider.WithAPI(api), provider.WithAPIKey(key))
	if err != nil {
		return nil, err
	}
	return client.LanguageModel, nil
}

func readAPIKey(path string) (string, error) {
	info, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0o077 != 0 {
		return "", errors.New("api_key_file must be a private regular file (mode 0600)")
	}
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	body, err := io.ReadAll(io.LimitReader(file, 16<<10+1))
	if err != nil {
		return "", err
	}
	key := strings.TrimSpace(string(body))
	if len(body) > 16<<10 || key == "" || strings.ContainsAny(key, "\r\n") {
		return "", errors.New("api_key_file must contain one non-empty API key")
	}
	return key, nil
}

func first(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

// Redact removes configured model credentials from user-facing diagnostics.
func (c Config) Redact(text string) string {
	keys := []string{c.apiKey, os.Getenv("CLARA_CODE_API_KEY"), os.Getenv(c.APIKeyEnv), os.Getenv("CLARA_CORE_API_KEY")}
	if c.APIKeyFile != "" {
		path := c.APIKeyFile
		if !filepath.IsAbs(path) {
			path = filepath.Join(c.Home, path)
		}
		if key, err := readAPIKey(path); err == nil {
			keys = append(keys, key)
		}
	}
	for _, key := range keys {
		if key != "" {
			text = strings.ReplaceAll(text, key, "[redacted]")
		}
	}
	return text
}
