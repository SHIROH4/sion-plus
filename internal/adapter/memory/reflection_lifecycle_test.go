package memory

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	domainMemory "github.com/SHIROH4/sion-plus/internal/domain/memory"
	"github.com/SHIROH4/sion-plus/internal/domain/types"
)

func newTestStack(t *testing.T) (*MemoryWorker, *SQLiteStore) {
	t.Helper()
	store, _ := NewSQLiteStore(filepath.Join(t.TempDir(), "test.db"))
	evidenceCfg := domainMemory.DefaultEvidenceConfig()
	evidence := NewEvidenceEngine(store, evidenceCfg)
	buffer := NewSessionBuffer(40, 0)
	recall := NewRecall(store, evidence)
	comp := NewCompressor(buffer, DefaultCompressorConfig())
	workerCfg := DefaultWorkerConfig()
	worker := NewMemoryWorker(store, evidence, recall, buffer, comp, workerCfg)
	return worker, store
}

func TestPromotionSweep(t *testing.T) {
	worker, store := newTestStack(t)
	ctx := context.Background()

	// Create a pending reflection with high evidence
	r := &types.ReflectionEntry{
		Text:          "主人喜欢在安静的环境工作",
		Entity:        "master",
		Status:        "pending",
		Reinforcement: 3.0,
		Disputation:   0.0,
		CreatedAt:     time.Now().Unix(),
		UpdatedAt:     time.Now().Unix(),
	}
	id, err := store.SaveReflection(ctx, r)
	if err != nil {
		t.Fatalf("SaveReflection: %v", err)
	}
	r.ID = id

	// Run promotion sweep
	worker.runPromotionSweep(ctx)

	// Should now be promoted (score 3.0 >= 2.0)
	refs, _ := store.ListReflectionsByStatus(ctx, []string{"promoted"}, 1)
	if len(refs) != 1 {
		t.Fatalf("expected 1 promoted, got %d", len(refs))
	}
	if refs[0].Status != "promoted" {
		t.Errorf("expected promoted, got %s", refs[0].Status)
	}
	if refs[0].ID != id {
		t.Errorf("wrong reflection ID: %d", refs[0].ID)
	}
}

func TestPromotionConfirmed(t *testing.T) {
	worker, store := newTestStack(t)
	ctx := context.Background()

	r := &types.ReflectionEntry{
		Text:          "主人最近在学习Rust",
		Entity:        "master",
		Status:        "pending",
		Reinforcement: 1.5,
		Disputation:   0.0,
		CreatedAt:     time.Now().Unix(),
		UpdatedAt:     time.Now().Unix(),
	}
	id, _ := store.SaveReflection(ctx, r)
	r.ID = id

	worker.runPromotionSweep(ctx)

	// 1.5 >= 1.0 → confirmed, but < 2.0 → not promoted
	refs, _ := store.ListReflectionsByStatus(ctx, []string{"confirmed"}, 1)
	if len(refs) != 1 {
		t.Fatalf("expected 1 confirmed, got %d", len(refs))
	}
	if refs[0].Status != "confirmed" {
		t.Errorf("expected confirmed, got %s", refs[0].Status)
	}
}

func TestPromotionDenied(t *testing.T) {
	worker, store := newTestStack(t)
	ctx := context.Background()

	r := &types.ReflectionEntry{
		Text:          "主人不喜欢编程",
		Entity:        "master",
		Status:        "pending",
		Reinforcement: 0.0,
		Disputation:   3.0,
		CreatedAt:     time.Now().Unix(),
		UpdatedAt:     time.Now().Unix(),
	}
	id, _ := store.SaveReflection(ctx, r)
	r.ID = id

	worker.runPromotionSweep(ctx)

	// disputation 3.0 → score -3.0 → denied
	refs, _ := store.ListReflectionsByStatus(ctx, []string{"denied"}, 1)
	if len(refs) != 1 {
		t.Fatalf("expected 1 denied, got %d", len(refs))
	}
}

func TestPromotionNoChange(t *testing.T) {
	worker, store := newTestStack(t)
	ctx := context.Background()

	r := &types.ReflectionEntry{
		Text:          "主人有时喝咖啡",
		Entity:        "master",
		Status:        "pending",
		Reinforcement: 0.5,
		Disputation:   0.0,
		CreatedAt:     time.Now().Unix(),
		UpdatedAt:     time.Now().Unix(),
	}
	id, _ := store.SaveReflection(ctx, r)
	r.ID = id

	worker.runPromotionSweep(ctx)

	// 0.5 < 1.0 → stays pending
	refs, _ := store.ListReflectionsByStatus(ctx, []string{"pending"}, 1)
	if len(refs) != 1 {
		t.Fatalf("expected 1 pending, got %d", len(refs))
	}
	if refs[0].Status != "pending" {
		t.Errorf("expected pending, got %s", refs[0].Status)
	}
}

func TestContradictionDetection(t *testing.T) {
	worker, store := newTestStack(t)
	ctx := context.Background()

	// Two promoted reflections about same entity with opposing evidence
	a := &types.ReflectionEntry{
		Text: "主人喜欢安静",
		Entity: "master", Status: "promoted",
		Reinforcement: 3.0, Disputation: 0.0,
		CreatedAt: time.Now().Unix(), UpdatedAt: time.Now().Unix(),
	}
	b := &types.ReflectionEntry{
		Text: "主人喜欢热闹",
		Entity: "master", Status: "promoted",
		Reinforcement: 0.0, Disputation: 2.0,
		CreatedAt: time.Now().Unix(), UpdatedAt: time.Now().Unix(),
	}
	aID, _ := store.SaveReflection(ctx, a)
	bID, _ := store.SaveReflection(ctx, b)
	a.ID = aID
	b.ID = bID

	worker.detectContradictions(ctx)

	// Weaker should be denied
	refs, _ := store.ListReflectionsByStatus(ctx, []string{"denied"}, 1)
	if len(refs) != 1 {
		t.Fatalf("expected 1 denied from contradiction, got %d", len(refs))
	}
}

func TestSelfModelStore(t *testing.T) {
	dir := t.TempDir()
	store := NewSelfModelStore(dir)
	ctx := context.Background()

	// First load should return empty defaults
	bundle, err := store.Load(ctx)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if bundle.UserModel != "" {
		t.Error("initial user model should be empty")
	}

	// Save and reload
	bundle.UserModel = "主人是Go后端工程师"
	bundle.SelfModel = "我擅长技术讨论"
	store.Save(ctx, bundle)

	store2 := NewSelfModelStore(dir)
	bundle2, _ := store2.Load(ctx)
	if bundle2.UserModel != "主人是Go后端工程师" {
		t.Errorf("user model not persisted: %q", bundle2.UserModel)
	}
	if bundle2.SelfModel != "我擅长技术讨论" {
		t.Errorf("self model not persisted: %q", bundle2.SelfModel)
	}
	if bundle2.Version != 2 {
		t.Errorf("version should be 2, got %d", bundle2.Version)
	}
}
