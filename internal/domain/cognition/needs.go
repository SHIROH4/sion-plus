package cognition

import "github.com/SHIROH4/sion-plus/internal/domain/types"

// NeedModelImpl manages 6 intrinsic needs with homeostatic dynamics.
// Satisfy() reduces a need; Grow() drifts toward baseline with natural growth.
type NeedModelImpl struct {
	needs       types.IntrinsicNeeds
	baseline    types.IntrinsicNeeds
	growthRates map[string]float64
}

func NewNeedModel() *NeedModelImpl {
	return &NeedModelImpl{
		needs:    types.DefaultNeeds(),
		baseline: types.DefaultNeeds(),
		growthRates: map[string]float64{
			"companionship": 0.03,
			"curiosity":     0.05,
			"care":          0.05,
			"play":          0.04,
			"rest":          0.02,
			"autonomy":      0.02,
		},
	}
}

func (n *NeedModelImpl) Grow(elapsedHours float64) {
	decayRate := 0.03 * elapsedHours
	n.needs.Companionship = decayToward(n.needs.Companionship, n.baseline.Companionship, decayRate)
	n.needs.Rest = decayToward(n.needs.Rest, n.baseline.Rest, decayRate)
	n.needs.Play = decayToward(n.needs.Play, n.baseline.Play, decayRate)
	n.needs.Curiosity = decayToward(n.needs.Curiosity, n.baseline.Curiosity, decayRate)
	n.needs.Care = decayToward(n.needs.Care, n.baseline.Care, decayRate)
	n.needs.Autonomy = decayToward(n.needs.Autonomy, n.baseline.Autonomy, decayRate)

	n.needs.Curiosity += n.growthRates["curiosity"] * elapsedHours
	n.needs.Autonomy += n.growthRates["autonomy"] * elapsedHours

	clampNeeds(&n.needs)
}

func (n *NeedModelImpl) Satisfy(action string, outcome types.OutcomeResult) {
	if outcome == types.OutcomeRejected {
		return
	}
	switch {
	case action == "speak_casual" || action == "speak_inquiry" || action == "greet_return":
		n.needs.Companionship = clamp01(n.needs.Companionship - 0.3)
		n.needs.Play = clamp01(n.needs.Play - 0.15)
	case action == "speak_care" || action == "care_rest" || action == "care_meal" ||
		action == "care_hydration" || action == "care_health" || action == "care_encourage":
		n.needs.Care = clamp01(n.needs.Care - 0.3)
		n.needs.Companionship = clamp01(n.needs.Companionship - 0.1)
	case action == "search" || action == "observe" || action == "reflect" || action == "analyze_patterns":
		n.needs.Curiosity = clamp01(n.needs.Curiosity - 0.35)
		n.needs.Autonomy = clamp01(n.needs.Autonomy - 0.2)
	case action == "none" || action == "care_quiet":
		n.needs.Rest = clamp01(n.needs.Rest - 0.15)
	}
}

func (n *NeedModelImpl) Current() *types.IntrinsicNeeds {
	c := n.needs
	return &c
}

func (n *NeedModelImpl) Modulation() *types.NeedModulation {
	return &types.NeedModulation{
		ConfidenceDecayMul:  1.0 + n.needs.Autonomy*0.3,
		CuriosityDecayMul:   1.0 + (1.0-n.needs.Curiosity)*0.2,
		PlayfulnessDecayMul: 1.0 + n.needs.Play*0.3,
		WorryDecayMul:       1.0 + n.needs.Care*0.5,
		LonelinessDecayMul:  1.0 + (1.0-n.needs.Companionship)*0.4,
		SleepinessGrowthMul: 1.0 + n.needs.Rest*0.6,
	}
}

func (n *NeedModelImpl) Reset() { n.needs = types.DefaultNeeds() }

func decayToward(current, target, rate float64) float64 {
	return current + (target-current)*rate
}

func clampNeeds(n *types.IntrinsicNeeds) {
	n.Companionship = clamp01(n.Companionship)
	n.Rest = clamp01(n.Rest)
	n.Play = clamp01(n.Play)
	n.Curiosity = clamp01(n.Curiosity)
	n.Care = clamp01(n.Care)
	n.Autonomy = clamp01(n.Autonomy)
}
