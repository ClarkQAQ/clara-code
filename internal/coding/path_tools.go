package coding

import (
	"encoding/json"
	"strings"

	"clara-code/pkg/models"
)

type pathTool struct {
	models.AgentTool
	info models.ToolInfo
}

func (t pathTool) Info() models.ToolInfo {
	return t.info
}

func hostPathTools(items []models.AgentTool) []models.AgentTool {
	out := make([]models.AgentTool, 0, len(items))
	for _, tool := range items {
		info := tool.Info()
		switch info.Name {
		case "list", "stat", "fs_instructions", "read", "write", "edit", "remove", "move", "copy", "glob", "grep":
		default:
			out = append(out, tool)
			continue
		}
		data, _ := json.Marshal(info)
		var copy models.ToolInfo
		_ = json.Unmarshal(data, &copy)
		copy.Description = "Use real host paths: absolute paths are native, relative paths start at the current working directory, ~/ starts at home. " + strings.ReplaceAll(info.Description, `"path":"/`, `"path":"`)
		for _, name := range []string{"path", "from", "to"} {
			if property, ok := copy.Parameters[name].(map[string]any); ok {
				property["description"] = "Real host path. Absolute paths, ../ and ~/ are allowed; relative paths start at the current working directory."
				if name == "path" {
					required := false
					for _, field := range info.Required {
						required = required || field == name
					}
					if !required {
						property["description"] = property["description"].(string) + " Omitted path means the current working directory."
					}
				}
			}
		}
		if copy.Name == "glob" {
			if pattern, ok := copy.Parameters["pattern"].(map[string]any); ok {
				pattern["description"] = "Glob filter: *.go matches filenames recursively; **/*.go includes root files; patterns starting / match real absolute paths, relative patterns with / start at the search root."
			}
		}
		if copy.InputSchema != nil {
			copy.InputSchema["properties"] = copy.Parameters
		}
		out = append(out, pathTool{AgentTool: tool, info: copy})
	}
	return out
}
