package terminal

import (
	"io"
	"path/filepath"
	"strings"
	"time"
	"unicode"

	"clara-code/internal/setting"
)

type titleActivity uint8

const (
	titleIdle titleActivity = iota
	titleWorking
	titleNeedsInput
)

var titleSpinner = [...]string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
var screenTitleSpinner = [...]string{"|", "/", "-", "\\"}

type terminalTitle struct {
	screen   bool
	root     string
	name     string
	activity titleActivity
	since    time.Time
	last     string
}

func (a *app) updateTitle(now time.Time) error {
	if a.conv == nil && a.op != nil && !a.options.PickSession {
		return nil
	}
	activity := titleIdle
	switch {
	case a.question != nil || (a.picker != nil && a.picker.kind == planPicker):
		activity = titleNeedsInput
	case a.run != nil || a.op != nil:
		activity = titleWorking
	}
	return a.title.update(a.view.output, a.space.Root, activity, setting.Enabled(a.host.Config.TUI.Animations), now)
}

func (t *terminalTitle) update(out io.Writer, root string, activity titleActivity, animate bool, now time.Time) error {
	if t.name == "" || t.root != root {
		t.root, t.name = root, titleProjectName(root, t.screen)
	}
	if t.activity != activity {
		t.activity, t.since = activity, now
	}
	var elapsed time.Duration
	if animate {
		elapsed = now.Sub(t.since)
	}
	title := t.name
	switch activity {
	case titleWorking:
		step := int(elapsed / (100 * time.Millisecond))
		frame := titleSpinner[step%len(titleSpinner)]
		if t.screen {
			frame = screenTitleSpinner[step%len(screenTitleSpinner)]
		}
		title = frame + " " + title
	case titleNeedsInput:
		marker := "[ ! ]"
		if elapsed/time.Second%2 == 1 {
			marker = "[ . ]"
		}
		title = marker + " Action Required | " + title
	}
	if title == t.last {
		return nil
	}
	sequence := "\x1b]0;" + title + "\x07"
	if t.last == "" {
		sequence = "\x1b[22;0t" + sequence
	}
	if _, err := io.WriteString(out, sequence); err != nil {
		return err
	}
	t.last = title
	return nil
}

func (t *terminalTitle) close(out io.Writer) error {
	if t.last == "" {
		return nil
	}
	// Clear the managed title even on terminals without a title stack.
	_, err := io.WriteString(out, "\x1b]0;\x07\x1b[23;0t")
	return err
}

func titleProjectName(root string, screen bool) string {
	name := strings.Map(func(r rune) rune {
		switch {
		case unicode.IsSpace(r):
			return ' '
		case unicode.IsControl(r) || unicode.Is(unicode.Cf, r):
			return -1
		case screen && r > unicode.MaxASCII:
			return '?'
		default:
			return r
		}
	}, filepath.Base(root))
	name = strings.Join(strings.Fields(name), " ")
	if name == "" || name == "." || name == string(filepath.Separator) {
		return "clara-code"
	}
	return name
}
