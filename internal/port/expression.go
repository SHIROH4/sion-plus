package port

import "context"

// ── Renderer ──

// Renderer controls the avatar visual output (Live2D, VRM, or MMD).
// Implementation: adapter/expression/live2d_controller.go
type Renderer interface {
	// PlayExpression applies an expression to the avatar (e.g., happy, sad, surprised).
	PlayExpression(ctx context.Context, params ExpressionParams) error

	// PlayMotion triggers a named motion animation.
	PlayMotion(ctx context.Context, group string, index int) error

	// ShowBubble displays a chat bubble above the avatar.
	ShowBubble(ctx context.Context, text string, durationSec int) error
}

// ── Speech Synthesizer ──

// SpeechSynthesizer converts text to speech output.
// Implementation: adapter/expression/tts_client.go
type SpeechSynthesizer interface {
	// Speak starts speaking the given text. Non-blocking.
	Speak(ctx context.Context, text string) error

	// IsSpeaking returns true if audio is currently playing.
	IsSpeaking() bool

	// Stop interrupts the current speech.
	Stop()
}
