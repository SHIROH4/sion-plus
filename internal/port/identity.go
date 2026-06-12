package port

import (
	"context"

	"github.com/SHIROH4/sion-plus/internal/domain/types"
)

// ── Identity Repository ──

// IdentityRepository persists the identity knowledge graph.
// Each node represents a fact about an entity (user, character, relationship).
// Implementation: adapter/identity/identity_repo.go
type IdentityRepository interface {
	SaveNode(ctx context.Context, node *types.IdentityNode) error
	GetNode(ctx context.Context, id int64) (*types.IdentityNode, error)
	ListAll(ctx context.Context) ([]types.IdentityNode, error)
	SearchByVector(ctx context.Context, vector []float32, topK int) ([]types.IdentityNode, error)
	Deactivate(ctx context.Context, id int64) error
}

// ── Self Model Store ──

// SelfModelStore persists the AI's evolving self-description.
// The self model is a single, versioned text blob that evolves
// through strategic reflection (L2→L3 consolidation).
// Implementation: adapter/identity/self_model_store.go
type SelfModelStore interface {
	Load(ctx context.Context) (*types.SelfModel, error)
	Save(ctx context.Context, model *types.SelfModel) error
}
