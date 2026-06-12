package port

import (
	"context"
	"github.com/shirohania/sion/internal/domain/types"
)

// ── Feature Computer ──

// FeatureComputer computes the 52-dimension quantified features.
// Tier 1: pure in-memory (~1ms), runs every tick.
// Tier 2: SQL-backed (~50ms), TTL-cached for 5 minutes.
type FeatureComputer interface {
	ComputeFull(ctx context.Context, state *types.CognitionState) (*types.QuantifiedFeatures, error)
	ComputeTier1(state *types.CognitionState) *types.QuantifiedFeatures
}

// ── Drive Computer ──

// DriveComputer maps 52 features → 5 drives using weighted formulas.
type DriveComputer interface {
	Compute(ctx context.Context, f *types.QuantifiedFeatures, needs *types.IntrinsicNeeds) *types.DriveVector
}

// ── Action Scorer ──

// ActionScorer scores all 16 actions using drive dot-product + context modulation.
type ActionScorer interface {
	Score(ctx context.Context, drives *types.DriveVector, features *types.QuantifiedFeatures) ([]types.ScoredAction, error)
	AllActions() []types.ActionDef
	UpdateWeight(action, drive string, delta float64)
	LoadWeights() error
	SaveWeights() error
}

// ── Decision Router ──

// DecisionRouter implements System 1 / System 2 routing.
// S1 (fast path): pure math, gap > 0.03, no extreme conditions.
// S2 (LLM fallback): close scores, extreme emotions, stuck loops.
type DecisionRouter interface {
	Route(ctx context.Context, scored []types.ScoredAction, features *types.QuantifiedFeatures) (*types.DecisionResult, error)
}

// ── Need Model ──

// NeedModel manages the 6 intrinsic needs with homeostatic decay and growth.
type NeedModel interface {
	Grow(elapsedHours float64)
	Satisfy(action string, outcome types.OutcomeResult)
	Current() *types.IntrinsicNeeds
	Modulation() *types.NeedModulation
	Reset()
}
