package session

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"clara-code/pkg/model"
)

type ToolActivity struct {
	Owner           string    `json:"owner,omitempty"`
	Interaction     string    `json:"interaction,omitempty"`
	Input           string    `json:"input,omitempty"`
	CommandRevision uint64    `json:"command_revision,omitempty"`
	ID              string    `json:"id,omitempty"`
	CallID          string    `json:"call_id"`
	Name            string    `json:"name"`
	Arguments       string    `json:"arguments,omitempty"`
	Title           string    `json:"title"`
	Detail          string    `json:"detail,omitempty"`
	Status          string    `json:"status"`
	Started         time.Time `json:"started"`
	Finished        time.Time `json:"finished,omitempty"`
	SessionID       string    `json:"session_id,omitempty"`
	ExitCode        *int      `json:"exit_code,omitempty"`
	DurationMS      int64     `json:"duration_ms,omitempty"`
	Output          string    `json:"output,omitempty"`
	Omitted         int       `json:"omitted,omitempty"`
	Error           string    `json:"error,omitempty"`
}

type CommandOutcome struct {
	Snapshot   bool   `json:"snapshot,omitempty"`
	Revision   uint64 `json:"revision,omitempty"`
	Output     string `json:"output,omitempty"`
	Running    bool   `json:"running"`
	ExitCode   *int   `json:"exit_code,omitempty"`
	SessionID  string `json:"session_id,omitempty"`
	DurationMS int64  `json:"duration_ms,omitempty"`
	TimedOut   bool   `json:"timed_out,omitempty"`
	Stopped    bool   `json:"stopped,omitempty"`
	Omitted    int    `json:"omitted,omitempty"`
}

type RetryStatus struct {
	Attempt int    `json:"attempt"`
	Limit   int    `json:"limit"`
	DelayMS int64  `json:"delay_ms"`
	Kind    string `json:"kind"`
	Message string `json:"message"`
}

func NewToolActivity(call model.ToolCallContent, now time.Time) ToolActivity {
	activity := ToolActivity{CallID: call.ToolCallID, Name: call.ToolName, Arguments: excerpt(call.Input, 16<<10), Status: "running", Started: now}
	var args struct {
		Command, Path, Query, Pattern, Input, Name, From, To string
	}
	_ = json.Unmarshal([]byte(call.Input), &args)
	switch call.ToolName {
	case "exec_command":
		activity.Title, activity.Detail = "Bash", args.Command
	case "write_stdin":
		activity.Title = "Background terminal"
	case "read", "document_read", "image_view":
		activity.Title, activity.Detail = "Read", args.Path
	case "list", "glob", "grep":
		activity.Title = "Search"
		activity.Detail = strings.TrimSpace(strings.Join([]string{args.Query, args.Pattern, args.Path}, " "))
	case "edit":
		activity.Title = "Edit"
		activity.Detail, _, _ = strings.Cut(args.Input, "\n")
	case "move", "copy":
		activity.Title = strings.ToUpper(call.ToolName[:1]) + call.ToolName[1:]
		activity.Detail = strings.TrimSpace(args.From + " → " + args.To)
	case "write", "remove":
		activity.Title = strings.ToUpper(call.ToolName[:1]) + call.ToolName[1:]
		activity.Detail = args.Path
	default:
		activity.Title = call.ToolName
		if strings.HasPrefix(call.ToolName, "mcp__") {
			activity.Title = "MCP"
			activity.Detail = strings.TrimPrefix(call.ToolName, "mcp__")
			if strings.TrimSpace(call.Input) != "" && strings.TrimSpace(call.Input) != "{}" {
				activity.Detail += " " + excerpt(call.Input, 240)
			}
		} else {
			activity.Detail = strings.TrimSpace(strings.Join([]string{args.Name, args.Path}, " "))
		}
	}
	activity.Detail = excerpt(strings.Join(strings.Fields(activity.Detail), " "), 8000)
	return activity
}

func (t ToolActivity) Key() string {
	if t.ID != "" {
		return t.ID
	}
	return t.CallID
}

func (t ToolActivity) DisplayBytes() int {
	return len(t.Arguments) + len(t.Detail) + len(t.Output) + len(t.Error) + len(t.Input)
}

func (t ToolActivity) Active() bool {
	return t.Status == "running"
}

func (t ToolActivity) Background() bool {
	return t.Status == "background"
}

func PreviewOutput(text string, limit int) (string, int) {
	text = strings.TrimRight(text, "\r\n")
	if text == "" {
		return "", 0
	}
	lines := strings.Split(text, "\n")
	if len(text) <= limit {
		return text, 0
	}
	headBudget := limit / 2
	tailBudget := limit - headBudget
	head := excerpt(text, headBudget)
	if cut := strings.LastIndexByte(head, '\n'); cut >= 0 {
		head = head[:cut]
	}
	tailStart := max(len(head), len(text)-tailBudget)
	for tailStart < len(text) && text[tailStart]&0xc0 == 0x80 {
		tailStart++
	}
	tail := text[tailStart:]
	if cut := strings.IndexByte(tail, '\n'); cut >= 0 {
		tail = tail[cut+1:]
	}
	omitted := max(1, len(lines)-len(strings.Split(head, "\n"))-len(strings.Split(tail, "\n")))
	return head + "\n…\n" + tail, omitted
}

func (t ToolActivity) WithResult(result model.ToolResultContent, now time.Time) ToolActivity {
	t.Status = "succeeded"
	t.Error = ""
	t.Finished = now
	t.DurationMS = max(0, now.Sub(t.Started).Milliseconds())
	var output string
	if text, ok := model.AsToolResultOutputType[model.ToolResultOutputContentText](result.Result); ok {
		output = text.Text
	}
	if failure, ok := model.AsToolResultOutputType[model.ToolResultOutputContentError](result.Result); ok {
		t.Status = "failed"
		if failure.Error != nil {
			t.Error = excerpt(failure.Error.Error(), 1000)
		}
	}
	var metadata struct {
		Command *CommandOutcome `json:"clara_command"`
	}
	if json.Unmarshal([]byte(result.ClientMetadata), &metadata) == nil && metadata.Command != nil {
		command := metadata.Command
		t.ExitCode = command.ExitCode
		t.DurationMS = max(t.DurationMS, command.DurationMS)
		t.SessionID = command.SessionID
		if command.Running {
			t.Status = "background"
			t.Finished = time.Time{}
		}
		if command.ExitCode != nil && *command.ExitCode != 0 {
			t.Status = "failed"
			t.Error = fmt.Sprintf("exit %d", *command.ExitCode)
		}
		if command.TimedOut {
			t.Status = "failed"
			t.Error = "Command timed out"
		}
		if command.Stopped {
			t.Status = "cancelled"
			t.Error = "Command stopped"
		}
		addition, omitted := PreviewOutput(command.Output, 16<<10)
		if t.Output != "" && !command.Snapshot {
			if addition == "" {
				addition = t.Output
			} else {
				addition = t.Output + "\n" + addition
			}
		}
		oldOmitted := t.Omitted
		t.Output, t.Omitted = PreviewOutput(addition, 16<<10)
		t.Omitted += oldOmitted + command.Omitted + omitted
		if command.Snapshot {
			t.Output, t.Omitted = PreviewOutput(command.Output, 16<<10)
			t.Omitted += command.Omitted
			t.CommandRevision = command.Revision
		}
		return t
	}
	t.Output, t.Omitted = PreviewOutput(output, 16<<10)
	return t
}

func (t ToolActivity) Bounded(limit int) ToolActivity {
	if t.DisplayBytes() <= limit {
		return t
	}
	t.Arguments = excerpt(t.Arguments, max(0, limit/4))
	t.Input = excerpt(t.Input, max(0, limit/8))
	t.Detail = excerpt(t.Detail, min(1000, max(0, limit/8)))
	t.Error = excerpt(t.Error, min(1000, max(0, limit/8)))
	outputLimit := max(0, limit-len(t.Arguments)-len(t.Detail)-len(t.Error)-len(t.Input))
	if outputLimit > 100 {
		var omitted int
		t.Output, omitted = PreviewOutput(t.Output, outputLimit)
		t.Omitted += omitted
	} else {
		t.Output = ""
		t.Omitted++
	}
	return t
}
