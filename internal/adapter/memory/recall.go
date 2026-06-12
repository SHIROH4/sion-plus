package memory

import (
	"context"
	"math"
	"math/rand"
	"sort"

	"github.com/shirohania/sion/internal/domain/types"
	"github.com/shirohania/sion/internal/port"
)

// Recall implements port.MemoryRecall using BM25 (FTS5) + vector cosine + RRF fusion.
// Falls back gracefully: no embedding service → BM25 only → evidence_score only.
type Recall struct {
	store     port.MemoryStore
	evidence  *EvidenceEngine
	embedding port.EmbeddingService

	// Mood-congruent recall (v2.1): set by app layer before each search.
	// Positive → boost joy-tagged facts. Negative → boost sadness-tagged facts.
	MoodBias float64
}

var _ port.MemoryRecall = (*Recall)(nil)

// NewRecall creates a hybrid recall engine.
func NewRecall(store port.MemoryStore, evidence *EvidenceEngine) *Recall {
	return &Recall{store: store, evidence: evidence}
}

// SetEmbeddingService wires an embedding service for vector search.
func (r *Recall) SetEmbeddingService(emb port.EmbeddingService) {
	r.embedding = emb
}

// SetMoodBias sets the mood-congruent recall bias from current emotion state.
// Called by the app layer before HybridSearch. val=current PAD valence.
func (r *Recall) SetMoodBias(valence float64) {
	r.MoodBias = valence
}

// ── Constants ───────────────────────────────────────────────────────

const (
	bm25Budget   = 100 // FTS5 candidates
	cosineBudget = 80  // vector candidates (when available)
	rrfK         = 60  // RRF smoothing constant
	exploreRatio = 0.2 // fraction of results reserved for cold facts
	finalBudget  = 5   // final result count for system prompt injection
)

// ── HybridSearch ─────────────────────────────────────────────────────

func (r *Recall) HybridSearch(ctx context.Context, query string, topK int) ([]port.MemorySearchResult, error) {
	if topK <= 0 {
		topK = finalBudget
	}

	// Stage 1: BM25 via FTS5
	bm25Results, _ := r.store.SearchFacts(ctx, query, bm25Budget)

	// Stage 2: Vector cosine (empty if embedding service unavailable)
	vectorResults := r.vectorSearch(ctx, query)

	// Stage 3: RRF fusion
	fused := rrfFuse(bm25Results, vectorResults, rrfK)

	// Stage 4: Sort by fused score, apply exploration quota
	results := r.selectWithExploration(fused, topK)

	return r.annotate(ctx, results), nil
}

// ── VectorSearch ─────────────────────────────────────────────────────

func (r *Recall) VectorSearch(ctx context.Context, vector []float32, topK int) ([]port.MemorySearchResult, error) {
	if topK <= 0 {
		topK = finalBudget
	}

	// Fallback: use evidence_score until sqlite-vec is wired
	active, err := r.store.ListActiveFacts(ctx, 0)
	if err != nil {
		return nil, err
	}

	// Sort by evidence score (via the evidence engine if available)
	sort.Slice(active, func(i, j int) bool {
		return r.scoreFact(&active[i]) > r.scoreFact(&active[j])
	})

	if len(active) > topK {
		active = active[:topK]
	}

	return r.annotate(ctx, active), nil
}

// ── UnifiedSearch ────────────────────────────────────────────────────

func (r *Recall) UnifiedSearch(ctx context.Context, query string, vector []float32, topK int) ([]port.MemorySearchResult, error) {
	// When both text and vector are available, prefer HybridSearch
	// which already handles text-only gracefully
	if query != "" {
		return r.HybridSearch(ctx, query, topK)
	}
	if len(vector) > 0 {
		return r.VectorSearch(ctx, vector, topK)
	}
	// Total fallback: return recent active facts
	return r.VectorSearch(ctx, nil, topK)
}

// ── RRF Fusion ──────────────────────────────────────────────────────

type scoredFact struct {
	fact  types.FactEntry
	score float64
}

func rrfFuse(bm25, vector []types.FactEntry, k int) []scoredFact {
	scores := make(map[int64]float64)
	kf := float64(k)

	// BM25 ranking
	for i, f := range bm25 {
		rank := float64(i + 1)
		scores[f.ID] += 1.0 / (kf + rank)
	}

	// Vector ranking
	for i, f := range vector {
		rank := float64(i + 1)
		scores[f.ID] += 1.0 / (kf + rank)
	}

	// Collect and sort
	result := make([]scoredFact, 0, len(scores))
	seen := make(map[int64]bool)
	for _, f := range bm25 {
		if !seen[f.ID] {
			seen[f.ID] = true
			result = append(result, scoredFact{f, scores[f.ID]})
		}
	}
	for _, f := range vector {
		if !seen[f.ID] {
			seen[f.ID] = true
			result = append(result, scoredFact{f, scores[f.ID]})
		}
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].score > result[j].score
	})

	return result
}

// ── Exploration quota ────────────────────────────────────────────────

// selectWithExploration reserves exploreRatio of the budget for facts
// that have never been recalled (recall_count=0, not archived).
func (r *Recall) selectWithExploration(fused []scoredFact, topK int) []types.FactEntry {
	exploreSlots := int(math.Ceil(float64(topK) * exploreRatio))
	mainSlots := topK - exploreSlots

	// Split cold vs hot
	var hot, cold []scoredFact
	for _, sf := range fused {
		if sf.fact.RecallCount == 0 && !sf.fact.Archived {
			cold = append(cold, sf)
		} else {
			hot = append(hot, sf)
		}
	}

	// Random shuffle cold facts, take up to exploreSlots
	rand.Shuffle(len(cold), func(i, j int) { cold[i], cold[j] = cold[j], cold[i] })

	result := make([]types.FactEntry, 0, topK)

	// Take main slots from hot
	for i := 0; i < mainSlots && i < len(hot); i++ {
		result = append(result, hot[i].fact)
	}

	// Take explore slots from cold
	for i := 0; i < exploreSlots && i < len(cold); i++ {
		result = append(result, cold[i].fact)
	}

	// Fill remaining from hot if cold is insufficient
	hotIdx := mainSlots
	for len(result) < topK && hotIdx < len(hot) {
		result = append(result, hot[hotIdx].fact)
		hotIdx++
	}

	return result
}

// ── Annotation ──────────────────────────────────────────────────────

func (r *Recall) annotate(ctx context.Context, facts []types.FactEntry) []port.MemorySearchResult {
	results := make([]port.MemorySearchResult, 0, len(facts))
	for _, f := range facts {
		res := port.MemorySearchResult{
			ID:      f.ID,
			Content: f.Content,
			Source:  "fact",
			Score:   r.scoreFact(&f),
		}
		if r.evidence != nil {
			snap := r.evidence.ScoreSnapshot(&f)
			res.Evidence = &snap
		}
		results = append(results, res)

		// Update recall stats (best-effort)
		f.RecallCount++
		_ = r.store.UpdateFact(ctx, &f)
	}
	return results
}

// ── Helpers ──────────────────────────────────────────────────────────

// vectorSearch performs cosine similarity search against all active facts with vectors.
// Returns up to cosineBudget results sorted by similarity.
func (r *Recall) vectorSearch(ctx context.Context, query string) []types.FactEntry {
	if r.embedding == nil || !r.embedding.IsAvailable() {
		return nil
	}

	queryVec, err := r.embedding.Vectorize(ctx, query)
	if err != nil || len(queryVec) == 0 {
		return nil
	}

	allFacts, err := r.store.ListActiveFacts(ctx, 0)
	if err != nil {
		return nil
	}

	type scored struct {
		fact  types.FactEntry
		score float64
	}
	var results []scored
	for _, f := range allFacts {
		if len(f.Vector) == 0 {
			continue
		}
		sim := cosineSimilarity(queryVec, f.Vector)
		if sim > 0 {
			results = append(results, scored{f, sim})
		}
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].score > results[j].score
	})

	if len(results) > cosineBudget {
		results = results[:cosineBudget]
	}

	out := make([]types.FactEntry, len(results))
	for i, r := range results {
		out[i] = r.fact
	}
	return out
}

// cosineSimilarity computes cosine similarity between two float32 vectors.
func cosineSimilarity(a, b []float32) float64 {
	if len(a) != len(b) || len(a) == 0 {
		return 0
	}
	var dot, normA, normB float64
	for i := range a {
		dot += float64(a[i]) * float64(b[i])
		normA += float64(a[i]) * float64(a[i])
		normB += float64(b[i]) * float64(b[i])
	}
	if normA == 0 || normB == 0 {
		return 0
	}
	return dot / (math.Sqrt(normA) * math.Sqrt(normB))
}

func (r *Recall) scoreFact(f *types.FactEntry) float64 {
	base := f.Evidence.Reinforcement
	if r.evidence != nil {
		base = r.evidence.RecalcScore(f)
	}
	// v2.1: mood-congruent bias. Positive mood → boost positive facts.
	// Negative mood → boost emotional/boundary facts (user needs support).
	if r.MoodBias != 0 {
		switch {
		case r.MoodBias > 0.2 && (f.RelationType == "preference" || f.RelationType == "habit"):
			base *= 1.15 // 15% boost: happy user → more personal facts
		case r.MoodBias < -0.2 && (f.RelationType == "emotional" || f.RelationType == "boundary"):
			base *= 1.15 // 15% boost: sad user → more emotional context
		}
		base += r.MoodBias * 0.1 // small universal shift
	}
	return base
}

// ── Boundary recall ─────────────────────────────────────────────────

// SearchBoundaries returns all active boundary facts sorted by evidence score.
// Boundaries bypass suppression and are always injected into context.
func (r *Recall) SearchBoundaries(ctx context.Context) ([]port.MemorySearchResult, error) {
	facts, err := r.store.ListActiveFacts(ctx, 0)
	if err != nil {
		return nil, err
	}

	var boundaries []types.FactEntry
	for _, f := range facts {
		if f.RelationType == string(types.RelBoundary) {
			boundaries = append(boundaries, f)
		}
	}

	sort.Slice(boundaries, func(i, j int) bool {
		return r.scoreFact(&boundaries[i]) > r.scoreFact(&boundaries[j])
	})

	return r.annotate(ctx, boundaries), nil
}

// ── Source-filtered recall (v2.0) ───────────────────────────────────

// SearchWithFilter runs hybrid search with a source/evidence filter.
func (r *Recall) SearchWithFilter(ctx context.Context, query string, topK int, tier types.FactSourceTier) ([]port.MemorySearchResult, error) {
	if topK <= 0 {
		topK = finalBudget
	}

	// Get BM25 results
	all, _ := r.store.SearchFacts(ctx, query, bm25Budget*2)

	// Filter by source tier
	var filtered []types.FactEntry
	for _, f := range all {
		if tier == "" || f.SourceTier == tier {
			filtered = append(filtered, f)
		}
	}

	// Fuse with empty vector (BM25 only) and select
	fused := rrfFuse(filtered, nil, rrfK)

	if len(fused) > topK {
		fused = fused[:topK]
	}

	results := make([]types.FactEntry, len(fused))
	for i, sf := range fused {
		results[i] = sf.fact
	}

	return r.annotate(ctx, results), nil
}

// ── Diary search ─────────────────────────────────────────────────────

// SearchDiaries returns the top-K most emotionally significant diary entries.
func (r *Recall) SearchDiaries(ctx context.Context, query string, topK int) ([]port.MemorySearchResult, error) {
	if topK <= 0 {
		topK = 3
	}

	diaries, err := r.store.ListDiaries(ctx, topK*2)
	if err != nil {
		return nil, err
	}

	results := make([]port.MemorySearchResult, 0, len(diaries))
	for _, d := range diaries {
		// Score by emotional intensity (absolute arousal)
		score := math.Abs(d.EmotionArousal) + math.Abs(d.EmotionValence)*0.5
		results = append(results, port.MemorySearchResult{
			ID:      d.ID,
			Content: d.Summary,
			Source:  "diary",
			Score:   score,
		})
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].Score > results[j].Score
	})

	if len(results) > topK {
		results = results[:topK]
	}

	return results, nil
}
