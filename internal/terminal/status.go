package terminal

import (
	"fmt"
	"math"
	"path/filepath"
	"strings"
	"time"

	"clara-code/internal/session"
	"clara-code/pkg/core/prompt"
	"clara-code/pkg/tool"
	"clara-code/pkg/util/localtime"
)

func tokenCount(count int64) string {
	count = max(0, count)
	if count < 1000 {
		return fmt.Sprint(count)
	}
	value, suffix := float64(count)/1000, "K"
	for _, unit := range []struct {
		scale  int64
		suffix string
	}{{1_000_000, "M"}, {1_000_000_000, "B"}, {1_000_000_000_000, "T"}} {
		if count >= unit.scale {
			value, suffix = float64(count)/float64(unit.scale), unit.suffix
		}
	}
	decimals := 0
	if value < 10 {
		decimals = 2
	} else if value < 100 {
		decimals = 1
	}
	formatted := fmt.Sprintf("%.*f", decimals, value)
	if strings.Contains(formatted, ".") {
		formatted = strings.TrimRight(strings.TrimRight(formatted, "0"), ".")
	}
	return formatted + suffix
}

func contextPercent(used, window int64) int64 {
	const baseline = 12000
	if window <= baseline {
		return 100
	}
	remaining := float64(max(0, window-baseline-max(0, used-baseline))) / float64(window-baseline) * 100
	return 100 - int64(math.Round(min(100, max(0, remaining))))
}

func (a *app) contextLabel() string {
	meta := a.conv.Store.Meta()
	window := a.host.Config.Models[meta.Model].EffectiveWindow()
	var used int64
	var prefix string
	switch {
	case meta.LastUsage != nil:
		used = meta.LastUsage.TotalTokens
	case meta.ContextTokens != nil:
		used, prefix = *meta.ContextTokens, "≈"
	default:
		if window > 0 {
			return "?/" + tokenCount(window) + " (unknown)"
		}
		return "context unknown"
	}
	if window <= 0 {
		return prefix + tokenCount(used) + " tokens"
	}
	return fmt.Sprintf("%s%s/%s (%d%%)", prefix, tokenCount(used), tokenCount(window), contextPercent(used, window))
}

func (a *app) status() string {
	if a.hint != "" {
		return a.hint
	}
	state := "Ready"
	if a.op != nil {
		state = a.opLabel + "…"
	} else if a.run != nil {
		state = "Working"
	}
	if a.conv == nil {
		return state
	}
	meta := a.conv.Store.Meta()
	fields := a.host.Config.TUI.StatusLine
	if fields == nil {
		fields = []string{"state", "model", "context", "directory", "git_branch"}
	}
	labels := map[string]string{"state": state, "model": strings.TrimSpace(meta.Model + " " + a.conv.ModeEffort(a.mode)), "context": a.contextLabel(), "directory": filepath.Base(a.space.Root), "git_branch": a.branch}
	labels["session_cache_efficiency"] = cacheLabel("Session cache", meta.SessionCache)
	round := "Last round cache"
	if a.run != nil {
		round = "Round cache"
	}
	labels["round_cache_efficiency"] = cacheLabel(round, meta.RoundCache)
	var items []string
	for _, field := range fields {
		if label := labels[field]; label != "" {
			items = append(items, label)
		}
	}
	return singleLine(strings.Join(items, " · "))
}

func (a *app) statusLine() string {
	p := &a.view
	goal := a.goalState()
	label := goalStatusLabel(goal.Status)
	compact := label
	right := compact
	if a.mode == prompt.Plan {
		compact, right = "Plan mode", "Plan mode"
		if a.run == nil && a.op == nil {
			if hint := a.hints("Shift+Tab"); hint != "" {
				right += " (" + hint + " to cycle)"
			}
		}
		if label != "" {
			compact += " · " + label
			right += " · " + label
		}
	} else if a.run == nil && a.op == nil && (goal.Status == tool.GoalPaused || goal.Status == tool.GoalBlocked || goal.Status == tool.GoalActive) {
		right += " (/continue)"
	}
	left := a.status()
	if widthOf(right)+24 > p.width {
		right = compact
	}
	if a.mode == prompt.Plan && widthOf(right) > p.width {
		switch {
		case label != "" && widthOf("Plan · "+label) <= p.width:
			right = "Plan · " + label
		case label != "" && widthOf(label) <= p.width:
			right = label
		case widthOf("Plan mode") <= p.width:
			right = "Plan mode"
		default:
			right = "Plan"
		}
	}
	right = truncate(right, p.width)
	available := max(0, p.width-widthOf(right))
	gap := 0
	if right != "" && available > 0 {
		gap = min(2, available)
		available -= gap
	}
	left = truncate("  "+left, available)
	spaces := max(gap, p.width-widthOf(left)-widthOf(right))
	return p.style(dim, left) + strings.Repeat(" ", spaces) + p.style(purple, right)
}

func goalStatusLabel(status tool.GoalStatus) string {
	switch status {
	case tool.GoalActive:
		return "Goal active"
	case tool.GoalPaused:
		return "Goal paused"
	case tool.GoalBlocked:
		return "Goal blocked"
	case tool.GoalCompleted:
		return "Goal completed"
	case tool.GoalExhausted:
		return "Goal budget exhausted"
	default:
		return ""
	}
}

func (a *app) showStatus() {
	now := time.Now()
	meta := a.conv.Store.Meta()
	total := "unknown"
	if meta.TotalUsage.TotalTokens > 0 {
		total = tokenCount(meta.TotalUsage.TotalTokens)
		if meta.UsageIncomplete {
			total = "≥" + total
		}
	}
	profile := a.host.Config.Models[meta.Model]
	connection := a.host.Config.Connection()
	auth := connection.APIKey != ""
	mode := "Agent"
	if a.mode == prompt.Plan {
		mode = "Plan"
	}
	compaction := fmt.Sprintf("Auto-compaction  %d%% of raw context window (capped by effective context window)", a.host.Config.CompactionPercent(profile))
	if profile.EffectiveWindow() <= 0 {
		compaction += " (estimated 256K-token window; model limit unknown)"
	}
	lines := []string{
		"Session  " + meta.ID, "Directory  " + a.space.Cwd, "Project  " + a.space.Root,
		"Local time  " + localtime.DateTime(now), "Time zone  " + localtime.Zone(now),
		"Provider  " + meta.Provider,
		"Model  " + modelLabel(meta),
		"Agent effort  " + meta.Effort, "Plan effort  " + a.conv.ModeEffort(prompt.Plan),
		fmt.Sprintf("Images  %t", a.conv.SupportsImages()),
		compaction,
		fmt.Sprintf("Capabilities  reasoning=%v · output=%d · parallel=%s · summary=%s · verbosity=%s", profile.Levels(), profile.MaxOutputTokens, parallelLabel(profile.SupportsParallelToolCalls), profile.DefaultReasoningSummary, profile.DefaultVerbosity),
		fmt.Sprintf("Provider authentication  configured=%t", auth),
		"Configuration sources  " + strings.Join(a.host.Config.Sources, " → "), "Mode  " + mode, "Context  " + a.contextLabel(), "Total usage  " + total + " tokens",
		cacheLabel("Session cache", meta.SessionCache), cacheLabel("Round / last round cache", meta.RoundCache),
		fmt.Sprintf("MCP  %d configured servers", len(a.conv.MCPStatuses())),
	}
	if goal := a.goalState(); goal.Status != "" {
		lines = append(lines, goalStatusLabel(goal.Status), "Goal objective  "+goal.Objective)
	}
	a.view.message("notice", strings.Join(lines, "\n"))
}

func cacheLabel(label string, usage session.CacheUsage) string {
	if efficiency, ok := usage.Efficiency(); ok {
		return fmt.Sprintf("%s %.1f%%", label, efficiency)
	}
	return label + " unknown"
}

func parallelLabel(value *bool) string {
	if value == nil {
		return "default"
	}
	return fmt.Sprint(*value)
}
