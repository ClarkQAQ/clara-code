package setting

import (
	"fmt"
	"net/url"
	"slices"
	"strings"
)

func (c Config) invalid(path, reason string) error {
	return fmt.Errorf("configuration %s: %s: %s", originFor(c.Origins, path), path, reason)
}
func (c Config) validate() error {
	if c.AutoCompactContextPercent < 1 || c.AutoCompactContextPercent > 100 {
		return c.invalid("auto_compact_context_percent", "must be between 1 and 100")
	}
	for id, p := range c.Providers {
		path := "model_providers." + id
		if id == "" {
			return c.invalid(path, "provider ID cannot be empty")
		}
		if strings.ContainsAny(p.APIKey, "\r\n") {
			return c.invalid(path+".api_key", "must not contain line breaks")
		}
		if p.BaseURL != "" && !ValidBaseURL(p.BaseURL) {
			return c.invalid(path+".base_url", "must be an HTTP(S) URL without credentials")
		}
		if p.WireAPI != "" && p.WireAPI != "responses" && p.WireAPI != "completions" {
			return c.invalid(path+".wire_api", "must be responses or completions")
		}
		if p.RequestMaxRetries != nil && (*p.RequestMaxRetries < 0 || *p.RequestMaxRetries > 5) {
			return c.invalid(path+".request_max_retries", "must be between 0 and 5")
		}
		if p.RequestTimeoutMS < 0 || p.StreamIdleTimeoutMS < 0 || p.RequestTimeoutMS > 86_400_000 || p.StreamIdleTimeoutMS > 86_400_000 {
			return c.invalid(path, "request timeouts must be between 0 and 86400000 ms")
		}
		for name, m := range p.Models {
			key := path + ".models." + name
			if strings.TrimSpace(name) == "" || m.ContextWindow < 0 || m.MaxOutputTokens < 0 || m.EffectiveContextWindowPercent < 0 || m.EffectiveContextWindowPercent > 100 {
				return c.invalid(key, "invalid model name or context/output budget")
			}
			if m.AutoCompactContextPercent != nil && (*m.AutoCompactContextPercent < 1 || *m.AutoCompactContextPercent > 100) {
				return c.invalid(key+".auto_compact_context_percent", "must be between 1 and 100")
			}
			if m.Visibility != "" && m.Visibility != "list" && m.Visibility != "hidden" {
				return c.invalid(key+".visibility", "must be list or hidden")
			}
			for _, modality := range m.InputModalities {
				if modality != "text" && modality != "image" {
					return c.invalid(key+".input_modalities", "only text and image are supported")
				}
			}
			for i, level := range m.SupportedReasoningLevels {
				if level == "" || !ValidEffort(level) || slices.Contains(m.SupportedReasoningLevels[:i], level) {
					return c.invalid(key+".supported_reasoning_levels", "levels must be unique nonempty identifiers")
				}
			}
			if !ValidEffort(m.DefaultReasoningLevel) || !m.Allows(m.DefaultReasoningLevel) {
				return c.invalid(key+".default_reasoning_level", "must be one of the supported reasoning levels")
			}
			if m.DefaultReasoningSummary != "" && !slices.Contains([]string{"auto", "concise", "detailed", "none"}, m.DefaultReasoningSummary) {
				return c.invalid(key+".default_reasoning_summary", "unsupported summary setting")
			}
			if m.DefaultVerbosity != "" && !slices.Contains([]string{"low", "medium", "high"}, string(m.DefaultVerbosity)) {
				return c.invalid(key+".default_verbosity", "must be low, medium or high")
			}
			if id == c.ModelProvider && name == c.DefaultModel {
				if !m.Allows(c.ReasoningEffort) {
					return c.invalid("model_reasoning_effort", "not supported by the selected model")
				}
				if !m.Allows(c.PlanEffort) {
					return c.invalid("plan_mode_reasoning_effort", "not supported by the selected model")
				}
			}
		}
	}
	if c.ModelProvider != "" {
		if _, ok := c.Providers[c.ModelProvider]; !ok {
			return c.invalid("model_provider", "provider is not configured")
		}
	}
	if c.TUI.Color != "" && !slices.Contains([]string{"auto", "always", "never"}, c.TUI.Color) {
		return c.invalid("tui.color", "must be auto, always or never")
	}
	if c.TUI.ToolOutputLines != nil && (*c.TUI.ToolOutputLines < 0 || *c.TUI.ToolOutputLines > 1000) {
		return c.invalid("tui.tool_output_lines", "must be between 0 and 1000")
	}
	for _, field := range c.TUI.StatusLine {
		if !slices.Contains([]string{"state", "model", "context", "directory", "git_branch", "session_cache_efficiency", "round_cache_efficiency"}, field) {
			return c.invalid("tui.status_line", "unknown status field")
		}
	}
	if err := c.TUI.ValidateKeys(); err != nil {
		return c.invalid("tui.keymap", err.Error())
	}
	if c.Tools.DefaultTimeoutSeconds < 0 || c.Tools.DefaultTimeoutSeconds > 86400 {
		return c.invalid("tools.default_timeout_seconds", "must be between 0 and 86400")
	}
	for name, n := range c.Tools.Timeouts {
		if n < 0 || n > 86400 {
			return c.invalid("tools.timeouts."+name, "must be between 0 and 86400")
		}
	}
	if c.Tools.Exec.YieldSeconds < 0 || c.Tools.Exec.YieldSeconds > 30 {
		return c.invalid("tools.exec_command.yield_seconds", "must be between 0 and 30")
	}
	if c.Tools.Exec.TimeoutSeconds != nil && (*c.Tools.Exec.TimeoutSeconds < 1 || *c.Tools.Exec.TimeoutSeconds > 600) {
		return c.invalid("tools.exec_command.timeout_seconds", "must be between 1 and 600; omit for no deadline")
	}
	for name, m := range c.MCP {
		if m.ConnectTimeoutSec < 0 || m.ToolTimeoutSec < 0 || m.ConnectTimeoutSec > 86400 || m.ToolTimeoutSec > 86400 {
			return c.invalid("mcp_servers."+name, "timeouts must be between 0 and 86400 seconds")
		}
	}
	i := c.Images.Defaults()
	if i.MaxAttachments < -1 || i.MaxAttachments > 16 || i.MaxSourceBytes < 1 || i.MaxSourceBytes > 64<<20 || i.MaxPixels < 1 || i.MaxPixels > 64_000_000 || i.MaxDimension < 1 || i.MaxDimension > 8192 || i.MaxTotalBytes < -1 || i.MaxTotalBytes > 16<<20 {
		return c.invalid("images", "invalid attachment count, dimensions or byte limit")
	}
	return nil
}

func ValidBaseURL(value string) bool {
	u, err := url.Parse(value)
	return err == nil && u.Hostname() != "" && (u.Scheme == "http" || u.Scheme == "https") && u.User == nil
}
