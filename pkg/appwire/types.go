// Package appwire defines the versioned Clara Code ACP extension contract.
package appwire

const (
	Version    = 1
	Namespace  = "dev.claracode/"
	Prefix     = "_" + Namespace
	Capability = Namespace + "extensions"
)

type Workspace struct {
	Root   string `json:"root"`
	Cwd    string `json:"cwd"`
	Branch string `json:"branch"`
	Git    bool   `json:"git"`
}

type Selection struct {
	Provider   string `json:"provider"`
	Model      string `json:"model"`
	Effort     string `json:"effort"`
	PlanEffort string `json:"planEffort"`
}

type Model struct {
	Provider      string   `json:"provider"`
	ProviderLabel string   `json:"providerLabel"`
	ID            string   `json:"id"`
	Label         string   `json:"label"`
	Description   string   `json:"description"`
	Efforts       []string `json:"efforts"`
	Default       string   `json:"default"`
	DefaultPlan   string   `json:"defaultPlan"`
	Images        bool     `json:"images"`
}

type Options struct {
	Models     []Model   `json:"models"`
	Selection  Selection `json:"selection"`
	NeedsSetup bool      `json:"needsSetup"`
	ConfigPath string    `json:"configPath"`
}

type Setup struct {
	ProviderID      string `json:"providerId"`
	BaseURL         string `json:"baseUrl"`
	WireAPI         string `json:"wireApi"`
	APIKey          string `json:"apiKey"`
	ModelID         string `json:"modelId"`
	ContextWindow   int64  `json:"contextWindow"`
	MaxOutputTokens int64  `json:"maxOutputTokens"`
}

type Session struct {
	ID        string    `json:"id"`
	Root      string    `json:"root"`
	Cwd       string    `json:"cwd"`
	Title     string    `json:"title"`
	Updated   string    `json:"updated"`
	Selection Selection `json:"selection"`
}

type SessionPage struct {
	Sessions []Session `json:"sessions"`
	Cursor   string    `json:"cursor"`
}

type Attachment struct {
	Hash      string `json:"hash"`
	Filename  string `json:"filename"`
	MediaType string `json:"mediaType"`
	Width     int    `json:"width"`
	Height    int    `json:"height"`
	Size      int    `json:"size"`
}

type AttachmentChunk struct {
	Data string `json:"data"`
	Next int    `json:"next"`
}

type Activity struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Title    string `json:"title"`
	Detail   string `json:"detail"`
	Args     string `json:"args"`
	Output   string `json:"output"`
	Error    string `json:"error"`
	Status   string `json:"status"`
	Omitted  int    `json:"omitted"`
	ExitCode *int   `json:"exitCode,omitempty"`
}

type Cell struct {
	Kind        string       `json:"kind"`
	Text        string       `json:"text"`
	Attachments []Attachment `json:"attachments,omitempty"`
	Tool        *Activity    `json:"tool,omitempty"`
}

type Turn struct {
	ID          string       `json:"id"`
	Prompt      string       `json:"prompt"`
	Attachments []Attachment `json:"attachments,omitempty"`
	Cells       []Cell       `json:"cells"`
	Planning    bool         `json:"planning"`
	Status      string       `json:"status"`
	DurationMS  *int64       `json:"durationMs,omitempty"`
}

type History struct {
	Session Session `json:"session"`
	Turns   []Turn  `json:"turns"`
	Before  int     `json:"before"`
}

type QuestionOption struct {
	Label       string `json:"label"`
	Description string `json:"description"`
}

type Question struct {
	ID      string           `json:"id"`
	Header  string           `json:"header"`
	Text    string           `json:"text"`
	Options []QuestionOption `json:"options"`
}

type QuestionSet struct {
	Items []Question `json:"items"`
}

type DiffPage struct {
	Text string `json:"text"`
	Next int    `json:"next"`
}

// Request contains the parameters shared by the Clara ACP extension methods.
type Request struct {
	Cwd         string       `json:"cwd,omitempty"`
	SessionID   string       `json:"sessionId,omitempty"`
	RunID       string       `json:"runId,omitempty"`
	Before      int          `json:"before,omitempty"`
	Text        string       `json:"text,omitempty"`
	Kind        string       `json:"kind,omitempty"`
	Path        string       `json:"path,omitempty"`
	Offset      int          `json:"offset,omitempty"`
	Setup       Setup        `json:"setup,omitzero"`
	Attachments []Attachment `json:"attachments,omitempty"`
}
