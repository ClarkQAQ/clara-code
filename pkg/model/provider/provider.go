// Package provider connects a LanguageModel to an explicit Responses or
// Chat Completions endpoint.
package provider

import (
	yuzu "clara-code/pkg/model"
	openai "clara-code/pkg/model/internal/wire"
	"cmp"
	"context"
	"fmt"
	"maps"
	"net/url"
	"strings"
)

// Name identifies the HTTP model adapter in provider metadata.
const Name = "http"

// API is the OpenAI API shape a Provider talks to.
type API string

const (
	// APIChatCompletions uses the Chat Completions API.
	APIChatCompletions API = "chat_completions"
	// APIResponses uses the Responses API.
	APIResponses API = "responses"
)

// Provider talks to OpenAI or any OpenAI-compatible endpoint using a single
// API shape, chosen when the provider is created.
type Provider struct {
	options options
}

type options struct {
	api                  API
	baseURL              string
	apiKey               string
	organization         string
	project              string
	headers              map[string]string
	userAgent            string
	client               openai.HTTPClient
	requestOptions       []openai.RequestOption
	objectMode           yuzu.ObjectMode
	languageModelOptions []LanguageModelOption
	// replayReasoning replays assistant reasoning items on the Responses API.
	// Providers that require the chain of thought back (DeepSeek thinking
	// mode) reject continuations that carry tools without it.
	replayReasoning    bool
	systemInstructions bool
	// reasoningModels names models this deployment declares as reasoning models,
	// because a compatible endpoint may be served under a name the provider
	// cannot recognise.
	reasoningModels []string
}

// Option defines a function that configures Provider options.
type Option = func(*options)

// New creates an HTTP model provider. Set an explicit base URL.
// The default protocol is Chat Completions.
func New(opts ...Option) (*Provider, error) {
	cfg := options{
		headers:              map[string]string{},
		languageModelOptions: make([]LanguageModelOption, 0),
	}
	for _, o := range opts {
		o(&cfg)
	}

	cfg.api = cmp.Or(cfg.api, APIChatCompletions)
	switch cfg.api {
	case APIChatCompletions, APIResponses:
	default:
		return nil, fmt.Errorf("unsupported API %q", cfg.api)
	}

	if strings.TrimSpace(cfg.baseURL) == "" {
		return nil, fmt.Errorf("model base URL is required")
	}
	parsed, err := url.Parse(cfg.baseURL)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return nil, fmt.Errorf("invalid model base URL")
	}

	if cfg.organization != "" {
		cfg.headers["OpenAi-Organization"] = cfg.organization
	}
	if cfg.project != "" {
		cfg.headers["OpenAi-Project"] = cfg.project
	}

	return &Provider{options: cfg}, nil
}

// WithAPI sets the API shape used for every model of the provider.
func WithAPI(api API) Option {
	return func(o *options) {
		o.api = api
	}
}

// WithSystemInstructions maps leading Responses system messages to instructions.
// Explicit ProviderOptions.Instructions wins; Completions is unchanged.
func WithSystemInstructions(enabled bool) Option {
	return func(o *options) { o.systemInstructions = enabled }
}

// WithBaseURL sets the base URL for the provider.
func WithBaseURL(baseURL string) Option {
	return func(o *options) {
		o.baseURL = baseURL
	}
}

// WithAPIKey sets the API key for the provider.
func WithAPIKey(apiKey string) Option {
	return func(o *options) {
		o.apiKey = apiKey
	}
}

// WithOrganization sets the organization for the provider.
func WithOrganization(organization string) Option {
	return func(o *options) {
		o.organization = organization
	}
}

// WithProject sets the project for the provider.
func WithProject(project string) Option {
	return func(o *options) {
		o.project = project
	}
}

// WithHeaders sets the headers for the provider.
func WithHeaders(headers map[string]string) Option {
	return func(o *options) {
		maps.Copy(o.headers, headers)
	}
}

// WithUserAgent sets an explicit User-Agent header, overriding the
// default and any value set via WithHeaders.
func WithUserAgent(ua string) Option {
	return func(o *options) {
		o.userAgent = ua
	}
}

// WithHTTPClient sets the HTTP client for the provider.
func WithHTTPClient(client openai.HTTPClient) Option {
	return func(o *options) {
		o.client = client
	}
}

// WithRequestOptions sets the request options applied to every call of the
// provider, such as extra headers, middlewares or retries.
func WithRequestOptions(opts ...openai.RequestOption) Option {
	return func(o *options) {
		o.requestOptions = append(o.requestOptions, opts...)
	}
}

// WithLanguageModelOptions sets the language model options for the provider.
func WithLanguageModelOptions(opts ...LanguageModelOption) Option {
	return func(o *options) {
		o.languageModelOptions = append(o.languageModelOptions, opts...)
	}
}

// WithObjectMode sets the object generation mode. ObjectModeJSON is not
// supported as a distinct mode and falls back to ObjectModeAuto, which uses
// the native structured output support of the selected API.
func WithObjectMode(om yuzu.ObjectMode) Option {
	return func(o *options) {
		if om == yuzu.ObjectModeJSON {
			om = yuzu.ObjectModeAuto
		}
		o.objectMode = om
	}
}

// WithReplayReasoning replays assistant reasoning items as Responses API input
// on every request. Enable it for providers that require the chain of thought
// of a tool-calling turn to be passed back (DeepSeek thinking mode); the
// default leaves reasoning out, which is what OpenAI expects.
func WithReplayReasoning(replay bool) Option {
	return func(o *options) {
		o.replayReasoning = replay
	}
}

// WithReasoningModels declares model IDs that take a reasoning setting even
// though their names say nothing about it. The Responses API only carries
// reasoning.effort for the models the provider knows are reasoning models, and
// a deployment that runs a compatible endpoint under its own model name is the
// one that knows best.
func WithReasoningModels(models ...string) Option {
	return func(o *options) {
		for _, model := range models {
			if model = strings.TrimSpace(model); model != "" {
				o.reasoningModels = append(o.reasoningModels, model)
			}
		}
	}
}

// isDeclaredReasoningModel reports whether the deployment named this model as
// one that understands a reasoning setting.
func (o options) isDeclaredReasoningModel(modelID string) bool {
	for _, model := range o.reasoningModels {
		if model == modelID {
			return true
		}
	}
	return false
}

// LanguageModel creates a language model for the given model ID.
func (p *Provider) LanguageModel(_ context.Context, modelID string) (yuzu.LanguageModel, error) {
	client := openai.NewClient(p.clientOptions()...)

	objectMode := p.options.objectMode
	if objectMode == yuzu.ObjectModeJSON {
		objectMode = yuzu.ObjectModeAuto
	}

	if p.options.api == APIResponses {
		model := newResponsesLanguageModel(
			modelID,
			Name,
			client,
			objectMode,
			p.options.replayReasoning,
			p.options.isDeclaredReasoningModel(modelID),
			languageModelHeaderFunc(p.options.languageModelOptions),
		)
		model.systemInstructions = p.options.systemInstructions
		return model, nil
	}

	languageModelOptions := append([]LanguageModelOption{}, p.options.languageModelOptions...)
	languageModelOptions = append(languageModelOptions, WithLanguageModelObjectMode(objectMode))

	return newLanguageModel(
		modelID,
		Name,
		client,
		languageModelOptions...,
	), nil
}

func (p *Provider) clientOptions() []openai.RequestOption {
	opts := make([]openai.RequestOption, 0, 4+len(p.options.headers)+len(p.options.requestOptions))

	if p.options.apiKey != "" {
		opts = append(opts, openai.WithAPIKey(p.options.apiKey))
	}
	if p.options.baseURL != "" {
		opts = append(opts, openai.WithBaseURL(p.options.baseURL))
	}

	for key, value := range ResolveHeaders(p.options.headers, p.options.userAgent, DefaultUserAgent) {
		opts = append(opts, openai.WithHeader(key, value))
	}

	if p.options.client != nil {
		opts = append(opts, openai.WithHTTPClient(p.options.client))
	}

	return append(opts, p.options.requestOptions...)
}
