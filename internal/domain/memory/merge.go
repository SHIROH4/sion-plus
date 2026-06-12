package memory

import "github.com/SHIROH4/sion-plus/internal/domain/types"

// ── Strategy Principle Merging ──
//
// When two strategies are semantically similar (cosine > 0.75),
// the merge function determines which to keep:
//   - New significantly better (confidence > existing + 0.1): replace old
//   - Similar confidence: flag for LLM merge
//   - Existing better: skip new

type MergeDecision int

const (
	MergeReplace MergeDecision = iota // new replaces old
	MergeLLM     MergeDecision = iota // need LLM synthesis
	MergeSkip    MergeDecision = iota // keep existing, drop new
)

// DecideMerge determines how to handle two similar strategies.
func DecideMerge(existing, new types.StrategyPrinciple) MergeDecision {
	if new.Confidence > existing.Confidence+0.1 {
		return MergeReplace
	}
	if new.Confidence >= existing.Confidence-0.1 {
		return MergeLLM
	}
	return MergeSkip
}

// MergeResult is the output of an LLM strategy merge.
type MergeResult struct {
	Situation    string  `json:"situation"`
	GoodStrategy string  `json:"good_strategy"`
	BadStrategy  string  `json:"bad_strategy"`
	Reason       string  `json:"reason"`
	Confidence   float64 `json:"confidence"`
}

// ApplyMerge creates a merged StrategyPrinciple from two sources.
func ApplyMerge(result MergeResult, existing, new types.StrategyPrinciple) types.StrategyPrinciple {
	// Average confidence, weighted toward higher
	confidence := (existing.Confidence + new.Confidence) / 2
	if confidence < 0.5 {
		confidence = 0.5
	}
	if confidence > 1.0 {
		confidence = 1.0
	}

	return types.StrategyPrinciple{
		Situation:    result.Situation,
		GoodStrategy: result.GoodStrategy,
		BadStrategy:  result.BadStrategy,
		Reason:       result.Reason,
		Confidence:   confidence,
		Source:       "merged",
		Active:       true,
	}
}
