package terminal

import (
	"strings"
	"testing"
	"time"

	"clara-code/pkg/core/prompt"
	"clara-code/pkg/util/localtime"
)

func TestStatusAndResumeUseLocalDateTime(t *testing.T) {
	a := testApp(t)
	before := time.Now().Truncate(time.Second)
	a.showStatus()
	text := a.view.history[len(a.view.history)-1].Text
	found := false
	for _, line := range strings.Split(text, "\n") {
		if raw, ok := strings.CutPrefix(line, "Local time  "); ok {
			parsed, err := time.ParseInLocation(time.DateTime, raw, time.Local)
			if err != nil || parsed.Before(before) || parsed.After(time.Now()) {
				t.Fatalf("status time is not local DateTime: %q %v", raw, err)
			}
			found = true
		}
	}
	if !found || !strings.Contains(text, "Time zone  "+localtime.Zone(before)) {
		t.Fatal("status clock or explicit time zone missing", text)
	}
	run, err := a.conv.Start(t.Context(), prompt.Execute, "Saved message", false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := run.Wait(t.Context()); err != nil {
		t.Fatal(err)
	}
	a.openResume(false)
	if a.picker == nil || len(a.picker.items) != 1 {
		t.Fatal("resume entry missing")
	}
	label := a.picker.items[0].label
	stamp, _, _ := strings.Cut(label, " · ")
	parsed, err := time.ParseInLocation(time.DateTime, stamp, time.Local)
	if err != nil || !parsed.Equal(a.conv.Store.Meta().Updated.Truncate(time.Second)) {
		t.Fatalf("resume timestamp lacks local seconds: %q %v", stamp, err)
	}
}
