package terminal

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"

	"clara-code/internal/session"
)

var shellToken = regexp.MustCompile(`'[^']*'|"(?:\\.|[^"])*"|\$[a-zA-Z_][a-zA-Z0-9_]*|\$\{[^}]*\}|&&|\|\||[|;<>]|--?[a-zA-Z][a-zA-Z0-9_-]*`)

func shellSpans(command string) []span {
	var spans []span
	at := 0
	for _, match := range shellToken.FindAllStringIndex(command, -1) {
		spans = append(spans, span{text: command[at:match[0]]})
		token := command[match[0]:match[1]]
		color := accent
		if strings.HasPrefix(token, "'") || strings.HasPrefix(token, "\"") {
			color = "\x1b[32m"
		}
		if strings.HasPrefix(token, "-") {
			color = "\x1b[33m"
		}
		spans = append(spans, span{text: token, style: color})
		at = match[1]
	}
	return append(spans, span{text: command[at:]})
}

func indicator(now time.Time) string {
	frames := []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
	return frames[(now.UnixMilli()/100)%int64(len(frames))]
}

var toolPresentations = map[string]struct {
	title    string
	argument string
}{
	"stat":              {title: "Stat", argument: "path"},
	"fs_instructions":   {title: "Instructions", argument: "path"},
	"review_changes":    {title: "ReviewChanges", argument: "kind"},
	"get_plan":          {title: "Plan"},
	"update_plan":       {title: "UpdatePlan"},
	"get_goal":          {title: "Goal"},
	"update_goal":       {title: "UpdateGoal", argument: "status"},
	"spawn_agent":       {title: "SpawnAgent", argument: "task_name"},
	"subagent_task":     {title: "SubagentTask", argument: "task_name"},
	"list_agents":       {title: "ListAgents"},
	"send_message":      {title: "SendMessage", argument: "target"},
	"followup_task":     {title: "FollowupTask", argument: "target"},
	"wait_agent":        {title: "WaitAgent", argument: "target"},
	"interrupt_agent":   {title: "InterruptAgent", argument: "target"},
	"close_agent":       {title: "CloseAgent", argument: "target"},
	"list_acp_sessions": {title: "ListACPSessions", argument: "role"},
}

func toolTitle(activity session.ToolActivity) string {
	if presentation, ok := toolPresentations[activity.Name]; ok {
		return presentation.title
	}
	switch activity.Name {
	case "read_skill", "read_skill_resource":
		return "Skill"
	default:
		return activity.Title
	}
}

func toolArgument(arguments, key string) string {
	if key == "" {
		return ""
	}
	var args map[string]json.RawMessage
	if json.Unmarshal([]byte(arguments), &args) != nil {
		return ""
	}
	var value string
	if json.Unmarshal(args[key], &value) != nil {
		return ""
	}
	return singleLine(value)
}

func toolDetail(activity session.ToolActivity) string {
	if presentation, ok := toolPresentations[activity.Name]; ok {
		return toolArgument(activity.Arguments, presentation.argument)
	}
	switch activity.Name {
	case "read_skill", "read_skill_resource":
		var args struct {
			Name string `json:"name"`
			Path string `json:"path"`
		}
		if json.Unmarshal([]byte(activity.Arguments), &args) != nil {
			return ""
		}
		name := singleLine(args.Name)
		if name != "" && activity.Name == "read_skill_resource" {
			if path := singleLine(args.Path); path != "" {
				return name + "/" + path
			}
		}
		return name
	default:
		return activity.Detail
	}
}

func (p *painter) toolRows(activity session.ToolActivity, now time.Time) []string {
	marker, color := "●", "\x1b[32m"
	switch activity.Status {
	case "running":
		marker, color = p.indicator(now), dim
	case "background":
		color = accent
	case "failed":
		color = "\x1b[31m"
	case "cancelled":
		marker, color = "○", dim
	}
	width := max(1, p.width-2)
	if activity.Interaction != "" {
		title := singleLine(activity.Title)
		header := "• " + title
		if activity.Active() {
			header = p.indicator(now) + " " + title
		}
		if activity.Detail != "" {
			header += " · " + singleLine(activity.Detail)
		}
		var lines []string
		for _, row := range wrap(header, max(1, p.width)) {
			lines = append(lines, p.style(dim, row.text))
		}
		if activity.Input != "" {
			lines = append(lines, p.style(dim, truncate("  └ "+singleLine(activity.Input), p.width)))
		}
		if activity.Error != "" {
			lines = append(lines, p.style(dim, truncate("  └ "+singleLine(activity.Error), p.width)))
		}
		return append(lines, "")
	}
	title := toolTitle(activity)
	if title == "" {
		title = activity.Name
	}
	var heading []span
	heading = append(heading, span{text: singleLine(title), style: bold})
	_, showEmptyParens := toolPresentations[activity.Name]
	if detail := toolDetail(activity); detail != "" || showEmptyParens {
		heading = append(heading, span{text: "(", style: dim})
		if activity.Name == "exec_command" {
			heading = append(heading, shellSpans(clean(detail))...)
		} else {
			heading = append(heading, span{text: clean(detail)})
		}
		heading = append(heading, span{text: ")", style: dim})
	}
	switch activity.Status {
	case "failed":
		detail := " · failed"
		if activity.ExitCode != nil && *activity.ExitCode != 0 {
			detail = fmt.Sprintf(" · exit %d", *activity.ExitCode)
		}
		heading = append(heading, span{text: detail, style: "\x1b[31m"})
	case "cancelled":
		heading = append(heading, span{text: " · cancelled", style: dim})
	case "background":
		heading = append(heading, span{text: " · background", style: dim})
	}
	rows := p.spanRows(heading, width)
	for i := range rows {
		prefix := "  "
		if i == 0 {
			prefix = p.style(color, marker) + " "
		}
		if p.width < 4 {
			prefix = ""
		}
		rows[i] = prefix + rows[i]
	}
	output := strings.TrimRight(clean(activity.Output), "\n")
	var lines []string
	if output != "" {
		lines = strings.Split(output, "\n")
	}
	omitted := activity.Omitted
	head, tail := 3, 2
	if p.previewLines != nil {
		head = (*p.previewLines + 1) / 2
		tail = *p.previewLines / 2
	}
	if p.previewLines != nil && *p.previewLines == 0 {
		lines = nil
	} else if len(lines) > head+tail+1 {
		omitted += len(lines) - head - tail
		preview := append([]string{}, lines[:head]...)
		preview = append(preview, fmt.Sprintf("… %d lines omitted", omitted))
		lines = append(preview, lines[len(lines)-tail:]...)
	} else if omitted > 0 {
		lines = append(lines, fmt.Sprintf("… %d lines omitted from retained output", omitted))
	}
	redundantExit := activity.ExitCode != nil && activity.Error == fmt.Sprintf("exit %d", *activity.ExitCode)
	if activity.Error != "" && (!redundantExit || output == "") {
		lines = append(lines, clean(activity.Error))
	}
	if len(lines) == 0 && output == "" && activity.Status == "succeeded" {
		lines = []string{"(no output)"}
	}
	for i, line := range lines {
		prefix := "    "
		if i == 0 {
			prefix = "  └ "
		}
		if p.width < 6 {
			prefix = ""
		}
		rows = append(rows, p.style(dim, prefix+truncate(line, max(1, p.width-widthOf(prefix)))))
	}
	return append(rows, "")
}

func (a *app) onToolActivity(activity session.ToolActivity) {
	if activity.CallID == "" {
		return
	}
	if a.conv != nil {
		for _, latest := range a.conv.Store.Commands() {
			if latest.Key() == activity.Key() {
				activity = latest
				break
			}
		}
	}
	a.flushStream()
	a.activity.quiet = false
	for i, cell := range a.view.history {
		if cell.Tool != nil && cell.Tool.Key() == activity.Key() {
			snapshot := activity
			a.view.sourceBytes += snapshot.DisplayBytes() - cell.Tool.DisplayBytes()
			a.view.history[i].Tool = &snapshot
			a.view.trimHistory()
			a.reflow = true
			return
		}
	}
	for i, tool := range a.toolGroup {
		if tool.Key() == activity.Key() {
			a.toolGroup[i] = activity
			a.flushTools()
			return
		}
	}
	a.toolGroup = append(a.toolGroup, activity)
	a.flushTools()
}

func (a *app) flushTools() {
	for _, tool := range a.toolGroup {
		if tool.Active() {
			return
		}
	}
	for i := range a.toolGroup {
		snapshot := a.toolGroup[i]
		if a.view.cell(session.Cell{Kind: "tool", Tool: &snapshot}) {
			a.reflow = true
		}
	}
	a.toolGroup = nil
}

func (a *app) finishTools(cancelled bool) {
	for i, tool := range a.toolGroup {
		if tool.Active() {
			tool.Status = "failed"
			tool.Error = "Execution ended before a result was received"
			if cancelled {
				tool.Status = "cancelled"
				tool.Error = "Interrupted"
			}
			tool.Finished = time.Now()
			a.toolGroup[i] = tool
		}
	}
	a.flushTools()
}

func (p *painter) indicator(now time.Time) string {
	if p.animate != nil && !*p.animate {
		return "•"
	}
	return indicator(now)
}
