package memory

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/SHIROH4/sion-plus/internal/domain/types"
)

func newPersonaStore(t *testing.T) (*PersonaStore, *SQLiteStore) {
	t.Helper()
	store, _ := NewSQLiteStore(filepath.Join(t.TempDir(), "test.db"))
	return NewPersonaStore(store), store
}

func seedReflection(t *testing.T, store *SQLiteStore, entity, relType, text, status string, rein, disp float64) int64 {
	t.Helper()
	r := &types.ReflectionEntry{
		Entity:        entity,
		RelationType:  relType,
		Text:          text,
		Status:        status,
		Reinforcement: rein,
		Disputation:   disp,
		CreatedAt:     time.Now().Unix(),
		UpdatedAt:     time.Now().Unix(),
	}
	id, err := store.SaveReflection(context.Background(), r)
	if err != nil {
		t.Fatalf("seed reflection: %v", err)
	}
	return id
}

func TestPersonaQueryByEntity(t *testing.T) {
	p, store := newPersonaStore(t)
	ctx := context.Background()

	seedReflection(t, store, "master", "preference", "likes Go language", "promoted", 3.0, 0)
	seedReflection(t, store, "master", "preference", "uses VS Code", "promoted", 2.5, 0)
	seedReflection(t, store, "neko", "self_awareness", "I should be concise", "promoted", 2.0, 0)
	seedReflection(t, store, "master", "preference", "not yet promoted", "pending", 1.0, 0)

	entries, err := p.Query(ctx, PersonaQuery{Entity: "master"})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if len(entries) != 2 {
		t.Errorf("expected 2 master entries, got %d", len(entries))
	}

	// Should be sorted by score desc
	if entries[0].Evidence.Score < entries[1].Evidence.Score {
		t.Error("entries should be sorted by score descending")
	}
}

func TestPersonaQueryByRelationType(t *testing.T) {
	p, store := newPersonaStore(t)
	ctx := context.Background()

	seedReflection(t, store, "master", "boundary", "do not interrupt when debugging", "promoted", 4.0, 0)
	seedReflection(t, store, "master", "preference", "likes dark mode", "promoted", 2.0, 0)

	entries, err := p.Query(ctx, PersonaQuery{
		Entity:       "master",
		RelationType: "boundary",
	})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 boundary, got %d", len(entries))
	}
	if entries[0].RelationType != "boundary" {
		t.Errorf("expected boundary, got %s", entries[0].RelationType)
	}
	if entries[0].Value != "do not interrupt when debugging" {
		t.Errorf("wrong value: %s", entries[0].Value)
	}
}

func TestPersonaBoundaries(t *testing.T) {
	p, store := newPersonaStore(t)
	ctx := context.Background()

	// High evidence boundary → returned
	seedReflection(t, store, "master", "boundary", "never mention my weight", "promoted", 3.0, 0)
	// Low evidence boundary → filtered by MinScore
	seedReflection(t, store, "master", "boundary", "uncertain boundary", "promoted", 0.3, 0)

	entries := p.Boundaries(ctx)
	if len(entries) != 1 {
		t.Errorf("expected 1 boundary (high confidence only), got %d", len(entries))
	}
}

func TestPersonaPreferences(t *testing.T) {
	p, store := newPersonaStore(t)
	ctx := context.Background()

	seedReflection(t, store, "master", "preference", "prefers Rust over Go now", "promoted", 2.5, 0)
	seedReflection(t, store, "master", "preference", "likes morning coding", "promoted", 2.0, 0)

	entries := p.Preferences(ctx)
	if len(entries) != 2 {
		t.Errorf("expected 2 preferences, got %d", len(entries))
	}
}

func TestPersonaWorkPatterns(t *testing.T) {
	p, store := newPersonaStore(t)
	ctx := context.Background()

	seedReflection(t, store, "master", "work_pattern", "works late at night", "promoted", 3.0, 0)

	entries := p.WorkPatterns(ctx)
	if len(entries) != 1 {
		t.Errorf("expected 1 work pattern, got %d", len(entries))
	}
}

func TestPersonaSelfTraits(t *testing.T) {
	p, store := newPersonaStore(t)
	ctx := context.Background()

	seedReflection(t, store, "neko", "self_awareness", "I am good at tech discussions", "promoted", 2.5, 0)
	seedReflection(t, store, "neko", "learned", "master prefers short answers", "promoted", 3.0, 0)

	entries := p.SelfTraits(ctx)
	if len(entries) != 2 {
		t.Errorf("expected 2 self traits, got %d", len(entries))
	}
}

func TestPersonaDynamics(t *testing.T) {
	p, store := newPersonaStore(t)
	ctx := context.Background()

	seedReflection(t, store, "relationship", "dynamic", "master likes morning chats", "promoted", 2.5, 0)

	entries := p.Dynamics(ctx)
	if len(entries) != 1 {
		t.Errorf("expected 1 dynamic, got %d", len(entries))
	}
}

func TestPersonaStats(t *testing.T) {
	p, store := newPersonaStore(t)
	ctx := context.Background()

	seedReflection(t, store, "master", "preference", "a", "promoted", 2.0, 0)
	seedReflection(t, store, "master", "boundary", "b", "promoted", 2.0, 0)
	seedReflection(t, store, "neko", "self_awareness", "c", "promoted", 2.0, 0)
	seedReflection(t, store, "relationship", "dynamic", "d", "promoted", 2.0, 0)

	stats := p.Stats(ctx)
	if stats["master"] != 2 {
		t.Errorf("master=%d", stats["master"])
	}
	if stats["neko"] != 1 {
		t.Errorf("neko=%d", stats["neko"])
	}
	if stats["relationship"] != 1 {
		t.Errorf("relationship=%d", stats["relationship"])
	}
}

func TestPersonaContradiction(t *testing.T) {
	p, store := newPersonaStore(t)
	ctx := context.Background()

	aID := seedReflection(t, store, "master", "preference", "likes quiet", "promoted", 3.0, 0)
	bID := seedReflection(t, store, "master", "preference", "likes music while working", "promoted", 3.0, 0)

	// Set contradiction: b contradicts a
	err := p.SetContradiction(ctx, aID, bID)
	if err != nil {
		t.Fatalf("SetContradiction: %v", err)
	}

	// b should now be denied
	refs, _ := store.ListReflectionsByStatus(ctx, []string{"denied"}, 1)
	if len(refs) != 1 {
		t.Fatalf("expected 1 denied, got %d", len(refs))
	}
	if refs[0].ID != bID {
		t.Errorf("expected b to be denied, got id=%d", refs[0].ID)
	}
}

func TestPersonaQueryMinScore(t *testing.T) {
	p, store := newPersonaStore(t)
	ctx := context.Background()

	seedReflection(t, store, "master", "preference", "strong", "promoted", 3.0, 0)
	seedReflection(t, store, "master", "preference", "weak", "promoted", 0.3, 0)

	entries, _ := p.Query(ctx, PersonaQuery{Entity: "master"})
	if len(entries) != 1 {
		t.Errorf("weak entry should be filtered by MinScore, got %d", len(entries))
	}
}

func TestPersonaGetByEntity(t *testing.T) {
	p, store := newPersonaStore(t)
	ctx := context.Background()

	seedReflection(t, store, "master", "preference", "p1", "promoted", 2.0, 0)
	seedReflection(t, store, "master", "boundary", "b1", "promoted", 2.0, 0)
	seedReflection(t, store, "master", "work_pattern", "w1", "promoted", 2.0, 0)

	grouped := p.GetByEntity(ctx, "master")
	if len(grouped["preference"]) != 1 {
		t.Error("missing preference")
	}
	if len(grouped["boundary"]) != 1 {
		t.Error("missing boundary")
	}
	if len(grouped["work_pattern"]) != 1 {
		t.Error("missing work_pattern")
	}
}
