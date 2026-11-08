package setting

import (
	"fmt"
	"maps"
	"strings"
)

var KeyDefaults = map[string]map[string]string{
	"composer": {"submit": "enter", "newline": "alt-enter", "cancel": "esc", "interrupt": "ctrl-c", "exit": "ctrl-d", "cycle_mode": "shift-tab", "paste_image": "ctrl-v", "attachments": "ctrl-g", "history_previous": "up", "history_next": "down", "clear_screen": "ctrl-l", "complete_command": "tab", "cursor_left": "left", "cursor_right": "right", "word_left": "ctrl-left", "word_right": "ctrl-right", "line_start": "home", "line_end": "end", "emacs_line_start": "ctrl-a", "emacs_line_end": "ctrl-e", "backspace": "backspace", "delete_forward": "delete", "delete_word": "ctrl-w", "kill_to_start": "ctrl-u", "kill_to_end": "ctrl-k"},
	"picker":   {"confirm": "enter", "cancel": "esc", "interrupt": "ctrl-c", "previous": "up", "next": "down", "first": "home", "last": "end", "page_up": "page-up", "page_down": "page-down", "delete_attachment": "delete"},
	"question": {"submit": "enter", "cancel": "esc", "interrupt": "ctrl-c", "notes": "tab", "previous_question": "left", "next_question": "right", "previous": "up", "next": "down"},
	"rewind":   {"confirm": "enter", "cancel": "esc", "interrupt": "ctrl-c", "details": "ctrl-t", "previous": "left", "next": "right", "scroll_up": "up", "scroll_down": "down"},
}

func KeyCode(key string) string {
	switch strings.ToLower(key) {
	case "esc", "escape":
		return "escape"
	case "shift-tab", "backtab":
		return "backtab"
	case "alt-enter", "shift-enter", "newline", "ctrl-j":
		return "newline"
	case "return", "ctrl-m":
		return "enter"
	case "ctrl-i":
		return "tab"
	case "ctrl-left", "alt-left":
		return "word-left"
	case "ctrl-right", "alt-right":
		return "word-right"
	}
	return strings.ToLower(key)
}
func keyValid(key string) bool {
	key = KeyCode(key)
	switch key {
	case "enter", "newline", "escape", "tab", "backtab", "up", "down", "left", "right", "home", "end", "page-up", "page-down", "backspace", "delete", "word-left", "word-right":
		return true
	}
	return (strings.HasPrefix(key, "ctrl-") && len(key) == 6 && key[5] >= 'a' && key[5] <= 'z') || (strings.HasPrefix(key, "alt-") && len(key) == 5 && key[4] >= 'a' && key[4] <= 'z')
}
func (t TUI) Bindings(context string) map[string]string {
	out := maps.Clone(KeyDefaults[context])
	for action, key := range t.Keymap[context] {
		out[action] = key
	}
	return out
}
func (t TUI) ValidateKeys() error {
	for context, values := range t.Keymap {
		defaults, ok := KeyDefaults[context]
		if !ok {
			return fmt.Errorf("unknown context %q", context)
		}
		for action, key := range values {
			if _, ok := defaults[action]; !ok {
				return fmt.Errorf("unknown action %s.%s", context, action)
			}
			if !keyValid(key) {
				return fmt.Errorf("unsupported key for %s.%s", context, action)
			}
		}
	}
	for context := range KeyDefaults {
		seen := map[string]string{}
		for action, key := range t.Bindings(context) {
			code := KeyCode(key)
			if old, ok := seen[code]; ok {
				return fmt.Errorf("%s.%s and %s.%s use the same key", context, old, context, action)
			}
			seen[code] = action
		}
	}
	return nil
}

// Dispatch maps configured keys to the canonical events used by each UI surface.
func (t TUI) Dispatch(context, code string) string {
	for action, key := range t.Bindings(context) {
		if KeyCode(key) == code {
			return KeyCode(KeyDefaults[context][action])
		}
	}
	for action, key := range KeyDefaults[context] {
		if KeyCode(key) == code && KeyCode(t.Bindings(context)[action]) != code {
			return "unbound"
		}
	}
	return code
}
func (t TUI) Hint(context, action string) string { return KeyLabel(t.Bindings(context)[action]) }
func KeyLabel(key string) string {
	switch key {
	case "esc", "escape":
		return "Esc"
	case "enter", "return":
		return "Enter"
	case "tab":
		return "Tab"
	case "up":
		return "↑"
	case "down":
		return "↓"
	case "left":
		return "←"
	case "right":
		return "→"
	case "delete":
		return "Delete"
	}
	parts := strings.Split(key, "-")
	for i, part := range parts {
		if len(part) == 1 {
			parts[i] = strings.ToUpper(part)
		} else if len(part) > 0 {
			parts[i] = strings.ToUpper(part[:1]) + part[1:]
		}
	}
	return strings.Join(parts, "+")
}
