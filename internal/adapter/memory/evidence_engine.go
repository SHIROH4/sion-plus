package memory

import (
	"context"
	"math"
	"time"

	"github.com/shirohania/sion/internal/domain/memory"
	"github.com/shirohania/sion/internal/domain/types"
	"github.com/shirohania/sion/internal/port"
)

// EvidenceEngine implements port.EvidenceEngine backed by a MemoryStore.
// Uses the pure math from domain/memory/evidence.go for decay, scoring, and status.
type EvidenceEngine struct {
	store    port.MemoryStore
	cfg      types.EvidenceConfig
	eventLog *EventLog
}

var _ port.EvidenceEngine = (*EvidenceEngine)(nil)

// NewEvidenceEngine creates an engine with the given store and evidence config.
func NewEvidenceEngine(store port.MemoryStore, cfg types.EvidenceConfig) *EvidenceEngine {
	return &EvidenceEngine{store: store, cfg: cfg}
}

// SetEventLog wires an EventLog for audit trail recording.
func (e *EvidenceEngine) SetEventLog(el *EventLog) {
	e.eventLog = el
}

// ── Constants (from design doc §5.2) ────────────────────────────────

const (
	signalHistoryMax = 20  // keep last N signals for oscillation detection
	oscillationFlips = 5   // type-flip threshold to mark oscillating
	oscillationMul   = 0.3 // signal strength multiplier when oscillating
)

// baseReinDelta returns the scaled reinforcement delta for a signal type
// applied to a fact of the given source tier (§5.3).
func baseReinDelta(tier types.FactSourceTier, sigType port.EvidenceSignalType) float64 {
	base := 0.50
	switch tier {
	case types.SourceExplicit:
		base *= 1.0 // 0.50
	case types.SourceObserved:
		base *= 0.5 // 0.25
	case types.SourceInferred:
		base *= 0.3 // 0.15
	}
	if sigType == port.SignalObservation {
		base *= 0.6
	}
	return base
}

// baseDispDelta returns the scaled disputation delta.
func baseDispDelta(tier types.FactSourceTier) float64 {
	switch tier {
	case types.SourceExplicit:
		return 0.50
	case types.SourceObserved:
		return 0.25
	default:
		return 0.15
	}
}

// signalStrength maps signal type to (reinDelta, dispDelta, sourceLabel).
func signalStrength(sig port.EvidenceSignal, tier types.FactSourceTier) (reinDelta, dispDelta float64, sourceLabel string) {
	switch sig.Type {
	case port.SignalUserConfirm:
		return baseReinDelta(tier, sig.Type), 0, "user_confirm"
	case port.SignalUserDeny:
		return 0, baseDispDelta(tier), "user_deny"
	case port.SignalUserFact:
		return baseReinDelta(tier, sig.Type), 0, "user_fact"
	case port.SignalBehaviorMatch:
		return baseReinDelta(tier, sig.Type), 0, "behavior_match"
	case port.SignalContradiction:
		return 0, baseDispDelta(tier), "contradiction"
	case port.SignalObservation:
		return baseReinDelta(tier, sig.Type), 0, "observation"
	default:
		return sig.Strength, 0, string(sig.Type)
	}
}

// ── ApplySignal ─────────────────────────────────────────────────────

func (e *EvidenceEngine) ApplySignal(ctx context.Context, entryID int64, sig port.EvidenceSignal) (*types.EvidenceSnapshot, error) {
	fact, err := e.store.GetFact(ctx, entryID)
	if err != nil {
		return nil, err
	}

	reinDelta, dispDelta, sourceLabel := signalStrength(sig, fact.SourceTier)

	// Oscillation detection from signal history
	e.detectOscillation(fact)

	// Apply oscillation penalty if flagged
	oscPenalty := 1.0
	if fact.Oscillating {
		oscPenalty = oscillationMul
	}

	delta := memory.EvidenceDelta{
		ReinDelta: reinDelta * oscPenalty,
		DispDelta: dispDelta * oscPenalty,
		Source:    sourceLabel,
	}

	now := time.Now()
	newEvidence, snapshot := memory.ApplySignal(fact.Evidence, delta, e.cfg, now)

	// Append signal record to history (keep last N)
	record := types.EvidenceSignalRecord{
		Type:      signalRecordType(reinDelta, dispDelta),
		Delta:     reinDelta + dispDelta,
		Timestamp: now.Unix(),
	}
	newEvidence.SignalHistory = append(newEvidence.SignalHistory, record)
	if len(newEvidence.SignalHistory) > signalHistoryMax {
		newEvidence.SignalHistory = newEvidence.SignalHistory[len(newEvidence.SignalHistory)-signalHistoryMax:]
	}

	// Tick sub-zero counter
	memory.TickSubZero(&newEvidence, now, e.cfg)

	fact.Evidence = newEvidence
	if err := e.store.UpdateFact(ctx, fact); err != nil {
		return nil, err
	}

	// Emit audit events
	if e.eventLog != nil {
		_ = e.eventLog.LogFactSignalApplied(ctx, fact, string(sig.Type), &snapshot)
		_ = e.eventLog.LogEvidenceUpdated(ctx, fact, &snapshot)
	}

	return &snapshot, nil
}

// ── Score / Status ─────────────────────────────────────────────────

func (e *EvidenceEngine) Score(ctx context.Context, entryID int64) (float64, error) {
	fact, err := e.store.GetFact(ctx, entryID)
	if err != nil {
		return 0, err
	}
	return memory.EvidenceScore(fact.Evidence, time.Now(), e.cfg), nil
}

func (e *EvidenceEngine) Status(ctx context.Context, entryID int64) (types.EvidenceStatus, error) {
	score, err := e.Score(ctx, entryID)
	if err != nil {
		return "", err
	}
	return types.EvidenceStatus(memory.DeriveStatus(score, e.cfg)), nil
}

// ── Protect / Unprotect ─────────────────────────────────────────────

func (e *EvidenceEngine) Protect(ctx context.Context, entryID int64) error {
	fact, err := e.store.GetFact(ctx, entryID)
	if err != nil {
		return err
	}
	fact.Evidence.Protected = true
	return e.store.UpdateFact(ctx, fact)
}

func (e *EvidenceEngine) Unprotect(ctx context.Context, entryID int64) error {
	fact, err := e.store.GetFact(ctx, entryID)
	if err != nil {
		return err
	}
	fact.Evidence.Protected = false
	return e.store.UpdateFact(ctx, fact)
}

// ── Archive Sweep ──────────────────────────────────────────────────

func (e *EvidenceEngine) ArchiveSweep(ctx context.Context) (int, error) {
	facts, err := e.store.ListActiveFacts(ctx, 0)
	if err != nil {
		return 0, err
	}

	now := time.Now()
	archived := 0
	for _, f := range facts {
		if memory.ShouldArchive(f.Evidence, now, e.cfg) {
			if err := e.store.ArchiveFact(ctx, f.ID); err != nil {
				return archived, err
			}
			archived++
		}
	}
	return archived, nil
}

// ── Oscillation Detection ──────────────────────────────────────────

// detectOscillation checks the signal history for alternating reinforce/contradict patterns.
// Sets f.Oscillating = true if the last 20 signals flip >= oscillationFlips times.
func (e *EvidenceEngine) detectOscillation(f *types.FactEntry) {
	history := f.Evidence.SignalHistory
	if len(history) < 6 {
		return
	}

	flips := 0
	lastType := ""
	for _, s := range history {
		if s.Type != lastType && lastType != "" {
			flips++
		}
		lastType = s.Type
	}

	if flips >= oscillationFlips {
		f.Oscillating = true
	}
}

// ── Helpers ─────────────────────────────────────────────────────────

func signalRecordType(reinDelta, dispDelta float64) string {
	if dispDelta > 0 && reinDelta == 0 {
		return "contradict"
	}
	return "reinforce"
}

// ── Batch operations ────────────────────────────────────────────────

// ApplySignalBatch applies the same signal type to multiple entries.
// Useful for SignalDetection LLM output: one batch of reinforces + contradicts.
func (e *EvidenceEngine) ApplySignalBatch(ctx context.Context, signals []port.EvidenceSignal) ([]types.EvidenceSnapshot, error) {
	snapshots := make([]types.EvidenceSnapshot, 0, len(signals))
	for _, sig := range signals {
		snap, err := e.ApplySignal(ctx, sig.EntryID, sig)
		if err != nil {
			return snapshots, err
		}
		snapshots = append(snapshots, *snap)
	}
	return snapshots, nil
}

// RecalcScore recomputes and returns the current score without applying a signal.
func (e *EvidenceEngine) RecalcScore(f *types.FactEntry) float64 {
	if f.Evidence.Protected {
		return math.Inf(1)
	}
	return memory.EvidenceScore(f.Evidence, time.Now(), e.cfg)
}

// ScoreSnapshot returns a snapshot for display without side effects.
func (e *EvidenceEngine) ScoreSnapshot(f *types.FactEntry) types.EvidenceSnapshot {
	score := e.RecalcScore(f)
	return types.EvidenceSnapshot{
		Reinforcement: f.Evidence.Reinforcement,
		Disputation:   f.Evidence.Disputation,
		EvidenceScore: score,
		Status:        memory.DeriveStatus(score, e.cfg),
		ComboCount:    f.Evidence.ReinComboCount,
	}
}
