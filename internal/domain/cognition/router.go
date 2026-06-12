package cognition

import (
	"github.com/shirohania/sion/internal/domain/types"
)

// Route implements System 1 / System 2 decision routing.
//
// System 1 (fast path): gap between #1 and #2 > 0.15 AND no extreme conditions.
//
// System 2 (LLM fallback): triggered when:
//   1. Score gap ≤ 0.15 (too close to call)
//   2. Extreme sleepiness (>0.85)
//   3. Extreme user emotion
//   4. Action stuck (same action ≥3 times)
//   5. Consecutive rejections ≥3
//   6. Acceptance rate collapse
//   7. Long silence reconnection
//
// The gap threshold (0.15) is the primary S1 confidence gate.
// Even when other S2 triggers fire, if the top action is decisively ahead,
// we trust S1. This saves LLM cost on obvious decisions.
func Route(scored []types.ScoredAction, features *types.QuantifiedFeatures) *types.DecisionResult {
	if len(scored) == 0 {
		return &types.DecisionResult{FastPath: true, Action: nil, Reason: "no_actions"}
	}

	top := scored[0]
	var gap float64
	if len(scored) >= 2 {
		gap = top.FinalScore - scored[1].FinalScore
	}

	// ── Primary gate: if S1 is decisive, short-circuit all S2 triggers ──
	if gap >= 0.15 {
		return &types.DecisionResult{FastPath: true, Action: &top, Reason: "decisive_gap"}
	}

	// ── S2 triggers (only evaluated when gap < 0.15) ──

	if features.A1_4_Sleepiness > 0.85 {
		return &types.DecisionResult{FastPath: false, Reason: "extreme_sleepiness"}
	}
	if (features.A2_PrimaryEmotion == "anger" || features.A2_PrimaryEmotion == "fear") &&
		features.A3_Intensity > 0.8 {
		return &types.DecisionResult{FastPath: false, Reason: "extreme_user_emotion"}
	}
	if features.A14_ConsecutiveCount >= 3 && top.Action.Name != "none" {
		return &types.DecisionResult{FastPath: false, Reason: "action_stuck"}
	}
	if features.R4_RecentRejections >= 3 {
		return &types.DecisionResult{FastPath: false, Reason: "consecutive_rejections"}
	}
	if features.R1_OverallAcceptRate < 0.3 && features.R1_SampleCount >= 10 &&
		features.R8_IntimacyTrend < -0.15 {
		return &types.DecisionResult{FastPath: false, Reason: "acceptance_collapse"}
	}
	if features.U14_TimeSinceChatMin > 240 && features.A6_DailyActionCount == 0 {
		return &types.DecisionResult{FastPath: false, Reason: "long_silence_reconnect"}
	}

	return &types.DecisionResult{FastPath: false, Reason: "gap_close"}
}

// HardGate checks whether an action is blocked by hard rules.
// Returns (allowed, reason_if_blocked).
func HardGate(action types.ScoredAction, features *types.QuantifiedFeatures) (bool, string) {
	if features.U12_NightTime > 0 && !action.Action.NightSafe {
		return false, "night_gate"
	}
	if features.E4_QuotaRemaining <= 0 && action.Action.Name != "none" {
		return false, "quota_exhausted"
	}
	consecutiveUnanswered := int(features.R4_RecentRejections)
	if consecutiveUnanswered >= 3 && action.Action.OutcomeType == "speak" {
		return false, "consecutive_unanswered"
	}
	return true, ""
}
