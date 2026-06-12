package cognition

import (
	"math"
	"sort"

	"github.com/shirohania/sion/internal/domain/types"
)

// ── DPO-Style Batch Weight Update ──
//
// The Learner records drive snapshots at decision time, receives rewards
// later (from outcome classification), and batch-updates the action weight
// matrix. This implements a simplified Direct Preference Optimization
// (DPO) variant designed for 16 actions × 5 drives.

// DriveRecord is a single training sample for DPO weight updates.
type DriveRecord struct {
	Action  string
	Social  float64
	Care    float64
	Curious float64
	Quiet   float64
	Explore float64
	Reward  float64 // +1 accepted, 0 ignored, -1 rejected
	At      int64   // unix timestamp of record creation
}

// WeightMatrix holds per-action drive weights (16 actions × 5 drives).
type WeightMatrix struct {
	Social  []float64 // index = action index
	Care    []float64
	Curious []float64
	Quiet   []float64
	Explore []float64
}

// NewWeightMatrix builds a WeightMatrix from the canonical action definitions.
func NewWeightMatrix() *WeightMatrix {
	actions := BuildActions()
	n := len(actions)
	wm := &WeightMatrix{
		Social:  make([]float64, n),
		Care:    make([]float64, n),
		Curious: make([]float64, n),
		Quiet:   make([]float64, n),
		Explore: make([]float64, n),
	}
	for i, a := range actions {
		wm.Social[i] = a.WeightSocial
		wm.Care[i] = a.WeightCare
		wm.Curious[i] = a.WeightCurious
		wm.Quiet[i] = a.WeightQuiet
		wm.Explore[i] = a.WeightExplore
	}
	return wm
}

// actionIndex returns the index of an action in the canonical list, or -1.
func actionIndex(name string) int {
	for i, a := range BuildActions() {
		if a.Name == name {
			return i
		}
	}
	return -1
}

// actionName returns the action name for an index, or "".
func actionName(idx int) string {
	actions := BuildActions()
	if idx < 0 || idx >= len(actions) {
		return ""
	}
	return actions[idx].Name
}

// ── Batch DPO Update ──

// DPOConfig holds the learning hyperparameters.
type DPOConfig struct {
	LearningRate  float64 // η, default 0.01
	Beta          float64 // temperature for preference strength, default 1.0
	L2Lambda      float64 // L2 regularization, default 0.001
	MinSamples    int     // minimum samples before learning, default 10
	MaxDelta      float64 // max per-weight change per batch, default 0.1
}

// DefaultDPOConfig returns sensible defaults.
func DefaultDPOConfig() DPOConfig {
	return DPOConfig{
		LearningRate: 0.01,
		Beta:         1.0,
		L2Lambda:     0.001,
		MinSamples:   10,
		MaxDelta:     0.1,
	}
}

// DPOUpdate runs a batch DPO weight update given the current weight matrix,
// a set of drive records with rewards, and the learning configuration.
//
// Algorithm:
//  1. For each action, compute the average drive vector of accepted outcomes
//     (reward > 0) and rejected outcomes (reward < 0).
//  2. The preference signal: weights should move toward accepted drives
//     and away from rejected drives.
//  3. Apply gradient update with L2 regularization.
//
// Returns the delta per action (for audit/logging).
func DPOUpdate(wm *WeightMatrix, records []DriveRecord, cfg DPOConfig) map[string][5]float64 {
	if len(records) < cfg.MinSamples {
		return nil
	}

	// Group records by action
	type actionGroup struct {
		accepted []DriveRecord // reward > 0
		rejected []DriveRecord // reward < 0
	}
	groups := make(map[string]*actionGroup)

	for _, r := range records {
		idx := actionIndex(r.Action)
		if idx < 0 {
			continue
		}
		g, ok := groups[r.Action]
		if !ok {
			g = &actionGroup{}
			groups[r.Action] = g
		}
		if r.Reward > 0 {
			g.accepted = append(g.accepted, r)
		} else if r.Reward < 0 {
			g.rejected = append(g.rejected, r)
		}
	}

	if len(groups) == 0 {
		return nil
	}

	deltas := make(map[string][5]float64)

	for actionName, g := range groups {
		idx := actionIndex(actionName)
		if idx < 0 {
			continue
		}

		// Average accepted drive vector
		avgAcc := avgDrives(g.accepted)
		// Average rejected drive vector
		avgRej := avgDrives(g.rejected)

		var delta [5]float64

		if len(g.accepted) > 0 && len(g.rejected) > 0 {
			// DPO-style preference: move toward accepted, away from rejected
			// gradient = β * (avg_accepted - avg_rejected)
			for d := 0; d < 5; d++ {
				grad := cfg.Beta * (avgAcc[d] - avgRej[d])
				// L2 regularization: -λ * current_weight
				currentWeight := wm.get(idx, d)
				reg := cfg.L2Lambda * currentWeight
				// Update: w_new = w + η * (grad - λ*w)
				rawDelta := cfg.LearningRate * (grad - reg)
				delta[d] = clampDelta(rawDelta, cfg.MaxDelta)
			}
		} else if len(g.accepted) > 0 {
			// Only positives: move weights toward accepted drives
			wm.get(idx, 0) // just to trigger compilation
			// Current weights
			curW := [5]float64{
				wm.Social[idx], wm.Care[idx], wm.Curious[idx],
				wm.Quiet[idx], wm.Explore[idx],
			}
			for d := 0; d < 5; d++ {
				grad := avgAcc[d] - curW[d]
				reg := cfg.L2Lambda * curW[d]
				rawDelta := cfg.LearningRate * (grad - reg)
				delta[d] = clampDelta(rawDelta, cfg.MaxDelta)
			}
		} else if len(g.rejected) > 0 {
			// Only negatives: move weights away from rejected drives
			curW := [5]float64{
				wm.Social[idx], wm.Care[idx], wm.Curious[idx],
				wm.Quiet[idx], wm.Explore[idx],
			}
			for d := 0; d < 5; d++ {
				grad := -(avgRej[d] - curW[d]) // push away
				reg := cfg.L2Lambda * curW[d]
				rawDelta := cfg.LearningRate * (grad - reg)
				delta[d] = clampDelta(rawDelta, cfg.MaxDelta)
			}
		}

		// Apply deltas
		wm.Social[idx] = clampWeight(wm.Social[idx] + delta[0])
		wm.Care[idx] = clampWeight(wm.Care[idx] + delta[1])
		wm.Curious[idx] = clampWeight(wm.Curious[idx] + delta[2])
		wm.Quiet[idx] = clampWeight(wm.Quiet[idx] + delta[3])
		wm.Explore[idx] = clampWeight(wm.Explore[idx] + delta[4])

		deltas[actionName] = delta
	}

	return deltas
}

// avgDrives computes the average drive vector from a set of records.
func avgDrives(records []DriveRecord) [5]float64 {
	if len(records) == 0 {
		return [5]float64{}
	}
	var sum [5]float64
	for _, r := range records {
		sum[0] += r.Social
		sum[1] += r.Care
		sum[2] += r.Curious
		sum[3] += r.Quiet
		sum[4] += r.Explore
	}
	n := float64(len(records))
	return [5]float64{
		sum[0] / n, sum[1] / n, sum[2] / n, sum[3] / n, sum[4] / n,
	}
}

func (wm *WeightMatrix) get(idx, dim int) float64 {
	switch dim {
	case 0:
		return wm.Social[idx]
	case 1:
		return wm.Care[idx]
	case 2:
		return wm.Curious[idx]
	case 3:
		return wm.Quiet[idx]
	case 4:
		return wm.Explore[idx]
	}
	return 0
}

func clampDelta(d, maxAbs float64) float64 {
	if d > maxAbs {
		return maxAbs
	}
	if d < -maxAbs {
		return -maxAbs
	}
	return d
}

func clampWeight(w float64) float64 {
	return math.Max(-1, math.Min(1, w))
}

// ── Audit ──

// AuditResult summarizes the health of the weight matrix and learning process.
type AuditResult struct {
	StuckActions   []string `json:"stuck_actions"`
	DriftWarning   bool     `json:"drift_warning"`
	Recommendation string   `json:"recommendation"`
}

// AuditWeights checks the weight matrix for problems: stuck actions
// (all weights near zero), drift (weights far from canonical), dead drives.
func AuditWeights(wm *WeightMatrix, records []DriveRecord) AuditResult {
	var result AuditResult
	actions := BuildActions()

	// Check for stuck actions: all weights within ±0.05 of zero
	for i, a := range actions {
		if a.Name == "none" {
			continue // "none" is always low-weight by design
		}
		absMax := math.Max(math.Abs(wm.Social[i]),
			math.Max(math.Abs(wm.Care[i]),
				math.Max(math.Abs(wm.Curious[i]),
					math.Max(math.Abs(wm.Quiet[i]),
						math.Abs(wm.Explore[i])))))
		if absMax < 0.05 {
			result.StuckActions = append(result.StuckActions, a.Name)
		}
	}

	// Drift warning: any weight > 1.5 or < -1.5
	driftDetected := false
	for i := range actions {
		if math.Abs(wm.Social[i]) > 1.5 || math.Abs(wm.Care[i]) > 1.5 ||
			math.Abs(wm.Curious[i]) > 1.5 || math.Abs(wm.Quiet[i]) > 1.5 ||
			math.Abs(wm.Explore[i]) > 1.5 {
			driftDetected = true
			break
		}
	}
	result.DriftWarning = driftDetected

	// Recommendation
	switch {
	case len(result.StuckActions) > 3:
		result.Recommendation = "Multiple actions have near-zero weights. Consider increasing learning rate or resetting to canonical weights."
	case driftDetected:
		result.Recommendation = "Weight drift detected. Increase L2 regularization or reduce learning rate."
	case len(records) < 10:
		result.Recommendation = "Insufficient training data. Wait for more outcome samples."
	default:
		result.Recommendation = "Weight matrix looks healthy."
	}

	return result
}

// ── Action Weight Persistence Helpers ──

// ApplyWeightDeltas applies per-action deltas to the canonical action list.
// Returns the updated action definitions.
func ApplyWeightDeltas(deltas map[string][5]float64) []types.ActionDef {
	actions := BuildActions()
	for i := range actions {
		if d, ok := deltas[actions[i].Name]; ok {
			actions[i].WeightSocial += d[0]
			actions[i].WeightCare += d[1]
			actions[i].WeightCurious += d[2]
			actions[i].WeightQuiet += d[3]
			actions[i].WeightExplore += d[4]
			// Clamp
			actions[i].WeightSocial = clampWeight(actions[i].WeightSocial)
			actions[i].WeightCare = clampWeight(actions[i].WeightCare)
			actions[i].WeightCurious = clampWeight(actions[i].WeightCurious)
			actions[i].WeightQuiet = clampWeight(actions[i].WeightQuiet)
			actions[i].WeightExplore = clampWeight(actions[i].WeightExplore)
		}
	}
	return actions
}

// ── Drive Record Management ──

// StaleRecordCleanup removes records older than maxAgeHours.
func StaleRecordCleanup(records []DriveRecord, maxAgeHours float64, now int64) []DriveRecord {
	out := make([]DriveRecord, 0, len(records))
	cutoff := now - int64(maxAgeHours*3600)
	for _, r := range records {
		// Records with zero At are kept (backwards compat)
		if r.At == 0 || r.At >= cutoff {
			out = append(out, r)
		}
	}
	return out
}

// RecordStats computes summary statistics for drive records.
type RecordStats struct {
	Total    int
	Accepted int
	Rejected int
	Ignored  int
	TopAction string
}

func ComputeRecordStats(records []DriveRecord) RecordStats {
	var s RecordStats
	s.Total = len(records)
	actionCount := make(map[string]int)
	for _, r := range records {
		switch {
		case r.Reward > 0:
			s.Accepted++
		case r.Reward < 0:
			s.Rejected++
		default:
			s.Ignored++
		}
		actionCount[r.Action]++
	}
	maxCount := 0
	for action, count := range actionCount {
		if count > maxCount {
			maxCount = count
			s.TopAction = action
		}
	}
	return s
}

// sortRecords sorts records by timestamp descending (newest first).
// Only used if records carry timestamps.
func sortRecordsDesc(records []DriveRecord) {
	sort.Slice(records, func(i, j int) bool {
		return records[i].At > records[j].At
	})
}
