package terminal

import (
	"strings"
	"testing"
	"time"
)

func TestTerminalTitleSafeWithScreenAndAnimationsDisabled(t *testing.T) {
	var output strings.Builder
	title := terminalTitle{screen: true}
	root := "/tmp/⠇\x1b\x07\u009c\u202eproject"
	now := time.Now()
	if err := title.update(&output, root, titleWorking, true, now); err != nil {
		t.Fatal(err)
	}
	if err := title.update(&output, root, titleWorking, true, now.Add(100*time.Millisecond)); err != nil {
		t.Fatal(err)
	}
	if err := title.update(&output, root, titleNeedsInput, false, now.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	if err := title.update(&output, root, titleNeedsInput, false, now.Add(2*time.Second)); err != nil {
		t.Fatal(err)
	}
	const want = "\x1b[22;0t\x1b]0;| ?project\x07\x1b]0;/ ?project\x07\x1b]0;[ ! ] Action Required | ?project\x07"
	if got := output.String(); got != want {
		t.Fatalf("title output = %q, want %q", got, want)
	}
}
