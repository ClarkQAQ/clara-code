package cli

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"clara-code/internal/setting"
)

func TestFirstRunSetup(t *testing.T) {
	for _, env := range []string{"CLARA_CODE_MODEL", "CLARA_CODE_MODEL_PROVIDER", "CLARA_CODE_ENDPOINT", "CLARA_CODE_PROTOCOL", "CLARA_CODE_API_KEY"} {
		t.Setenv(env, "")
	}
	const secret = "setup-fixture-secret"
	const modelID = "test/model.v1"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			Model           string `json:"model"`
			MaxOutputTokens int    `json:"max_tokens"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil || request.Model != modelID || request.MaxOutputTokens != 4096 ||
			r.URL.Path != "/v1/chat/completions" || r.Header.Get("Authorization") != "Bearer "+secret {
			t.Error("configured model request did not use the saved provider and model settings")
		}
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: "+`{"id":"setup","choices":[{"index":0,"delta":{"content":"Setup works"},"finish_reason":"stop"}]}`+"\n\ndata: [DONE]\n\n")
	}))
	t.Cleanup(server.Close)
	home, cwd := t.TempDir(), t.TempDir()
	path := filepath.Join(home, "config.toml")
	cancelled := startConfiguredPTY(t, home, "--cwd", cwd)
	cancelled.await(t, 0, "First-time setup")
	cancelled.send(t, "\x03")
	if err := cancelled.cmd.Wait(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("cancelled setup wrote a configuration: %v", err)
	}

	p := startConfiguredPTY(t, home, "--cwd", cwd)
	p.await(t, 0, "Provider ID")
	step := func(keys, next string) {
		t.Helper()
		at := len(p.output())
		p.send(t, keys)
		p.await(t, at, next)
	}
	step("test.gateway\r", "API protocol")
	step("\x1b[B\r", "Provider base URL")
	step("invalid\r", "Enter an HTTP(S) base URL")
	step("\x15"+server.URL+"/v1\r", "API key")
	step(secret, strings.Repeat("*", len(secret)))
	step("\r", "Default model ID")
	step("\x1b[Z", "API key")
	step("\r", "Default model ID")
	step(modelID+"\r", "Model context window")
	step("64000\r", "Model max output tokens")
	step("-1\r", "Enter a positive token count")
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("unfinished setup wrote a configuration: %v", err)
	}
	step("\x154096\r", "Run /review on my current changes")
	step("hello\r", "Setup works")
	if strings.Contains(p.output(), secret) {
		t.Fatal("API key appeared in the terminal")
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0600 {
		t.Fatalf("configuration permissions: %o", info.Mode().Perm())
	}
	t.Setenv("CLARA_CODE_HOME", home)
	config, err := setting.LoadWith(t.Context(), setting.LoadOptions{Cwd: cwd, Root: cwd})
	if err != nil {
		t.Fatal(err)
	}
	if config.ModelProvider != "test.gateway" || config.Models[modelID].ContextWindow != 64000 {
		t.Fatal("provider ID or context window was not saved")
	}
	reopened := startConfiguredPTY(t, home, "--cwd", cwd)
	reopened.await(t, 0, "Ready")
	if strings.Contains(reopened.output(), "First-time setup") {
		t.Fatal("existing configuration started setup again")
	}
}
