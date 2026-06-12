package memory

import (
	"context"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/SHIROH4/sion-plus/internal/domain/memory"
	"github.com/SHIROH4/sion-plus/internal/domain/types"
	"github.com/SHIROH4/sion-plus/internal/port"
)

func newTestRecall(t *testing.T) (*Recall, *SQLiteStore) {
	t.Helper()
	store := newTestStore(t)
	cfg := memory.DefaultEvidenceConfig()
	engine := NewEvidenceEngine(store, cfg)
	recall := NewRecall(store, engine)
	return recall, store
}

func TestHybridSearch(t *testing.T) {
	ctx := context.Background()
	recall, store := newTestRecall(t)

	// Create some facts
	facts := []struct {
		content string
		entity  string
	}{
		{"likes Go programming", "master"},
		{"works with Rust on weekends", "master"},
		{"prefers dark theme in IDE", "master"},
		{"enjoys coffee in the morning", "master"},
		{"plays guitar as a hobby", "master"},
	}
	for _, f := range facts {
		fact := &types.FactEntry{
			Entity:       f.entity,
			RelationType: "preference",
			Content:      f.content,
			SourceTier:   types.SourceExplicit,
			TemporalScope: types.ScopePattern,
			Importance:   5,
			Source:       "chat",
			MemCellType:  "prefer",
			Evidence: types.MemoryEvidenceEntry{
				Reinforcement:    1.0,
				ReinLastSignalAt: time.Now().Unix(),
			},
			CreatedAt: time.Now().Unix(),
		}
		if err := store.SaveFact(ctx, fact); err != nil {
			t.Fatalf("SaveFact: %v", err)
		}
	}

	results, err := recall.HybridSearch(ctx, "Go programming", 3)
	if err != nil {
		t.Fatalf("HybridSearch: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("expected at least 1 result")
	}

	// The Go-related fact should be ranked highest
	t.Logf("top result: %s (score=%.4f)", results[0].Content, results[0].Score)
}

func TestHybridSearchEmpty(t *testing.T) {
	ctx := context.Background()
	recall, _ := newTestRecall(t)

	results, err := recall.HybridSearch(ctx, "nonexistent query xyz123", 3)
	if err != nil {
		t.Fatalf("HybridSearch: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results for nonsense query, got %d", len(results))
	}
}

func TestVectorSearch(t *testing.T) {
	ctx := context.Background()
	recall, store := newTestRecall(t)

	// Create facts with different evidence scores
	f1 := &types.FactEntry{
		Entity: "master", RelationType: "preference", Content: "high score",
		SourceTier: types.SourceExplicit, TemporalScope: types.ScopePattern,
		Importance: 8, Source: "test", MemCellType: "fact",
		Evidence:  types.MemoryEvidenceEntry{Reinforcement: 2.5, ReinLastSignalAt: time.Now().Unix()},
		CreatedAt: time.Now().Unix(),
	}
	f2 := &types.FactEntry{
		Entity: "master", RelationType: "preference", Content: "low score",
		SourceTier: types.SourceExplicit, TemporalScope: types.ScopePattern,
		Importance: 3, Source: "test", MemCellType: "fact",
		Evidence:  types.MemoryEvidenceEntry{Reinforcement: 0.3, ReinLastSignalAt: time.Now().Unix()},
		CreatedAt: time.Now().Unix(),
	}
	store.SaveFact(ctx, f1)
	store.SaveFact(ctx, f2)

	results, err := recall.VectorSearch(ctx, nil, 2)
	if err != nil {
		t.Fatalf("VectorSearch: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	// Higher score should come first
	if results[0].Score < results[1].Score {
		t.Errorf("results not sorted by score: [0]=%f, [1]=%f", results[0].Score, results[1].Score)
	}
}

func TestUnifiedSearch(t *testing.T) {
	ctx := context.Background()
	recall, store := newTestRecall(t)

	fact := &types.FactEntry{
		Entity: "master", RelationType: "identity", Content: "backend engineer",
		SourceTier: types.SourceExplicit, TemporalScope: types.ScopePattern,
		Importance: 7, Source: "test", MemCellType: "fact",
		Evidence:  types.MemoryEvidenceEntry{Reinforcement: 1.0, ReinLastSignalAt: time.Now().Unix()},
		CreatedAt: time.Now().Unix(),
	}
	store.SaveFact(ctx, fact)

	// Text-only
	results, err := recall.UnifiedSearch(ctx, "engineer", nil, 3)
	if err != nil {
		t.Fatalf("UnifiedSearch text: %v", err)
	}
	if len(results) == 0 {
		t.Error("text search should find 'engineer'")
	}

	// Vector-only (fallback to evidence sort)
	results2, err := recall.UnifiedSearch(ctx, "", []float32{0.1, 0.2}, 3)
	if err != nil {
		t.Fatalf("UnifiedSearch vector: %v", err)
	}
	if len(results2) == 0 {
		t.Error("vector search should return fallback results")
	}

	// Empty both → fallback
	results3, err := recall.UnifiedSearch(ctx, "", nil, 3)
	if err != nil {
		t.Fatalf("UnifiedSearch empty: %v", err)
	}
	if len(results3) == 0 {
		t.Error("empty search should return fallback results")
	}
}

func TestRRFFusion(t *testing.T) {
	// Create two lists with some overlap
	bm25 := []types.FactEntry{
		{ID: 1, Content: "A"},
		{ID: 2, Content: "B"},
		{ID: 3, Content: "C"},
	}
	vec := []types.FactEntry{
		{ID: 2, Content: "B"}, // overlap
		{ID: 4, Content: "D"},
		{ID: 3, Content: "C"}, // overlap
	}

	fused := rrfFuse(bm25, vec, 60)
	if len(fused) != 4 {
		t.Fatalf("expected 4 unique entries, got %d", len(fused))
	}

	// ID 2 should rank highly (appears in both lists near top)
	ids := make([]int64, len(fused))
	for i, sf := range fused {
		ids[i] = sf.fact.ID
	}
	t.Logf("RRF order: %v", ids)
}

func TestExplorationQuota(t *testing.T) {
	ctx := context.Background()
	recall, store := newTestRecall(t)

	// Create hot+keyword facts and cold+keyword facts sharing a common FTS5-matchable word
	commonWord := "favorite"
	for i := 0; i < 5; i++ {
		f := &types.FactEntry{
			Entity: "master", RelationType: "preference",
			Content: commonWord + " hot", SourceTier: types.SourceExplicit,
			TemporalScope: types.ScopePattern, Source: "test", MemCellType: "fact",
			RecallCount: 10,
			Evidence:   types.MemoryEvidenceEntry{Reinforcement: 2.0, ReinLastSignalAt: time.Now().Unix()},
			CreatedAt:  time.Now().Unix(),
		}
		store.SaveFact(ctx, f)
	}
	for i := 0; i < 5; i++ {
		f := &types.FactEntry{
			Entity: "master", RelationType: "preference",
			Content: commonWord + " cold", SourceTier: types.SourceExplicit,
			TemporalScope: types.ScopePattern, Source: "test", MemCellType: "fact",
			RecallCount: 0,
			Evidence:   types.MemoryEvidenceEntry{Reinforcement: 2.0, ReinLastSignalAt: time.Now().Unix()},
			CreatedAt:  time.Now().Unix(),
		}
		store.SaveFact(ctx, f)
	}

	results, err := recall.HybridSearch(ctx, commonWord, 5)
	if err != nil {
		t.Fatalf("HybridSearch: %v", err)
	}

	// At least 1 result should come from cold facts (20% * 5 = 1)
	hasHot := false
	hasCold := false
	for _, r := range results {
		if strings.Contains(r.Content, "hot") {
			hasHot = true
		}
		if strings.Contains(r.Content, "cold") {
			hasCold = true
		}
	}
	if !hasHot {
		t.Error("expected at least one hot fact in results")
	}
	if !hasCold {
		t.Error("exploration quota should include at least one cold fact")
	}
}

func TestSearchBoundaries(t *testing.T) {
	ctx := context.Background()
	recall, store := newTestRecall(t)

	store.SaveFact(ctx, &types.FactEntry{
		Entity: "master", RelationType: string(types.RelBoundary), Content: "don't interrupt during meetings",
		SourceTier: types.SourceExplicit, TemporalScope: types.ScopePattern,
		Importance: 9, Source: "chat", MemCellType: "fact",
		Evidence:  types.MemoryEvidenceEntry{Reinforcement: 3.0, ReinLastSignalAt: time.Now().Unix()},
		CreatedAt: time.Now().Unix(),
	})
	store.SaveFact(ctx, &types.FactEntry{
		Entity: "master", RelationType: "preference", Content: "likes tea",
		SourceTier: types.SourceExplicit, TemporalScope: types.ScopePattern,
		Importance: 5, Source: "chat", MemCellType: "prefer",
		Evidence:  types.MemoryEvidenceEntry{Reinforcement: 1.0, ReinLastSignalAt: time.Now().Unix()},
		CreatedAt: time.Now().Unix(),
	})

	results, err := recall.SearchBoundaries(ctx)
	if err != nil {
		t.Fatalf("SearchBoundaries: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 boundary, got %d", len(results))
	}
	if results[0].Content != "don't interrupt during meetings" {
		t.Errorf("unexpected boundary: %s", results[0].Content)
	}
}

func TestSearchWithFilter(t *testing.T) {
	ctx := context.Background()
	recall, store := newTestRecall(t)

	store.SaveFact(ctx, &types.FactEntry{
		Entity: "master", RelationType: "preference", Content: "I said I like Go",
		SourceTier: types.SourceExplicit, TemporalScope: types.ScopePattern,
		Importance: 7, Source: "chat", MemCellType: "fact",
		Evidence:  types.MemoryEvidenceEntry{Reinforcement: 1.5, ReinLastSignalAt: time.Now().Unix()},
		CreatedAt: time.Now().Unix(),
	})
	store.SaveFact(ctx, &types.FactEntry{
		Entity: "master", RelationType: "work_pattern", Content: "observed coding in Go",
		SourceTier: types.SourceObserved, TemporalScope: types.ScopePattern,
		Importance: 5, Source: "perception", MemCellType: "fact",
		Evidence:  types.MemoryEvidenceEntry{Reinforcement: 0.5, ReinLastSignalAt: time.Now().Unix()},
		CreatedAt: time.Now().Unix(),
	})

	// Filter to explicit only
	results, err := recall.SearchWithFilter(ctx, "Go", 5, types.SourceExplicit)
	if err != nil {
		t.Fatalf("SearchWithFilter: %v", err)
	}
	for _, r := range results {
		if r.Content == "observed coding in Go" {
			t.Error("observed fact should be filtered out when tier=explicit")
		}
	}
	// Should find the explicit fact
	found := false
	for _, r := range results {
		if r.Content == "I said I like Go" {
			found = true
		}
	}
	if !found {
		t.Error("explicit fact should be included")
	}
}

func TestSearchDiaries(t *testing.T) {
	ctx := context.Background()
	recall, store := newTestRecall(t)

	store.SaveDiary(ctx, &types.DiaryEntry{
		Title: "boring meeting", Summary: "user attended a long meeting",
		EmotionValence: -0.2, EmotionArousal: -0.1,
	})
	store.SaveDiary(ctx, &types.DiaryEntry{
		Title: "exciting breakthrough", Summary: "user solved a hard bug",
		EmotionValence: 0.8, EmotionArousal: 0.7,
	})

	results, err := recall.SearchDiaries(ctx, "", 2)
	if err != nil {
		t.Fatalf("SearchDiaries: %v", err)
	}
	if len(results) < 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}

	// The high-emotion diary should rank first
	sort.Slice(results, func(i, j int) bool {
		return results[i].Score > results[j].Score
	})
	if results[0].Content != "user solved a hard bug" {
		t.Errorf("high-emotion diary should rank first, got %s", results[0].Content)
	}
}

func TestRecallCompileCheck(t *testing.T) {
	// Compile-time interface check
	var _ port.MemoryRecall = (*Recall)(nil)
}
