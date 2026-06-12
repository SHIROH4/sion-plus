package expression

import (
	"math"
	"testing"

	"github.com/SHIROH4/sion-plus/internal/domain/types"
)

func TestEyeOpen(t *testing.T) {
	m := NewEmotionMapper()
	tests := []struct {
		name string
		vec  types.EmotionVector
		want float64
	}{
		{"neutral", types.EmotionVector{Sleepiness: 0, Confidence: 0.5}, 0.85},
		{"very sleepy", types.EmotionVector{Sleepiness: 1.0, Confidence: 0.5}, 0.10},
		{"wide awake", types.EmotionVector{Sleepiness: 0, Confidence: 0.5}, 0.85},
		{"low confidence gaze", types.EmotionVector{Sleepiness: 0, Confidence: 0.0}, 0.79},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := m.eyeOpen(tt.vec)
			if math.Abs(got-tt.want) > 0.01 {
				t.Errorf("eyeOpen = %.3f, want %.3f", got, tt.want)
			}
		})
	}
}

func TestMouthOpen(t *testing.T) {
	m := NewEmotionMapper()
	tests := []struct {
		name string
		vec  types.EmotionVector
		want float64
	}{
		{"neutral", types.EmotionVector{}, 0.03},
		{"playful", types.EmotionVector{Playfulness: 0.8}, 0.174},
		{"annoyed", types.EmotionVector{Annoyance: 0.8}, 0.15},
		{"excited combo", types.EmotionVector{Playfulness: 1.0, Annoyance: 0, Curiosity: 0.5}, 0.25},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := m.mouthOpen(tt.vec)
			if math.Abs(got-tt.want) > 0.01 {
				t.Errorf("mouthOpen = %.3f, want %.3f", got, tt.want)
			}
		})
	}
}

func TestBrowAngle(t *testing.T) {
	m := NewEmotionMapper()
	tests := []struct {
		name string
		vec  types.EmotionVector
		want float64
	}{
		{"neutral", types.EmotionVector{Affection: 0.5, Worry: 0, Annoyance: 0}, 0.0},
		{"happy", types.EmotionVector{Affection: 1.0, Worry: 0, Annoyance: 0}, 0.70},
		{"worried", types.EmotionVector{Affection: 0.5, Worry: 1.0, Annoyance: 0}, -0.70},
		{"angry", types.EmotionVector{Affection: 0.5, Worry: 0, Annoyance: 1.0}, -0.60},
		{"mixed happy-worried", types.EmotionVector{Affection: 0.8, Worry: 0.3, Annoyance: 0}, 0.21},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := m.browAngle(tt.vec)
			if math.Abs(got-tt.want) > 0.01 {
				t.Errorf("browAngle = %.3f, want %.3f", got, tt.want)
			}
		})
	}
}

func TestBlush(t *testing.T) {
	m := NewEmotionMapper()
	tests := []struct {
		name string
		vec  types.EmotionVector
		want float64
	}{
		{"neutral", types.EmotionVector{Affection: 0.5, Confidence: 0.5}, 0.425},
		{"shy affection", types.EmotionVector{Affection: 0.9, Confidence: 0.1}, 0.873},
		{"confident affection", types.EmotionVector{Affection: 0.9, Confidence: 0.9}, 0.657},
		{"no affection", types.EmotionVector{Affection: 0, Confidence: 0.5}, 0.0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := m.blush(tt.vec)
			if math.Abs(got-tt.want) > 0.01 {
				t.Errorf("blush = %.3f, want %.3f", got, tt.want)
			}
		})
	}
}

func TestHeadTilt(t *testing.T) {
	m := NewEmotionMapper()
	tests := []struct {
		name string
		vec  types.EmotionVector
		want float64
	}{
		{"neutral", types.EmotionVector{Curiosity: 0.5, Playfulness: 0.5, Loneliness: 0}, 0.0},
		{"curious", types.EmotionVector{Curiosity: 1.0, Playfulness: 0.5, Loneliness: 0}, 0.40},
		{"lonely", types.EmotionVector{Curiosity: 0.5, Playfulness: 0.5, Loneliness: 1.0}, -0.20},
		{"curious & playful", types.EmotionVector{Curiosity: 0.9, Playfulness: 0.9, Loneliness: 0}, 0.44},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := m.headTilt(tt.vec)
			if math.Abs(got-tt.want) > 0.01 {
				t.Errorf("headTilt = %.3f, want %.3f", got, tt.want)
			}
		})
	}
}

func TestBreathRate(t *testing.T) {
	m := NewEmotionMapper()
	tests := []struct {
		name string
		vec  types.EmotionVector
		want float64
	}{
		{"neutral", types.EmotionVector{}, 0.45},
		{"excited", types.EmotionVector{Playfulness: 1.0}, 0.80},
		{"sleepy", types.EmotionVector{Sleepiness: 1.0}, 0.20},
		{"worried", types.EmotionVector{Worry: 1.0}, 0.60},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := m.breathRate(tt.vec)
			if math.Abs(got-tt.want) > 0.01 {
				t.Errorf("breathRate = %.3f, want %.3f", got, tt.want)
			}
		})
	}
}

func TestMotion(t *testing.T) {
	m := NewEmotionMapper()
	tests := []struct {
		name string
		vec  types.EmotionVector
		want string
	}{
		{"neutral", types.EmotionVector{}, "idle"},
		{"sleepy", types.EmotionVector{Sleepiness: 0.8}, "sleepy"},
		{"angry", types.EmotionVector{Annoyance: 0.7}, "angry"},
		{"sad", types.EmotionVector{Worry: 0.7, Loneliness: 0.5}, "sad"},
		{"excited", types.EmotionVector{Playfulness: 0.8, Affection: 0.6}, "excited"},
		{"curious", types.EmotionVector{Curiosity: 0.8}, "curious"},
		{"shy", types.EmotionVector{Affection: 0.8, Confidence: 0.3}, "shy"},
		{"happy", types.EmotionVector{Affection: 0.7}, "happy"},
		{"priority: sleepy beats annoyed", types.EmotionVector{Sleepiness: 0.8, Annoyance: 0.7}, "sleepy"},
		{"priority: annoyed beats worried", types.EmotionVector{Annoyance: 0.7, Worry: 0.7}, "angry"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := m.motion(tt.vec)
			if got != tt.want {
				t.Errorf("motion = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestMapToParameters_Integration(t *testing.T) {
	m := NewEmotionMapper()

	params, err := m.MapToParameters(types.EmotionVector{
		Affection:   0.6,
		Worry:       0.1,
		Curiosity:   0.5,
		Sleepiness:  0.3,
		Playfulness: 0.7,
		Loneliness:  0.1,
		Confidence:  0.5,
		Annoyance:   0.05,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if params == nil {
		t.Fatal("expected non-nil params")
	}

	// All values should be within valid ranges
	if params.EyeOpen < 0 || params.EyeOpen > 1 {
		t.Errorf("EyeOpen out of range: %.3f", params.EyeOpen)
	}
	if params.MouthOpen < 0 || params.MouthOpen > 1 {
		t.Errorf("MouthOpen out of range: %.3f", params.MouthOpen)
	}
	if params.BrowAngle < -1 || params.BrowAngle > 1 {
		t.Errorf("BrowAngle out of range: %.3f", params.BrowAngle)
	}
	if params.BlushIntensity < 0 || params.BlushIntensity > 1 {
		t.Errorf("BlushIntensity out of range: %.3f", params.BlushIntensity)
	}
	if params.HeadTilt < -1 || params.HeadTilt > 1 {
		t.Errorf("HeadTilt out of range: %.3f", params.HeadTilt)
	}
	if params.BreathRate < 0 || params.BreathRate > 1 {
		t.Errorf("BreathRate out of range: %.3f", params.BreathRate)
	}
	if params.Motion == "" {
		t.Error("Motion should not be empty")
	}
}

func TestMapToParameters_AllExtremes(t *testing.T) {
	m := NewEmotionMapper()

	extremes := []struct {
		name string
		vec  types.EmotionVector
	}{
		{"all zeros", types.EmotionVector{}},
		{"all ones", types.EmotionVector{Affection: 1, Worry: 1, Curiosity: 1, Sleepiness: 1, Playfulness: 1, Loneliness: 1, Confidence: 1, Annoyance: 1}},
		{"all neutral", types.EmotionVector{Affection: 0.5, Curiosity: 0.5, Playfulness: 0.5, Confidence: 0.5}},
	}

	for _, tt := range extremes {
		t.Run(tt.name, func(t *testing.T) {
			params, err := m.MapToParameters(tt.vec)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if params.EyeOpen < 0 || params.EyeOpen > 1 {
				t.Errorf("EyeOpen out of range: %.3f", params.EyeOpen)
			}
			if params.MouthOpen < 0 || params.MouthOpen > 1 {
				t.Errorf("MouthOpen out of range: %.3f", params.MouthOpen)
			}
			if params.BrowAngle < -1 || params.BrowAngle > 1 {
				t.Errorf("BrowAngle out of range: %.3f", params.BrowAngle)
			}
			if params.BlushIntensity < 0 || params.BlushIntensity > 1 {
				t.Errorf("BlushIntensity out of range: %.3f", params.BlushIntensity)
			}
			if params.HeadTilt < -1 || params.HeadTilt > 1 {
				t.Errorf("HeadTilt out of range: %.3f", params.HeadTilt)
			}
			if params.BreathRate < 0 || params.BreathRate > 1 {
				t.Errorf("BreathRate out of range: %.3f", params.BreathRate)
			}
		})
	}
}

func TestMapToParameters_ReturnsCopy(t *testing.T) {
	m := NewEmotionMapper()
	vec := types.EmotionVector{Affection: 0.8}

	p1, _ := m.MapToParameters(vec)
	p2, _ := m.MapToParameters(vec)

	// Should return independent structs
	p1.EyeOpen = 0.123
	if p2.EyeOpen == 0.123 {
		t.Error("expected independent copies, got same pointer")
	}
}

func BenchmarkMapToParameters(b *testing.B) {
	m := NewEmotionMapper()
	vec := types.EmotionVector{
		Affection: 0.6, Worry: 0.1, Curiosity: 0.5, Sleepiness: 0.3,
		Playfulness: 0.7, Loneliness: 0.1, Confidence: 0.5, Annoyance: 0.05,
	}
	b.ResetTimer()
	for b.Loop() {
		m.MapToParameters(vec)
	}
}
