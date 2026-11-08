package prompt

import (
	"slices"
	"strings"

	"clara-code/pkg/model"
)

// Fragment is host context, separate from the durable conversation.
// Kind identifies its content; Source and Scope describe provenance, not authority.
type Fragment struct {
	Kind      string            `json:"kind"`
	Source    string            `json:"source"`
	Scope     string            `json:"scope,omitempty"`
	Lifecycle string            `json:"lifecycle"`
	Role      model.MessageRole `json:"role"`
	Text      string            `json:"text,omitempty"`
}

func (f Fragment) Message() model.Message {
	return renderChanges([]Change{{Fragment: f}})[0]
}

// Instructions separates stable collaboration guidance from capability policy.
func Instructions(config Config) (string, model.Message) {
	base := render("base", config)
	policy := model.NewDeveloperMessage()
	for _, fragment := range Policy(config) {
		policy.Content = append(policy.Content, model.TextPart{Text: fragment.Text})
	}
	return base, policy
}

// Policy returns independently replaceable capability instructions below the base.
func Policy(config Config) []Fragment {
	var fragments []Fragment
	for _, name := range sections(config) {
		if name == "base" {
			continue
		}
		if text := render(name, config); text != "" {
			fragments = append(fragments, Fragment{Kind: name, Source: "prompt", Lifecycle: "window", Role: model.MessageRoleDeveloper, Text: text})
		}
	}
	return fragments
}

// Assemble inserts the latest host snapshot before the active user turn.
// It never modifies the caller's history or persists context fragments.
func Assemble(history []model.Message, fragments []Fragment, policies ...model.Message) []model.Message {
	cut := ActiveTurn(history)
	out := slices.Clone(policies)
	out = append(out, history[:cut]...)
	for _, fragment := range fragments {
		if strings.TrimSpace(fragment.Text) != "" {
			out = append(out, fragment.Message())
		}
	}
	return append(out, history[cut:]...)
}

// ActiveTurn finds the most recent genuine submission, excluding steering,
// contextual messages and compaction handoffs. Unannotated user messages are
// treated conservatively as legacy submissions.
func ActiveTurn(history []model.Message) int {
	for i := len(history) - 1; i >= 0; i-- {
		message := history[i]
		if message.Role != model.MessageRoleUser {
			continue
		}
		if message.Metadata != nil {
			if message.Metadata.Kind == "user" || message.Metadata.Kind == "followup" {
				return i
			}
			continue
		}
		if !isLegacyHandoff(message) {
			return i
		}
	}
	return 0
}

func isLegacyHandoff(message model.Message) bool {
	for _, part := range message.Content {
		if text, ok := model.AsMessagePart[model.TextPart](part); ok && strings.HasPrefix(text.Text, "Prior conversation handoff:\n") {
			return true
		}
	}
	return false
}
