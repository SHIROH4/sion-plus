package cognition

import (
	"time"

	"github.com/SHIROH4/sion-plus/internal/domain/types"
)

// ── Tier 1 Feature Computation (Pure In-Memory, ~1ms) ──
//
// These features require only the current state snapshot — no database queries.
// Tier 2 features (SQL-backed) live in adapter/memory/feature_store.go.

// ComputeTier1 populates all Tier-1 features from the current state.
func ComputeTier1(state *types.CognitionState) *types.QuantifiedFeatures {
	now := time.Now()
	f := &types.QuantifiedFeatures{ComputedAt: now.Unix()}

	// ── A组: Agent自身 ──
	f.A1_1_Affection = state.EmotionVec.Affection
	f.A1_2_Worry = state.EmotionVec.Worry
	f.A1_3_Curiosity = state.EmotionVec.Curiosity
	f.A1_4_Sleepiness = state.EmotionVec.Sleepiness
	f.A1_5_Playfulness = state.EmotionVec.Playfulness
	f.A1_6_Loneliness = state.EmotionVec.Loneliness
	f.A1_7_Confidence = state.EmotionVec.Confidence
	f.A1_8_Annoyance = state.EmotionVec.Annoyance
	f.A2_PrimaryEmotion = state.Emotion.Primary
	f.A3_Intensity = state.Emotion.Intensity

	// Valence trend from history (simplified: current vs neutral)
	f.A4_ValenceTrend = state.Emotion.Valence - state.HistoryAverageValence

	f.A5_1_AnnoySensitivity = state.Personality.AnnoyanceSensitivity
	f.A5_2_AffectWarmth = state.Personality.AffectionWarmth
	f.A5_3_WorryTendency = state.Personality.WorryTendency
	f.A6_DailyActionCount = state.ActionCount
	f.A14_ConsecutiveCount = state.ConsecutiveCount

	// ── U组: User状态 ──
	if state.ScreenObs != nil {
		f.U1_AppCategory = state.ScreenObs.AppCategory
		f.U2_WindowSubtype = state.ScreenObs.AppCategory // simplified
		f.U3_IsWorking = boolToFloat(state.ScreenObs.AppCategory == "work")
	} else {
		f.U1_AppCategory = "idle"
		f.U2_WindowSubtype = ""
		f.U3_IsWorking = 0
	}

	// Continuous work minutes
	if state.IsWorking && state.WorkStartAt > 0 {
		f.U4_ContinuousWorkMin = now.Sub(time.Unix(state.WorkStartAt, 0)).Minutes()
	}

	f.U5_AppSwitchCount = float64(state.AppSwitchCount)
	f.U7_LengthTrend = state.LengthTrend
	f.U8_EngagementNorm = state.EngagementNorm

	// Time-based features
	hour := now.Hour()
	f.U11_MealTime = mealTimeBonus(hour)
	f.U12_NightTime = nightTimeBonus(hour)
	f.U13_IsWeekend = isWeekend(now)

	if state.LastChatAt > 0 {
		f.U14_TimeSinceChatMin = now.Sub(time.Unix(state.LastChatAt, 0)).Minutes()
	} else {
		f.U14_TimeSinceChatMin = 999 // never chatted → max value
	}

	f.U15_FatigueMentionHr = state.FatigueMentionHours
	f.U16_PrefDiversity = float64(state.PrefDiversity) / 10.0
	if f.U16_PrefDiversity > 1 {
		f.U16_PrefDiversity = 1
	}

	// ── E组: Environment ──
	f.E1_Hour = hour
	f.E2_DayOfWeek = int(now.Weekday())
	f.E3_CooldownNorm = cooldownNorm(state.LastActionAt, now)
	f.E4_QuotaRemaining = state.QuotaRemaining
	f.E5_MinSinceDecision = now.Sub(time.Unix(state.LastDecisionAt, 0)).Minutes()
	f.E6_LLMAvailable = state.LLMAvailable
	f.E7_ReflectionDue = reflectionUrgency(state.LastReflectionAt, now)

	// ── R组: Relationship ──
	f.R1_OverallAcceptRate = state.OverallAcceptRate
	f.R1_SampleCount = float64(state.AcceptSampleCount)
	f.R2_TimeWindowAccept = state.TimeWindowAccept
	f.R3_SourceAcceptRate = state.SourceAcceptRate
	f.R4_RecentRejections = float64(state.RecentRejections)
	f.R4_RejectionSeverity = rejectionSeverity(state.RecentRejections)
	f.R5_NeglectHours = f.U14_TimeSinceChatMin / 60.0
	f.R6_DepthTrend = state.DepthTrend
	f.R7_UserInitiative24h = float64(state.UserInitiative24h)
	f.R8_IntimacyTrend = state.IntimacyTrend

	// ── T组: Task Context ──
	f.T1_PrincipleCount = float64(state.ActivePrincipleCount)
	f.T2_PatternCount = float64(state.ActivePatternCount)
	f.T3_ReflexionLogCount = float64(state.ReflexionLogCount)
	f.T5_TodayActivityCount = float64(state.ActionCount)

	return f
}

// ── Helper Functions ──

func boolToFloat(b bool) float64 {
	if b {
		return 1
	}
	return 0
}

func mealTimeBonus(hour int) float64 {
	if (hour >= 12 && hour < 13) || (hour >= 18 && hour < 19) {
		return 0.5
	}
	return 0
}

func nightTimeBonus(hour int) float64 {
	if hour >= 22 || hour < 8 {
		return 0.6
	}
	return 0
}

func isWeekend(t time.Time) float64 {
	if t.Weekday() == time.Saturday || t.Weekday() == time.Sunday {
		return 1
	}
	return 0
}

func cooldownNorm(lastActionAt int64, now time.Time) float64 {
	if lastActionAt == 0 {
		return 1.0 // no actions yet → fully cooled
	}
	minsSince := now.Sub(time.Unix(lastActionAt, 0)).Minutes()
	if minsSince >= 30 {
		return 1.0
	}
	return minsSince / 30.0
}

func reflectionUrgency(lastReflectionAt int64, now time.Time) float64 {
	if lastReflectionAt == 0 {
		return 1.0 // never reflected → urgent
	}
	hoursSince := now.Sub(time.Unix(lastReflectionAt, 0)).Hours()
	if hoursSince >= 24 {
		return 1.0
	}
	return hoursSince / 24.0
}

func rejectionSeverity(recentRejections int) float64 {
	if recentRejections >= 5 {
		return 1.0
	}
	return float64(recentRejections) / 5.0
}
