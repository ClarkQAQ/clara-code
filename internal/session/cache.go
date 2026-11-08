package session

import "clara-code/pkg/model"

// CacheUsage accumulates input cache hits across model requests, not percentages.
type CacheUsage struct {
	InputTokens  int64
	CachedTokens int64
	Requests     uint64
	Incomplete   bool
}

func (usage *CacheUsage) add(step model.Usage) {
	usage.Requests++
	usage.InputTokens += max(0, step.InputTokens) + max(0, step.CacheReadTokens)
	usage.CachedTokens += max(0, step.CacheReadTokens)
	if (!step.CacheReadReported && step.CacheReadTokens <= 0) || step.InputTokens < 0 || step.CacheReadTokens < 0 {
		usage.Incomplete = true
	}
}

func (usage CacheUsage) Efficiency() (float64, bool) {
	if usage.Requests == 0 || usage.Incomplete || usage.InputTokens <= 0 {
		return 0, false
	}
	return min(100, max(0, 100*float64(usage.CachedTokens)/float64(usage.InputTokens))), true
}
