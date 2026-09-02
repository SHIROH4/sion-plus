package cognition

import "github.com/SHIROH4/sion-plus/internal/domain/types"

// ── Action Scoring: Drive Dot-Product + Context Modulation ──

// ScoreActions computes final scores for all 16 actions.
//  1. baseScore = drive · weight_vector (dot product)
//  2. careSuggestion bonus (if CareEngine suggested this action type)
//  3. contextModulator multiplier (history, time-window, engagement)
//  4. Hard gate filter (night, quota, consecutive unanswered)
func ScoreActions(
	drives *types.DriveVector,
	features *types.QuantifiedFeatures,
	actions []types.ActionDef,
	careSuggestions map[string]float64, // action_name → bonus (0.10~0.30)
) []types.ScoredAction {
	scored := make([]types.ScoredAction, 0, len(actions))

	for _, action := range actions {
		// 1. Dot-product base score
		baseScore := drives.Social*action.WeightSocial +
			drives.Care*action.WeightCare +
			drives.Curious*action.WeightCurious +
			drives.Quiet*action.WeightQuiet +
			drives.Explore*action.WeightExplore

		// 2. CareEngine suggestion bonus
		if bonus, ok := careSuggestions[action.Name]; ok {
			baseScore += bonus
		}

		// 3. Context modulation
		modulators := computeModulators(action, features)
		finalScore := baseScore
		for _, m := range modulators {
			finalScore *= m
		}

		scored = append(scored, types.ScoredAction{
			Action:     action,
			RawScore:   baseScore,
			FinalScore: finalScore,
			Modulators: modulators,
		})
	}

	// Sort descending by FinalScore
	sortScoredActions(scored)
	return scored
}

// computeModulators returns a map of modulator_name → multiplier.
func computeModulators(action types.ActionDef, f *types.QuantifiedFeatures) map[string]float64 {
	m := make(map[string]float64)

	// 1. Historical success rate
	if action.OutcomeType == "speak" || action.OutcomeType == "action" {
		if rate, ok := f.R3_SourceAcceptRate[action.Source]; ok && rate >= 0 {
			m["source_accept"] = 0.5 + rate*0.5
		}
	}

	// 2. Time window preference (social actions)
	if action.Category == "social" {
		m["time_window"] = 0.4 + f.U10_TimeWindowPref*0.6
	}

	// 3. Engagement (social actions)
	if action.OutcomeType == "speak" && action.Category == "social" {
		m["engagement"] = 0.6 + f.U8_EngagementNorm*0.4
	}

	// 4. Depth trend (inquiry bonus)
	if action.Name == "speak_inquiry" && f.R6_DepthTrend > 0.2 {
		m["depth"] = 1.0 + f.R6_DepthTrend*0.3
	}

	// 5. Active inquiries (inquiry bonus)
	if action.Name == "speak_inquiry" && f.A11_ActiveInquiries > 0 {
		m["active_inquiry"] = 1.0 + clamp01(float64(f.A11_ActiveInquiries)/3.0)*0.3
	}

	// 6. User disengagement (social penalty)
	if (action.Name == "speak_casual" || action.Name == "speak_inquiry") && f.U7_LengthTrend < -0.3 {
		m["disengaged"] = 1.0 + f.U7_LengthTrend*0.4
	}

	// 7. Search-specific
	if action.Name == "search" {
		m["inquiry_boost"] = 1.0 + clamp01(float64(f.A11_ActiveInquiries)/5.0)*0.3
		m["gap_boost"] = 1.0 + gapNorm(f.A12_KnowledgeGaps)*0.2
		m["learning_momentum"] = 1.0 + f.A13_LearningMomentum*0.1
		m["cooldown"] = 0.3 + f.E3_CooldownNorm*0.7
		if f.E4_QuotaRemaining < 3 {
			m["quota_protection"] = 0.3
		}
	}

	// 8. Warmth bonus: high affection → social actions get boosted
	if action.Category == "social" && f.A1_1_Affection > 0.55 {
		bonus := 1.0 + (f.A1_1_Affection-0.55)*2.0
		if bonus > 1.5 {
			bonus = 1.5
		}
		m["warmth"] = bonus
	}

	// 9. Clamp all modulators to [0.1, 1.5]
	for k, v := range m {
		if v < 0.1 {
			m[k] = 0.1
		} else if v > 1.5 {
			m[k] = 1.5
		}
	}

	return m
}

// sortScoredActions sorts by FinalScore descending.
func sortScoredActions(s []types.ScoredAction) {
	for i := 0; i < len(s)-1; i++ {
		for j := i + 1; j < len(s); j++ {
			if s[j].FinalScore > s[i].FinalScore {
				s[i], s[j] = s[j], s[i]
			}
		}
	}
}
