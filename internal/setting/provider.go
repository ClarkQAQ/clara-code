package setting

import (
	"context"
	"errors"
	"os"
	"slices"
	"strings"
	"time"

	"clara-code/pkg/model"
	"clara-code/pkg/model/provider"
)

func resolveHeaders(literal, env map[string]string) map[string]string {
	out := map[string]string{}
	for k, v := range literal {
		out[k] = v
	}
	for k, name := range env {
		if v := os.Getenv(name); v != "" {
			out[k] = v
		}
	}
	return out
}
func (c Config) Connection() Provider {
	if p, ok := c.Providers[c.ModelProvider]; ok {
		return p
	}
	// Programmatic hosts may supply resolved connection values directly.
	return Provider{BaseURL: c.Endpoint, WireAPI: c.Protocol, APIKey: c.APIKey, Models: c.Models}
}
func (c Config) Factory() (func(context.Context, string) (model.LanguageModel, error), error) {
	p := c.Connection()
	if p.BaseURL == "" || c.DefaultModel == "" {
		return nil, errors.New("configure model, model_provider and model_providers.<id>.base_url, or use --demo")
	}
	api := provider.APIResponses
	switch p.WireAPI {
	case "", "responses":
	case "completions":
		api = provider.APIChatCompletions
	default:
		return nil, errors.New("wire_api must be responses or completions")
	}
	key := p.APIKey
	if key == "" && len(c.Providers) == 0 {
		key = os.Getenv("CLARA_CODE_API_KEY")
	}
	client, err := provider.New(provider.WithBaseURL(p.BaseURL), provider.WithAPI(api), provider.WithAPIKey(key), provider.WithHeaders(resolveHeaders(p.HTTPHeaders, p.EnvHTTPHeaders)), provider.WithHTTPClient(requestClient{timeout: time.Duration(p.RequestTimeoutMS) * time.Millisecond, idle: time.Duration(p.StreamIdleTimeoutMS) * time.Millisecond}))
	if err != nil {
		return nil, errors.New("invalid provider connection settings")
	}
	return client.LanguageModel, nil
}
func (c Config) Redact(text string) string {
	keys := []string{c.APIKey, os.Getenv("CLARA_CODE_API_KEY"), os.Getenv("CLARA_CORE_API_KEY")}
	providers := []Provider{c.Connection()}
	for _, p := range c.Providers {
		providers = append(providers, p)
	}
	for _, p := range providers {
		keys = append(keys, p.APIKey)
		for _, v := range resolveHeaders(p.HTTPHeaders, p.EnvHTTPHeaders) {
			keys = append(keys, v)
		}
	}
	for _, m := range c.MCP {
		for _, v := range resolveHeaders(m.Headers, m.EnvHeaders) {
			keys = append(keys, v)
		}
		for _, v := range m.Env {
			keys = append(keys, v)
		}
	}
	for _, a := range c.ACP {
		for _, v := range a.Env {
			keys = append(keys, v)
		}
	}
	slices.SortFunc(keys, func(a, b string) int { return len(b) - len(a) })
	for _, key := range keys {
		if key != "" {
			text = strings.ReplaceAll(text, key, "[redacted]")
		}
	}
	return text
}
