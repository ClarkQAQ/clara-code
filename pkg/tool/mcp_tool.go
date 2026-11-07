package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// MCPTool is the model-visible metadata of one MCP tool.
type MCPTool struct {
	Server      string
	Name        string
	Description string
	InputSchema json.RawMessage
	ReadOnly    bool
}

// MCPResource is one MCP resource summary.
type MCPResource struct {
	Server      string `json:"server"`
	URI         string `json:"uri"`
	Name        string `json:"name"`
	Title       string `json:"title,omitempty"`
	Description string `json:"description,omitempty"`
	MIMEType    string `json:"mime_type,omitempty"`
}

// MCPResourceContent is one resource body.
type MCPResourceContent struct {
	URI      string `json:"uri"`
	MIMEType string `json:"mime_type,omitempty"`
	Text     string `json:"text,omitempty"`
	Blob     []byte `json:"blob,omitempty"`
}

// MCPPrompt is one MCP prompt summary.
type MCPPrompt struct {
	Server      string   `json:"server"`
	Name        string   `json:"name"`
	Title       string   `json:"title,omitempty"`
	Description string   `json:"description,omitempty"`
	Arguments   []string `json:"arguments,omitempty"`
}

// MCPCaller is implemented by the MCP connector.
type MCPCaller interface {
	CanonicalTools() []MCPTool
	CallTool(context.Context, string, string, json.RawMessage) (Result, error)
	ListResources(context.Context, string) ([]MCPResource, error)
	ReadResource(context.Context, string, string) (MCPResourceContent, error)
	ListPrompts(context.Context, string) ([]MCPPrompt, error)
	GetPrompt(context.Context, string, string, map[string]string) (string, error)
}

// MCPTools exposes one MCP connection as Clara Core
type MCPTools struct {
	Caller MCPCaller
	Server string
}

// All returns dynamic MCP tools plus resource and prompt
func (s *MCPTools) All() []Tool {
	if s == nil || s.Caller == nil {
		return nil
	}
	var out []Tool
	for _, remote := range s.Caller.CanonicalTools() {
		if s.Server != "" && remote.Server != s.Server {
			continue
		}
		out = append(out, s.remoteTool(remote))
	}
	if s.Server != "" {
		out = append(out, s.resourceTools()...)
	}
	return out
}

// Tools returns the dynamic tool snapshot.
func (s *MCPTools) Tools() []Tool { return s.All() }

func (s *MCPTools) remoteTool(remote MCPTool) Tool {
	name := "mcp__" + sanitize(remote.Server) + "__" + sanitize(remote.Name)
	effects := []Effect{EffectOther}
	if remote.ReadOnly {
		effects = []Effect{EffectRead}
	}
	return Func(Definition{
		Name: name, Description: fmt.Sprintf("[%s] %s", remote.Server, remote.Description),
		InputSchema: remote.InputSchema, Effects: effects,
	}, func(ctx context.Context, in json.RawMessage) (Result, error) {
		return s.Caller.CallTool(ctx, remote.Server, remote.Name, in)
	})
}

func sanitize(value string) string {
	var builder strings.Builder
	for _, character := range value {
		if (character >= 'a' && character <= 'z') || (character >= 'A' && character <= 'Z') ||
			(character >= '0' && character <= '9') || character == '_' || character == '-' {
			builder.WriteRune(character)
		} else {
			builder.WriteByte('_')
		}
	}
	return strings.Trim(builder.String(), "_")
}

func (s *MCPTools) resourceTools() []Tool {
	serverName := s.Server
	return []Tool{
		Func(Definition{
			Name:        "mcp__" + sanitize(serverName) + "__list_resources",
			Description: "List resources published by the " + serverName + " MCP server.",
			Effects:     []Effect{EffectNetwork}, ParallelSafe: true,
		}, func(ctx context.Context, _ struct{}) (Result, error) {
			resources, err := s.Caller.ListResources(ctx, serverName)
			if err != nil {
				return ErrorResult(err.Error()), nil
			}
			return TextResult(renderResources(resources)), nil
		}),
		Func(Definition{
			Name:        "mcp__" + sanitize(serverName) + "__read_resource",
			Description: "Read one resource by URI from the " + serverName + " MCP server.",
			Effects:     []Effect{EffectNetwork, EffectRead}, ParallelSafe: true,
		}, func(ctx context.Context, in struct {
			URI string `json:"uri" description:"resource URI"`
		}) (Result, error) {
			content, err := s.Caller.ReadResource(ctx, serverName, in.URI)
			if err != nil {
				return ErrorResult(err.Error()), nil
			}
			if content.Text != "" {
				return TextResult(content.Text), nil
			}
			return TextResult(fmt.Sprintf("resource %s contains %d bytes of %s", content.URI, len(content.Blob), content.MIMEType)), nil
		}),
		Func(Definition{
			Name:        "mcp__" + sanitize(serverName) + "__list_prompts",
			Description: "List prompt templates published by the " + serverName + " MCP server.",
			Effects:     []Effect{EffectNetwork}, ParallelSafe: true,
		}, func(ctx context.Context, _ struct{}) (Result, error) {
			prompts, err := s.Caller.ListPrompts(ctx, serverName)
			if err != nil {
				return ErrorResult(err.Error()), nil
			}
			return TextResult(renderPrompts(prompts)), nil
		}),
		Func(Definition{
			Name:        "mcp__" + sanitize(serverName) + "__get_prompt",
			Description: "Resolve one prompt template from the " + serverName + " MCP server.",
			Effects:     []Effect{EffectNetwork, EffectRead}, ParallelSafe: true,
		}, func(ctx context.Context, in struct {
			Name      string            `json:"name" description:"prompt name"`
			Arguments map[string]string `json:"arguments,omitempty" description:"prompt arguments"`
		}) (Result, error) {
			text, err := s.Caller.GetPrompt(ctx, serverName, in.Name, in.Arguments)
			if err != nil {
				return ErrorResult(err.Error()), nil
			}
			return TextResult(text), nil
		}),
	}
}

func renderResources(resources []MCPResource) string {
	if len(resources) == 0 {
		return "No MCP resources."
	}
	var builder strings.Builder
	for _, resource := range resources {
		fmt.Fprintf(&builder, "%s\t%s\t%s\n", resource.URI, resource.Name, resource.MIMEType)
	}
	return strings.TrimSpace(builder.String())
}

func renderPrompts(prompts []MCPPrompt) string {
	if len(prompts) == 0 {
		return "No MCP prompts."
	}
	var builder strings.Builder
	for _, prompt := range prompts {
		fmt.Fprintf(&builder, "%s\t%s\n", prompt.Name, prompt.Description)
	}
	return strings.TrimSpace(builder.String())
}
