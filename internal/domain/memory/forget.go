package memory

import "time"

// ── Ebbinghaus Forgetting Curve ──
//
// Simplified model: retention = importance_factor × recall_boost × recency_decay
//
//	importance_factor = importance / 10.0
//	recall_boost       = 1.0 + recall_count * 0.15
//	recency_decay      = 0.5 ^ (days_since_last_recall / half_life_days)
//
// A fact is "forgotten" when its decay weight falls below the active threshold.

const (
	ActiveThreshold       = 0.05
	CoreThreshold         = 0.4
	DefaultHalfLifeDays   = 30.0
	DefaultBoostPerRecall = 0.15
)

// DecayWeight computes the retention score for a fact.
// Higher = more important / recently used / frequently accessed.
func DecayWeight(
	importance int,
	lastRecalledAt int64,
	recallCount int,
	halfLifeDays float64,
	boostPerRecall float64,
) float64 {
	if halfLifeDays <= 0 {
		halfLifeDays = DefaultHalfLifeDays
	}
	if boostPerRecall <= 0 {
		boostPerRecall = DefaultBoostPerRecall
	}

	impFactor := float64(importance) / 10.0
	if impFactor <= 0 {
		impFactor = 0.1
	}

	recallBoost := 1.0 + float64(recallCount)*boostPerRecall

	now := time.Now()
	lastRecall := time.Unix(lastRecalledAt, 0)
	daysSince := now.Sub(lastRecall).Hours() / 24
	if daysSince < 0 {
		daysSince = 0
	}
	recencyDecay := 1.0
	if halfLifeDays > 0 && daysSince > 0 {
		recencyDecay = 0.5 * (daysSince / halfLifeDays) // simplified linear approximation
		if recencyDecay < 0.01 {
			recencyDecay = 0.01
		}
	}

	return impFactor * recallBoost * recencyDecay
}

// ShouldArchiveByDecay checks whether a fact's decay weight is below the active threshold.
func ShouldArchiveByDecay(importance int, lastRecalledAt int64, recallCount int) bool {
	return DecayWeight(importance, lastRecalledAt, recallCount, DefaultHalfLifeDays, DefaultBoostPerRecall) < ActiveThreshold
}
