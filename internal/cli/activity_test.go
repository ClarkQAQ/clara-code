//go:build unix

package cli

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/creack/pty"
)

func TestInlineToolActivityAndRetry(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch requests.Add(1) {
		case 1:
			w.Header().Set("retry-after-ms", "150")
			w.WriteHeader(http.StatusServiceUnavailable)
			fmt.Fprint(w, `{"error":{"message":"fixture temporarily unavailable","type":"server_error"}}`)
		case 2:
			w.Header().Set("Content-Type", "text/event-stream")
			chunk := map[string]any{"id": "tool-reply", "model": "fixture", "choices": []any{map[string]any{
				"index": 0, "delta": map[string]any{"role": "assistant", "content": "I will check the command.", "tool_calls": []any{map[string]any{
					"index": 0, "id": "shell", "type": "function", "function": map[string]any{"name": "exec_command", "arguments": `{"command":"printf 'tool preview\\n'; sleep 0.7; exit 7"}`},
				}}}, "finish_reason": nil,
			}}}
			body, _ := json.Marshal(chunk)
			fmt.Fprintf(w, "data: %s\n\ndata: %s\n\ndata: [DONE]\n\n", body, `{"id":"tool-reply","choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}]}`)
		default:
			time.Sleep(200 * time.Millisecond)
			w.Header().Set("Content-Type", "text/event-stream")
			fmt.Fprint(w, "data: "+`{"id":"final","choices":[{"index":0,"delta":{"content":"The command exited with code 7."},"finish_reason":null}]}`+"\n\ndata: "+`{"id":"final","choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}`+"\n\ndata: [DONE]\n\n")
		}
	}))
	defer server.Close()
	t.Setenv("CLARA_CODE_ENDPOINT", server.URL+"/v1")
	t.Setenv("CLARA_CODE_PROTOCOL", "completions")
	t.Setenv("CLARA_CODE_MODEL", "fixture")
	t.Setenv("CLARA_CODE_API_KEY", "fixture")
	home, cwd := t.TempDir(), t.TempDir()
	p := startConfiguredPTY(t, home, "--cwd", cwd)
	p.await(t, 0, "Ready")
	mark := len(p.output())
	p.send(t, "Check the command.\r")
	p.await(t, mark, "Reconnecting… 1/3")
	p.await(t, mark, "I will check the command.")
	p.await(t, mark, "Running Bash")
	if err := pty.Setsize(p.file, &pty.Winsize{Rows: 18, Cols: 77}); err != nil {
		t.Fatal(err)
	}
	_ = p.cmd.Process.Signal(syscall.SIGWINCH)
	p.await(t, mark, "exit 7")
	p.await(t, mark, "└ tool preview")
	p.await(t, mark, "The command exited with code 7.")
	p.await(t, mark, "Ready")
	if requests.Load() != 3 {
		t.Fatalf("unexpected requests %d", requests.Load())
	}
	if strings.Contains(p.output(), "\x1b[?1049") {
		t.Fatal("activity switched to the alternate screen")
	}
}
