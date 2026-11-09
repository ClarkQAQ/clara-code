package setting

import (
	"os"
	"path/filepath"
	"testing"
)

func TestInitialConfigurationGuards(t *testing.T) {
	home, cwd := t.TempDir(), t.TempDir()
	t.Setenv("CLARA_CODE_HOME", home)
	t.Setenv("CLARA_CODE_MODEL_PROVIDER", "")
	t.Setenv("CLARA_CODE_ENDPOINT", "")
	t.Setenv("CLARA_CODE_PROTOCOL", "")
	t.Setenv("CLARA_CODE_API_KEY", "")
	t.Setenv("CLARA_CODE_MODEL", "")
	options := LoadOptions{Cwd: cwd, Root: cwd}
	load := func() Config {
		t.Helper()
		c, err := LoadWith(t.Context(), options)
		if err != nil {
			t.Fatal(err)
		}
		return c
	}
	t.Setenv("CLARA_CODE_ENDPOINT", "https://test.invalid/v1")
	t.Setenv("CLARA_CODE_MODEL", "environment-model")
	if load().NeedsSetup() {
		t.Fatal("environment-only model triggered setup")
	}
	t.Setenv("CLARA_CODE_ENDPOINT", "")
	t.Setenv("CLARA_CODE_MODEL", "")
	initial := load()
	if !initial.NeedsSetup() {
		t.Fatal("missing configuration did not trigger setup")
	}

	project := filepath.Join(cwd, ".agents", "clara-code.toml")
	configFile(t, project, "# Empty project configuration still counts as a file.\n")
	if load().NeedsSetup() {
		t.Fatal("an existing project configuration triggered setup")
	}
	draft := InitialConfig{ProviderID: "first", BaseURL: "https://test.invalid/v1", WireAPI: "responses", ModelID: "model"}
	if _, err := initial.SaveInitial(t.Context(), draft); err == nil {
		t.Fatal("setup saved after a project configuration appeared")
	}
	if err := os.Remove(project); err != nil {
		t.Fatal(err)
	}

	global := filepath.Join(home, "config.toml")
	const original = "# Another process created this.\n"
	configFile(t, global, original)
	if err := writeInitial(t.Context(), global, []byte("replacement")); err == nil {
		t.Fatal("initial write replaced an existing configuration")
	}
	if got, err := os.ReadFile(global); err != nil || string(got) != original {
		t.Fatalf("existing configuration changed: %q, %v", got, err)
	}
}
