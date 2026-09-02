package cognition

import "github.com/SHIROH4/sion-plus/internal/domain/types"

// ComputeDrives maps 52-dimension features → 5-dimension drives.
// This is the PURE FUNCTION implementing all the weighted formulas
// from the original Sion v0.3 ARCHITECTURE.md §4.2.
//
// Each drive = emotion_basis(50%) + need_push(15%) + user_context(20%) + relationship_gate(15%).
// All values clamped to [0, 1].
func ComputeDrives(f *types.QuantifiedFeatures, needs *types.IntrinsicNeeds) *types.DriveVector {
	return &types.DriveVector{
		Social:  clamp01(computeSocial(f, needs)),
		Care:    clamp01(computeCare(f, needs)),
		Curious: clamp01(computeCurious(f, needs)),
		Quiet:   clamp01(computeQuiet(f, needs)),
		Explore: clamp01(computeExplore(f, needs)),
	}
}

// ── Social Drive ──

func computeSocial(f *types.QuantifiedFeatures, needs *types.IntrinsicNeeds) float64 {
	s := 0.30*f.A1_6_Loneliness +
		0.25*f.A1_5_Playfulness +
		0.25*f.A1_1_Affection +
		0.15*idleBonus(f.U14_TimeSinceChatMin) +
		0.05*(1.0-f.A1_8_Annoyance)

	if needs != nil {
		s += needs.Companionship*0.12 + needs.Play*0.08
	}
	s -= f.U3_IsWorking*0.15 + f.U12_NightTime*0.15 + f.R4_RejectionSeverity*0.35
	s *= interactionGate(f.R1_OverallAcceptRate)

	return s
}

// ── Care Drive ──

func computeCare(f *types.QuantifiedFeatures, needs *types.IntrinsicNeeds) float64 {
	c := 0.40*f.A1_2_Worry +
		0.20*f.A1_1_Affection +
		0.15*f.U12_NightTime +
		0.10*f.U11_MealTime

	if needs != nil {
		c += needs.Care * 0.18
	}
	c += (f.U4_ContinuousWorkMin/180.0)*0.15 +
		f.U12_NightTime*f.U3_IsWorking*0.10 +
		f.U13_IsWeekend*0.05
	c *= interactionGate(f.R1_OverallAcceptRate)

	return c
}

// ── Curious Drive ──

func computeCurious(f *types.QuantifiedFeatures, needs *types.IntrinsicNeeds) float64 {
	c := 0.35*f.A1_3_Curiosity +
		0.25*hasAny(f.A11_ActiveInquiries) +
		0.20*hasAnyInt(f.A12_KnowledgeGaps) +
		0.15*(1.0-timeNorm(f.U14_TimeSinceChatMin))

	if needs != nil {
		c += needs.Curiosity * 0.18
	}
	c += f.A13_LearningMomentum*0.07 + f.U16_PrefDiversity*0.05

	gate := interactionGate(f.R1_OverallAcceptRate)
	c *= 0.7 + gate*0.3

	return c
}

// ── Quiet Drive ──

func computeQuiet(f *types.QuantifiedFeatures, needs *types.IntrinsicNeeds) float64 {
	q := 0.20*f.A1_4_Sleepiness +
		0.15*timeNorm(f.U14_TimeSinceChatMin) +
		0.25*f.A1_8_Annoyance +
		0.10*actionBias(f.A6_DailyActionCount)

	if needs != nil {
		q += needs.Rest * 0.18
	}
	q += (1.0-f.E3_CooldownNorm)*0.15 +
		f.U3_IsWorking*0.12 +
		f.U12_NightTime*0.08

	if f.E4_QuotaRemaining < 5 {
		q += 0.10
	}
	q += f.R4_RejectionSeverity * 0.40

	return q
}

// ── Explore Drive ──

func computeExplore(f *types.QuantifiedFeatures, needs *types.IntrinsicNeeds) float64 {
	e := 0.30*f.A1_3_Curiosity +
		0.20*(1.0-timeNorm(f.U14_TimeSinceChatMin)) +
		gapNorm(f.A12_KnowledgeGaps)*0.25

	if needs != nil {
		e += needs.Curiosity*0.15 + needs.Autonomy*0.15
	}
	e += (1.0-f.U3_IsWorking)*0.08 +
		f.E7_ReflectionDue*0.10 +
		inquiryNorm(f.A11_ActiveInquiries)*0.12

	if f.E5_MinSinceDecision > 30 {
		e += 0.10
	}

	gate := interactionGate(f.R1_OverallAcceptRate)
	e *= 0.8 + gate*0.2

	return e
}

// ── Helpers ──

func clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

// interactionGate: acceptance rate < 0.5 softens social/care drives.
func interactionGate(acceptRate float64) float64 {
	if acceptRate <= 0 {
		return 1.0
	}
	if acceptRate >= 0.5 {
		return 1.0
	}
	return 0.5 + acceptRate
}

// idleBonus shapes the social-drive boost from silence duration.
// It rises to 1.0 around 30 min, plateaus briefly, then decays —
// because a user silent for hours is likely away, not lonely.
func idleBonus(minSinceChat float64) float64 {
	if minSinceChat < 0 {
		return 0
	}
	// 0–30 min: rise linearly 0→1.0
	if minSinceChat <= 30 {
		return clamp01(minSinceChat / 30.0)
	}
	// 30–90 min: plateau at 1.0
	if minSinceChat <= 90 {
		return 1.0
	}
	// 90–240 min: decay 1.0→0.15
	if minSinceChat <= 240 {
		t := (minSinceChat - 90) / 150.0 // 0→1 over the decay window
		return 1.0 - t*0.85
	}
	// >4 hours: floor at 0.15
	return 0.15
}

func timeNorm(minSinceChat float64) float64 {
	return clamp01(minSinceChat / 10.0)
}

func hasAny(count int) float64 {
	if count > 0 {
		return 0.6
	}
	return 0
}

func hasAnyInt(count int) float64 {
	if count > 0 {
		return 0.4
	}
	return 0
}

func gapNorm(count int) float64 {
	return clamp01(float64(count) / 5.0)
}

func inquiryNorm(count int) float64 {
	return clamp01(float64(count) / 5.0)
}

func actionBias(dailyActions int) float64 {
	if dailyActions >= 10 {
		return 1.0
	}
	return clamp01(float64(dailyActions) / 10.0)
}
