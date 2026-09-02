package cognition

import (
	"testing"

	"github.com/SHIROH4/sion-plus/internal/domain/types"
)

func TestScoreActionsAllScored(t *testing.T) {
	actions := BuildActions()
	drives := &types.DriveVector{
		Social: 0.5, Care: 0.5, Curious: 0.5, Quiet: 0.5, Explore: 0.5,
	}
	f := &types.QuantifiedFeatures{
		R1_OverallAcceptRate: 0.7,
		R1_SampleCount:       10,
	}

	scored := ScoreActions(drives, f, actions, nil)
	if len(scored) != len(actions) {
		t.Errorf("expected %d scored actions, got %d", len(actions), len(scored))
	}

	// Check sorted descending
	for i := 1; i < len(scored); i++ {
		if scored[i].FinalScore > scored[i-1].FinalScore {
			t.Errorf("not sorted: scored[%d].FinalScore=%.3f > scored[%d].FinalScore=%.3f",
				i, scored[i].FinalScore, i-1, scored[i-1].FinalScore)
		}
	}

	// "none" should have non-zero score when quiet drive is active
	var noneScore float64
	for _, s := range scored {
		if s.Action.Name == "none" {
			noneScore = s.FinalScore
			break
		}
	}
	if noneScore <= 0 {
		t.Error("'none' should have positive score when quiet drive is active")
	}
}

func TestScoreActionsSocialDriveFavorsSocialActions(t *testing.T) {
	actions := BuildActions()
	drives := &types.DriveVector{
		Social: 0.9, Care: 0.1, Curious: 0.1, Quiet: 0.0, Explore: 0.1,
	}
	f := &types.QuantifiedFeatures{
		R1_OverallAcceptRate: 0.7,
		R1_SampleCount:       10,
		U10_TimeWindowPref:   1.0,
		U8_EngagementNorm:    1.0,
	}

	scored := ScoreActions(drives, f, actions, nil)
	top := scored[0].Action.Name
	if top != "speak_casual" && top != "greet_return" {
		t.Errorf("top action with social=0.9 should be social, got %s", top)
	}
}

func TestScoreActionsCareDriveFavorsCareActions(t *testing.T) {
	actions := BuildActions()
	drives := &types.DriveVector{
		Social: 0.1, Care: 0.9, Curious: 0.0, Quiet: 0.0, Explore: 0.0,
	}
	f := &types.QuantifiedFeatures{
		R1_OverallAcceptRate: 0.7,
		R1_SampleCount:       10,
	}

	scored := ScoreActions(drives, f, actions, nil)
	top := scored[0].Action.Name
	if top != "care_rest" && top != "care_meal" && top != "care_hydration" && top != "care_health" && top != "care_encourage" {
		t.Errorf("top action with care=0.9 should be care, got %s", top)
	}
}

func TestScoreActionsQuietDriveFavorsNone(t *testing.T) {
	actions := BuildActions()
	drives := &types.DriveVector{
		Social: 0.0, Care: 0.0, Curious: 0.0, Quiet: 0.9, Explore: 0.0,
	}
	f := &types.QuantifiedFeatures{
		R1_OverallAcceptRate: 0.7,
		R1_SampleCount:       10,
	}

	scored := ScoreActions(drives, f, actions, nil)
	if scored[0].Action.Name != "none" {
		t.Errorf("top action with quiet=0.9 should be 'none', got %s", scored[0].Action.Name)
	}
}

func TestScoreActionsWarmthModulator(t *testing.T) {
	actions := BuildActions()
	drives := &types.DriveVector{
		Social: 0.3, Care: 0.3, Curious: 0.3, Quiet: 0.3, Explore: 0.3,
	}

	// Low warmth
	fCold := &types.QuantifiedFeatures{
		R1_OverallAcceptRate: 0.7,
		R1_SampleCount:       10,
		A1_1_Affection:       0.2,
	}
	scoredCold := ScoreActions(drives, fCold, actions, nil)

	// High warmth
	fWarm := &types.QuantifiedFeatures{
		R1_OverallAcceptRate: 0.7,
		R1_SampleCount:       10,
		A1_1_Affection:       0.8,
	}
	scoredWarm := ScoreActions(drives, fWarm, actions, nil)

	// Social actions should score higher with high warmth
	casualCold := findScore(scoredCold, "speak_casual")
	casualWarm := findScore(scoredWarm, "speak_casual")
	if casualWarm <= casualCold {
		t.Errorf("warmth should boost social: cold=%.3f warm=%.3f", casualCold, casualWarm)
	}
}

func TestScoreActionsWithCareSuggestion(t *testing.T) {
	actions := BuildActions()
	drives := &types.DriveVector{
		Social: 0.1, Care: 0.1, Curious: 0.1, Quiet: 0.1, Explore: 0.1,
	}
	f := &types.QuantifiedFeatures{
		R1_OverallAcceptRate: 0.7,
		R1_SampleCount:       10,
	}

	noSuggestion := ScoreActions(drives, f, actions, nil)
	withSuggestion := ScoreActions(drives, f, actions, map[string]float64{
		"care_rest": 0.25,
	})

	restNo := findScore(noSuggestion, "care_rest")
	restYes := findScore(withSuggestion, "care_rest")
	if restYes <= restNo {
		t.Errorf("care suggestion should boost: no=%.3f yes=%.3f", restNo, restYes)
	}
}

func TestScoreActionsDisengagementPenalty(t *testing.T) {
	actions := BuildActions()
	drives := &types.DriveVector{
		Social: 0.8, Care: 0.1, Curious: 0.0, Quiet: 0.0, Explore: 0.1,
	}

	fEngaged := &types.QuantifiedFeatures{
		R1_OverallAcceptRate: 0.7,
		R1_SampleCount:       10,
		U10_TimeWindowPref:   1.0,
		U8_EngagementNorm:    1.0,
	}
	fDisengaged := &types.QuantifiedFeatures{
		R1_OverallAcceptRate: 0.7,
		R1_SampleCount:       10,
		U10_TimeWindowPref:   0.3,
		U8_EngagementNorm:    0.3,
		U7_LengthTrend:       -0.5,
	}

	scoredEngaged := ScoreActions(drives, fEngaged, actions, nil)
	scoredDisengaged := ScoreActions(drives, fDisengaged, actions, nil)

	casualEng := findScore(scoredEngaged, "speak_casual")
	casualDis := findScore(scoredDisengaged, "speak_casual")
	if casualDis >= casualEng {
		t.Errorf("disengagement should penalize social: eng=%.3f dis=%.3f", casualEng, casualDis)
	}
}

func findScore(scored []types.ScoredAction, name string) float64 {
	for _, s := range scored {
		if s.Action.Name == name {
			return s.FinalScore
		}
	}
	return -1
}

func TestScoreActionsSearchModulators(t *testing.T) {
	actions := BuildActions()
	drives := &types.DriveVector{
		Social: 0.0, Care: 0.0, Curious: 0.8, Quiet: 0.0, Explore: 0.8,
	}

	f := &types.QuantifiedFeatures{
		R1_OverallAcceptRate: 0.7,
		R1_SampleCount:       10,
		A11_ActiveInquiries:  5,
		A12_KnowledgeGaps:    5,
		A13_LearningMomentum: 0.8,
		E3_CooldownNorm:      1.0,
		E4_QuotaRemaining:    10,
	}

	scored := ScoreActions(drives, f, actions, nil)
	searchScore := findScore(scored, "search")
	if searchScore <= 0 {
		t.Error("search should have positive score with high curious+explore")
	}

	// Verify search is crushed by quota protection
	fLow := &types.QuantifiedFeatures{
		R1_OverallAcceptRate: 0.7,
		R1_SampleCount:       10,
		A11_ActiveInquiries:  5,
		A12_KnowledgeGaps:    5,
		E3_CooldownNorm:      1.0,
		E4_QuotaRemaining:    2,
	}
	scoredLow := ScoreActions(drives, fLow, actions, nil)
	searchScoreLow := findScore(scoredLow, "search")
	if searchScoreLow >= searchScore {
		t.Errorf("search should be penalized by low quota: normal=%.3f low_quota=%.3f", searchScore, searchScoreLow)
	}
}
