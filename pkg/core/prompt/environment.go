package prompt

import (
	"encoding/xml"
	"html"
	"strings"

	"clara-code/pkg/model"
)

// Environment describes host-supplied state. It does not discover the host's
// filesystem, permissions, clock or shell. Empty fields are omitted.
type Environment struct {
	Cwd              string
	Shell            string
	CurrentDate      string
	Timezone         string
	WorkspaceRoots   []string
	FileSystemAccess string
}

// Fragments keeps each field independently comparable by Window. Workspace roots
// describe context, not an access restriction; FileSystemAccess describes the
// host's actual execution boundary, for example unrestricted or read_only.
func (e Environment) Fragments() []Fragment {
	fields := []struct{ name, text string }{
		{"cwd", e.Cwd}, {"shell", e.Shell}, {"current_date", e.CurrentDate}, {"timezone", e.Timezone},
	}
	if len(e.WorkspaceRoots) > 0 || e.FileSystemAccess != "" {
		fields = append(fields, struct{ name, text string }{"filesystem", renderFileSystem(environmentFileSystem{Roots: e.WorkspaceRoots, Access: e.FileSystemAccess})})
	}
	var fragments []Fragment
	for _, field := range fields {
		if strings.TrimSpace(field.text) != "" {
			fragments = append(fragments, Fragment{Kind: "environment", Source: field.name, Lifecycle: "step", Role: model.MessageRoleUser, Text: field.text})
		}
	}
	return fragments
}

type environmentFileSystem struct {
	XMLName xml.Name `xml:"filesystem"`
	Roots   []string `xml:"workspace_roots>root"`
	Access  string   `xml:"file_system>access"`
}

func renderFileSystem(fs environmentFileSystem) string {
	var out strings.Builder
	out.WriteString("<filesystem>")
	if len(fs.Roots) > 0 {
		out.WriteString("<workspace_roots>")
		for _, root := range fs.Roots {
			out.WriteString("<root>" + html.EscapeString(root) + "</root>")
		}
		out.WriteString("</workspace_roots>")
	}
	if fs.Access != "" {
		out.WriteString("<file_system><access>" + html.EscapeString(fs.Access) + "</access></file_system>")
	}
	out.WriteString("</filesystem>")
	return out.String()
}

func renderEnvironmentField(name, value string) string {
	if name == "filesystem" {
		var fs environmentFileSystem
		if xml.Unmarshal([]byte(value), &fs) == nil {
			return renderFileSystem(fs) + "\n"
		}
	}
	return "<" + name + ">" + html.EscapeString(value) + "</" + name + ">\n"
}
