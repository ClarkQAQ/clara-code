package clipboard

import (
	"context"
	"encoding/base64"
	"fmt"
	"os/exec"
	"runtime"
	"strings"
	"testing"
	"time"
)

// A private named pasteboard exercises AppKit without touching the user's clipboard.
func TestDarwinPrivatePasteboardImageAndEmpty(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("AppKit is macOS-only")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	name := fmt.Sprintf("clara-code-test-%d", time.Now().UnixNano())
	png := "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+a2ioAAAAASUVORK5CYII="
	for _, filled := range []bool{false, true} {
		setup := fmt.Sprintf(`ObjC.import('AppKit'); var p=$.NSPasteboard.pasteboardWithName(%q); p.clearContents;`, name)
		if filled {
			setup += fmt.Sprintf(`p.setDataForType($.NSData.alloc.initWithBase64EncodedStringOptions(%q,0),$.NSPasteboardTypePNG);`, png)
		}
		script := strings.Replace(darwinImageScript, "ObjC.import('AppKit'); var p=$.NSPasteboard.generalPasteboard;", setup, 1)
		script = strings.Replace(script, "d.isNil() ? '' : ObjC.unwrap(d.base64EncodedStringWithOptions(0));", `var result=d.isNil() ? '' : ObjC.unwrap(d.base64EncodedStringWithOptions(0)); p.releaseGlobally; result;`, 1)
		out, err := exec.CommandContext(ctx, "/usr/bin/osascript", "-l", "JavaScript", "-e", script).CombinedOutput()
		if err != nil {
			t.Fatalf("AppKit read failed: %s %v", out, err)
		}
		raw := strings.TrimSpace(string(out))
		if !filled && raw != "" {
			t.Fatal("empty private pasteboard returned data")
		}
		if filled {
			data, err := base64.StdEncoding.DecodeString(raw)
			if err != nil || len(data) == 0 {
				t.Fatal("private image read failed", err)
			}
		}
	}
}
