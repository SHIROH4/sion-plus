package emotion

import (
	"math"

	"github.com/SHIROH4/sion-plus/internal/domain/types"
)

// ── Neutral Points (decay attractors) ──

const (
	NeutralAffection   = 0.5
	NeutralConfidence  = 0.5
	NeutralCuriosity   = 0.5
	NeutralPlayfulness = 0.5
	NeutralWorry       = 0.0
	NeutralAnnoyance   = 0.0
	NeutralLoneliness  = 0.0
)

// ── Decay Rates (per hour, higher = faster drift) ──

const (
	DecayAffection   = 0.005
	DecayConfidence  = 0.008
	DecayCuriosity   = 0.05
	DecayPlayfulness = 0.05
	DecayWorry       = 0.08
	DecayAnnoyance   = 0.12
	DecayLoneliness  = 0.03
)

// DecayToward drifts a value toward its neutral point.
func DecayToward(current, neutral, ratePerHr, elapsedHours float64) float64 {
	delta := (neutral - current) * math.Min(ratePerHr*elapsedHours, 1.0)
	return current + delta
}

// DecayVector applies decay to all dimensions.
func DecayVector(
	vec types.EmotionVector,
	elapsedHours float64,
	isAsleep bool,
	mod *types.NeedModulation,
) types.EmotionVector {
	decayMult := 1.0
	if isAsleep {
		decayMult = 0.15
	}

	confDecayMul := 1.0
	curDecayMul := 1.0
	playDecayMul := 1.0
	worryDecayMul := 1.0
	loneDecayMul := 1.0
	if mod != nil {
		confDecayMul = mod.ConfidenceDecayMul
		curDecayMul = mod.CuriosityDecayMul
		playDecayMul = mod.PlayfulnessDecayMul
		worryDecayMul = mod.WorryDecayMul
		loneDecayMul = mod.LonelinessDecayMul
	}

	return types.EmotionVector{
		Affection:   DecayToward(vec.Affection, NeutralAffection, DecayAffection*decayMult, elapsedHours),
		Worry:       DecayToward(vec.Worry, NeutralWorry, DecayWorry*decayMult*worryDecayMul, elapsedHours),
		Curiosity:   DecayToward(vec.Curiosity, NeutralCuriosity, DecayCuriosity*decayMult*curDecayMul, elapsedHours),
		Playfulness: DecayToward(vec.Playfulness, NeutralPlayfulness, DecayPlayfulness*decayMult*playDecayMul, elapsedHours),
		Confidence:  DecayToward(vec.Confidence, NeutralConfidence, DecayConfidence*decayMult*confDecayMul, elapsedHours),
		Annoyance:   DecayToward(vec.Annoyance, NeutralAnnoyance, DecayAnnoyance*decayMult, elapsedHours),
		Loneliness:  DecayToward(vec.Loneliness, NeutralLoneliness, DecayLoneliness*decayMult*loneDecayMul, elapsedHours),
		Sleepiness:  vec.Sleepiness, // circadian-driven, handled externally
	}
}
