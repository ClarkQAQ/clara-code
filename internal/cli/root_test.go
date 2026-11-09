package cli

import (
	"context"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"clara-code/pkg/core"
)

func TestRootEntrypoint(t *testing.T) {
	binary := filepath.Join(t.TempDir(), "clara-code.exe")
	build := exec.CommandContext(t.Context(), "go", "build", "-o", binary, "-ldflags", "-X main.version=entry-fixture", "../..")
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("root build: %v\n%s", err, output)
	}
	for _, test := range []struct{ arg, want string }{{"--help", "clara-code resume"}, {"--version", "Clara Code entry-fixture"}} {
		output, err := exec.CommandContext(t.Context(), binary, test.arg).CombinedOutput()
		if err != nil || !strings.Contains(string(output), test.want) {
			t.Fatalf("%s: %v\n%s", test.arg, err, output)
		}
	}
	t.Run("terminal signals", func(t *testing.T) { testRootSignals(t, binary) })
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	client, err := core.ConnectACPExternal(ctx, core.ACPExternalConfig{
		Command: binary, Args: []string{"acp", "--demo"}, Env: map[string]string{"CLARA_CODE_HOME": t.TempDir()},
	})
	if err != nil {
		t.Fatal(err)
	}
	id, err := client.NewSession(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close(ctx, id)
	if text, reason, err := client.Prompt(ctx, id, "root ACP probe"); err != nil || text != "Demo response: root ACP probe" || reason != "end_turn" {
		t.Fatal("root ACP failed", text, reason, err)
	}
}
