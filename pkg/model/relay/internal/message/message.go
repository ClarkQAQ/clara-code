// Package message defines the model-independent message and content model.
package message

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
)

// Role identifies who produced a message.
type Role string

const (
	RoleSystem    Role = "system"
	RoleDeveloper Role = "developer"
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
	RoleTool      Role = "tool"
)

// Status is the lifecycle state of a message or item.
type Status string

const (
	StatusInProgress Status = "in_progress"
	StatusCompleted  Status = "completed"
	StatusIncomplete Status = "incomplete"
)

// PartType identifies one content part.
type PartType string

const (
	PartText       PartType = "text"
	PartReasoning  PartType = "reasoning"
	PartImage      PartType = "image"
	PartFile       PartType = "file"
	PartToolCall   PartType = "tool_call"
	PartToolResult PartType = "tool_result"
	PartRefusal    PartType = "refusal"
)

// Message is one model conversation message.
type Message struct {
	ID       string            `json:"id,omitempty"`
	Role     Role              `json:"role"`
	Status   Status            `json:"status,omitempty"`
	Phase    string            `json:"phase,omitempty"`
	Parts    []Part            `json:"content"`
	Metadata map[string]string `json:"metadata,omitempty"`
}

// Part is a tagged content union. Exactly one matching field is set by constructors.
type Part struct {
	Type PartType `json:"type"`

	Text       string      `json:"text,omitempty"`
	Reasoning  *Reasoning  `json:"reasoning,omitempty"`
	Image      *Image      `json:"image,omitempty"`
	File       *File       `json:"file,omitempty"`
	ToolCall   *ToolCall   `json:"tool_call,omitempty"`
	ToolResult *ToolResult `json:"tool_result,omitempty"`
	Refusal    string      `json:"refusal,omitempty"`
}

// Reasoning carries visible or opaque model reasoning.
type Reasoning struct {
	Text             string `json:"text,omitempty"`
	EncryptedContent string `json:"encrypted_content,omitempty"`
}

// Image is a base64 image input or output.
type Image struct {
	Data     []byte `json:"data,omitempty"`
	URL      string `json:"url,omitempty"`
	MIME     string `json:"mime,omitempty"`
	Detail   string `json:"detail,omitempty"`
	Filename string `json:"filename,omitempty"`
}

// File is a file input or output.
type File struct {
	Data     []byte `json:"data,omitempty"`
	URL      string `json:"url,omitempty"`
	MIME     string `json:"mime,omitempty"`
	Filename string `json:"filename,omitempty"`
}

// ToolCall asks the host to run a function.
type ToolCall struct {
	ID               string `json:"id"`
	Name             string `json:"name"`
	Arguments        string `json:"arguments"`
	ProviderExecuted bool   `json:"provider_executed,omitempty"`
}

// ToolResult returns the output of a tool call.
type ToolResult struct {
	CallID  string          `json:"call_id"`
	Name    string          `json:"name,omitempty"`
	Output  string          `json:"output,omitempty"`
	Content []Part          `json:"content,omitempty"`
	IsError bool            `json:"is_error,omitempty"`
	Details json.RawMessage `json:"details,omitempty"`
}

func Text(text string) Part {
	return Part{Type: PartText, Text: text}
}

func Refusal(text string) Part {
	return Part{Type: PartRefusal, Refusal: text}
}

func ReasoningPart(reasoning Reasoning) Part {
	return Part{Type: PartReasoning, Reasoning: &reasoning}
}

func ImagePart(image Image) Part {
	return Part{Type: PartImage, Image: &image}
}

func FilePart(file File) Part {
	return Part{Type: PartFile, File: &file}
}

func ToolCallPart(call ToolCall) Part {
	return Part{Type: PartToolCall, ToolCall: &call}
}

func ToolResultPart(result ToolResult) Part {
	return Part{Type: PartToolResult, ToolResult: &result}
}

func User(parts ...Part) Message {
	return Message{Role: RoleUser, Status: StatusCompleted, Parts: cloneParts(parts)}
}

func UserText(text string) Message {
	return User(Text(text))
}

func Assistant(parts ...Part) Message {
	return Message{Role: RoleAssistant, Status: StatusCompleted, Parts: cloneParts(parts)}
}

func System(text string) Message {
	return Message{Role: RoleSystem, Status: StatusCompleted, Parts: []Part{Text(text)}}
}

func Developer(text string) Message {
	return Message{Role: RoleDeveloper, Status: StatusCompleted, Parts: []Part{Text(text)}}
}

func ToolMessage(results ...ToolResult) Message {
	parts := make([]Part, 0, len(results))
	for _, result := range results {
		parts = append(parts, ToolResultPart(result))
	}
	return Message{Role: RoleTool, Status: StatusCompleted, Parts: parts}
}

// Text concatenates text and refusal parts.
func (m Message) Text() string {
	var builder strings.Builder
	for _, part := range m.Parts {
		switch part.Type {
		case PartText:
			builder.WriteString(part.Text)
		case PartRefusal:
			builder.WriteString(part.Refusal)
		}
	}
	return builder.String()
}

// ReasoningText concatenates visible reasoning parts.
func (m Message) ReasoningText() string {
	var builder strings.Builder
	for _, part := range m.Parts {
		if part.Type == PartReasoning && part.Reasoning != nil {
			builder.WriteString(part.Reasoning.Text)
		}
	}
	return builder.String()
}

// ToolCalls returns every tool call part.
func (m Message) ToolCalls() []ToolCall {
	var calls []ToolCall
	for _, part := range m.Parts {
		if part.Type == PartToolCall && part.ToolCall != nil {
			calls = append(calls, *part.ToolCall)
		}
	}
	return calls
}

// Clone returns a deep-enough copy for immutable session snapshots.
func (m Message) Clone() Message {
	m.Parts = cloneParts(m.Parts)
	if m.Metadata != nil {
		metadata := make(map[string]string, len(m.Metadata))
		for key, value := range m.Metadata {
			metadata[key] = value
		}
		m.Metadata = metadata
	}
	return m
}

func cloneParts(parts []Part) []Part {
	if len(parts) == 0 {
		return nil
	}
	out := make([]Part, len(parts))
	for index := range parts {
		out[index] = parts[index]
		if parts[index].Reasoning != nil {
			value := *parts[index].Reasoning
			out[index].Reasoning = &value
		}
		if parts[index].Image != nil {
			value := *parts[index].Image
			value.Data = append([]byte(nil), value.Data...)
			out[index].Image = &value
		}
		if parts[index].File != nil {
			value := *parts[index].File
			value.Data = append([]byte(nil), value.Data...)
			out[index].File = &value
		}
		if parts[index].ToolCall != nil {
			value := *parts[index].ToolCall
			out[index].ToolCall = &value
		}
		if parts[index].ToolResult != nil {
			value := *parts[index].ToolResult
			value.Content = cloneParts(value.Content)
			out[index].ToolResult = &value
		}
	}
	return out
}

// Validate checks the tagged part invariant at trust boundaries.
func (p Part) Validate() error {
	set := 0
	switch p.Type {
	case PartText:
		set = boolInt(p.Text != "")
	case PartRefusal:
		set = boolInt(p.Refusal != "")
	case PartReasoning:
		set = boolInt(p.Reasoning != nil)
	case PartImage:
		set = boolInt(p.Image != nil)
	case PartFile:
		set = boolInt(p.File != nil)
	case PartToolCall:
		set = boolInt(p.ToolCall != nil)
	case PartToolResult:
		set = boolInt(p.ToolResult != nil)
	default:
		return fmt.Errorf("unknown part type %q", p.Type)
	}
	if set == 0 {
		return fmt.Errorf("part %q has no matching payload", p.Type)
	}
	return nil
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

// DataURL returns an RFC 2397 data URL for image or file content.
func DataURL(data []byte, mime string) string {
	return "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(data)
}
