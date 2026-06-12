package types

// EmotionDelta is a signed change to each of the 8 internal emotion dimensions.
// Positive = increase toward 1, negative = decrease toward 0.
// Each value is clamped to [-1, +1] by the store.
type EmotionDelta struct {
	Affection   float64 `json:"affection"`
	Worry       float64 `json:"worry"`
	Curiosity   float64 `json:"curiosity"`
	Sleepiness  float64 `json:"sleepiness"`
	Playfulness float64 `json:"playfulness"`
	Loneliness  float64 `json:"loneliness"`
	Confidence  float64 `json:"confidence"`
	Annoyance   float64 `json:"annoyance"`
	Source      string  `json:"source,omitempty"` // "chat", "screen", "proactive", "system"
	Reason      string  `json:"reason,omitempty"`  // LLM self-explanation (debug aid)
}

// ClampDelta bounds each dimension to [-1, 1].
func (d *EmotionDelta) ClampDelta() {
	d.Affection = Clamp1(d.Affection)
	d.Worry = Clamp1(d.Worry)
	d.Curiosity = Clamp1(d.Curiosity)
	d.Sleepiness = Clamp1(d.Sleepiness)
	d.Playfulness = Clamp1(d.Playfulness)
	d.Loneliness = Clamp1(d.Loneliness)
	d.Confidence = Clamp1(d.Confidence)
	d.Annoyance = Clamp1(d.Annoyance)
}
