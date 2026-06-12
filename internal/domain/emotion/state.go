package emotion

import (
	"math"

	"github.com/SHIROH4/sion-plus/internal/domain/types"
)

// ── PAD Computation ──

// ComputeState derives PAD (Pleasure-Arousal-Dominance) + primary emotion + intensity
// from the 8D internal emotion vector.
func ComputeState(vec types.EmotionVector) types.EmotionState {
	valence := types.Clamp1(vec.Affection - vec.Annoyance)
	arousal := types.Clamp1((vec.Playfulness+vec.Curiosity)/2 - vec.Sleepiness)
	dominance := types.Clamp1(vec.Confidence - 0.5 - vec.Worry*0.3)
	intensity := maxDeviation(vec)
	primary := InferPrimary(vec)

	return types.EmotionState{
		Valence:   valence,
		Arousal:   arousal,
		Dominance: dominance,
		Primary:   primary,
		Intensity: intensity,
	}
}

func maxDeviation(v types.EmotionVector) float64 {
	deviations := []float64{
		math.Abs(v.Affection-0.5) / 0.5,
		math.Abs(v.Worry) / 0.7,
		math.Abs(v.Curiosity-0.5) / 0.5,
		math.Abs(v.Playfulness-0.5) / 0.5,
		math.Abs(v.Loneliness) / 0.7,
		math.Abs(v.Confidence-0.5) / 0.5,
		math.Abs(v.Annoyance) / 0.7,
	}
	max := 0.0
	for _, d := range deviations {
		if d > max {
			max = d
		}
	}
	return types.Clamp01(max)
}

// InferPrimary maps the 8D vector to a primary emotion label.
func InferPrimary(v types.EmotionVector) string {
	switch {
	case v.Annoyance > 0.5:
		return "anger"
	case v.Worry > 0.5:
		return "fear"
	case v.Sleepiness > 0.7:
		return "neutral"
	case v.Playfulness > 0.6:
		return "joy"
	case v.Affection > 0.7:
		return "joy"
	case v.Loneliness > 0.6:
		return "sadness"
	case v.Curiosity > 0.6:
		return "surprise"
	default:
		return "neutral"
	}
}
