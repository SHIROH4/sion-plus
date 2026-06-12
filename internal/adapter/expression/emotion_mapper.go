package expression

import (
	"github.com/shirohania/sion/internal/domain/types"
	"github.com/shirohania/sion/internal/port"
)

// EmotionMapper implements port.ExpressionMapper.
// Maps the 8D internal emotion vector to Live2D/VRM expression parameters
// using weighted linear combinations with clamping.
type EmotionMapper struct{}

var _ port.ExpressionMapper = (*EmotionMapper)(nil)

func NewEmotionMapper() *EmotionMapper {
	return &EmotionMapper{}
}

func (m *EmotionMapper) MapToParameters(vec types.EmotionVector) (*port.ExpressionParams, error) {
	return &port.ExpressionParams{
		EyeOpen:        m.eyeOpen(vec),
		MouthOpen:      m.mouthOpen(vec),
		BrowAngle:      m.browAngle(vec),
		BlushIntensity: m.blush(vec),
		HeadTilt:       m.headTilt(vec),
		BreathRate:     m.breathRate(vec),
		Motion:         m.motion(vec),
	}, nil
}

// eyeOpen: sleepiness is the primary driver. Low confidence slightly averts gaze.
func (m *EmotionMapper) eyeOpen(vec types.EmotionVector) float64 {
	v := 0.85 - vec.Sleepiness*0.75 - (0.5-vec.Confidence)*0.12
	return types.Clamp01(v)
}

// mouthOpen: driven by playfulness (smiling/talking) and annoyance (complaining).
func (m *EmotionMapper) mouthOpen(vec types.EmotionVector) float64 {
	v := 0.03 + vec.Playfulness*0.18 + vec.Annoyance*0.15 + vec.Curiosity*0.08
	return types.Clamp01(v)
}

// browAngle: positive = raised (happy), negative = furrowed (sad/angry).
// Affection lifts brows, worry and annoyance furrow them.
func (m *EmotionMapper) browAngle(vec types.EmotionVector) float64 {
	v := (vec.Affection-0.5)*1.4 - vec.Worry*0.7 - vec.Annoyance*0.6
	return types.Clamp1(v)
}

// blush: affection drives blush. Low confidence amplifies it (shy affection).
func (m *EmotionMapper) blush(vec types.EmotionVector) float64 {
	shy := 1.0 - vec.Confidence
	v := vec.Affection*0.7 + vec.Affection*shy*0.3
	return types.Clamp01(v)
}

// headTilt: curiosity and playfulness tilt the head. Loneliness tilts it down.
func (m *EmotionMapper) headTilt(vec types.EmotionVector) float64 {
	v := (vec.Curiosity-0.5)*0.8 + (vec.Playfulness-0.5)*0.3 - vec.Loneliness*0.2
	return types.Clamp1(v)
}

// breathRate: excitement (playfulness) raises rate, sleepiness and worry modulate.
func (m *EmotionMapper) breathRate(vec types.EmotionVector) float64 {
	v := 0.45 + vec.Playfulness*0.35 - vec.Sleepiness*0.25 + vec.Worry*0.15
	return types.Clamp01(v)
}

// motion selects a named animation based on the dominant emotion dimensions.
func (m *EmotionMapper) motion(vec types.EmotionVector) string {
	switch {
	case vec.Sleepiness > 0.75:
		return "sleepy"
	case vec.Annoyance > 0.6:
		return "angry"
	case vec.Worry > 0.6 && vec.Loneliness > 0.4:
		return "sad"
	case vec.Playfulness > 0.7 && vec.Affection > 0.5:
		return "excited"
	case vec.Curiosity > 0.75:
		return "curious"
	case vec.Affection > 0.7 && vec.Confidence < 0.4:
		return "shy"
	case vec.Affection > 0.6:
		return "happy"
	default:
		return "idle"
	}
}
