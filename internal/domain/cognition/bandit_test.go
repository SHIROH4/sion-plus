package cognition

import (
	"testing"

	"github.com/SHIROH4/sion-plus/internal/domain/types"
)

func TestConstrainedBanditShadowRequiresDataAndBoundsDelta(t *testing.T) {
	base := []types.ScoredAction{
		{Action: types.ActionDef{Name: "speak_care", OutcomeType: "speak"}, FinalScore: 0.5, Modulators: map[string]float64{}},
		{Action: types.ActionDef{Name: "none", OutcomeType: "silent"}, FinalScore: 0.49, Modulators: map[string]float64{}},
	}
	result := RankConstrainedBanditShadow(base, nil,
		[]types.ActionFeedbackStats{{Action: "speak_care", Samples: 5, RewardSum: 5}},
		DefaultConstrainedBanditConfig())
	if result.Ready {
		t.Fatal("five samples must not make the candidate promotion-ready")
	}
	for _, action := range result.Actions {
		if action.Action.Name == "speak_care" && action.FinalScore > 0.600001 {
			t.Fatalf("bandit delta exceeded safety cap: %.3f", action.FinalScore-0.5)
		}
		if action.Action.Name == "none" && action.FinalScore != 0.49 {
			t.Fatal("shadow bandit must not synthesize rewards for silence")
		}
	}
}

func TestConstrainedBanditShadowReadiness(t *testing.T) {
	stats := []types.ActionFeedbackStats{
		{Action: "a", Samples: 20, RewardSum: 5},
		{Action: "b", Samples: 20, RewardSum: 0},
		{Action: "c", Samples: 10, RewardSum: -2},
	}
	result := RankConstrainedBanditShadow(nil, nil, stats, DefaultConstrainedBanditConfig())
	if !result.Ready || result.TotalSamples != 50 || result.CoveredActions != 3 {
		t.Fatalf("unexpected readiness: %#v", result)
	}
}
