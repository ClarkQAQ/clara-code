package session

import (
	"encoding/json"
	"slices"
	"strings"
	"time"

	"clara-code/pkg/core"
	"clara-code/pkg/models"
)

func Text(message models.Message) string {
	var out strings.Builder
	for _, part := range message.Content {
		if text, ok := models.AsMessagePart[models.TextPart](part); ok {
			out.WriteString(text.Text)
		}
	}
	return out.String()
}

func (t *Turn) collect(events []core.Event) {
	var partial strings.Builder
	partial.WriteString(t.Partial)
	for _, event := range events {
		if event.Sequence <= t.Cursor || (event.RunID != t.ID && event.RunID != "") {
			continue
		}
		t.Cursor = event.Sequence
		switch event.Kind {
		case "text_delta":
			var part models.StreamPart
			if !t.Staged && partial.Len() < 256<<10 && json.Unmarshal(event.Data, &part) == nil {
				partial.WriteString(excerpt(part.Delta, (256<<10)-partial.Len()))
			}
		case "text_end":
			if !t.Staged && partial.Len() > 0 {
				t.StreamedCells = append(t.StreamedCells, len(t.Cells))
				t.append("assistant", partial.String())
				t.StreamedText += partial.String()
				partial.Reset()
			}
		case "step.completed":
			partial.Reset()
			t.StreamedText = ""
			t.StreamedCells = nil
		case "tool.activity":
			var activity ToolActivity
			if json.Unmarshal(event.Data, &activity) == nil {
				t.upsertTool(activity)
			}
		case "tool.result":
			var result models.ToolResultContent
			if json.Unmarshal(event.Data, &result) == nil && result.ToolName == "request_user_input" {
				cell := ToolOutput(result)
				t.append(cell.Kind, cell.Text)
			}
		case "model.error":
			var failure Failure
			if json.Unmarshal(event.Data, &failure) == nil {
				t.Failure = &failure
			}
		case "input.steering", "input.follow":
			var input string
			if json.Unmarshal(event.Data, &input) == nil {
				t.append("user", input)
			}
		case "run.cancelled":
			t.append("assistant", partial.String())
			partial.Reset()
			if t.Failure != nil && t.Failure.Kind == "timeout" {
				t.finishTools("failed", event.At)
				t.append("error", t.Failure.Kind+" · "+t.Failure.Message)
			} else {
				t.finishTools("cancelled", event.At)
				t.append("notice", "Interrupted")
			}
		case "run.failed":
			t.finishTools("failed", event.At)
			t.append("assistant", partial.String())
			partial.Reset()
			var failure struct{ Error string }
			if json.Unmarshal(event.Data, &failure) == nil {
				if t.Failure != nil {
					t.append("error", t.Failure.Kind+" · "+t.Failure.Message)
				} else {
					t.append("error", failure.Error)
				}
			}
		}
	}
	t.Partial = partial.String()
}

func ToolLabel(call models.ToolCallContent) string {
	var args struct{ Command, Path, Query, Input string }
	_ = json.Unmarshal([]byte(call.Input), &args)
	if call.ToolName == "request_user_input" {
		return ""
	}
	if call.ToolName == "edit" {
		header, _, _ := strings.Cut(args.Input, "\n")
		return "edit " + excerpt(header, 160)
	}
	for _, value := range []string{args.Command, args.Path, args.Query} {
		if value != "" {
			return call.ToolName + " " + excerpt(strings.Join(strings.Fields(value), " "), 160)
		}
	}
	return call.ToolName
}

func ToolOutput(result models.ToolResultContent) Cell {
	if failure, ok := models.AsToolResultOutputType[models.ToolResultOutputContentError](result.Result); ok {
		detail := result.ToolName + " failed"
		if failure.Error != nil {
			detail += " · " + excerpt(failure.Error.Error(), 500)
		}
		return Cell{Kind: "notice", Text: detail}
	}
	if result.ToolName == "request_user_input" {
		if text, ok := models.AsToolResultOutputType[models.ToolResultOutputContentText](result.Result); ok {
			var value struct {
				Questions map[string]string
				Answers   map[string]struct {
					Answers []string
					Notes   string
				}
			}
			if json.Unmarshal([]byte(text.Text), &value) == nil {
				keys := make([]string, 0, len(value.Answers))
				for key := range value.Answers {
					keys = append(keys, key)
				}
				slices.Sort(keys)
				lines := []string{"Answered questions"}
				for _, key := range keys {
					answer := value.Answers[key]
					question := key
					if value.Questions[key] != "" {
						question = value.Questions[key]
					}
					line := question + " · " + strings.Join(answer.Answers, ", ")
					if answer.Notes != "" {
						line += " · " + answer.Notes
					}
					lines = append(lines, line)
				}
				return Cell{Kind: "activity", Text: strings.Join(lines, "\n")}
			}
		}
	}
	if result.ToolName == "exec_command" || result.ToolName == "write_stdin" || result.ToolName == "edit" || result.ToolName == "write" || result.ToolName == "remove" || result.ToolName == "move" || result.ToolName == "copy" {
		if text, ok := models.AsToolResultOutputType[models.ToolResultOutputContentText](result.Result); ok {
			lines := strings.SplitN(excerpt(strings.TrimSpace(text.Text), 1200), "\n", 9)
			if len(lines) == 9 {
				lines[8] = "…"
			}
			return Cell{Kind: "activity", Text: strings.Join(lines, "\n")}
		}
	}
	return Cell{}
}

func (t *Turn) append(kind, text string) {
	room := (1 << 20) - t.Bytes
	if text == "" {
		return
	}
	if len(t.Cells) >= 4096 || room <= 0 {
		if len(t.Cells) == 0 || t.Cells[len(t.Cells)-1].Text != "[Display history truncated. Full messages remain in session context.]" {
			t.Cells = append(t.Cells, Cell{Kind: "notice", Text: "[Display history truncated. Full messages remain in session context.]"})
		}
		return
	}
	limit := 16 << 10
	if kind == "assistant" || kind == "user" {
		limit = 256 << 10
	}
	limit = min(room, limit)
	if len(text) > limit {
		const note = "\n[Display truncated. Full messages remain in session context.]"
		if limit > len(note) {
			text = excerpt(text, limit-len(note)) + note
		} else {
			text = excerpt(text, limit)
		}
	}
	t.Cells = append(t.Cells, Cell{Kind: kind, Text: text})
	t.Bytes += len(text)
}

func excerpt(text string, limit int) string {
	if len(text) <= limit {
		return text
	}
	for limit > 0 && text[limit]&0xc0 == 0x80 {
		limit--
	}
	return text[:limit]
}

func (t *Turn) upsertTool(activity ToolActivity) {
	if activity.CallID == "" {
		return
	}
	for i, cell := range t.Cells {
		if cell.Tool != nil && cell.Tool.Key() == activity.Key() {
			budget := max(512, (1<<20)-t.Bytes+cell.Tool.DisplayBytes())
			activity = activity.Bounded(budget)
			t.Bytes += activity.DisplayBytes() - cell.Tool.DisplayBytes()
			t.Cells[i].Tool = &activity
			return
		}
	}
	if len(t.Cells) >= 4096 || t.Bytes >= 1<<20 {
		t.append("notice", "[Additional tool display records omitted.]")
		return
	}
	activity = activity.Bounded(max(512, (1<<20)-t.Bytes))
	t.Cells = append(t.Cells, Cell{Kind: "tool", Tool: &activity})
	t.Bytes += activity.DisplayBytes()
}

func (t *Turn) finishTools(status string, at time.Time) {
	for i, cell := range t.Cells {
		if cell.Tool == nil || !cell.Tool.Active() {
			continue
		}
		activity := *cell.Tool
		activity.Status = status
		activity.Finished = at
		activity.DurationMS = max(0, at.Sub(activity.Started).Milliseconds())
		if activity.Error == "" {
			activity.Error = "Interrupted before a result was recorded"
		}
		t.Cells[i].Tool = &activity
	}
}
