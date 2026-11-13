package terminal

import (
	"fmt"
	"math/rand/v2"
	"strings"
	"time"

	"clara-code/internal/session"
)

type activityState struct {
	quiet      bool
	started    time.Time
	paused     time.Time
	header     string
	retry      *session.RetryStatus
	retryUntil time.Time
	failure    *session.Failure
}

var workingVerbs = []string{"Proofing", "Considering", "Pondering", "Brewing", "Exploring", "Composing", "Thinking", "Crafting", "Checking", "Cogitating"}

func (s *activityState) start(now time.Time) {
	*s = activityState{started: now, header: workingVerbs[rand.IntN(len(workingVerbs))]}
}

func (s *activityState) pause(now time.Time) {
	if s.paused.IsZero() {
		s.paused = now
	}
}

func (s *activityState) resume(now time.Time) {
	if !s.paused.IsZero() {
		s.started = s.started.Add(now.Sub(s.paused))
		s.paused = time.Time{}
	}
}

func elapsedCompact(duration time.Duration) string {
	return elapsedSeconds(int64(duration / time.Second))
}

func elapsedSeconds(seconds int64) string {
	seconds = max(0, seconds)
	if seconds < 60 {
		return fmt.Sprintf("%ds", seconds)
	}
	if seconds < 3600 {
		return fmt.Sprintf("%dm %02ds", seconds/60, seconds%60)
	}
	return fmt.Sprintf("%dh %02dm %02ds", seconds/3600, seconds/60%60, seconds%60)
}

func summaryTitle(text string) string {
	text = clean(text)
	if at := strings.Index(text, "**"); at >= 0 {
		if end := strings.Index(text[at+2:], "**"); end >= 0 {
			return truncate(singleLine(text[at+2:at+2+end]), 100)
		}
	}
	first, _, _ := strings.Cut(strings.TrimSpace(text), "\n")
	return truncate(strings.Trim(first, " #*"), 100)
}

func (a *app) activityRows(now time.Time) []string {
	if (a.run == nil && a.op == nil) || (a.op == nil && a.activity.quiet && a.activity.retry == nil && a.activity.failure == nil) {
		return nil
	}
	p := &a.view
	header := a.activity.header
	if header == "" {
		header = "Working"
	}
	detail := ""
	style := dim
	if a.op != nil {
		header = a.opLabel
	} else if a.activity.failure != nil {
		header = "Error · " + a.activity.failure.Kind
		detail = a.activity.failure.Message
		style = "\x1b[31m"
	} else if retry := a.activity.retry; retry != nil {
		header = fmt.Sprintf("Reconnecting… %d/%d", retry.Attempt, retry.Limit)
		detail = retry.Kind + " · " + retry.Message
		if wait := a.activity.retryUntil.Sub(now); wait > 0 {
			detail += " · retry in " + elapsedCompact(wait+time.Second)
		}
	} else {
		for _, tool := range a.toolGroup {
			if tool.Active() {
				header = "Running " + toolTitle(tool)
				if tool.Interaction != "" {
					header = tool.Title
				}
				break
			}
		}
		for _, cell := range a.view.history {
			if cell.Tool != nil && cell.Tool.Active() {
				header = "Running " + toolTitle(*cell.Tool)
			}
		}
	}
	started := a.activity.started
	if started.IsZero() {
		started = now
	}
	clock := now
	if !a.activity.paused.IsZero() {
		clock = a.activity.paused
	}
	label := singleLine(header) + " (" + elapsedCompact(clock.Sub(started))
	if hint := a.hints("Esc"); hint != "" {
		label += " · " + strings.ToLower(hint) + " to interrupt"
	}
	label += ")"
	prefix := p.indicator(now) + " "
	if p.width < 4 {
		prefix = ""
	}
	rows := []string{p.style(style, prefix+truncate(label, max(1, p.width-widthOf(prefix))))}
	if detail != "" {
		for i, row := range wrap(clean(detail), max(1, p.width-4)) {
			prefix := "    "
			if i == 0 {
				prefix = "  └ "
			}
			if p.width < 6 {
				prefix = ""
			}
			rows = append(rows, p.style(dim, prefix+truncate(row.text, max(1, p.width-widthOf(prefix)))))
			if i == 1 {
				break
			}
		}
	}
	if summary := a.backgroundSummary(); summary != "" {
		combined := label + " · " + summary
		if widthOf(prefix+combined) <= p.width {
			rows[0] = p.style(style, prefix+combined)
		} else {
			rows = append(rows, p.style(dim, truncate("  "+summary, p.width)))
		}
	}
	return append(rows, "")
}

func (a *app) runError(err error) {
	failure := session.DescribeFailure(err)
	if a.activity.failure != nil {
		failure = *a.activity.failure
	}
	failure.Message = a.host.Config.Redact(failure.Message)
	a.view.message("error", failure.Kind+" · "+failure.Message)
}
