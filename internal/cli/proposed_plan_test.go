package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"syscall"
	"testing"
	"time"

	"clara-code/internal/coding"
	"clara-code/internal/session"
	"github.com/creack/pty"
)

func TestPlanImplementationPTY(t *testing.T) {
	for _, fresh := range []bool{false, true} {
		name := "keep context"
		if fresh {
			name = "fresh context"
		}
		t.Run(name, func(t *testing.T) {
			requests := make(chan string, 8)
			finishFirst := make(chan struct{})
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body, err := io.ReadAll(r.Body)
				if err != nil {
					t.Error(err)
					return
				}
				select {
				case requests <- string(body):
				case <-r.Context().Done():
					return
				}
				w.Header().Set("Content-Type", "text/event-stream")
				flush := w.(http.Flusher)
				send := func(text string) {
					chunk, _ := json.Marshal(map[string]any{
						"id": "plan", "choices": []any{map[string]any{"index": 0, "delta": map[string]string{"content": text}}},
					})
					fmt.Fprintf(w, "data: %s\n\n", chunk)
					flush.Flush()
				}
				switch {
				case strings.Contains(string(body), "Implement the plan"):
					send("Implementation started.")
				case strings.Contains(string(body), "Refine the approach"):
					send("<proposed_plan>\n# Revised plan\n\n- Run verification.\n</proposed_plan>")
				default:
					send("Planning preface.\n\n<proposed_")
					send("plan>\n# Example plan\n\n- Verify `go test`.\n</proposed_")
					select {
					case <-finishFirst:
					case <-r.Context().Done():
						return
					}
					send("plan>")
				}
				fmt.Fprint(w, "data: "+`{"id":"plan","choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}`+"\n\ndata: [DONE]\n\n")
				flush.Flush()
			}))
			t.Cleanup(server.Close)
			t.Setenv("CLARA_CODE_ENDPOINT", server.URL+"/v1")
			t.Setenv("CLARA_CODE_PROTOCOL", "completions")
			t.Setenv("CLARA_CODE_MODEL", "fixture")
			t.Setenv("CLARA_CODE_API_KEY", "fixture")
			home, cwd := t.TempDir(), t.TempDir()
			space, err := coding.ResolveWorkspace(cwd)
			if err != nil {
				t.Fatal(err)
			}
			nextRequest := func() string {
				t.Helper()
				select {
				case request := <-requests:
					return request
				case <-time.After(6 * time.Second):
					t.Fatal("model request did not arrive")
					return ""
				}
			}

			p := startConfiguredPTY(t, home, "--cwd", cwd)
			p.await(t, 0, "Ready")
			p.send(t, "\x1b[Z")
			p.await(t, 0, "Plan mode (Shift+Tab to cycle)")
			p.send(t, "Original planning context\r")
			if request := nextRequest(); strings.Contains(request, `"name":"write"`) {
				t.Fatal("Plan mode exposed a write tool")
			}
			p.await(t, 0, "# Example plan")
			if output := p.output(); strings.Contains(output, "Implement this plan?") || strings.Contains(output, "<proposed_") {
				t.Fatalf("stream exposed a tag or asked for early approval: %q", output)
			}
			p.send(t, "Keep my draft")
			p.await(t, 0, "Keep my draft")
			close(finishFirst)
			p.await(t, 0, "Implement this plan?")
			p.await(t, 0, "2. Yes, clear context and implement")
			mark := len(p.output())
			p.send(t, "\x1b")
			p.await(t, mark, "Keep my draft")
			p.await(t, mark, "Plan mode (Shift+Tab to cycle)")
			p.send(t, "\x03Refine the approach\r")
			if request := nextRequest(); !strings.Contains(request, "Original planning context") {
				t.Fatal("continuing in Plan mode lost the original context")
			}
			p.await(t, mark, "Implement this plan?")
			items, err := session.List(home, space.Root)
			if err != nil || len(items) != 1 {
				t.Fatalf("planning sessions: %+v, %v", items, err)
			}
			original := items[0].ID

			mark = len(p.output())
			if err := pty.Setsize(p.file, &pty.Winsize{Rows: 20, Cols: 64}); err != nil {
				t.Fatal(err)
			}
			if err := p.cmd.Process.Signal(syscall.SIGWINCH); err != nil {
				t.Fatal(err)
			}
			p.await(t, mark, "# Revised plan")
			p.await(t, mark, "Implement this plan?")
			if fresh {
				p.send(t, "\x1b[B\r")
			} else {
				p.send(t, "\r")
			}
			request := nextRequest()
			if !strings.Contains(request, `"name":"write"`) || !strings.Contains(request, "Implement the plan") {
				t.Fatal("approval did not start an Agent turn")
			}
			if strings.Contains(request, "Original planning context") == fresh || !strings.Contains(request, "# Revised plan") {
				t.Fatalf("implementation received the wrong context (fresh=%v): %s", fresh, request)
			}
			p.await(t, mark, "Implementation started.")
			p.await(t, strings.LastIndex(p.output(), "Implementation started."), "Ready")
			items, err = session.List(home, space.Root)
			count := 1
			if fresh {
				count++
			}
			if err != nil || len(items) != count {
				t.Fatalf("sessions after approval: %+v, %v", items, err)
			}
			if fresh {
				resumed := startConfiguredPTY(t, home, "--cwd", cwd, "resume", original)
				resumed.await(t, 0, "# Revised plan")
				resumed.await(t, 0, "Ready")
				if output := resumed.output(); strings.Contains(output, "<proposed_plan>") || strings.Contains(output, "Implement this plan?") {
					t.Fatalf("replay leaked tags or reopened approval: %q", output)
				}
			}
		})
	}
}
