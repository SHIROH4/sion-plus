package memory

import (
	"context"
	"fmt"
	"log"
	"sort"

	"github.com/SHIROH4/sion-plus/internal/domain/types"
)

// PersonaStore provides structured query access to the identity knowledge graph.
// It is a VIEW over the reflections table — persona entries ARE promoted reflections.
// No separate table needed.
//
// Entity → RelationType → Value:
//   "master"        → "preference"    → "likes Go language"
//   "master"        → "work_pattern"  → "works late at night"
//   "master"        → "boundary"      → "do not interrupt during debugging"
//   "neko"          → "self_awareness" → "I should be quiet when master focuses"
//   "neko"          → "learned"       → "master prefers concise answers"
//   "relationship"  → "dynamic"       → "master likes morning chats"
type PersonaStore struct {
	db *SQLiteStore
}

// PersonaEntry is a structured identity node derived from a promoted reflection.
type PersonaEntry struct {
	ID           int64              `json:"id"`
	Entity       string             `json:"entity"`
	RelationType string             `json:"relation_type"`
	Value        string             `json:"value"`
	Evidence     PersonaEvidence    `json:"evidence"`
	Status       string             `json:"status"`
	SourceCount  int                `json:"source_count"`  // how many facts contributed
	ContradictedBy []int64          `json:"contradicted_by,omitempty"`
	CreatedAt    int64              `json:"created_at"`
}

// PersonaEvidence is the evidence snapshot for a persona entry.
type PersonaEvidence struct {
	Reinforcement float64 `json:"reinforcement"`
	Disputation   float64 `json:"disputation"`
	Score         float64 `json:"score"`
	LastUpdatedAt int64   `json:"last_updated_at"`
}

// PersonaQuery filters for persona entries.
type PersonaQuery struct {
	Entity       string // "master" | "neko" | "relationship" | "" = all
	RelationType string // specific trait or "" = all
	MinScore     float64 // minimum evidence score
	Limit        int
}

func NewPersonaStore(db *SQLiteStore) *PersonaStore {
	return &PersonaStore{db: db}
}

// ── Query methods ──────────────────────────────────────────────────

// Query returns persona entries matching the filter, sorted by evidence score desc.
func (p *PersonaStore) Query(ctx context.Context, q PersonaQuery) ([]PersonaEntry, error) {
	if q.Limit <= 0 {
		q.Limit = 50
	}
	if q.MinScore <= 0 {
		q.MinScore = 0.5 // only reasonably confident entries
	}

	// Get all promoted reflections (these ARE persona entries)
	refs, err := p.db.ListReflectionsByStatus(ctx, []string{"promoted"}, 0)
	if err != nil {
		return nil, fmt.Errorf("persona query: %w", err)
	}

	var entries []PersonaEntry
	for _, r := range refs {
		// Filter by entity
		if q.Entity != "" && r.Entity != q.Entity {
			continue
		}
		// Filter by relation type
		if q.RelationType != "" && r.RelationType != q.RelationType {
			continue
		}

		score := r.Reinforcement - r.Disputation
		if score < q.MinScore {
			continue
		}

		entry := PersonaEntry{
			ID:           r.ID,
			Entity:       r.Entity,
			RelationType: r.RelationType,
			Value:        r.Text,
			Status:       r.Status,
			SourceCount:  len(r.SourceFactIDs),
			CreatedAt:    r.CreatedAt,
			Evidence: PersonaEvidence{
				Reinforcement: r.Reinforcement,
				Disputation:   r.Disputation,
				Score:         score,
			},
		}
		entries = append(entries, entry)
	}

	// Sort by evidence score descending
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Evidence.Score > entries[j].Evidence.Score
	})

	if len(entries) > q.Limit {
		entries = entries[:q.Limit]
	}

	return entries, nil
}

// ── Convenience queries ────────────────────────────────────────────

// UserTraits returns all confirmed facts about the user, organized by category.
func (p *PersonaStore) UserTraits(ctx context.Context) map[string][]PersonaEntry {
	result := make(map[string][]PersonaEntry)
	entries, err := p.Query(ctx, PersonaQuery{Entity: "master"})
	if err != nil {
		log.Printf("[PersonaStore] UserTraits: %v", err)
		return result
	}
	for _, e := range entries {
		result[e.RelationType] = append(result[e.RelationType], e)
	}
	return result
}

// SelfTraits returns all confirmed self-knowledge entries.
func (p *PersonaStore) SelfTraits(ctx context.Context) []PersonaEntry {
	entries, _ := p.Query(ctx, PersonaQuery{Entity: "neko"})
	return entries
}

// Boundaries returns all user boundaries (relation_type=boundary).
// These are critical — always respected, never violated.
func (p *PersonaStore) Boundaries(ctx context.Context) []PersonaEntry {
	entries, _ := p.Query(ctx, PersonaQuery{
		Entity:       "master",
		RelationType: string(types.RelBoundary),
		MinScore:     1.0, // boundaries need higher confidence
	})
	return entries
}

// WorkPatterns returns the user's work habit entries.
func (p *PersonaStore) WorkPatterns(ctx context.Context) []PersonaEntry {
	entries, _ := p.Query(ctx, PersonaQuery{
		Entity:       "master",
		RelationType: string(types.RelWorkPattern),
	})
	return entries
}

// Preferences returns the user's confirmed preferences.
func (p *PersonaStore) Preferences(ctx context.Context) []PersonaEntry {
	entries, _ := p.Query(ctx, PersonaQuery{
		Entity:       "master",
		RelationType: string(types.RelPreference),
	})
	return entries
}

// Dynamics returns relationship interaction patterns.
func (p *PersonaStore) Dynamics(ctx context.Context) []PersonaEntry {
	entries, _ := p.Query(ctx, PersonaQuery{
		Entity:       "relationship",
		RelationType: string(types.RelDynamic),
	})
	return entries
}

// GetByEntity returns all persona entries for an entity, organized by trait.
func (p *PersonaStore) GetByEntity(ctx context.Context, entity string) map[string][]PersonaEntry {
	result := make(map[string][]PersonaEntry)
	entries, err := p.Query(ctx, PersonaQuery{Entity: entity})
	if err != nil {
		return result
	}
	for _, e := range entries {
		result[e.RelationType] = append(result[e.RelationType], e)
	}
	return result
}

// ── Contradiction tracking ─────────────────────────────────────────

// SetContradiction records that entryB contradicts entryA.
func (p *PersonaStore) SetContradiction(ctx context.Context, entryA, entryB int64) error {
	// Store as feedback on the contradicted entry
	return p.db.UpdateReflectionStatus(ctx, entryB, "denied",
		fmt.Sprintf("contradicted by reflection %d", entryA))
}

// Stats returns persona statistics.
func (p *PersonaStore) Stats(ctx context.Context) map[string]int {
	stats := map[string]int{
		"master":        0,
		"neko":          0,
		"relationship":  0,
		"boundaries":    0,
		"preferences":   0,
		"work_patterns": 0,
	}
	for _, entity := range []string{"master", "neko", "relationship"} {
		entries, _ := p.Query(ctx, PersonaQuery{Entity: entity})
		stats[entity] = len(entries)
	}
	for _, e := range p.Boundaries(ctx) {
		_ = e
		stats["boundaries"]++
	}
	for _, e := range p.Preferences(ctx) {
		_ = e
		stats["preferences"]++
	}
	for _, e := range p.WorkPatterns(ctx) {
		_ = e
		stats["work_patterns"]++
	}
	return stats
}
