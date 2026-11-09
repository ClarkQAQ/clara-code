package terminal

import (
	"fmt"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"clara-code/internal/setting"
)

const (
	setupProvider = iota
	setupProtocol
	setupURL
	setupKey
	setupModel
	setupContext
	setupOutput
	setupSteps
)

var setupFields = [setupSteps]struct{ title, placeholder string }{
	{"Provider ID", "custom"},
	{"API protocol", ""},
	{"Provider base URL", "https://gateway.example/v1"},
	{"API key", "Optional · input is hidden"},
	{"Default model ID", "Model name used by the provider"},
	{"Model context window (tokens)", "Optional · leave blank if unknown"},
	{"Model max output tokens", "Optional · leave blank for provider default"},
}

var setupProtocols = []choice{
	{id: "responses", label: "Responses"},
	{id: "completions", label: "Chat Completions"},
}

type setupForm struct {
	step, protocol int
	drafts         [setupSteps]editor
	values         setting.InitialConfig
	validation     string
}

func newSetupForm(config setting.Config) *setupForm {
	s := &setupForm{}
	p := config.Connection()
	s.drafts[setupProvider].replace(config.ModelProvider)
	s.drafts[setupURL].replace(p.BaseURL)
	s.drafts[setupModel].replace(config.DefaultModel)
	if p.WireAPI == "completions" {
		s.protocol = 1
	}
	return s
}

func (s *setupForm) layout(p *painter, hints func(string) string) layout {
	height := p.contentHeight()
	field := setupFields[s.step]
	title := fmt.Sprintf("First-time setup %d/%d · %s", s.step+1, setupSteps, field.title)
	help := "Enter Continue · Esc Cancel"
	if s.step == setupProtocol {
		help = "↑/↓ Select · " + help
	}
	if s.step == setupOutput {
		help = "Enter Save and start · Esc Cancel"
	}
	if s.step > 0 {
		help += " · Shift+Tab Back"
	}
	help = hints(help)
	footerStyle := dim
	if s.validation != "" {
		help, footerStyle = clean(s.validation), red
	}
	footer := p.style(footerStyle, truncate(help, p.width))
	if s.step == setupProtocol {
		rows := listRows(p, title, setupProtocols, s.protocol, "", 0)
		if height == 1 {
			return layout{rows: rows[1:]}
		}
		if len(rows) < height && footer != "" {
			rows = append(rows, footer)
		}
		return layout{rows: rows}
	}
	e := s.drafts[s.step]
	if s.step == setupKey {
		// Mask the display copy while keeping the real editor and cursor intact.
		e = editor{
			text: strings.Repeat("*", utf8.RuneCountInString(e.text)),
			at:   utf8.RuneCountInString(e.text[:e.at]),
		}
	}
	chrome := min(4, height-1)
	view := p.inputBox(e, field.placeholder, min(4, height-chrome), max(0, chrome-2))
	if height >= 2 {
		view = view.prepend([]string{p.style(bold, truncate(title, p.width))})
	}
	if len(view.rows) < height && footer != "" {
		view.rows = append(view.rows, footer)
	}
	return view
}

func (s *setupForm) accept() string {
	value := strings.TrimSpace(s.drafts[s.step].text)
	if len(value) > 4096 || strings.ContainsFunc(value, unicode.IsControl) {
		return "Use a single line of at most 4096 bytes."
	}
	switch s.step {
	case setupProvider:
		s.values.ProviderID = value
		if value == "" {
			s.values.ProviderID = "custom"
		}
	case setupProtocol:
		s.values.WireAPI = setupProtocols[s.protocol].id
	case setupURL:
		if !setting.ValidBaseURL(value) {
			return "Enter an HTTP(S) base URL without credentials."
		}
		s.values.BaseURL = value
	case setupKey:
		s.values.APIKey = value
	case setupModel:
		if value == "" {
			return "Enter the default model ID."
		}
		s.values.ModelID = value
	case setupContext, setupOutput:
		var tokens int64
		if value != "" {
			n, err := strconv.ParseInt(value, 10, 64)
			if err != nil || n <= 0 {
				return "Enter a positive token count or leave this blank."
			}
			tokens = n
		}
		if s.step == setupContext {
			s.values.ContextWindow = tokens
		} else {
			s.values.MaxOutputTokens = tokens
		}
	}
	return ""
}

func (a *app) handleSetup(input key) bool {
	s := a.setup
	input = a.bindKey(input)
	switch input.code {
	case "escape", "ctrl-c":
		return true
	case "backtab":
		s.step = max(0, s.step-1)
		s.validation = ""
	case "enter":
		s.validation = s.accept()
		if s.validation != "" {
			break
		}
		if s.step < setupOutput {
			s.step++
			break
		}
		config, err := a.host.Config.SaveInitial(a.ctx, s.values)
		if err != nil {
			s.validation = a.host.Config.Redact(err.Error())
			break
		}
		a.host.Config = config
		a.setup = nil
	default:
		if s.step == setupProtocol {
			switch input.code {
			case "up", "left", "home":
				s.protocol = 0
			case "down", "right", "end":
				s.protocol = 1
			}
		} else if input.code != "newline" && s.drafts[s.step].key(input) {
			s.validation = ""
		}
	}
	return false
}
