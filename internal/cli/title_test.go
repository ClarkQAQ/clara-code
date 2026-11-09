package cli

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
)

func TestTerminalTitleTracksRunAndQuestion(t *testing.T) {
	t.Setenv("STY", "")
	ask := make(chan struct{})
	finish := make(chan struct{})
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		switch requests.Add(1) {
		case 1:
			select {
			case <-ask:
			case <-r.Context().Done():
				return
			}
			fmt.Fprint(w, "data: "+`{"id":"question","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"ask","type":"function","function":{"name":"request_user_input","arguments":"{\"questions\":[{\"id\":\"pick\",\"question\":\"Pick an option\",\"options\":[\"First\",\"Second\"]}]}"}}]},"finish_reason":null}]}`+"\n\n")
			fmt.Fprint(w, "data: "+`{"id":"question","choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}]}`+"\n\n")
		default:
			select {
			case <-finish:
			case <-r.Context().Done():
				return
			}
			fmt.Fprint(w, "data: "+`{"id":"final","choices":[{"index":0,"delta":{"content":"Answer received."},"finish_reason":null}]}`+"\n\n")
			fmt.Fprint(w, "data: "+`{"id":"final","choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}`+"\n\n")
		}
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	t.Cleanup(server.Close)
	t.Setenv("CLARA_CODE_ENDPOINT", server.URL+"/v1")
	t.Setenv("CLARA_CODE_PROTOCOL", "completions")
	t.Setenv("CLARA_CODE_MODEL", "fixture")
	t.Setenv("CLARA_CODE_API_KEY", "fixture")
	cwd := filepath.Join(t.TempDir(), "title-project")
	if err := os.Mkdir(cwd, 0o700); err != nil {
		t.Fatal(err)
	}
	p := startConfiguredPTY(t, t.TempDir(), "--cwd", cwd)
	const idle = "\x1b]0;title-project\x07"
	const working = "\x1b]0;⠋ title-project\x07"
	p.await(t, 0, "\x1b[22;0t"+idle)
	mark := len(p.output())
	p.send(t, "Ask me a question.\r")
	p.await(t, mark, working)
	p.await(t, mark, "\x1b]0;⠙ title-project\x07")
	close(ask)
	p.await(t, mark, "\x1b]0;[ ! ] Action Required | title-project\x07")
	p.await(t, mark, "\x1b]0;[ . ] Action Required | title-project\x07")
	mark = len(p.output())
	p.send(t, "\r")
	p.await(t, mark, working)
	close(finish)
	p.await(t, mark, "Answer received.")
	p.await(t, mark, idle)
	p.send(t, "\x04")
	p.await(t, mark, "\x1b]0;\x07\x1b[23;0t")
}
