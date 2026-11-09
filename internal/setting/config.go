package setting

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"clara-code/pkg/model"
	"clara-code/pkg/tool/mcp"
)

type Model struct {
	ID                            string                  `toml:"id"`
	DisplayName                   string                  `toml:"display_name"`
	Description                   string                  `toml:"description"`
	Priority                      int                     `toml:"priority"`
	Visibility                    string                  `toml:"visibility"`
	ContextWindow                 int64                   `toml:"context_window"`
	EffectiveContextWindowPercent int                     `toml:"effective_context_window_percent"`
	AutoCompactContextPercent     *int                    `toml:"auto_compact_context_percent"`
	MaxOutputTokens               int64                   `toml:"max_output_tokens"`
	InputModalities               []string                `toml:"input_modalities"`
	DefaultReasoningLevel         model.ReasoningEffort   `toml:"default_reasoning_level"`
	SupportedReasoningLevels      []model.ReasoningEffort `toml:"supported_reasoning_levels"`
	DefaultReasoningSummary       string                  `toml:"default_reasoning_summary"`
	DefaultVerbosity              model.TextVerbosity     `toml:"default_verbosity"`
	SupportsParallelToolCalls     *bool                   `toml:"supports_parallel_tool_calls"`
	Effort                        model.ReasoningEffort   `toml:"-"`
}

func (m Model) EffectiveWindow() int64 {
	percent := m.EffectiveContextWindowPercent
	if percent == 0 {
		percent = 95
	}
	return m.ContextWindow/100*int64(percent) + m.ContextWindow%100*int64(percent)/100
}
func (m Model) Images() bool {
	return m.InputModalities == nil || slices.Contains(m.InputModalities, "image")
}
func (m Model) Levels() []model.ReasoningEffort {
	if m.SupportedReasoningLevels != nil {
		return slices.Clone(m.SupportedReasoningLevels)
	}
	return []model.ReasoningEffort{"", "none", "minimal", "low", "medium", "high", "xhigh", "max"}
}
func (m Model) Allows(e model.ReasoningEffort) bool { return e == "" || slices.Contains(m.Levels(), e) }
func ValidEffort(e model.ReasoningEffort) bool {
	return !strings.ContainsAny(string(e), " \t\r\n") && len(e) < 64
}

type Provider struct {
	Name                string            `toml:"name"`
	BaseURL             string            `toml:"base_url"`
	WireAPI             string            `toml:"wire_api"`
	APIKey              string            `toml:"api_key"`
	HTTPHeaders         map[string]string `toml:"http_headers"`
	EnvHTTPHeaders      map[string]string `toml:"env_http_headers"`
	RequestMaxRetries   *int              `toml:"request_max_retries"`
	RequestTimeoutMS    int               `toml:"request_timeout_ms"`
	StreamIdleTimeoutMS int               `toml:"stream_idle_timeout_ms"`
	Models              map[string]Model  `toml:"models"`
}

type MCP struct {
	Command           string            `toml:"command"`
	Args              []string          `toml:"args"`
	Env               map[string]string `toml:"env"`
	URL               string            `toml:"url"`
	Headers           map[string]string `toml:"http_headers"`
	EnvHeaders        map[string]string `toml:"env_http_headers"`
	Required          bool              `toml:"required"`
	Enabled           *bool             `toml:"enabled"`
	ConnectTimeoutSec int               `toml:"connect_timeout_sec"`
	ToolTimeoutSec    int               `toml:"tool_timeout_sec"`
}

func (m MCP) Server(name, cwd string) mcp.ServerConfig {
	return mcp.ServerConfig{Name: name, Command: m.Command, Args: m.Args, Env: m.Env, URL: m.URL, Headers: resolveHeaders(m.Headers, m.EnvHeaders), Required: m.Required, Cwd: cwd, Timeout: time.Duration(m.ConnectTimeoutSec) * time.Second}
}

type ACP struct {
	Command string            `toml:"command"`
	Args    []string          `toml:"args"`
	Env     map[string]string `toml:"env"`
}
type TUI struct {
	Color           string                       `toml:"color"`
	Markdown        *bool                        `toml:"markdown"`
	Animations      *bool                        `toml:"animations"`
	ShowTooltips    *bool                        `toml:"show_tooltips"`
	ToolOutputLines *int                         `toml:"tool_output_lines"`
	StatusLine      []string                     `toml:"status_line"`
	Keymap          map[string]map[string]string `toml:"keymap"`
}
type Exec struct {
	YieldSeconds   int  `toml:"yield_seconds"`
	TimeoutSeconds *int `toml:"timeout_seconds"`
}
type Tools struct {
	DefaultTimeoutSeconds int            `toml:"default_timeout_seconds"`
	Timeouts              map[string]int `toml:"timeouts"`
	Exec                  Exec           `toml:"exec_command"`
}
type Images struct {
	MaxAttachments int   `toml:"max_attachments"`
	MaxSourceBytes int64 `toml:"max_source_bytes"`
	MaxPixels      int64 `toml:"max_pixels"`
	MaxDimension   int   `toml:"max_dimension"`
	MaxTotalBytes  int   `toml:"max_total_bytes"`
}

func (i Images) Defaults() Images {
	if i.MaxAttachments == 0 {
		i.MaxAttachments = 4
	}
	if i.MaxSourceBytes == 0 {
		i.MaxSourceBytes = 16 << 20
	}
	if i.MaxPixels == 0 {
		i.MaxPixels = 64_000_000
	}
	if i.MaxDimension == 0 {
		i.MaxDimension = 2048
	}
	if i.MaxTotalBytes == 0 {
		i.MaxTotalBytes = 8 << 20
	}
	return i
}
func Enabled(value *bool) bool { return value == nil || *value }

type Config struct {
	Home                      string                `toml:"-"`
	DefaultModel              string                `toml:"model"`
	ModelProvider             string                `toml:"model_provider"`
	ReasoningEffort           model.ReasoningEffort `toml:"model_reasoning_effort"`
	PlanEffort                model.ReasoningEffort `toml:"plan_mode_reasoning_effort"`
	AutoCompactContextPercent int                   `toml:"auto_compact_context_percent"`
	Providers                 map[string]Provider   `toml:"model_providers"`
	MCP                       map[string]MCP        `toml:"mcp_servers"`
	ACP                       map[string]ACP        `toml:"acp_agents"`
	TUI                       TUI                   `toml:"tui"`
	Tools                     Tools                 `toml:"tools"`
	Images                    Images                `toml:"images"`
	Sources                   []string              `toml:"-"`
	Origins                   map[string]string     `toml:"-"`
	Models                    map[string]Model      `toml:"-"`
	// Connection fields are resolved internal values, never accepted as TOML keys.
	Endpoint string      `toml:"-"`
	Protocol string      `toml:"-"`
	APIKey   string      `toml:"-"`
	Options  LoadOptions `toml:"-"`
}

// CompactionPercent resolves a per-model override and applies Codex's 90% ceiling.
func (c Config) CompactionPercent(m Model) int {
	if m.AutoCompactContextPercent != nil {
		return min(90, *m.AutoCompactContextPercent)
	}
	if c.AutoCompactContextPercent != 0 {
		return min(90, c.AutoCompactContextPercent)
	}
	return 90
}

func (c Config) SelectProvider(name string) (Config, error) {
	p, ok := c.Providers[name]
	if !ok {
		return c, fmt.Errorf("model_provider %q is not configured", name)
	}
	c.ModelProvider = name
	c.Models = make(map[string]Model, len(p.Models))
	for id, m := range p.Models {
		if m.ID == "" {
			m.ID = id
		}
		m.Effort = m.DefaultReasoningLevel
		if id == c.DefaultModel && c.ReasoningEffort != "" && m.Allows(c.ReasoningEffort) {
			m.Effort = c.ReasoningEffort
		}
		c.Models[id] = m
	}
	return c, nil
}
func (c Config) Reload(ctx context.Context, cwd, root string) (Config, error) {
	if !c.Options.Reload {
		return c, nil
	}
	o := c.Options
	o.Cwd = cwd
	o.Root = root
	return LoadWith(ctx, o)
}
