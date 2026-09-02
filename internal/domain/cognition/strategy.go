package cognition

import (
	"sort"
	"time"

	"github.com/SHIROH4/sion-plus/internal/domain/types"
)

// ── Strategy Reflection Domain Logic ──
//
// Pure functions for determining when strategic reflection should run,
// and for distilling outcome patterns into strategy recommendations.

// StrategyConfig holds the configuration for strategy reflection scheduling.
type StrategyConfig struct {
	MinIntervalHours    float64 // minimum hours between reflections, default 6
	MaxIntervalHours    float64 // maximum hours before forced reflection, default 24
	BusyThreshold       int     // interactions needed to trigger busy reflection, default 20
	OutcomeThreshold    int     // new outcomes needed since last reflection, default 10
	EmotionSpikeTrigger float64 // emotion intensity that triggers early reflection, default 0.8
}

// DefaultStrategyConfig returns sensible defaults.
func DefaultStrategyConfig() StrategyConfig {
	return StrategyConfig{
		MinIntervalHours:    6,
		MaxIntervalHours:    24,
		BusyThreshold:       20,
		OutcomeThreshold:    10,
		EmotionSpikeTrigger: 0.8,
	}
}

// ShouldReflect determines whether strategic reflection should run now.
func ShouldReflect(cfg StrategyConfig, lastRunAt int64, interactionCount int, newOutcomeCount int, maxEmotionIntensity float64) (bool, string) {
	now := time.Now().Unix()
	hoursSince := float64(now-lastRunAt) / 3600.0

	// Hard gate: must be at least MinInterval
	if hoursSince < cfg.MinIntervalHours {
		return false, "too_soon"
	}

	// Force gate: must run if past MaxInterval
	if hoursSince >= cfg.MaxIntervalHours {
		return true, "max_interval"
	}

	// Busy trigger: enough interactions since last reflection
	if interactionCount >= cfg.BusyThreshold {
		return true, "busy"
	}

	// Outcome trigger: enough new outcomes to learn from
	if newOutcomeCount >= cfg.OutcomeThreshold {
		return true, "outcomes"
	}

	// Emotion spike trigger: strong emotional event occurred
	if maxEmotionIntensity >= cfg.EmotionSpikeTrigger {
		return true, "emotion_spike"
	}

	return false, ""
}

// ── Outcome Pattern Analysis ──

// OutcomePattern describes a discovered pattern in action outcomes.
type OutcomePattern struct {
	Action     string  `json:"action"`
	TimeBucket string  `json:"time_bucket"` // "morning"|"afternoon"|"evening"|"night"
	AcceptRate float64 `json:"accept_rate"`
	Count      int     `json:"count"`
}

// AnalyzeOutcomePatterns groups outcomes by action × time bucket and
// computes acceptance rates. Returns patterns sorted by count descending.
func AnalyzeOutcomePatterns(outcomes []types.ActionOutcome) []OutcomePattern {
	type key struct {
		action string
		bucket string
	}
	groups := make(map[key]struct {
		total    int
		accepted int
	})

	for _, o := range outcomes {
		bucket := timeBucket(o.HourOfDay)
		k := key{action: o.ActionType, bucket: bucket}
		g := groups[k]
		g.total++
		if o.Outcome == types.OutcomeEngaged || o.Outcome == types.OutcomeReplied {
			g.accepted++
		}
		groups[k] = g
	}

	var patterns []OutcomePattern
	for k, g := range groups {
		rate := float64(0)
		if g.total > 0 {
			rate = float64(g.accepted) / float64(g.total)
		}
		patterns = append(patterns, OutcomePattern{
			Action:     k.action,
			TimeBucket: k.bucket,
			AcceptRate: rate,
			Count:      g.total,
		})
	}

	sort.Slice(patterns, func(i, j int) bool {
		return patterns[i].Count > patterns[j].Count
	})
	return patterns
}

// timeBucket maps hour of day to a named bucket.
func timeBucket(hour int) string {
	switch {
	case hour >= 6 && hour < 12:
		return "morning"
	case hour >= 12 && hour < 18:
		return "afternoon"
	case hour >= 18 && hour < 22:
		return "evening"
	default:
		return "night"
	}
}

// ── Stuck Action Detection ──

// StuckActionInfo describes an action that may be stuck in a loop.
type StuckActionInfo struct {
	Action           string  `json:"action"`
	ConsecutiveCount int     `json:"consecutive_count"`
	AvgReward        float64 `json:"avg_reward"`
}

// DetectStuckActions finds actions that have been repeated consecutively
// with diminishing returns. Returns stuck actions sorted by severity.
func DetectStuckActions(records []DriveRecord) []StuckActionInfo {
	if len(records) < 3 {
		return nil
	}

	// Sort by timestamp descending for proper consecutive analysis
	sorted := make([]DriveRecord, len(records))
	copy(sorted, records)
	sortRecordsDesc(sorted)

	// Count consecutive same-action runs
	type runInfo struct {
		action string
		count  int
		reward float64
	}
	var runs []runInfo
	current := runInfo{action: sorted[0].Action, count: 1, reward: sorted[0].Reward}

	for i := 1; i < len(sorted); i++ {
		if sorted[i].Action == current.action {
			current.count++
			current.reward += sorted[i].Reward
		} else {
			runs = append(runs, current)
			current = runInfo{action: sorted[i].Action, count: 1, reward: sorted[i].Reward}
		}
	}
	runs = append(runs, current)

	var stuck []StuckActionInfo
	for _, r := range runs {
		if r.count >= 3 {
			stuck = append(stuck, StuckActionInfo{
				Action:           r.action,
				ConsecutiveCount: r.count,
				AvgReward:        r.reward / float64(r.count),
			})
		}
	}

	sort.Slice(stuck, func(i, j int) bool {
		return stuck[i].ConsecutiveCount > stuck[j].ConsecutiveCount
	})
	return stuck
}

// ── Knowledge Gap Scoring ──

// KnowledgeGap represents a detected gap in the AI's understanding.
type KnowledgeGap struct {
	Topic        string  `json:"topic"`
	Source       string  `json:"source"`
	Priority     float64 `json:"priority"`
	LastExplored int64   `json:"last_explored"`
}

// ScoreKnowledgeGaps takes a list of knowledge gaps and scores them
// for exploration priority. Factors: staleness, topic relevance, source type.
func ScoreKnowledgeGaps(gaps []KnowledgeGap, now int64) []KnowledgeGap {
	for i := range gaps {
		hoursSince := float64(now-gaps[i].LastExplored) / 3600.0
		if gaps[i].LastExplored == 0 {
			hoursSince = 168 // 1 week for never-explored gaps
		}

		// Base priority from source type
		sourceWeight := map[string]float64{
			"fact_contradiction": 0.9,
			"thread_dormant":     0.7,
			"pattern_incomplete": 0.5,
		}[gaps[i].Source]
		if sourceWeight == 0 {
			sourceWeight = 0.3
		}

		// Staleness boost: older gaps get higher priority
		stalenessBoost := mathMin(1.0, hoursSince/72.0)

		gaps[i].Priority = sourceWeight*0.6 + stalenessBoost*0.4
	}

	sort.Slice(gaps, func(i, j int) bool {
		return gaps[i].Priority > gaps[j].Priority
	})
	return gaps
}

func mathMin(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}
