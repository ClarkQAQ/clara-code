//go:build unix

package cli

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestInlineStreamingGapInTmux(t *testing.T) {
	chunks := make(chan string)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		flush := w.(http.Flusher)
		for {
			select {
			case text, ok := <-chunks:
				if !ok {
					fmt.Fprint(w, "data: "+`{"id":"gap","choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}`+"\n\ndata: [DONE]\n\n")
					flush.Flush()
					return
				}
				body, _ := json.Marshal(map[string]any{"id": "gap", "choices": []any{map[string]any{"index": 0, "delta": map[string]any{"content": text}, "finish_reason": nil}}})
				fmt.Fprintf(w, "data: %s\n\n", body)
				flush.Flush()
			case <-r.Context().Done():
				return
			}
		}
	}))
	t.Cleanup(server.Close)
	t.Setenv("CLARA_CODE_ENDPOINT", server.URL+"/v1")
	t.Setenv("CLARA_CODE_PROTOCOL", "completions")
	t.Setenv("CLARA_CODE_MODEL", "gap-fixture")
	t.Setenv("CLARA_CODE_API_KEY", "fixture")
	home, cwd := t.TempDir(), t.TempDir()
	call := startTmux(t, home, "--cwd", cwd)
	await := func(needle string) []string {
		t.Helper()
		deadline := time.Now().Add(8 * time.Second)
		var text string
		for time.Now().Before(deadline) {
			text = call("capture-pane", "-p", "-t", "cursor")
			if strings.Contains(text, needle) {
				return strings.Split(text, "\n")
			}
			time.Sleep(15 * time.Millisecond)
		}
		t.Fatalf("missing %s:\n%s", needle, text)
		return nil
	}
	gap := func(lines []string) int {
		t.Helper()
		input := -1
		for row, line := range lines {
			if strings.HasPrefix(line, "❯ Run /review") {
				input = row
			}
		}
		if input < 3 || lines[input-2] != "" || lines[input-3] == "" || !strings.HasPrefix(lines[input-1], "──") {
			t.Fatalf("missing or duplicate streaming gap:\n%s", strings.Join(lines, "\n"))
		}
		var x, y, visible int
		if _, err := fmt.Sscanf(call("display-message", "-p", "-t", "cursor", "#{cursor_x} #{cursor_y} #{cursor_flag}"), "%d %d %d", &x, &y, &visible); err != nil {
			t.Fatal(err)
		}
		if x != 2 || y != input || visible != 1 {
			t.Fatalf("gap fix displaced real cursor: %d %d %d, input row %d", x, y, visible, input)
		}
		return input
	}
	await("Ready")
	call("send-keys", "-t", "cursor", "-l", "spacing probe")
	call("send-keys", "-t", "cursor", "Enter")

	for _, part := range []struct{ text, marker string }{
		{"First", "First"}, {" update", "update"}, {"\n\n## Finding", "Finding"},
		{"\n- result", "result"}, {"\n\n```go\npackage main\n```", "package main"}, {"\n喵~", "喵~"},
	} {
		select {
		case chunks <- part.text:
		case <-time.After(8 * time.Second):
			t.Fatal("model request did not arrive")
		}
		gap(await(part.marker))
	}

	before := gap(await("喵~"))
	close(chunks)
	after := gap(await("Ready"))
	if before != after {
		t.Fatalf("completion shifted input row: %d -> %d", before, after)
	}
}
