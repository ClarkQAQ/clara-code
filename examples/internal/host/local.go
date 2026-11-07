package host

import (
	"context"
	"errors"
	"path/filepath"

	"clara-code/examples/internal/config"
	"clara-code/pkg/core"
	modelapi "clara-code/pkg/model"
	"clara-code/pkg/model/provider"
	"clara-code/pkg/tool"
	"clara-code/pkg/tool/fs"
	"clara-code/pkg/tool/fs/local"
)

func Local(root string) (Config, error) {
	workspace, err := local.New(root)
	if err != nil {
		return Config{}, err
	}
	return Config{
		FS: workspace,
		Workspace: "The workspace root in file tools is /. Use paths like /main.go or ./main.go. " +
			"Use the same workspace paths for the exec workdir. Shell output may show native paths. " +
			"Translate a native path under the workspace into a workspace-relative path before passing it to a tool.",
		Runner: localRunner{workspace: workspace, runner: tool.NewLocal()},
	}, nil
}

type localRunner struct {
	workspace *local.FS
	runner    tool.Runner
}

func (r localRunner) Start(ctx context.Context, spec tool.Spec) (tool.Process, error) {
	path, err := fs.Resolve("/", spec.Workdir)
	if err != nil {
		return nil, err
	}
	spec.Workdir, err = r.workspace.ResolveDir(ctx, path)
	if err != nil {
		return nil, err
	}
	return r.runner.Start(ctx, spec)
}

func FileStore(ctx context.Context, dir string) (core.Store, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return nil, err
	}
	parent, err := local.New(filepath.Dir(abs))
	if err != nil {
		return nil, err
	}
	if err := parent.MkdirAll(ctx, fs.Path("/"+filepath.Base(abs)), 0o700); err != nil {
		return nil, err
	}
	storage, err := local.New(abs)
	if err != nil {
		return nil, err
	}
	return core.NewFileStore(blobs{storage}), nil
}

type blobs struct{ storage *local.FS }

func (b blobs) Read(ctx context.Context, name string, limit int64) ([]byte, error) {
	return b.storage.ReadFile(ctx, fs.Path("/"+name), limit)
}
func (b blobs) Write(ctx context.Context, name string, body []byte) error {
	_, err := b.storage.WriteFile(ctx, fs.Path("/"+name), body, fs.WriteOptions{Mode: 0o600, Overwrite: true})
	return err
}

func ModelFromEnv(ctx context.Context) (modelapi.LanguageModel, func(context.Context, string) (modelapi.LanguageModel, error), error) {
	endpoint, id := config.Env("ENDPOINT"), config.Env("MODEL")
	if endpoint == "" || id == "" {
		return nil, nil, errors.New("set CLARA_CORE_ENDPOINT and CLARA_CORE_MODEL, or use -demo")
	}
	api := provider.APIResponses
	switch config.Env("PROTOCOL") {
	case "", "responses":
	case "completions":
		api = provider.APIChatCompletions
	default:
		return nil, nil, errors.New("CLARA_CORE_PROTOCOL must be responses or completions")
	}
	client, err := provider.New(provider.WithAPI(api), provider.WithBaseURL(endpoint), provider.WithAPIKey(config.Env("API_KEY")))
	if err != nil {
		return nil, nil, err
	}
	model, err := client.LanguageModel(ctx, id)
	return model, client.LanguageModel, err
}
