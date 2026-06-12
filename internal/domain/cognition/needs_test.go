package cognition

import (
	"testing"

	"github.com/shirohania/sion/internal/domain/types"
)

func TestNewNeedModel(t *testing.T) {
	nm := NewNeedModel()
	needs := nm.Current()
	if needs.Companionship < 0 || needs.Companionship > 1 {
		t.Errorf("companionship out of range: %.2f", needs.Companionship)
	}
	if needs.Curiosity < 0 || needs.Curiosity > 1 {
		t.Errorf("curiosity out of range: %.2f", needs.Curiosity)
	}
	if needs.Care < 0 || needs.Care > 1 {
		t.Errorf("care out of range: %.2f", needs.Care)
	}
}

func TestNeedModelGrow(t *testing.T) {
	nm := NewNeedModel()
	initial := nm.Current()

	// Grow for 10 hours
	nm.Grow(10)
	after := nm.Current()

	// Curiosity should grow toward baseline
	if after.Curiosity < initial.Curiosity {
		t.Errorf("curiosity should grow after 10h: %.3f → %.3f", initial.Curiosity, after.Curiosity)
	}
	// Autonomy should grow
	if after.Autonomy < initial.Autonomy {
		t.Errorf("autonomy should grow after 10h: %.3f → %.3f", initial.Autonomy, after.Autonomy)
	}
}

func TestNeedModelSatisfy(t *testing.T) {
	nm := NewNeedModel()

	// Satisfy companionship through social action
	nm.Satisfy("speak_casual", types.OutcomeReplied)
	needs := nm.Current()
	if needs.Companionship > 0.8 {
		t.Errorf("companionship should be reduced after speak_casual: %.2f", needs.Companionship)
	}
	if needs.Play > 0.9 {
		t.Errorf("play should be reduced: %.2f", needs.Play)
	}
}

func TestNeedModelSatisfyCare(t *testing.T) {
	nm := NewNeedModel()

	nm.Satisfy("care_rest", types.OutcomeReplied)
	needs := nm.Current()
	if needs.Care > 0.8 {
		t.Errorf("care need should be reduced after care action: %.2f", needs.Care)
	}
}

func TestNeedModelSatisfyRejectedNoEffect(t *testing.T) {
	nm := NewNeedModel()
	initial := nm.Current()

	nm.Satisfy("speak_casual", types.OutcomeRejected)
	after := nm.Current()

	if initial.Companionship != after.Companionship {
		t.Error("rejected action should not satisfy needs")
	}
}

func TestNeedModelSatisfyQuiet(t *testing.T) {
	nm := NewNeedModel()

	// Increase rest need first
	needs := nm.Current()
	needs.Rest = 0.8

	nm.Satisfy("care_quiet", types.OutcomeReplied)
	after := nm.Current()
	if after.Rest > 0.7 {
		t.Errorf("rest need should be reduced after quiet action: %.2f", after.Rest)
	}
}

func TestNeedModelModulation(t *testing.T) {
	nm := NewNeedModel()
	mod := nm.Modulation()

	if mod.ConfidenceDecayMul < 0.9 || mod.ConfidenceDecayMul > 1.5 {
		t.Errorf("confidence modulation out of expected range: %.2f", mod.ConfidenceDecayMul)
	}
	if mod.WorryDecayMul < 0.9 || mod.WorryDecayMul > 2.0 {
		t.Errorf("worry modulation out of expected range: %.2f", mod.WorryDecayMul)
	}
	if mod.SleepinessGrowthMul < 0.9 || mod.SleepinessGrowthMul > 2.0 {
		t.Errorf("sleepiness modulation out of expected range: %.2f", mod.SleepinessGrowthMul)
	}
}

func TestNeedModelReset(t *testing.T) {
	nm := NewNeedModel()
	nm.Grow(20)
	nm.Reset()

	needs := nm.Current()
	def := types.DefaultNeeds()
	if needs.Companionship != def.Companionship {
		t.Error("reset should restore default needs")
	}
}

func TestDecayToward(t *testing.T) {
	result := decayToward(0.8, 0.5, 0.1)
	if result < 0.75 || result > 0.8 {
		t.Errorf("decayToward(0.8, 0.5, 0.1) = %.3f, expected ~0.77", result)
	}
}
