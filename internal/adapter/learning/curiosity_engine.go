package learning

import (
	"context"
	"log"
	"sync"
	"time"

	"github.com/shirohania/sion/internal/domain/cognition"
	"github.com/shirohania/sion/internal/port"
)

// CuriosityEngineImpl implements port.CuriosityEngine.
// Scans for knowledge gaps (fact contradictions, dormant threads,
// incomplete patterns) and schedules exploration actions.
type CuriosityEngineImpl struct {
	mu sync.Mutex

	memoryStore port.MemoryStore
	memoryRecall port.MemoryRecall

	gaps []cognition.KnowledgeGap

	// Cooldown per topic to prevent spam
	lastExplore map[string]int64

	// Configuration
	minExploreInterval time.Duration
	maxGaps            int
}

var _ port.CuriosityEngine = (*CuriosityEngineImpl)(nil)

// NewCuriosityEngine creates a CuriosityEngine.
func NewCuriosityEngine(
	memoryStore port.MemoryStore,
	memoryRecall port.MemoryRecall,
) *CuriosityEngineImpl {
	return &CuriosityEngineImpl{
		memoryStore:        memoryStore,
		memoryRecall:       memoryRecall,
		lastExplore:        make(map[string]int64),
		minExploreInterval: 1 * time.Hour,
		maxGaps:            20,
	}
}

// SetMinExploreInterval sets the minimum time between explorations of the same topic.
func (e *CuriosityEngineImpl) SetMinExploreInterval(d time.Duration) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.minExploreInterval = d
}

// ScanGaps detects knowledge gaps from the current memory state.
// Sources:
//   - fact_contradiction: facts with conflicting evidence signals
//   - thread_dormant: active threads with no updates for >24h
//   - pattern_incomplete: action types with insufficient outcome samples
func (e *CuriosityEngineImpl) ScanGaps(ctx context.Context) ([]port.KnowledgeGap, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	var gaps []cognition.KnowledgeGap
	now := time.Now().Unix()

	// 1. Fact contradictions: facts that have disputation signals
	facts, err := e.memoryStore.ListAllFacts(ctx)
	if err == nil {
		for _, f := range facts {
			score := f.Evidence.Reinforcement - f.Evidence.Disputation
			if f.Evidence.Disputation > 0 && score < 0.3 && !f.Archived {
				gaps = append(gaps, cognition.KnowledgeGap{
					Topic:        f.Content,
					Source:       "fact_contradiction",
					LastExplored: 0,
				})
			}
		}
	}

	// 2. Dormant threads: active threads with no update for >24h
	threads, err := e.memoryStore.ListActiveThreads(ctx)
	if err == nil {
		cutoff := now - 86400 // 24h
		for _, t := range threads {
			if t.UpdatedAt < cutoff {
				gaps = append(gaps, cognition.KnowledgeGap{
					Topic:        t.Goal,
					Source:       "thread_dormant",
					LastExplored: t.UpdatedAt,
				})
			}
		}
	}

	// 3. Incomplete patterns: check outcome coverage
	outcomes, err := e.memoryStore.QueryOutcomes(ctx, port.OutcomeFilter{Limit: 100})
	if err == nil {
		actionCounts := make(map[string]int)
		for _, o := range outcomes {
			actionCounts[o.ActionType]++
		}
		// Actions with <5 samples are "incomplete patterns"
		allActions := []string{
			"speak_casual", "speak_care", "speak_inquiry",
			"search", "observe", "reflect", "analyze_patterns",
			"care_rest", "care_meal", "care_hydration",
		}
		for _, action := range allActions {
			if actionCounts[action] < 5 {
				gaps = append(gaps, cognition.KnowledgeGap{
					Topic:        "action_pattern:" + action,
					Source:       "pattern_incomplete",
					LastExplored: 0,
				})
			}
		}
	}

	// Score and deduplicate
	scored := cognition.ScoreKnowledgeGaps(gaps, now)

	// Truncate
	if len(scored) > e.maxGaps {
		scored = scored[:e.maxGaps]
	}

	e.gaps = scored

	// Convert to port type
	result := make([]port.KnowledgeGap, len(scored))
	for i, g := range scored {
		result[i] = port.KnowledgeGap{
			Topic:        g.Topic,
			Source:       g.Source,
			Priority:     g.Priority,
			LastExplored: g.LastExplored,
		}
	}

	log.Printf("[CuriosityEngine] scan: %d gaps found (%d contradictions, tracked)", len(result), countBySource(scored, "fact_contradiction"))
	return result, nil
}

// ShouldExplore checks if a gap is worth exploring now.
// Criteria: priority > 0.3 AND cooldown elapsed.
func (e *CuriosityEngineImpl) ShouldExplore(gap port.KnowledgeGap) bool {
	e.mu.Lock()
	defer e.mu.Unlock()

	if gap.Priority < 0.3 {
		return false
	}

	last, ok := e.lastExplore[gap.Topic]
	if ok && time.Since(time.Unix(last, 0)) < e.minExploreInterval {
		return false
	}

	return true
}

// ExecuteExploration performs an exploration action for a knowledge gap.
// Schedules a "search" or "observe" action depending on the gap source.
func (e *CuriosityEngineImpl) ExecuteExploration(ctx context.Context, gap port.KnowledgeGap) error {
	e.mu.Lock()
	e.lastExplore[gap.Topic] = time.Now().Unix()
	e.mu.Unlock()

	// For fact contradictions: try to search for clarification
	// For dormant threads: re-engage with a follow-up
	// For incomplete patterns: observe more
	switch gap.Source {
	case "fact_contradiction":
		log.Printf("[CuriosityEngine] exploring contradiction: %s", gap.Topic)
		// Search memory for related facts to resolve contradiction
		if e.memoryRecall != nil {
			_, _ = e.memoryRecall.HybridSearch(ctx, gap.Topic, 5)
		}
	case "thread_dormant":
		log.Printf("[CuriosityEngine] re-engaging dormant thread: %s", gap.Topic)
		// The thread will be picked up by the proactive system
	case "pattern_incomplete":
		log.Printf("[CuriosityEngine] observing incomplete pattern: %s", gap.Topic)
		// More observation cycles will fill in the pattern
	}

	return nil
}

func countBySource(gaps []cognition.KnowledgeGap, source string) int {
	count := 0
	for _, g := range gaps {
		if g.Source == source {
			count++
		}
	}
	return count
}
