package coding

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"

	"clara-code/internal/filelock"
)

type skillPreferences struct {
	Version  int      `json:"version"`
	Disabled []string `json:"disabled,omitempty"`
}

func readSkillPreferences(home string) (skillPreferences, error) {
	state := skillPreferences{Version: 1}
	body, err := boundedFile(filepath.Join(home, "skills-state.json"), 1<<20)
	if errors.Is(err, os.ErrNotExist) {
		return state, nil
	}
	if err != nil {
		return state, err
	}
	if err := json.Unmarshal(body, &state); err != nil || state.Version != 1 || len(state.Disabled) > 4096 {
		return state, errors.New("invalid skills-state.json: expected version 1 and at most 4096 disabled skill paths")
	}
	return state, nil
}

func skillFile(source string) (string, error) {
	return filepath.EvalSymlinks(filepath.Join(source, "SKILL.md"))
}

// ManageSkills includes disabled skills; Skills and model tools expose enabled ones.
func (c *Conversation) ManageSkills(ctx context.Context) ([]Skill, error) {
	skills, _, err := c.Workspace.skillCatalog(ctx, c.Host.Config.Home, true)
	return skills, err
}

func (c *Conversation) SetSkillEnabled(ctx context.Context, source string, enabled bool) error {
	skills, err := c.ManageSkills(ctx)
	if err != nil {
		return err
	}
	if !slices.ContainsFunc(skills, func(skill Skill) bool { return skill.Source == source }) {
		return errors.New("skill is no longer available; reopen the skills list")
	}
	path, err := skillFile(source)
	if err != nil {
		return err
	}
	home := c.Host.Config.Home
	if err := os.MkdirAll(home, 0700); err != nil {
		return err
	}
	lock, err := filelock.Acquire(ctx, filepath.Join(home, "skills-state.lock"))
	if err != nil {
		return err
	}
	defer lock.Close()
	state, err := readSkillPreferences(home)
	if err != nil {
		return err
	}
	state.Disabled = slices.DeleteFunc(state.Disabled, func(value string) bool { return value == path })
	if !enabled {
		state.Disabled = append(state.Disabled, path)
	}
	if len(state.Disabled) > 4096 {
		return errors.New("disabled skill limit reached (4096)")
	}
	slices.Sort(state.Disabled)
	body, err := json.Marshal(state)
	if err != nil {
		return err
	}
	if len(body) > 1<<20 {
		return errors.New("skill preferences exceed 1 MiB")
	}
	file, err := os.CreateTemp(home, ".skills-state-*")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	_, writeErr := file.Write(body)
	err = errors.Join(writeErr, file.Sync(), file.Close())
	if err == nil {
		err = os.Rename(file.Name(), filepath.Join(home, "skills-state.json"))
	}
	if err != nil {
		return fmt.Errorf("save skill preferences: %w", err)
	}
	return nil
}
