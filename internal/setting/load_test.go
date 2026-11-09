package setting

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func configFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
}

const exampleConfig = `model="same"
model_provider="one"
model_reasoning_effort="low"
plan_mode_reasoning_effort="high"
[model_providers.one]
base_url="http://127.0.0.1:1234/v1"
wire_api="completions"
api_key="secret-fixture"
[model_providers.one.models.same]
input_modalities=["text","image"]
supported_reasoning_levels=["low","high"]
default_reasoning_level="low"
context_window=100000
[model_providers.two]
base_url="http://127.0.0.1:4321/v1"
[model_providers.two.models.same]
input_modalities=["text"]
supported_reasoning_levels=["high"]
default_reasoning_level="high"
`

func TestLayeredConfigurationAndRepeatedOverrides(t *testing.T) {
	home, root := t.TempDir(), t.TempDir()
	cwd := filepath.Join(root, "nested")
	if err := os.Mkdir(cwd, 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CLARA_CODE_HOME", home)
	configFile(t, filepath.Join(home, "config.toml"), exampleConfig)
	configFile(t, filepath.Join(root, ".agents", "clara-code.toml"), "[tui]\ncolor=\"never\"\nstatus_line=[\"model\"]\n")
	configFile(t, filepath.Join(cwd, ".agents", "clara-code.toml"), "model_reasoning_effort=\"high\"\n[model_providers.one]\napi_key=\"project-key\"\n")
	t.Setenv("CLARA_CODE_EFFORT", "low")
	t.Setenv("CLARA_CODE_ENDPOINT", "http://env.invalid/v1")
	cfg, err := LoadWith(t.Context(), LoadOptions{Cwd: cwd, Root: root, Overrides: []string{`model_reasoning_effort="high"`, `model_providers.one.base_url="http://cli.invalid/v1?a=b"`, `tui.status_line=["state","model"]`, `tui.color=always`}})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Models["same"].Effort != "high" || cfg.Connection().BaseURL != "http://cli.invalid/v1?a=b" || cfg.TUI.Color != "always" || len(cfg.TUI.StatusLine) != 2 {
		t.Fatalf("layer precedence failed: effort %s, UI %+v", cfg.Models["same"].Effort, cfg.TUI)
	}
	if cfg.Connection().APIKey != "project-key" {
		t.Fatal("project API key did not override global configuration")
	}
	cfg, err = cfg.SelectProvider("two")
	if err != nil || cfg.Models["same"].Images() {
		t.Fatal("provider-qualified model collision", err)
	}
}

func TestStatusLineCacheFields(t *testing.T) {
	home, root := t.TempDir(), t.TempDir()
	t.Setenv("CLARA_CODE_HOME", home)
	configFile(t, filepath.Join(home, "config.toml"), exampleConfig)
	cfg, err := LoadWith(t.Context(), LoadOptions{Cwd: root, Root: root, Overrides: []string{`tui.status_line=["session_cache_efficiency","state","round_cache_efficiency"]`}})
	if err != nil || len(cfg.TUI.StatusLine) != 3 || cfg.TUI.StatusLine[0] != "session_cache_efficiency" {
		t.Fatal("cache fields not accepted", err)
	}
	if _, err := LoadWith(t.Context(), LoadOptions{Cwd: root, Root: root, Overrides: []string{`tui.status_line=["unknown_cache_field"]`}}); err == nil {
		t.Fatal("unknown status field accepted")
	}
}
func TestQuotedKeysAndUnknownFieldsAreActionable(t *testing.T) {
	home, root := t.TempDir(), t.TempDir()
	t.Setenv("CLARA_CODE_HOME", home)
	configFile(t, filepath.Join(home, "config.toml"), exampleConfig)
	cfg, err := LoadWith(t.Context(), LoadOptions{Cwd: root, Root: root, Overrides: []string{`model_providers.one.models."a.b"={input_modalities=["text"],supported_reasoning_levels=["high"]}`, `model=a.b`, `model_reasoning_effort=high`}})
	if err != nil || cfg.DefaultModel != "a.b" || cfg.Models["a.b"].Images() {
		t.Fatal("quoted TOML paths failed", err)
	}
	for _, override := range []string{`profiles.test.model=x`, `model_providers.one.api_key="private-fixture"`, `tui.keymap.composer.submit="esc"`} {
		if strings.Contains(override, "api_key") {
			override += "\nnot_a_field=\"secret-value\""
		}
		_, err := LoadWith(context.Background(), LoadOptions{Cwd: root, Root: root, Overrides: []string{override}})
		if err == nil {
			t.Fatal("invalid configuration accepted", override)
		}
		if strings.Contains(err.Error(), "private-fixture") || strings.Contains(err.Error(), "secret-value") {
			t.Fatal("secret in diagnostic", err)
		}
	}
}
func TestNonGitScopeAndInlineSecrets(t *testing.T) {
	home, parent := t.TempDir(), t.TempDir()
	cwd := filepath.Join(parent, "child")
	if err := os.Mkdir(cwd, 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CLARA_CODE_HOME", home)
	configFile(t, filepath.Join(home, "config.toml"), exampleConfig)
	configFile(t, filepath.Join(parent, ".agents", "clara-code.toml"), `model="wrong"`)
	cfg, err := LoadWith(t.Context(), LoadOptions{Cwd: cwd, Root: cwd})
	if err != nil || cfg.DefaultModel != "same" {
		t.Fatal("non-Git parent leaked", err)
	}
	if _, err := cfg.Factory(); err != nil {
		t.Fatal(err)
	}
	if got := cfg.Redact("key=secret-fixture"); got != "key=[redacted]" {
		t.Fatal(got)
	}
	if _, err := LoadWith(t.Context(), LoadOptions{Cwd: parent, Root: cwd}); err == nil {
		t.Fatal("escaped project boundary")
	}
}

func TestDocumentedExampleAndEmptyReasoningCatalog(t *testing.T) {
	home, root := t.TempDir(), t.TempDir()
	t.Setenv("CLARA_CODE_HOME", home)
	data, err := os.ReadFile("../../config.example.toml")
	if err != nil {
		t.Fatal(err)
	}
	configFile(t, filepath.Join(home, "config.toml"), string(data))
	cfg, err := LoadWith(t.Context(), LoadOptions{Cwd: root, Root: root})
	if err != nil {
		t.Fatal(err)
	}
	other, err := cfg.SelectProvider("other")
	if err != nil {
		t.Fatal(err)
	}
	if other.Models["coding"].Effort != "" || !other.Models["coding"].Allows("") || other.Models["coding"].Images() {
		t.Fatal("root reasoning leaked into incompatible provider")
	}
	if len(cfg.TUI.Bindings("picker")) == 0 {
		t.Fatal("documented keys not loaded")
	}
}

func TestInlineAPIKeyOverridesAndRedaction(t *testing.T) {
	home, root := t.TempDir(), t.TempDir()
	t.Setenv("CLARA_CODE_HOME", home)
	configFile(t, filepath.Join(home, "config.toml"), exampleConfig)
	t.Setenv("CLARA_CODE_API_KEY", "environment-secret")
	cfg, err := LoadWith(t.Context(), LoadOptions{Cwd: root, Root: root, Overrides: []string{`model_providers.one.api_key="command-secret"`}})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Connection().APIKey != "command-secret" {
		t.Fatal("command-line API key did not override environment")
	}
	if got := cfg.Redact("command-secret environment-secret secret-fixture"); strings.Contains(got, "command-secret") || strings.Contains(got, "environment-secret") {
		t.Fatal("API key appeared in diagnostics")
	}
	for _, field := range []string{"api_key_file", "env_key", "key_env", "bearer_token"} {
		_, err := LoadWith(t.Context(), LoadOptions{Cwd: root, Root: root, Overrides: []string{"model_providers.one." + field + `="private-value"`}})
		if err == nil || strings.Contains(err.Error(), "private-value") {
			t.Fatal("obsolete authentication setting accepted or leaked its value", field)
		}
	}
}

func TestImageSubmissionLimitsAreOptional(t *testing.T) {
	home, root := t.TempDir(), t.TempDir()
	t.Setenv("CLARA_CODE_HOME", home)
	configFile(t, filepath.Join(home, "config.toml"), exampleConfig)
	for _, overrides := range [][]string{
		nil,
		{"images.max_attachments=0", "images.max_total_bytes=0"},
		{"images.max_attachments=-1", "images.max_total_bytes=-1"},
	} {
		cfg, err := LoadWith(t.Context(), LoadOptions{Cwd: root, Root: root, Overrides: overrides})
		if err != nil {
			t.Fatal(err)
		}
		if limits := cfg.Images.Defaults(); limits.ExceedsAttachmentCount(17) || limits.ExceedsTotalBytes(17<<20) {
			t.Fatalf("unexpected image submission caps: %+v", limits)
		}
	}
	for _, override := range []string{"images.max_attachments=-2", "images.max_total_bytes=-2"} {
		if _, err := LoadWith(t.Context(), LoadOptions{Cwd: root, Root: root, Overrides: []string{override}}); err == nil {
			t.Fatalf("accepted %s", override)
		}
	}
}
