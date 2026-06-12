package app

import (
	"context"

	"github.com/shirohania/sion/internal/adapter/memory"
	"github.com/shirohania/sion/internal/adapter/proactive"
)

// personaBridge adapts memory.PersonaStore to proactive.PersonaQuerier.
type personaBridge struct {
	store *memory.PersonaStore
}

func (b *personaBridge) Boundaries(ctx context.Context) []proactive.PersonaEntry {
	entries := b.store.Boundaries(ctx)
	out := make([]proactive.PersonaEntry, len(entries))
	for i, e := range entries {
		out[i] = proactive.PersonaEntry{
			Entity:       e.Entity,
			RelationType: e.RelationType,
			Value:        e.Value,
			Evidence:     proactive.PersonaEvidence{Score: e.Evidence.Score},
		}
	}
	return out
}

func (b *personaBridge) WorkPatterns(ctx context.Context) []proactive.PersonaEntry {
	entries := b.store.WorkPatterns(ctx)
	out := make([]proactive.PersonaEntry, len(entries))
	for i, e := range entries {
		out[i] = proactive.PersonaEntry{
			Entity:       e.Entity,
			RelationType: e.RelationType,
			Value:        e.Value,
			Evidence:     proactive.PersonaEvidence{Score: e.Evidence.Score},
		}
	}
	return out
}

func (b *personaBridge) Preferences(ctx context.Context) []proactive.PersonaEntry {
	entries := b.store.Preferences(ctx)
	out := make([]proactive.PersonaEntry, len(entries))
	for i, e := range entries {
		out[i] = proactive.PersonaEntry{
			Entity:       e.Entity,
			RelationType: e.RelationType,
			Value:        e.Value,
			Evidence:     proactive.PersonaEvidence{Score: e.Evidence.Score},
		}
	}
	return out
}
