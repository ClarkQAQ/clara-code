package setting

import "testing"

func TestHistorySearchKeymap(t *testing.T) {
	for _, test := range []struct {
		name     string
		bindings map[string]string
		hint     string
		ctrlR    string
		altR     string
		wantErr  bool
	}{
		{name: "default", hint: "Ctrl+R", ctrlR: "ctrl-r", altR: "alt-r"},
		{name: "remapped", bindings: map[string]string{"history_search": "alt-r"}, hint: "Alt+R", ctrlR: "unbound", altR: "ctrl-r"},
		{name: "preserves existing binding", bindings: map[string]string{"history_previous": "Ctrl-R"}, ctrlR: "up", altR: "alt-r"},
		{name: "explicit search alongside existing binding", bindings: map[string]string{"history_previous": "ctrl-r", "history_search": "alt-r"}, hint: "Alt+R", ctrlR: "up", altR: "ctrl-r"},
		{name: "explicit conflict", bindings: map[string]string{"history_previous": "ctrl-r", "history_search": "ctrl-r"}, wantErr: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			keys := TUI{Keymap: map[string]map[string]string{"composer": test.bindings}}
			if err := keys.ValidateKeys(); (err != nil) != test.wantErr {
				t.Fatalf("ValidateKeys() returned %v, want error %t", err, test.wantErr)
			}
			if test.wantErr {
				return
			}
			if got := keys.Hint("composer", "history_search"); got != test.hint {
				t.Errorf("history search hint: got %q, want %q", got, test.hint)
			}
			if got := keys.Dispatch("composer", "ctrl-r"); got != test.ctrlR {
				t.Errorf("Ctrl+R dispatch: got %q, want %q", got, test.ctrlR)
			}
			if got := keys.Dispatch("composer", "alt-r"); got != test.altR {
				t.Errorf("Alt+R dispatch: got %q, want %q", got, test.altR)
			}
		})
	}
}
