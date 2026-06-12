package memory

import (
	"context"
	"fmt"
	"log"
	"math"

	"github.com/SHIROH4/sion-plus/internal/domain/types"
	"github.com/SHIROH4/sion-plus/internal/port"
)

// ── Promotion Sweep ─────────────────────────────────────────────────

// runPromotionSweep scans all non-archived reflections, computes evidence scores,
// and auto-promotes those that meet thresholds. Called from maintenance loop.
func (w *MemoryWorker) runPromotionSweep(ctx context.Context) {
	refs, err := w.store.ListReflectionsByStatus(ctx, []string{"pending"}, 0)
	if err != nil {
		return
	}

	// Also include confirmed that might be ready for promoted
	confirmed, _ := w.store.ListReflectionsByStatus(ctx, []string{"confirmed"}, 0)
	refs = append(refs, confirmed...)

	if len(refs) == 0 {
		return
	}

	promoted := 0
	confirmedCount := 0
	deniedCount := 0

	for _, r := range refs {
		score := reflectionScore(&r)

		var newStatus string
		switch {
		case score >= 2.0 && r.Status != "promoted":
			newStatus = "promoted"
			promoted++
		case score >= 1.0 && r.Status == "pending":
			newStatus = "confirmed"
			confirmedCount++
		case score <= -2.0 && r.Status != "denied":
			newStatus = "denied"
			deniedCount++
		default:
			continue
		}

		if err := w.transitionReflection(ctx, r.ID, r.Status, newStatus,
			fmt.Sprintf("auto-promote: evidence_score=%.2f", score)); err != nil {
			log.Printf("[ReflectionLifecycle] transition failed: %v", err)
			continue
		}

		// Promoted reflections → trigger identity build
		if newStatus == "promoted" {
			w.onReflectionPromoted(ctx, &r)
		}
	}

	if promoted+confirmedCount+deniedCount > 0 {
		log.Printf("[ReflectionLifecycle] sweep: %d promoted, %d confirmed, %d denied (scanned %d)",
			promoted, confirmedCount, deniedCount, len(refs))
	}

	// Check promoted reflections for stale source facts
	w.checkSourceFactStaleness(ctx)
}

// onReflectionPromoted is called when a reflection reaches "promoted" status.
func (w *MemoryWorker) onReflectionPromoted(ctx context.Context, r *types.ReflectionEntry) {
	if r.AbsorbedInto > 0 {
		return // already absorbed
	}

	// Mark source facts as absorbed into this reflection
	if len(r.SourceFactIDs) > 0 {
		if err := w.store.MarkFactsAbsorbed(ctx, r.SourceFactIDs); err != nil {
			log.Printf("[ReflectionLifecycle] MarkFactsAbsorbed: %v", err)
		}
	}

	log.Printf("[ReflectionLifecycle] promoted: %q (entity=%s, score=%.2f)",
		truncate(r.Text, 80), r.Entity, evidenceScore(r))
}

// checkSourceFactStaleness verifies that promoted reflections still have valid
// source facts. If source facts were later disputed/archived, the reflection's
// evidence is reduced and may be demoted. This closes the data consistency gap
// between the evidence engine and the reflection lifecycle.
func (w *MemoryWorker) checkSourceFactStaleness(ctx context.Context) {
	promoted, err := w.store.ListReflectionsByStatus(ctx, []string{"promoted"}, 0)
	if err != nil || len(promoted) == 0 {
		return
	}

	demoted := 0
	for _, r := range promoted {
		if len(r.SourceFactIDs) == 0 {
			continue
		}

		// Check each source fact's current evidence
		stale := false
		for _, fid := range r.SourceFactIDs {
			fact, err := w.store.GetFact(ctx, fid)
			if err != nil {
				continue
			}
			// If source fact was archived or denied, it's stale
			if fact.Archived {
				stale = true
				break
			}
			// If source fact has high disputation, it's unreliable
			if fact.Evidence.Disputation > fact.Evidence.Reinforcement {
				stale = true
				break
			}
		}

		if stale {
			// Reduce evidence: subtract 1.0 from reinforcement
			newScore := r.Reinforcement - r.Disputation - 1.0
			newStatus := r.Status
			if newScore < 1.0 {
				newStatus = "pending" // demote back to pending
			}
			if err := w.transitionReflection(ctx, r.ID, r.Status, newStatus,
				"source fact staleness detected"); err == nil {
				demoted++
			}
		}
	}

	if demoted > 0 {
		log.Printf("[ReflectionLifecycle] staleness check: %d reflections demoted — rebuilding persona", demoted)
		// Rebuild persona to remove stale entries
		if w.identityBuilder != nil {
			if err := w.identityBuilder.BuildIdentity(ctx); err != nil {
				log.Printf("[ReflectionLifecycle] persona rebuild after demotion: %v", err)
			}
		}
	}
}

// ── Contradiction Detection ─────────────────────────────────────────

// detectContradictions scans reflections for the same entity that have
// opposing sentiment (one reinforced, another disputed by the same source).
func (w *MemoryWorker) detectContradictions(ctx context.Context) {
	promoted, err := w.store.ListReflectionsByStatus(ctx, []string{"promoted"}, 0)
	if err != nil || len(promoted) < 2 {
		return
	}

	// Group by entity
	byEntity := make(map[string][]types.ReflectionEntry)
	for _, r := range promoted {
		byEntity[r.Entity] = append(byEntity[r.Entity], r)
	}

	for entity, refs := range byEntity {
		if len(refs) < 2 {
			continue
		}

		// Check each pair for contradiction: one has high reinforcement,
		// another has high disputation on the same entity
		for i := 0; i < len(refs); i++ {
			for j := i + 1; j < len(refs); j++ {
				if isContradiction(&refs[i], &refs[j]) {
					w.handleContradiction(ctx, &refs[i], &refs[j])
				}
			}
		}
		_ = entity
	}
}

func isContradiction(a, b *types.ReflectionEntry) bool {
	// Contradiction: one is strong positive, other is strong negative
	// Both must have non-trivial evidence
	aScore := math.Abs(evidenceScore(a))
	bScore := math.Abs(evidenceScore(b))

	if aScore < 0.5 || bScore < 0.5 {
		return false
	}

	// Check for opposing signals
	aPos := a.Reinforcement > a.Disputation
	bPos := b.Reinforcement > b.Disputation

	return aPos != bPos
}

func (w *MemoryWorker) handleContradiction(ctx context.Context, a, b *types.ReflectionEntry) {
	log.Printf("[ReflectionLifecycle] contradiction detected: %q vs %q (entity=%s)",
		truncate(a.Text, 60), truncate(b.Text, 60), a.Entity)

	// The one with lower evidence score gets denied
	aScore := evidenceScore(a)
	bScore := evidenceScore(b)

	var loser *types.ReflectionEntry
	if math.Abs(aScore) < math.Abs(bScore) {
		loser = a
	} else {
		loser = b
	}

	if err := w.transitionReflection(ctx, loser.ID, loser.Status, "denied",
		"contradiction: weaker evidence"); err != nil {
		log.Printf("[ReflectionLifecycle] contradiction deny failed: %v", err)
		return
	}
	// Rebuild persona when a promoted reflection is contradicted
	if loser.Status == "promoted" && w.identityBuilder != nil {
		if err := w.identityBuilder.BuildIdentity(ctx); err != nil {
			log.Printf("[ReflectionLifecycle] persona rebuild after contradiction: %v", err)
		}
	}
}

// ── Evidence Helpers ──────────────────────────────────────────────

// reflectionScore computes evidence score: reinforcement - disputation.
func reflectionScore(r *types.ReflectionEntry) float64 {
	return r.Reinforcement - r.Disputation
}

func evidenceScore(r *types.ReflectionEntry) float64 { return reflectionScore(r) }

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

// ── Reflection Signal Detection ─────────────────────────────────────

// runReflectionSignalDetection applies signal detection to newly created reflections,
// matching them against existing confirmed/promoted reflections to accumulate evidence.
func (w *MemoryWorker) runReflectionSignalDetection(ctx context.Context, newRefs []types.ReflectionEntry) {
	if w.detectSignalsFn == nil || len(newRefs) == 0 {
		return
	}

	// Get existing reflections that can serve as signal targets
	existing, err := w.store.ListReflectionsByStatus(ctx, []string{"confirmed"}, 0)
	if err != nil {
		return
	}
	promoted, _ := w.store.ListReflectionsByStatus(ctx, []string{"promoted"}, 0)
	existing = append(existing, promoted...)

	if len(existing) == 0 {
		return
	}

	// Convert reflections to fact-like entries for the existing signal detection fn
	newFacts := make([]types.FactEntry, len(newRefs))
	for i, r := range newRefs {
		newFacts[i] = types.FactEntry{
			ID:      r.ID,
			Content: r.Text,
			Entity:  r.Entity,
		}
	}
	existingFacts := make([]types.FactEntry, len(existing))
	for i, r := range existing {
		existingFacts[i] = types.FactEntry{
			ID:      r.ID,
			Content: r.Text,
			Entity:  r.Entity,
		}
	}

	results, err := w.detectSignalsFn(ctx, newFacts, existingFacts)
	if err != nil {
		return
	}

	// Apply signals to reflections via the evidence engine
	for _, sig := range results {
		sigType := portSignalType(sig.Type)
		_, err := w.evidence.ApplySignal(ctx, sig.EntryID, port.EvidenceSignal{
			EntryID: sig.EntryID,
			Type:   sigType,
		})
		if err != nil {
			log.Printf("[ReflectionLifecycle] signal apply failed: %v", err)
		}
	}
}
