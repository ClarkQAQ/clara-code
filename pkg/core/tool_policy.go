package core

import (
	"cmp"
	"context"
	"slices"
)

// ToolAuthorizer can deny a validated call before it reaches the tool.
type ToolAuthorizer func(context.Context, ToolInfo, ToolCall) error

func filterAgentTools(registered []AgentTool, active []string, disabled bool) []AgentTool {
	if disabled {
		return nil
	}
	tools := make([]AgentTool, 0, len(registered))
	seen := make(map[string]bool, len(registered))
	for _, tool := range registered {
		if tool == nil {
			continue
		}
		name := tool.Info().Name
		if name != "" && !seen[name] && (active == nil || slices.Contains(active, name)) {
			tools = append(tools, tool)
			seen[name] = true
		}
	}
	slices.SortFunc(tools, func(a, b AgentTool) int { return cmp.Compare(a.Info().Name, b.Info().Name) })
	return tools
}

func cloneSchema(input map[string]any) map[string]any {
	out := make(map[string]any, len(input))
	for key, value := range input {
		switch value := value.(type) {
		case map[string]any:
			out[key] = cloneSchema(value)
		case []any:
			items := slices.Clone(value)
			for i, item := range items {
				if node, ok := item.(map[string]any); ok {
					items[i] = cloneSchema(node)
				}
			}
			out[key] = items
		default:
			out[key] = value
		}
	}
	return out
}
