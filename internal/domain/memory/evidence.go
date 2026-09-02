package memory

import (
	"math"
	"time"

	"github.com/SHIROH4/sion-plus/internal/domain/types"
)

// ── Defaults (overridable via types.EvidenceConfig) ──

const (
	DefaultReinHalfLifeDays   = 60.0
	DefaultDispHalfLifeDays   = 14.0
	DefaultConfirmedThreshold = 1.0
	DefaultPromotedThreshold  = 2.0
	DefaultArchiveThreshold   = 0.0
	DefaultComboThreshold     = 3
	DefaultComboBonus         = 0.5
	DefaultBaseReinDelta      = 0.5
)

// DefaultEvidenceConfig returns the built-in defaults as a types.EvidenceConfig.
func DefaultEvidenceConfig() types.EvidenceConfig {
	return types.EvidenceConfig{
		ReinHalfLifeDays:   DefaultReinHalfLifeDays,
		DispHalfLifeDays:   DefaultDispHalfLifeDays,
		ConfirmedThreshold: DefaultConfirmedThreshold,
		PromotedThreshold:  DefaultPromotedThreshold,
		ArchiveThreshold:   DefaultArchiveThreshold,
		ComboThreshold:     DefaultComboThreshold,
		ComboBonus:         DefaultComboBonus,
		BaseReinDelta:      DefaultBaseReinDelta,
	}
}

// ── Core Functions ──

// EffectiveReinforcement computes decayed reinforcement at now.
//
//	rein * 0.5^(age_days / half_life_days)
func EffectiveReinforcement(entry types.MemoryEvidenceEntry, now time.Time, halfLifeDays float64) float64 {
	r := entry.Reinforcement
	if r <= 0 {
		return 0
	}
	t := time.Unix(entry.ReinLastSignalAt, 0)
	age := now.Sub(t).Hours() / 24
	if age <= 0 {
		return r
	}
	return r * math.Pow(0.5, age/halfLifeDays)
}

// EffectiveDisputation computes decayed disputation at now.
func EffectiveDisputation(entry types.MemoryEvidenceEntry, now time.Time, halfLifeDays float64) float64 {
	d := entry.Disputation
	if d <= 0 {
		return 0
	}
	t := time.Unix(entry.DispLastSignalAt, 0)
	age := now.Sub(t).Hours() / 24
	if age <= 0 {
		return d
	}
	return d * math.Pow(0.5, age/halfLifeDays)
}

// EvidenceScore is net credibility = effective_rein - effective_disp.
// Protected entries return +inf.
func EvidenceScore(entry types.MemoryEvidenceEntry, now time.Time, cfg types.EvidenceConfig) float64 {
	if entry.Protected {
		return math.Inf(1)
	}
	return EffectiveReinforcement(entry, now, cfg.ReinHalfLifeDays) -
		EffectiveDisputation(entry, now, cfg.DispHalfLifeDays)
}

// DeriveStatus maps evidence score to a status tier.
func DeriveStatus(score float64, cfg types.EvidenceConfig) string {
	if math.IsInf(score, 1) || score >= cfg.PromotedThreshold {
		return "promoted"
	}
	if score >= cfg.ConfirmedThreshold {
		return "confirmed"
	}
	if score <= cfg.ArchiveThreshold {
		return "archive_candidate"
	}
	return "pending"
}

// ── Signal Application ──

// EvidenceDelta describes a change to evidence values.
type EvidenceDelta struct {
	ReinDelta float64 // positive=strengthen, negative=weaken
	DispDelta float64 // always non-negative
	Source    string  // "user_fact"|"user_confirm"|"user_deny"|"contradiction"|"observation"
}

// ApplySignal computes the new evidence state after a signal delta.
// EvidenceSnapshot is defined in types/memory_entry.go, shared with port layer.
func ApplySignal(
	entry types.MemoryEvidenceEntry,
	delta EvidenceDelta,
	cfg types.EvidenceConfig,
	now time.Time,
) (types.MemoryEvidenceEntry, types.EvidenceSnapshot) {

	if delta.ReinDelta != 0 {
		entry.Reinforcement += delta.ReinDelta
		entry.ReinLastSignalAt = now.Unix()
	}
	if delta.DispDelta > 0 {
		entry.Disputation = math.Max(0, entry.Disputation+delta.DispDelta)
		entry.DispLastSignalAt = now.Unix()
	}

	// Combo: consecutive user_fact reinforces get bonus
	if delta.Source == "user_fact" && delta.ReinDelta > 0 {
		entry.ReinComboCount++
		if entry.ReinComboCount > cfg.ComboThreshold {
			entry.Reinforcement += cfg.ComboBonus
		}
	}

	score := EvidenceScore(entry, now, cfg)
	status := DeriveStatus(score, cfg)

	snapshot := types.EvidenceSnapshot{
		Reinforcement: entry.Reinforcement,
		Disputation:   entry.Disputation,
		EvidenceScore: score,
		Status:        status,
		ComboCount:    entry.ReinComboCount,
	}

	return entry, snapshot
}

// ── Archive Logic ──

// ShouldArchive checks if an entry should be archived.
func ShouldArchive(entry types.MemoryEvidenceEntry, now time.Time, cfg types.EvidenceConfig) bool {
	if entry.Protected {
		return false
	}
	score := EvidenceScore(entry, now, cfg)
	return score <= cfg.ArchiveThreshold && entry.SubZeroDays >= 7
}

// TickSubZero increments sub_zero_days if score < 0 today. Returns true if incremented.
func TickSubZero(entry *types.MemoryEvidenceEntry, now time.Time, cfg types.EvidenceConfig) bool {
	if entry.Protected {
		return false
	}
	score := EvidenceScore(*entry, now, cfg)
	if score >= 0 {
		return false
	}
	today := now.Format("2006-01-02")
	if entry.SubZeroLastIncrDate == today {
		return false
	}
	entry.SubZeroDays++
	entry.SubZeroLastIncrDate = today
	return true
}

// ── Importance → Initial Reinforcement ──

var importanceToInitialRein = []struct {
	threshold int
	seed      float64
}{
	{10, 0.8},
	{9, 0.6},
	{8, 0.4},
	{7, 0.2},
}

// InitialReinforcement returns the starting reinforcement based on fact importance.
func InitialReinforcement(importance int) float64 {
	for _, t := range importanceToInitialRein {
		if importance >= t.threshold {
			return t.seed
		}
	}
	return 0.0
}
