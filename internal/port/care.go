package port

import (
	"context"

	"github.com/SHIROH4/sion-plus/internal/domain/types"
)

// ── Care Engine ──

// CareEngine monitors user wellbeing from screen observations and chat context,
// checks trigger conditions, and emits care action suggestions.
// Implementation: domain/care/triggers.go (domain logic) +
//
//	app/modules/care_svc.go (scheduling)
type CareEngine interface {
	// UpdateState feeds a new screen observation into the care state.
	UpdateState(ctx context.Context, obs *ScreenObservation)

	// UpdateStress adjusts the stress level from chat/behavior signals.
	UpdateStress(level float64)

	// CheckTriggers evaluates all care triggers against the current state.
	// Returns suggested care actions, ordered by priority.
	CheckTriggers(ctx context.Context) []types.CareAction

	// Snapshot returns the current care state for injection into chat context.
	Snapshot() types.UserCareState

	// RecordResponse records the user's reaction to a care action.
	RecordResponse(action types.CareAction, outcome types.OutcomeResult)
}

// ── Action Outcome Repository ──

// ActionOutcomeRepository persists and queries the outcomes of proactive actions.
// Used by Learner and StrategicAgent for DPO weight updates and strategy distillation.
type ActionOutcomeRepository interface {
	SaveOutcome(ctx context.Context, o *types.ActionOutcome) error
	SuccessRate(ctx context.Context, filter OutcomeFilter) (accepted, total int)
	RecentOutcomes(ctx context.Context, limit int) ([]types.ActionOutcome, error)
}
