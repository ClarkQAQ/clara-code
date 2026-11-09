package setting

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCompactionPercentLayersAndModelOverrides(t *testing.T) {
	home, root := t.TempDir(), t.TempDir()
	t.Setenv("CLARA_CODE_HOME", home)
	configFile(t, filepath.Join(home, "config.toml"), "auto_compact_context_percent=85\n"+exampleConfig)
	configFile(t, filepath.Join(root, ".agents/clara-code.toml"), "auto_compact_context_percent=80\n[model_providers.one.models.same]\nauto_compact_context_percent=60\n")
	t.Setenv("CLARA_CODE_AUTO_COMPACT_CONTEXT_PERCENT", "75")
	cfg, err := LoadWith(t.Context(), LoadOptions{Cwd: root, Root: root, Overrides: []string{"auto_compact_context_percent=70"}})
	if err != nil || cfg.AutoCompactContextPercent != 70 || cfg.CompactionPercent(cfg.Models["same"]) != 60 {
		t.Fatal("wrong percentage precedence", cfg.AutoCompactContextPercent, err)
	}
	cfg, err = cfg.SelectProvider("two")
	if err != nil || cfg.CompactionPercent(cfg.Models["same"]) != 70 {
		t.Fatal("another provider inherited wrong override", err)
	}
	for _, override := range []string{"auto_compact_context_percent=0", "auto_compact_context_percent=101", "model_providers.one.models.same.auto_compact_context_percent=0", "model_providers.one.models.same.auto_compact_context_percent=101"} {
		_, err := LoadWith(t.Context(), LoadOptions{Cwd: root, Root: root, Overrides: []string{override}})
		if err == nil || !strings.Contains(err.Error(), "auto_compact_context_percent") {
			t.Fatal("invalid percentage accepted", override, err)
		}
	}
}

func TestRemovedContextBytesReportsMigration(t *testing.T) {
	home, root := t.TempDir(), t.TempDir()
	t.Setenv("CLARA_CODE_HOME", home)
	path := filepath.Join(home, "config.toml")
	configFile(t, path, "context_bytes=0\n")
	if _, err := LoadWith(t.Context(), LoadOptions{Cwd: root, Root: root}); err == nil || !strings.Contains(err.Error(), path) || !strings.Contains(err.Error(), "was removed") {
		t.Fatal("missing configuration migration diagnostic", err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CLARA_CODE_CONTEXT_BYTES", "0")
	if _, err := LoadWith(t.Context(), LoadOptions{Cwd: root, Root: root}); err == nil || !strings.Contains(err.Error(), "CLARA_CODE_AUTO_COMPACT_CONTEXT_PERCENT") {
		t.Fatal("removed environment setting silently accepted", err)
	}
}

func TestCompactionPercentClampsLikeCodex(t *testing.T) {
	for _, percent := range []int{90, 95, 100} {
		config := Config{AutoCompactContextPercent: percent}
		if config.CompactionPercent(Model{}) != 90 {
			t.Fatal("global value exceeded ceiling", percent)
		}
		config.AutoCompactContextPercent = 60
		if config.CompactionPercent(Model{AutoCompactContextPercent: new(percent)}) != 90 {
			t.Fatal("model override exceeded ceiling", percent)
		}
	}
}
