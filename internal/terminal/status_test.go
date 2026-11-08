package terminal

import (
	"context"
	"strings"
	"testing"
	"time"

	"clara-code/internal/coding"
	"clara-code/pkg/core/prompt"
	"clara-code/pkg/model"
)

type cacheStatusModel struct {
	coding.DemoModel
	gate <-chan struct{}
}

func (m cacheStatusModel) Stream(ctx context.Context, _ model.Call) (model.StreamResponse, error) {
	return func(yield func(model.StreamPart) bool) {
		if m.gate != nil {
			select {
			case <-ctx.Done():
				return
			case <-m.gate:
			}
		}
		yield(model.StreamPart{Type: model.StreamPartTypeFinish, FinishReason: model.FinishReasonStop, Usage: model.Usage{InputTokens: 20, CacheReadTokens: 80, TotalTokens: 110, CacheReadReported: true}})
	}, nil
}

func TestConfiguredStatusFieldsAndCacheEfficiency(t *testing.T) {
	a := testApp(t)
	a.host.Config.TUI.StatusLine = []string{"round_cache_efficiency", "state", "session_cache_efficiency"}
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	a.conv.Host.Models = func(context.Context, string) (model.LanguageModel, error) { return cacheStatusModel{}, nil }
	if err := a.conv.ChangeModel(ctx, "demo", ""); err != nil {
		t.Fatal(err)
	}
	run, err := a.conv.Start(ctx, prompt.Execute, "first", false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := run.Wait(ctx); err != nil {
		t.Fatal(err)
	}
	if text := a.status(); text != "Last round cache 80.0% · Ready · Session cache 80.0%" {
		t.Fatal("configured field order or cache ratio", text)
	}
	gate := make(chan struct{})
	a.conv.Host.Models = func(context.Context, string) (model.LanguageModel, error) { return cacheStatusModel{gate: gate}, nil }
	if err := a.conv.ChangeModel(ctx, "demo", ""); err != nil {
		t.Fatal(err)
	}
	a.run, err = a.conv.Start(ctx, prompt.Execute, "second", false)
	if err != nil {
		t.Fatal(err)
	}
	if text := a.status(); !strings.HasPrefix(text, "Round cache unknown") || !strings.Contains(text, "Session cache 80.0%") {
		t.Fatal("new round must not reuse prior cache rate", text)
	}
	close(gate)
	if _, err := a.run.Wait(ctx); err != nil {
		t.Fatal(err)
	}
	if text := a.status(); !strings.HasPrefix(text, "Round cache 80.0% · Working") {
		t.Fatal("active round label", text)
	}
	a.run = nil
	a.host.Config.TUI.StatusLine = []string{}
	if text := a.status(); text != "" {
		t.Fatal("empty field list should hide the left status", text)
	}
}
