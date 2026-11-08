package session

import (
	"encoding/json"
	"testing"

	"clara-code/pkg/core"
	"clara-code/pkg/model"
	"clara-code/pkg/util/id"
)

func usageEvent(sequence uint64, run string, usage model.Usage) core.Event {
	body, _ := json.Marshal(model.Response{Usage: usage})
	return core.Event{Sequence: sequence, RunID: run, Kind: "step.completed", Data: body}
}

func TestCacheUsageAggregatesWeightedRoundsAndResumes(t *testing.T) {
	meta := Meta{Project: "/project"}
	first := []core.Event{
		{Sequence: 1, RunID: "run_one", Kind: "run.started"},
		usageEvent(2, "run_one", model.Usage{InputTokens: 20, CacheReadTokens: 80, TotalTokens: 110, OutputTokens: 10, CacheReadReported: true}),
		usageEvent(3, "run_one", model.Usage{InputTokens: 900, CacheReadTokens: 0, TotalTokens: 910, OutputTokens: 10, CacheReadReported: true}),
	}
	collectUsage(&meta, first)
	if rate, ok := meta.RoundCache.Efficiency(); !ok || rate != 8 {
		t.Fatal("round must weight input tokens, not average rates", rate, ok)
	}
	before := meta
	collectUsage(&meta, first)
	if meta != before {
		t.Fatal("replayed events counted twice")
	}
	collectUsage(&meta, []core.Event{{Sequence: 4, RunID: "run_two", Kind: "run.started"}})
	if _, ok := meta.RoundCache.Efficiency(); ok {
		t.Fatal("new round retained old usage")
	}
	collectUsage(&meta, []core.Event{usageEvent(5, "run_two", model.Usage{CacheReadTokens: 1000, TotalTokens: 1000, CacheReadReported: true}), {Sequence: 6, RunID: "run_two", Kind: "run.cancelled"}})
	if rate, _ := meta.RoundCache.Efficiency(); rate != 100 {
		t.Fatal("cancel lost accepted usage")
	}
	if rate, _ := meta.SessionCache.Efficiency(); rate != 54 {
		t.Fatal("session weighted efficiency", rate)
	}
	store := New(t.TempDir(), meta)
	state := core.State{ID: id.New("session"), Revision: 1}
	if err := store.Save(t.Context(), state); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	resumed, err := Open(store.home, state.ID, meta.Project)
	if err != nil {
		t.Fatal(err)
	}
	defer resumed.Close()
	if resumed.Meta().SessionCache != meta.SessionCache || resumed.Meta().RoundCache != meta.RoundCache {
		t.Fatal("cache counters not persisted")
	}
	fork, err := resumed.Fork(t.Context(), -1)
	if err != nil {
		t.Fatal(err)
	}
	defer fork.Close()
	if fork.Meta().SessionCache.Requests != 0 || fork.Meta().RoundCache.Requests != 0 {
		t.Fatal("fork inherited consumption")
	}
}

func TestCacheUnknownAndExplicitMiss(t *testing.T) {
	var cache CacheUsage
	cache.add(model.Usage{InputTokens: 100, CacheReadReported: true})
	if rate, ok := cache.Efficiency(); !ok || rate != 0 {
		t.Fatal("reported zero is a cache miss", rate, ok)
	}
	cache.add(model.Usage{InputTokens: 100})
	if _, ok := cache.Efficiency(); ok {
		t.Fatal("missing cache statistics reported as zero")
	}
	legacy := Meta{TotalUsage: model.Usage{InputTokens: 100, TotalTokens: 100}, UsageCursor: 1}
	collectUsage(&legacy, []core.Event{usageEvent(2, "run_legacy", model.Usage{InputTokens: 1, CacheReadTokens: 99, TotalTokens: 100, CacheReadReported: true})})
	if _, ok := legacy.SessionCache.Efficiency(); ok {
		t.Fatal("legacy unrecorded history presented as complete")
	}
	gap := Meta{UsageCursor: 1}
	collectUsage(&gap, []core.Event{usageEvent(5, "run_gap", model.Usage{InputTokens: 10, CacheReadTokens: 90, TotalTokens: 100, CacheReadReported: true})})
	if _, ok := gap.RoundCache.Efficiency(); ok {
		t.Fatal("retention gap hidden in round usage")
	}
	if _, ok := gap.SessionCache.Efficiency(); ok {
		t.Fatal("retention gap hidden in session usage")
	}
	truncated := Meta{}
	collectUsage(&truncated, []core.Event{{Sequence: 1, Kind: "step.completed", Truncated: true}})
	if !truncated.SessionCache.Incomplete || !truncated.RoundCache.Incomplete {
		t.Fatal("truncated usage hidden")
	}
}
