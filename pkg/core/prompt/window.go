package prompt

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"

	"clara-code/pkg/core"
	"clara-code/pkg/model"
)

// Window keeps request-only context anchored to the durable transcript.
// Its journal is saved independently of transcript messages and never replayed as UI history.
type Window struct {
	Version      int
	MessageCount int
	Fingerprint  string
	Snapshot     []Fragment
	Entries      []ContextEntry
}

type ContextEntry struct {
	At      int
	Changes []Change
}

type Change struct {
	Fragment  Fragment
	Operation string `json:",omitempty"`
}

type fragmentKey struct {
	Role                model.MessageRole
	Kind, Source, Scope string
}

func key(fragment Fragment) fragmentKey {
	return fragmentKey{fragment.Role, fragment.Kind, fragment.Source, fragment.Scope}
}

func LoadWindow(raw json.RawMessage) (Window, error) {
	var window Window
	if len(raw) == 0 || string(raw) == "null" {
		return window, nil
	}
	if len(raw) > core.MaxContextStateBytes {
		return window, errors.New("context window checkpoint exceeds limit")
	}
	if err := json.Unmarshal(raw, &window); err != nil {
		return Window{}, fmt.Errorf("invalid context window checkpoint: %w", err)
	}
	if window.Version != 1 {
		return Window{}, errors.New("unsupported context window checkpoint version")
	}
	if len(window.Entries) > 8192 || window.MessageCount < 0 {
		return Window{}, errors.New("invalid context window journal")
	}
	last := 0
	for _, entry := range window.Entries {
		if entry.At < last || entry.At > window.MessageCount {
			return Window{}, errors.New("invalid context window anchor")
		}
		last = entry.At
	}
	return window, nil
}

func (w Window) Marshal() (json.RawMessage, error) {
	body, err := json.Marshal(w)
	if err == nil && len(body) > core.MaxContextStateBytes {
		return nil, errors.New("context window checkpoint exceeds limit; compact the conversation")
	}
	return body, err
}

func fingerprint(history []model.Message) (string, error) {
	body, err := json.Marshal(history)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:]), nil
}

// Matches reports whether the checkpoint belongs to this transcript prefix.
func (w Window) Matches(history []model.Message) bool {
	if w.Version != 1 || w.MessageCount < 0 || w.MessageCount > len(history) {
		return false
	}
	digest, err := fingerprint(history[:w.MessageCount])
	return err == nil && digest == w.Fingerprint
}

// Prepare creates a stable initial prefix and appends changes at safe boundaries.
// The returned window is a candidate; callers checkpoint it before model dispatch.
func (w Window) Prepare(history []model.Message, current []Fragment) ([]model.Message, Window, error) {
	if w.MessageCount < 0 || len(w.Entries) > 8192 {
		return nil, w, errors.New("invalid context window journal")
	}
	previousAnchor := 0
	for _, entry := range w.Entries {
		if entry.At < previousAnchor || entry.At > w.MessageCount {
			return nil, w, errors.New("invalid context window anchor")
		}
		previousAnchor = entry.At
	}
	if w.Version != 0 && w.Version != 1 {
		return nil, w, errors.New("unsupported context window version")
	}
	if w.Version != 0 && !w.Matches(history) {
		w = Window{}
	}
	var snapshot []Fragment
	seen := map[fragmentKey]bool{}
	for _, fragment := range current {
		if strings.TrimSpace(fragment.Text) == "" {
			continue
		}
		if fragment.Role != model.MessageRoleUser && fragment.Role != model.MessageRoleDeveloper {
			return nil, w, errors.New("context requires user or developer roles")
		}
		if seen[key(fragment)] {
			return nil, w, errors.New("duplicate context fragment identity")
		}
		seen[key(fragment)] = true
		snapshot = append(snapshot, fragment)
	}
	old := map[fragmentKey]Fragment{}
	for _, fragment := range w.Snapshot {
		old[key(fragment)] = fragment
	}
	var changes []Change
	for _, fragment := range snapshot {
		previous, exists := old[key(fragment)]
		if !exists {
			changes = append(changes, Change{Fragment: fragment})
		} else if previous != fragment {
			changes = append(changes, Change{Fragment: fragment, Operation: "replace"})
		}
	}
	for _, fragment := range w.Snapshot {
		if !seen[key(fragment)] {
			changes = append(changes, Change{Fragment: fragment, Operation: "remove"})
		}
	}
	initial := w.Version == 0
	at := len(history)
	if initial {
		at = 0
	} else {
		for i := len(history) - 1; i >= w.MessageCount; i-- {
			if history[i].Role == model.MessageRoleUser {
				at = i
				break
			}
		}
	}
	for at < len(history) && !completeTools(history[:at]) {
		at++
	}
	if !completeTools(history[:at]) {
		return nil, w, errors.New("no safe context insertion boundary")
	}
	w.Entries = slices.Clone(w.Entries)
	if len(changes) > 0 {
		if len(w.Entries) >= 8192 {
			return nil, w, errors.New("context window journal exceeds limit; compact the conversation")
		}
		// Developer policy precedes contextual user data at each insertion point.
		slices.SortStableFunc(changes, func(a, b Change) int {
			if a.Fragment.Role == b.Fragment.Role {
				return 0
			}
			if a.Fragment.Role == model.MessageRoleDeveloper {
				return -1
			}
			return 1
		})
		w.Entries = append(w.Entries, ContextEntry{At: at, Changes: changes})
	}
	w.Version, w.MessageCount, w.Snapshot = 1, len(history), snapshot
	var err error
	w.Fingerprint, err = fingerprint(history)
	if err != nil {
		return nil, w, err
	}
	var messages []model.Message
	position := 0
	for _, entry := range w.Entries {
		messages = append(messages, history[position:entry.At]...)
		messages = append(messages, renderChanges(entry.Changes)...)
		position = entry.At
	}
	messages = append(messages, history[position:]...)
	return messages, w, nil
}

func renderChanges(changes []Change) []model.Message {
	var messages []model.Message
	for i := 0; i < len(changes); {
		change := changes[i]
		text := renderChange(change)
		next := i + 1
		if change.Fragment.Kind == "environment" {
			var fields strings.Builder
			for next = i; next < len(changes) && changes[next].Fragment.Kind == "environment" && changes[next].Fragment.Role == change.Fragment.Role; next++ {
				field := changes[next]
				value := field.Fragment.Text
				if field.Operation == "remove" {
					value = "[removed]"
				}
				name := field.Fragment.Source
				if !environmentName(name) {
					name = "value"
				}
				fields.WriteString(renderEnvironmentField(name, value))
			}
			text = "<environment_context>\n" + fields.String() + "</environment_context>"
		}
		role := change.Fragment.Role
		if len(messages) == 0 || messages[len(messages)-1].Role != role {
			messages = append(messages, model.Message{Role: role, Metadata: &model.MessageMetadata{Kind: "context"}})
		}
		last := len(messages) - 1
		messages[last].Content = append(messages[last].Content, model.TextPart{Text: text})
		i = next
	}
	for i := range messages {
		if messages[i].Role != model.MessageRoleDeveloper || len(messages[i].Content) < 2 {
			continue
		}
		var parts []string
		for _, part := range messages[i].Content {
			text, _ := model.AsMessagePart[model.TextPart](part)
			parts = append(parts, text.Text)
		}
		messages[i].Content = []model.MessagePart{model.TextPart{Text: strings.Join(parts, "\n\n")}}
	}
	return messages
}

func environmentName(name string) bool {
	if name == "" {
		return false
	}
	for _, char := range name {
		if char != '_' && (char < 'a' || char > 'z') {
			return false
		}
	}
	return true
}

// Prefix restores historical context for a complete transcript prefix, excluding
// updates anchored to the next turn. It does not apply the current snapshot.
func (w Window) Prefix(history []model.Message) ([]model.Message, error) {
	if len(history) > w.MessageCount {
		return nil, errors.New("context prefix exceeds checkpoint transcript")
	}
	var messages []model.Message
	position := 0
	for _, entry := range w.Entries {
		if entry.At >= len(history) {
			break
		}
		if entry.At < position {
			return nil, errors.New("invalid context prefix anchor")
		}
		messages = append(messages, history[position:entry.At]...)
		messages = append(messages, renderChanges(entry.Changes)...)
		position = entry.At
	}
	return append(messages, history[position:]...), nil
}

func renderChange(change Change) string {
	f := change.Fragment
	label := map[string]string{
		"global_instructions": "global AGENTS.md instructions", "workspace_instructions": "directory AGENTS.md instructions",
		"collaboration_mode": "collaboration mode instructions", "permissions_instructions": "permissions instructions",
		"agent_role": "agent role instructions", "context_instructions": "context guidance", "tool_workflow": "tool workflow guidance",
		"plan": "execution plan", "goal": "goal state", "tasks": "child task state", "background_terminals": "background terminal state", "skills": "skill catalog",
	}[f.Kind]
	if label == "" {
		label = "host guidance"
	}
	if change.Operation == "remove" {
		return fmt.Sprintf("Previously supplied %s from %s no longer applies.", label, f.Source)
	}
	text := f.Text
	if f.Role == model.MessageRoleUser {
		switch f.Kind {
		case "global_instructions", "workspace_instructions":
			text = fmt.Sprintf("# AGENTS.md instructions for %s\nSource: %s\n\n<INSTRUCTIONS>\n%s\n</INSTRUCTIONS>", f.Scope, f.Source, f.Text)
		case "skills":
			text = "# Available skills\n" + f.Text
		case "plan":
			text = "<execution_plan>\n" + f.Text + "\n</execution_plan>"
		case "goal":
			text = "<active_goal>\n" + f.Text + "\n</active_goal>"
		case "tasks":
			text = "<child_tasks>\n" + f.Text + "\n</child_tasks>"
		case "background_terminals":
			text = "<background_terminals>\n" + f.Text + "\n</background_terminals>"
		case "task_assignment":
			text = "<task_assignment>\n" + f.Text + "\n</task_assignment>"
		}
	}
	if change.Operation == "replace" && f.Kind != "environment" {
		text = fmt.Sprintf("This %s replaces the previous version from %s.\n\n%s", label, f.Source, text)
	}
	return text
}
