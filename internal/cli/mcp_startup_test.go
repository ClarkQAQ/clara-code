//go:build unix

package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"strings"
	"testing"

	"github.com/creack/pty"
	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestMCPStartupProgressAndWarningPTY(t *testing.T) {
	home, cwd := t.TempDir(), t.TempDir()
	ready, reject := make(chan struct{}), make(chan struct{})
	server := mcpsdk.NewServer(&mcpsdk.Implementation{Name: "fixture", Version: "1"}, nil)
	handler := mcpsdk.NewStreamableHTTPHandler(func(*http.Request) *mcpsdk.Server { return server }, nil)
	endpoint := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/ready" {
			select {
			case <-ready:
				handler.ServeHTTP(w, r)
			case <-r.Context().Done():
			}
			return
		}
		select {
		case <-reject:
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			_, _ = fmt.Fprintf(w, `{"jsonrpc":"2.0","id":1,"error":{"code":-32000,"message":"rejected %s"}}`, r.Header.Get("X-Startup"))
		case <-r.Context().Done():
		}
	}))
	t.Cleanup(endpoint.Close)
	options := []string{
		"-c", fmt.Sprintf(`mcp_servers.a_ready={url=%q}`, endpoint.URL+"/ready"),
		"-c", fmt.Sprintf(`mcp_servers.b_rejected={url=%q,http_headers={X-Startup="startup-private-token"}}`, endpoint.URL+"/reject"),
		"-c", `mcp_servers.disabled={enabled=false,command="unused"}`,
	}
	p := startPTY(t, home, cwd, options...)
	p.await(t, 0, "Starting MCP servers (0/2) · a_ready")
	if strings.Contains(p.output(), "Run /review on my current changes") {
		t.Fatal("composer appeared before startup completed")
	}
	close(ready)
	p.await(t, 0, "Starting MCP servers (1/2) · b_rejected")
	close(reject)

	const warning = "Warning · MCP startup incomplete"
	p.await(t, 0, warning)
	p.await(t, 0, "rejected [redacted]")
	after := strings.Index(p.output(), warning)
	p.await(t, after, "Ready")
	output := p.output()
	if strings.Contains(output, "startup-private-token") || strings.Contains(output[after:], "Starting MCP server") {
		t.Fatalf("secret or stale MCP progress after startup: %q", output)
	}
	p.send(t, "/mcp\r")
	p.await(t, len(output), "a_ready · connected (0 tools)")
	p.await(t, len(output), "b_rejected ·")
	p.send(t, "MCP startup probe\r")
	p.await(t, 0, "Demo response: MCP startup probe")
	p.send(t, "/exit\r")
	if err := p.cmd.Wait(); err != nil {
		t.Fatal(err)
	}

	p = startPTY(t, home, cwd, append(options, "resume")...)
	p.await(t, 0, "Resume a conversation")
	p.send(t, "\r")
	p.await(t, 0, warning)
	output = p.output()
	after = strings.Index(output, warning)
	if history := strings.Index(output, "Demo response: MCP startup probe"); history < 0 || history > after {
		t.Fatalf("startup warning should follow restored history: %q", output)
	}
	p.await(t, after, "Ready")
	after = len(p.output())
	if err := pty.Setsize(p.file, &pty.Winsize{Rows: 25, Cols: 100}); err != nil {
		t.Fatal(err)
	}
	p.await(t, after, "Ready")
	if output := p.output()[after:]; strings.Count(output, warning) > 1 || strings.Contains(output, "Starting MCP server") {
		t.Fatalf("duplicate warnings or stale progress on redraw: %q", output)
	}

	p = startPTY(t, home, cwd, options[:2]...)
	p.await(t, 0, "Ready")
	if output := p.output(); strings.Contains(output, warning) {
		t.Fatalf("successful startup produced a warning: %q", output)
	}
}

func TestMCPStartupRequiredFailurePTY(t *testing.T) {
	home, cwd := t.TempDir(), t.TempDir()
	p := startPTY(t, home, cwd, "-c", `mcp_servers.broken={enabled=false,command="unused"}`)
	p.await(t, 0, "Ready")
	if output := p.output(); strings.Contains(output, "Starting MCP server") || strings.Contains(output, "MCP startup incomplete") {
		t.Fatalf("disabled server produced startup messages: %q", output)
	}
	p.send(t, "Save for MCP recovery\r")
	p.await(t, 0, "Demo response: Save for MCP recovery")
	p.send(t, "/exit\r")
	if err := p.cmd.Wait(); err != nil {
		t.Fatal(err)
	}

	config := fmt.Sprintf(`mcp_servers.broken={command=%q,required=true}`, cwd+"/missing-command")
	p = startPTY(t, home, cwd, "-c", config)
	const failure = "mcp: required server broken failed"
	p.await(t, 0, failure)
	var exit *exec.ExitError
	if err := p.cmd.Wait(); !errors.As(err, &exit) || exit.ExitCode() != 1 {
		t.Fatalf("required failure exit: %v", err)
	}

	p = startPTY(t, home, cwd, "-c", config, "resume")
	p.await(t, 0, "Resume a conversation")
	after := len(p.output())
	p.send(t, "\r")
	p.await(t, after, failure)
	p.await(t, strings.Index(p.output(), failure), "Resume a conversation")
}

func TestMCPStartupCancellationPTY(t *testing.T) {
	ctx := t.Context()
	entered := make(chan struct{}, 1)
	endpoint := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct{ Method string }
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if request.Method == "notifications/cancelled" {
			w.WriteHeader(http.StatusAccepted)
			return
		}
		select {
		case entered <- struct{}{}:
		default:
		}
		select {
		case <-r.Context().Done():
		case <-ctx.Done():
		}
	}))
	t.Cleanup(endpoint.Close)
	p := startPTY(t, t.TempDir(), t.TempDir(), "-c", fmt.Sprintf(`mcp_servers.waiting={url=%q}`, endpoint.URL))
	p.await(t, 0, "Starting MCP server waiting")
	select {
	case <-entered:
	case <-ctx.Done():
		t.Fatal("MCP server did not receive the connection request")
	}
	after := len(p.output())
	p.send(t, "\x03")
	p.await(t, after, "\x1b[?2004l")
	if err := p.cmd.Wait(); err != nil {
		t.Fatal(err)
	}
	if output := p.output()[after:]; strings.Contains(output, "MCP startup incomplete") || strings.Contains(output, "Ready") {
		t.Fatalf("canceled startup opened a conversation: %q", output)
	}
}
