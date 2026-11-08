package coding

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"clara-code/internal/session"
	"clara-code/pkg/core"
	"clara-code/pkg/core/prompt"
	"clara-code/pkg/model"
	"clara-code/pkg/tool"
)

func installSkill(t *testing.T, root, name, description string) {
	t.Helper()
	writeTestFile(t, root, filepath.Join(name, "SKILL.md"), "---\nname: "+name+"\ndescription: "+description+"\n---\nUse this workflow.")
}

func TestSkillDisablePersistsAndDoesNotFallBackToGlobal(t *testing.T) {
	c, host := testHost(t, t.TempDir(), DemoModel{})
	installSkill(t, filepath.Join(host.Config.Home, "skills"), "example", "global skill")
	installSkill(t, filepath.Join(c.Workspace.Cwd, ".agents/skills"), "example", "project skill")
	installSkill(t, filepath.Join(c.Workspace.Cwd, ".agents/skills"), "other", "other skill")
	all, err := c.ManageSkills(t.Context())
	if err != nil || len(all) != 2 || all[0].Description != "project skill" || !all[0].Enabled {
		t.Fatal(all, err)
	}
	if err := c.SetSkillEnabled(t.Context(), all[0].Source, false); err != nil {
		t.Fatal(err)
	}
	enabled, err := c.Skills(t.Context())
	if err != nil || len(enabled) != 1 || enabled[0].Name != "other" {
		t.Fatal("disabled skill survived or fell back to global", enabled, err)
	}
	_, result, err := c.Call(prompt.Execute, false).PrepareStep(t.Context(), core.PrepareStepFunctionOptions{})
	if err != nil || strings.Contains(contextText(result.Messages), "project skill") || strings.Contains(contextText(result.Messages), "global skill") {
		t.Fatal("disabled skill entered model catalog", err)
	}
	for _, item := range result.Tools {
		if item.Info().Name == "read_skill" || item.Info().Name == "read_skill_resource" {
			input := `{"name":"example"}`
			if item.Info().Name == "read_skill_resource" {
				input = `{"name":"example","path":"SKILL.md"}`
			}
			response, readErr := item.Run(t.Context(), model.ToolCall{Input: input})
			if readErr == nil && !strings.Contains(response.Content, "unknown skill") {
				t.Fatal("disabled skill remains readable through catalog tools", response)
			}
		}
	}
	// A separately opened conversation, including task preparation, sees the same state.
	other, err := host.Open(t.Context(), c.Workspace, session.New(host.Config.Home, session.Meta{Project: c.Workspace.Root, Cwd: c.Workspace.Cwd, Model: "fixture"}), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer other.Close()
	all, err = other.ManageSkills(t.Context())
	if err != nil || all[0].Enabled {
		t.Fatal("disabled preference did not persist", all, err)
	}
	child, err := other.buildTask(t.Context(), tool.TaskSpec{Name: "inspect", Role: "explorer"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	_, result, err = child.PrepareStep(t.Context(), core.PrepareStepFunctionOptions{})
	if err != nil || strings.Contains(contextText(result.Messages), "project skill") {
		t.Fatal("child ignored preferences", err)
	}
	if err := other.SetSkillEnabled(t.Context(), all[0].Source, true); err != nil {
		t.Fatal(err)
	}
	enabled, err = c.Skills(t.Context())
	if err != nil || len(enabled) != 2 {
		t.Fatal("re-enable did not refresh another conversation", enabled, err)
	}
}

func TestConcurrentSkillPreferencesAndInvalidState(t *testing.T) {
	c, _ := testHost(t, t.TempDir(), DemoModel{})
	for _, name := range []string{"first", "second"} {
		installSkill(t, filepath.Join(c.Workspace.Cwd, ".agents/skills"), name, name+" workflow")
	}
	all, err := c.ManageSkills(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for _, skill := range all {
		wg.Go(func() {
			if err := c.SetSkillEnabled(t.Context(), skill.Source, false); err != nil {
				t.Error(err)
			}
		})
	}
	wg.Wait()
	enabled, err := c.Skills(t.Context())
	if err != nil || len(enabled) != 0 {
		t.Fatal("concurrent toggles lost an update", enabled, err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if err := c.SetSkillEnabled(ctx, all[0].Source, true); err == nil {
		t.Fatal("cancelled toggle succeeded")
	}
	if err := os.WriteFile(filepath.Join(c.Host.Config.Home, "skills-state.json"), []byte(`{"version":99}`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Skills(t.Context()); err == nil {
		t.Fatal("invalid preferences silently enabled skills")
	}
}
