package emotion

import "github.com/shirohania/sion/internal/domain/types"

// ── Smoothing Parameters ──

const (
	SmoothAffection   = 0.10
	SmoothConfidence  = 0.15
	SmoothCuriosity   = 0.30
	SmoothPlayfulness = 0.30
	SmoothWorry       = 0.60
	SmoothAnnoyance   = 0.70
	SmoothSleepiness  = 0.50
	SmoothLoneliness  = 0.40
)

// SmoothingParams holds per-dimension blending alphas, modulated by personality.
type SmoothingParams struct {
	AffectionAlpha   float64
	ConfidenceAlpha  float64
	CuriosityAlpha   float64
	PlayfulnessAlpha float64
	WorryAlpha       float64
	AnnoyanceAlpha   float64
	SleepinessAlpha  float64
	LonelinessAlpha  float64
}

// SmoothPersonality returns personality-modulated smoothing alphas.
func SmoothPersonality(p types.PersonalityScale) SmoothingParams {
	return SmoothingParams{
		AffectionAlpha:   SmoothAffection * (0.5 + p.AffectionWarmth),
		ConfidenceAlpha:  SmoothConfidence,
		CuriosityAlpha:   SmoothCuriosity,
		PlayfulnessAlpha: SmoothPlayfulness,
		WorryAlpha:       SmoothWorry * (0.5 + p.WorryTendency),
		AnnoyanceAlpha:   SmoothAnnoyance * p.AnnoyanceSensitivity,
		SleepinessAlpha:  SmoothSleepiness,
		LonelinessAlpha:  SmoothLoneliness,
	}
}

// Blend applies exponential moving average: old*(1-alpha) + new*alpha.
func Blend(old, new, alpha float64) float64 {
	return old*(1-alpha) + new*alpha
}

// BlendVector blends a new evaluation into the current vector using per-dimension alphas.
func BlendVector(current, new types.EmotionVector, p SmoothingParams) types.EmotionVector {
	return types.EmotionVector{
		Affection:   types.Clamp01(Blend(current.Affection, new.Affection, p.AffectionAlpha)),
		Worry:       types.Clamp01(Blend(current.Worry, new.Worry, p.WorryAlpha)),
		Curiosity:   types.Clamp01(Blend(current.Curiosity, new.Curiosity, p.CuriosityAlpha)),
		Sleepiness:  types.Clamp01(Blend(current.Sleepiness, new.Sleepiness, p.SleepinessAlpha)),
		Playfulness: types.Clamp01(Blend(current.Playfulness, new.Playfulness, p.PlayfulnessAlpha)),
		Loneliness:  types.Clamp01(Blend(current.Loneliness, new.Loneliness, p.LonelinessAlpha)),
		Confidence:  types.Clamp01(Blend(current.Confidence, new.Confidence, p.ConfidenceAlpha)),
		Annoyance:   types.Clamp01(Blend(current.Annoyance, new.Annoyance, p.AnnoyanceAlpha)),
	}
}
