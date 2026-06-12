package port

import (
	"context"
	"github.com/SHIROH4/sion-plus/internal/domain/types"
)

// ── Emotion State Manager ──

// EmotionStateManager is the persistent, stateful emotion model.
// It maintains the 8D internal vector and derives the PAD external state.
// A background goroutine decays emotions toward neutral every 5 minutes.
type EmotionStateManager interface {
	Current() (types.EmotionState, types.EmotionVector)
	Evaluate(ctx context.Context, recentTurns string) error
	NotifyActivity()
	ApplyDelta(delta *types.EmotionDelta)
	SetPersonality(p types.PersonalityScale)
	Personality() types.PersonalityScale
	LearnPersonality(ctx context.Context) error
	SetNeedModulation(mod *types.NeedModulation)
	History() ([]types.EmotionState, []types.EmotionVector)
	Load(ctx context.Context) error
	Save(ctx context.Context) error
	Start()
	Stop()
}

// ── Emotion Signal Source ──

// EmotionSignalSource evaluates external input into an emotion delta.
// Each source (chat, screen, proactive) has its own implementation
// but produces a unified EmotionDelta wrapped in EmotionEvalResult.
type EmotionSignalSource interface {
	Name() string
	Evaluate(ctx context.Context, input *EmotionEvalInput) (*EmotionEvalResult, error)
}

// EmotionEvalInput is the standard input for all emotion sources.
type EmotionEvalInput struct {
	SourceType  string // "chat", "screen", "proactive"
	CurrentMsg  string // the current message or event description
	RecentTurns string // formatted recent context
	SelfProfile string // optional self-model context (v3)
}

// EmotionEvalResult wraps the delta with metadata for the chat pipeline.
type EmotionEvalResult struct {
	Delta  *types.EmotionDelta
	State  types.EmotionState
	Vector types.EmotionVector
	Source string // "llm" | "rule"
}

// ── Expression Mapper ──

// ExpressionMapper maps the 8D emotion vector to Live2D/VRM expression parameters.
type ExpressionMapper interface {
	MapToParameters(vec types.EmotionVector) (*ExpressionParams, error)
}

type ExpressionParams struct {
	Motion         string  `json:"motion"`
	EyeOpen        float64 `json:"eye_open"`
	MouthOpen      float64 `json:"mouth_open"`
	BrowAngle      float64 `json:"brow_angle"`
	BlushIntensity float64 `json:"blush_intensity"`
	HeadTilt       float64 `json:"head_tilt"`
	BreathRate     float64 `json:"breath_rate"`
}
