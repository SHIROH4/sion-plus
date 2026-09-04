package port

import (
	"context"

	"github.com/SHIROH4/sion-plus/internal/domain/types"
)

// ProactiveFeedbackStore is deliberately a narrow optional port. Existing
// memory implementations can keep working while SQLite persists the learning
// dataset used by the proactive policy.
type ProactiveFeedbackStore interface {
	SaveProactiveDecision(ctx context.Context, decision *types.ProactiveDecision) error
	SaveProactiveFeedback(ctx context.Context, feedback *types.ProactiveFeedback) error
	ResolveProactiveDecision(ctx context.Context, decisionID string, at int64) error
	LatestPendingProactiveDecision(ctx context.Context, since int64) (*types.ProactiveDecision, error)
	SaveProactiveReply(ctx context.Context, reply *types.ProactiveReply) error
	ListProactiveDecisions(ctx context.Context, limit int) ([]types.ProactiveDecision, error)
	ActionFeedbackStats(ctx context.Context) ([]types.ActionFeedbackStats, error)
	ActionFeedbackStatsForContext(ctx context.Context, contextKey string) ([]types.ActionFeedbackStats, error)
	EvaluateProactivePolicy(ctx context.Context, since int64) (*types.ProactivePolicyEvaluation, error)
	UpsertProactiveControl(ctx context.Context, control *types.ProactiveControl) error
	ClearProactiveControl(ctx context.Context, scope, scopeValue string) error
	ProactiveAllowed(ctx context.Context, action, category string, at int64) (bool, string, error)
}
