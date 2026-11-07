package cli

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestBackgroundTerminalsInTmux(t *testing.T) {
	home, cwd := t.TempDir(), t.TempDir()
	var requests atomic.Int32
	ids := map[string]string{}
	ready := make(chan map[string]string, 1)
	terminalID := regexp.MustCompile(`exec_[0-9]{8}T[0-9]{6}\.[0-9]{9}_[0-9a-f]{20}`)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Messages []struct {
				ToolCallID string          `json:"tool_call_id"`
				Content    json.RawMessage `json:"content"`
			}
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		tool := func(index int, id, name string, args map[string]any) map[string]any {
			encoded, _ := json.Marshal(args)
			return map[string]any{"index": index, "id": id, "type": "function", "function": map[string]any{"name": name, "arguments": string(encoded)}}
		}
		var delta map[string]any
		finish := "stop"
		switch step := requests.Add(1); step {
		case 1:
			delta = map[string]any{"tool_calls": []any{
				tool(0, "job_one", "exec_command", map[string]any{"command": "printf first-marker; while [ ! -f release ]; do sleep 0.05; done; printf final-marker", "yield_seconds": 1}),
				tool(1, "job_two", "exec_command", map[string]any{"command": "printf second-marker; sleep 30", "yield_seconds": 1}),
			}}
			finish = "tool_calls"
		case 2:
			for _, message := range body.Messages {
				if message.ToolCallID != "" {
					ids[message.ToolCallID] = terminalID.FindString(string(message.Content))
				}
			}
			ready <- map[string]string{"one": ids["job_one"], "two": ids["job_two"]}
			delta = map[string]any{"content": "Background jobs started."}
		case 3, 5:
			id := ids["job_one"]
			if step == 5 {
				id = ids["job_two"]
			}
			delta = map[string]any{"tool_calls": []any{tool(0, fmt.Sprintf("wait_%d", step), "write_stdin", map[string]any{"session_id": id, "yield_seconds": 30})}}
			finish = "tool_calls"
		case 4:
			delta = map[string]any{"content": "The first background job finished."}
		default:
			t.Errorf("unexpected automatic model request %d", step)
			delta = map[string]any{"content": "Unexpected request."}
		}
		encoded, _ := json.Marshal(map[string]any{"id": "background-fixture", "choices": []any{map[string]any{"index": 0, "delta": delta, "finish_reason": finish}}})
		fmt.Fprintf(w, "data: %s\n\ndata: [DONE]\n\n", encoded)
	}))
	t.Cleanup(server.Close)
	t.Setenv("CLARA_CODE_ENDPOINT", server.URL+"/v1")
	t.Setenv("CLARA_CODE_PROTOCOL", "completions")
	t.Setenv("CLARA_CODE_MODEL", "background-fixture")
	t.Setenv("CLARA_CODE_API_KEY", "fixture")
	call := startTmux(t, home, "--cwd", cwd)
	call("resize-window", "-t", "cursor", "-x", "100", "-y", "32")
	await := func(label string, predicate func(string) bool) string {
		t.Helper()
		deadline := time.Now().Add(8 * time.Second)
		var text string
		for time.Now().Before(deadline) {
			text = call("capture-pane", "-p", "-t", "cursor")
			if predicate(text) {
				return text
			}
			time.Sleep(20 * time.Millisecond)
		}
		t.Fatalf("%s:\n%s", label, text)
		return ""
	}
	send := func(text string) {
		call("send-keys", "-t", "cursor", "-l", text)
		call("send-keys", "-t", "cursor", "Enter")
	}
	checkCursor := func() {
		t.Helper()
		var cursor tmuxCursor
		if _, err := fmt.Sscanf(call("display-message", "-p", "-t", "cursor", "#{cursor_x} #{cursor_y} #{cursor_flag}"), "%d %d %d", &cursor.x, &cursor.y, &cursor.visible); err != nil {
			t.Fatal(err)
		}
		lines := strings.Split(call("capture-pane", "-p", "-t", "cursor"), "\n")
		if cursor.x != 2 || cursor.visible != 1 || !strings.HasPrefix(lines[cursor.y], "❯ Run /review") || cursor.y < 2 || strings.TrimSpace(lines[cursor.y-2]) != "" {
			t.Fatalf("background footer displaced cursor or gap: %+v\n%s", cursor, strings.Join(lines, "\n"))
		}
	}
	await("startup", func(text string) bool { return strings.Contains(text, "Ready") })
	send("start background probes")
	await("two running terminals", func(text string) bool {
		return strings.Contains(text, "2 background terminals running · /ps to view · /stop to close") && strings.Contains(text, "Ready")
	})
	var terminalIDs map[string]string
	select {
	case terminalIDs = <-ready:
	case <-time.After(8 * time.Second):
		t.Fatal("missing session IDs")
	}
	if terminalIDs["one"] == "" || terminalIDs["two"] == "" {
		t.Fatal("missing real terminal IDs", terminalIDs)
	}
	checkCursor()
	send("/ps")
	await("snapshot", func(text string) bool {
		return strings.Contains(text, "directory: ") && strings.Contains(text, "second-marker")
	})
	if requests.Load() != 2 {
		t.Fatal("/ps started an Agent turn")
	}
	send("wait for first background probe")
	await("waiting", func(text string) bool { return strings.Contains(text, "Waiting for background terminal") })
	for _, width := range []string{"58", "100"} {
		call("resize-window", "-t", "cursor", "-x", width, "-y", "32")
		await("reflow", func(text string) bool {
			if !strings.Contains(text, "Waiting for background terminal") || !strings.Contains(text, "2 background terminals running") {
				return false
			}
			columns := 58
			if width == "100" {
				columns = 100
			}
			lines := strings.Split(text, "\n")
			for i, line := range lines {
				if strings.HasPrefix(line, "❯ Run /review") && i > 1 {
					return lines[i-1] == strings.Repeat("─", columns) && lines[i-2] == ""
				}
			}
			return false
		})
		checkCursor()
	}
	if err := os.WriteFile(filepath.Join(cwd, "release"), []byte("finish"), 0600); err != nil {
		t.Fatal(err)
	}
	await("completed wait", func(text string) bool {
		return strings.Contains(text, "Waited for background terminal") && strings.Contains(text, "first background job finished") && strings.Contains(text, "1 background terminal running") && strings.Contains(text, "Ready")
	})
	checkCursor()
	send("wait for second background probe")
	await("second wait", func(text string) bool {
		return strings.Contains(text, "Waiting for background terminal") && requests.Load() == 5
	})
	call("send-keys", "-t", "cursor", "Escape")
	await("Esc retains detached terminal", func(text string) bool {
		return strings.Contains(text, "Interrupted waiting") && strings.Contains(text, "1 background terminal running") && strings.Contains(text, "Ready")
	})
	// Cancelled prompts are restored as drafts; clear the draft before a command.
	call("send-keys", "-t", "cursor", "C-u")
	send("/stop " + terminalIDs["two"])
	await("stop completed", func(text string) bool {
		return strings.Contains(text, "Background terminal stop completed") && !strings.Contains(text, "background terminal running")
	})
	checkCursor()
	send("/ps")
	await("no running terminals", func(text string) bool { return strings.Contains(text, "No background terminals running.") })
	if requests.Load() != 5 {
		t.Fatal("background state changes started an Agent turn", requests.Load())
	}
}
