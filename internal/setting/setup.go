package setting

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/pelletier/go-toml/v2"
)

type InitialConfig struct {
	ProviderID, BaseURL, WireAPI, APIKey, ModelID string
	ContextWindow, MaxOutputTokens                int64
}

func (c Config) NeedsSetup() bool {
	if c.hasConfigFile {
		return false
	}
	_, err := c.Factory()
	return err != nil
}

func (c Config) SaveInitial(ctx context.Context, setup InitialConfig) (Config, error) {
	path := filepath.Join(c.Home, "config.toml")
	if c.hasConfigFile {
		return Config{}, configAlreadyExists(path)
	}
	profile := map[string]any{"id": setup.ModelID}
	if setup.ContextWindow != 0 {
		profile["context_window"] = setup.ContextWindow
	}
	if setup.MaxOutputTokens != 0 {
		profile["max_output_tokens"] = setup.MaxOutputTokens
	}
	provider := map[string]any{
		"base_url": setup.BaseURL,
		"wire_api": setup.WireAPI,
		"models":   map[string]any{setup.ModelID: profile},
	}
	if setup.APIKey != "" {
		provider["api_key"] = setup.APIKey
	}
	initial := map[string]any{
		"model":           setup.ModelID,
		"model_provider":  setup.ProviderID,
		"model_providers": map[string]any{setup.ProviderID: provider},
	}
	body, err := toml.Marshal(initial)
	if err != nil {
		return Config{}, errors.New("could not encode initial configuration")
	}
	loaded, err := loadWith(ctx, c.Options, c.Home, initial)
	if err != nil {
		return Config{}, err
	}
	if _, err := loaded.Factory(); err != nil {
		return Config{}, err
	}
	if err := writeInitial(ctx, path, body); err != nil {
		return Config{}, err
	}
	return loaded, nil
}

func configAlreadyExists(path string) error {
	return fmt.Errorf("configuration %s already exists; restart Clara Code to load it", path)
}

func writeInitial(ctx context.Context, path string, body []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	file, err := os.CreateTemp(dir, ".config-*")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	_, err = file.Write(body)
	if err == nil {
		err = file.Sync()
	}
	if err := errors.Join(err, file.Close()); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	// Publish a complete private file without replacing another process's config.
	if err := os.Link(file.Name(), path); errors.Is(err, os.ErrExist) {
		return configAlreadyExists(path)
	} else {
		return err
	}
}
