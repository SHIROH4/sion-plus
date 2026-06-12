package cognition

import (
	"testing"

	"github.com/SHIROH4/sion-plus/internal/domain/types"
)

func makeScoredAction(name string, score float64) types.ScoredAction {
	return types.ScoredAction{
		Action:     *ActionByName(name),
		FinalScore: score,
	}
}

func TestRouteFastPath(t *testing.T) {
	scored := []types.ScoredAction{
		makeScoredAction("speak_casual", 0.85),
		makeScoredAction("speak_care", 0.60),
		makeScoredAction("none", 0.10),
	}
	f := &types.QuantifiedFeatures{}

	result := Route(scored, f)
	if !result.FastPath {
		t.Error("expected FastPath with decisive gap")
	}
}

func TestRouteGapClose(t *testing.T) {
	scored := []types.ScoredAction{
		makeScoredAction("speak_casual", 0.45),
		makeScoredAction("speak_care", 0.44),
	}
	f := &types.QuantifiedFeatures{}

	result := Route(scored, f)
	if result.FastPath {
		t.Error("expected System2 with close gap")
	}
	if result.Reason != "gap_close" {
		t.Errorf("reason = %s, want gap_close", result.Reason)
	}
}

func TestRouteExtremeSleepiness(t *testing.T) {
	scored := []types.ScoredAction{
		makeScoredAction("speak_casual", 0.45),
		makeScoredAction("none", 0.44),
	}
	f := &types.QuantifiedFeatures{A1_4_Sleepiness: 0.9}

	result := Route(scored, f)
	if result.FastPath {
		t.Error("expected System2 with extreme sleepiness")
	}
	if result.Reason != "extreme_sleepiness" {
		t.Errorf("reason = %s", result.Reason)
	}
}

func TestRouteActionStuck(t *testing.T) {
	scored := []types.ScoredAction{
		makeScoredAction("speak_casual", 0.45),
		makeScoredAction("speak_care", 0.44),
	}
	f := &types.QuantifiedFeatures{A14_ConsecutiveCount: 4}

	result := Route(scored, f)
	if result.FastPath {
		t.Error("expected System2 with stuck action")
	}
	if result.Reason != "action_stuck" {
		t.Errorf("reason = %s", result.Reason)
	}
}

func TestRouteConsecutiveRejections(t *testing.T) {
	scored := []types.ScoredAction{
		makeScoredAction("speak_casual", 0.45),
		makeScoredAction("speak_care", 0.44),
	}
	f := &types.QuantifiedFeatures{R4_RecentRejections: 4}

	result := Route(scored, f)
	if result.FastPath {
		t.Error("expected System2 with consecutive rejections")
	}
	if result.Reason != "consecutive_rejections" {
		t.Errorf("reason = %s", result.Reason)
	}
}

func TestRouteAcceptanceCollapse(t *testing.T) {
	scored := []types.ScoredAction{
		makeScoredAction("speak_casual", 0.45),
		makeScoredAction("speak_care", 0.44),
	}
	f := &types.QuantifiedFeatures{
		R1_OverallAcceptRate: 0.2,
		R1_SampleCount:       15,
		R8_IntimacyTrend:     -0.2,
	}

	result := Route(scored, f)
	if result.FastPath {
		t.Error("expected System2 with acceptance collapse")
	}
	if result.Reason != "acceptance_collapse" {
		t.Errorf("reason = %s", result.Reason)
	}
}

func TestRouteDealbreakerOverridesS2Trigger(t *testing.T) {
	// Even with S2 trigger, if gap is decisive, still go S1
	scored := []types.ScoredAction{
		makeScoredAction("none", 0.85),
		makeScoredAction("speak_casual", 0.30),
	}
	f := &types.QuantifiedFeatures{A1_4_Sleepiness: 0.9}

	result := Route(scored, f)
	if !result.FastPath {
		t.Error("expected FastPath when top action has decisive gap, even with S2 trigger")
	}
}

func TestHardGateNightBlock(t *testing.T) {
	action := makeScoredAction("speak_casual", 0.8)
	f := &types.QuantifiedFeatures{U12_NightTime: 1.0}

	allowed, reason := HardGate(action, f)
	if allowed {
		t.Error("speak_casual should be blocked at night")
	}
	if reason != "night_gate" {
		t.Errorf("reason = %s", reason)
	}
}

func TestHardGateNightSafePasses(t *testing.T) {
	action := makeScoredAction("care_rest", 0.8)
	f := &types.QuantifiedFeatures{U12_NightTime: 1.0, E4_QuotaRemaining: 10}

	allowed, _ := HardGate(action, f)
	if !allowed {
		t.Error("care_rest should pass at night (NightSafe=true)")
	}
}

func TestHardGateQuotaExhausted(t *testing.T) {
	action := makeScoredAction("speak_casual", 0.8)
	f := &types.QuantifiedFeatures{E4_QuotaRemaining: 0}

	allowed, reason := HardGate(action, f)
	if allowed {
		t.Error("should be blocked when quota exhausted")
	}
	if reason != "quota_exhausted" {
		t.Errorf("reason = %s", reason)
	}
}

func TestHardGateNoneAlwaysPasses(t *testing.T) {
	action := makeScoredAction("none", 0.8)
	f := &types.QuantifiedFeatures{U12_NightTime: 1.0, E4_QuotaRemaining: 0}

	allowed, _ := HardGate(action, f)
	if !allowed {
		t.Error("'none' should always pass hard gate")
	}
}

func TestHardGateConsecutiveUnanswered(t *testing.T) {
	action := makeScoredAction("speak_casual", 0.8)
	f := &types.QuantifiedFeatures{R4_RecentRejections: 3, E4_QuotaRemaining: 10}

	allowed, reason := HardGate(action, f)
	if allowed {
		t.Error("speak should be blocked after 3 consecutive unanswered")
	}
	if reason != "consecutive_unanswered" {
		t.Errorf("reason = %s", reason)
	}

	// 2 unanswered should still be allowed (threshold is 3)
	f2 := &types.QuantifiedFeatures{R4_RecentRejections: 2, E4_QuotaRemaining: 10}
	allowed2, _ := HardGate(action, f2)
	if !allowed2 {
		t.Error("speak should pass with only 2 unanswered")
	}
}
