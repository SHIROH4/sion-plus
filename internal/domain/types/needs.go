package types

// IntrinsicNeeds models 6 homeostatic drives that grow over time
// and are satisfied by specific actions. Inspired by The Sims 4 AI
// need system and Oudeyer & Kaplan (2007) intrinsic motivation theory.
type IntrinsicNeeds struct {
	Companionship float64 `json:"companionship"` // 陪伴需求 — grows when user is active but not interacting
	Rest          float64 `json:"rest"`          // 休息需求 — grows at night / after many actions
	Play          float64 `json:"play"`          // 娱乐需求 — grows when user is gaming/relaxing
	Curiosity     float64 `json:"curiosity"`     // 好奇需求 — grows naturally over time
	Care          float64 `json:"care"`          // 关怀需求 — grows when user seems stressed/working
	Autonomy      float64 `json:"autonomy"`      // 自主需求 — grows when AI hasn't initiated actions
}

// DefaultNeeds returns the baseline need levels.
func DefaultNeeds() IntrinsicNeeds {
	return IntrinsicNeeds{
		Companionship: 0.3,
		Rest:          0.3,
		Play:          0.3,
		Curiosity:     0.4,
		Care:          0.3,
		Autonomy:      0.3,
	}
}

// NeedModulation factors applied to emotion decay/growth rates.
// High unmet needs → slower decay of related emotions.
type NeedModulation struct {
	ConfidenceDecayMul  float64 `json:"confidence_decay_mul"`
	CuriosityDecayMul   float64 `json:"curiosity_decay_mul"`
	PlayfulnessDecayMul float64 `json:"playfulness_decay_mul"`
	WorryDecayMul       float64 `json:"worry_decay_mul"`
	LonelinessDecayMul  float64 `json:"loneliness_decay_mul"`
	SleepinessGrowthMul float64 `json:"sleepiness_growth_mul"`
}
