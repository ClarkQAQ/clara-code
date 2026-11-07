package terminal

import (
	"fmt"
	"strings"
	"time"

	"clara-code/internal/session"
)

func (a *app) backgroundSummary() string {
	if a.conv == nil {
		return ""
	}
	count := 0
	for _, terminal := range a.conv.Terminals() {
		if terminal.Status == "running" {
			count++
		}
	}
	if count == 0 {
		return ""
	}
	plural := ""
	if count != 1 {
		plural = "s"
	}
	return fmt.Sprintf("%d background terminal%s running · /ps to view · /stop to close", count, plural)
}

func (a *app) syncBackground() {
	if a.conv == nil {
		return
	}
	if err := a.conv.BackgroundError(); err != nil {
		a.notice(err)
	}
	for _, activity := range a.conv.Store.Commands() {
		found := false
		for i, cell := range a.view.history {
			if cell.Tool == nil || cell.Tool.Key() != activity.Key() {
				continue
			}
			found = true
			if cell.Tool.Status == activity.Status && cell.Tool.Output == activity.Output && cell.Tool.Error == activity.Error && cell.Tool.Title == activity.Title && cell.Tool.Omitted == activity.Omitted {
				break
			}
			copy := activity
			a.view.sourceBytes += copy.DisplayBytes() - cell.Tool.DisplayBytes()
			a.view.history[i].Tool = &copy
			a.view.trimHistory()
			a.reflow = true
			break
		}
		if found {
			continue
		}
		for i, tool := range a.toolGroup {
			if tool.Key() == activity.Key() {
				a.toolGroup[i] = activity
				found = true
				break
			}
		}
		if !found {
			if activity.Active() {
				a.toolGroup = append(a.toolGroup, activity)
			} else {
				copy := activity
				if a.view.cell(session.Cell{Kind: "tool", Tool: &copy}) {
					a.reflow = true
				}
			}
		}
	}
	a.flushTools()
}

func (a *app) showTerminals() {
	a.flushStream()
	var lines []string
	for _, terminal := range a.conv.Terminals() {
		if terminal.Status != "running" {
			continue
		}
		elapsed := terminal.Poll.Duration
		if !terminal.Started.IsZero() {
			elapsed = time.Since(terminal.Started)
		}
		lines = append(lines, fmt.Sprintf("%s · %s\n  %s\n  directory: %s", terminal.ID, elapsedCompact(elapsed), singleLine(terminal.Command), singleLine(terminal.Workdir)))
		if !terminal.Observable {
			lines = append(lines, "  automatic status unavailable; use write_stdin to poll")
		}
		output, _ := session.PreviewOutput(clean(terminal.Poll.Output), 1000)
		if output != "" {
			lines = append(lines, "  └ "+strings.ReplaceAll(output, "\n", "\n    "))
		}
	}
	if len(lines) == 0 {
		lines = []string{"No background terminals running."}
	}
	a.view.message("notice", "Background terminals\n\n"+strings.Join(lines, "\n\n"))
}

func (a *app) stopTerminals(id string) {
	if a.stoppingTerminals {
		a.hint = "Background terminals are already stopping"
		return
	}
	a.stoppingTerminals = true
	a.stopTerminalResults = make(chan error, 1)
	c := a.conv
	result := a.stopTerminalResults
	go func() { result <- c.StopTerminals(a.ctx, strings.TrimSpace(id)) }()
}
