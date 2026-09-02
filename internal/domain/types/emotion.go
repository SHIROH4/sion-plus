package types

// ── PAD 3D Emotional State ──

// EmotionState is the PAD (Pleasure-Arousal-Dominance) model output.
// This is the externally visible emotional expression, derived from
// the internal 8D EmotionVector.
type EmotionState struct {
	SchemaVersion int     `json:"schema_version"`
	Valence       float64 `json:"valence"`   // -1~+1, pleasure-displeasure
	Arousal       float64 `json:"arousal"`   // -1~+1, excitation-calm
	Dominance     float64 `json:"dominance"` // -1~+1, control-submission
	Primary       string  `json:"primary"`   // "joy"|"sadness"|"anger"|"fear"|"surprise"|"neutral"
	Intensity     float64 `json:"intensity"` // 0~1
}

// ── 8D Emotion Vector (Internal State) ──

// EmotionVector is the internal emotional state driving the AI's personality.
// Each dimension has its own decay rate, neutral point, and smoothing alpha.
// The external PAD state is computed from this vector, not stored independently.
type EmotionVector struct {
	Affection   float64 `json:"affection"`   // 0~1, neutral=0.5
	Worry       float64 `json:"worry"`       // 0~1, neutral=0.0
	Curiosity   float64 `json:"curiosity"`   // 0~1, neutral=0.5
	Sleepiness  float64 `json:"sleepiness"`  // 0~1
	Playfulness float64 `json:"playfulness"` // 0~1, neutral=0.5
	Loneliness  float64 `json:"loneliness"`  // 0~1, neutral=0.0
	Confidence  float64 `json:"confidence"`  // 0~1, neutral=0.5
	Annoyance   float64 `json:"annoyance"`   // 0~1, neutral=0.0
}

// ── Personality ──

// PersonalityScale modulates emotional reactivity.
// These parameters are LEARNED from interaction outcomes, not hardcoded.
type PersonalityScale struct {
	AnnoyanceSensitivity float64 `json:"annoyance_sensitivity"` // 0~1: how easily annoyed
	AffectionWarmth      float64 `json:"affection_warmth"`      // 0~1: how quickly affection grows
	WorryTendency        float64 `json:"worry_tendency"`        // 0~1: how much the AI worries
}

// DefaultPersonality returns a balanced starting personality.
func DefaultPersonality() PersonalityScale {
	return PersonalityScale{
		AnnoyanceSensitivity: 0.5,
		AffectionWarmth:      0.5,
		WorryTendency:        0.5,
	}
}

// ── Math Helpers ──

// Clamp01 clamps v to [0, 1].
func Clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

// Clamp1 clamps v to [-1, 1].
func Clamp1(v float64) float64 {
	if v < -1 {
		return -1
	}
	if v > 1 {
		return 1
	}
	return v
}
