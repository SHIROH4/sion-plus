package llm

import (
	"context"
	"log"

	"github.com/shirohania/sion/internal/port"
)

// TrackedExecutor wraps an LLMExecutor and records token usage after each call.
// Estimates tokens from char counts (crude but zero-cost until usage fields are parsed from API).
type TrackedExecutor struct {
	inner   port.LLMExecutor
	tracker *TokenTracker
	label   string // "chat"|"emotion"|"memory_extraction"|"memory_signal"|"compression"
}

var _ port.LLMExecutor = (*TrackedExecutor)(nil)

// WrapExecutor wraps an LLMExecutor with token tracking.
// label identifies the call type for dashboards: "chat", "emotion", "memory_extraction",
// "memory_signal", "reflection_and_diary", "compression", etc.
func WrapExecutor(inner port.LLMExecutor, tracker *TokenTracker, label string) *TrackedExecutor {
	return &TrackedExecutor{inner: inner, tracker: tracker, label: label}
}

func (t *TrackedExecutor) Chat(ctx context.Context, systemPrompt string, msgs []port.LLMMessage) (string, error) {
	promptChars := promptCharCount(systemPrompt, msgs)
	resp, err := t.inner.Chat(ctx, systemPrompt, msgs)
	if err != nil {
		t.record(ctx, promptChars, 0)
		return resp, err
	}
	t.record(ctx, promptChars, len(resp))
	return resp, nil
}

func (t *TrackedExecutor) ChatWithTools(ctx context.Context, systemPrompt string, msgs []port.LLMMessage, tools []port.ToolDef, onToolCall func(string, string) string, maxRounds int, toolChoice string) (string, error) {
	promptChars := promptCharCount(systemPrompt, msgs)
	resp, err := t.inner.ChatWithTools(ctx, systemPrompt, msgs, tools, onToolCall, maxRounds, toolChoice)
	if err != nil {
		t.record(ctx, promptChars, 0)
		return resp, err
	}
	t.record(ctx, promptChars, len(resp))
	return resp, nil
}

func (t *TrackedExecutor) ChatStream(ctx context.Context, systemPrompt string, msgs []port.LLMMessage, onChunk func(string) error) error {
	var totalChars int
	wrapped := func(chunk string) error {
		totalChars += len(chunk)
		return onChunk(chunk)
	}
	promptChars := promptCharCount(systemPrompt, msgs)
	err := t.inner.ChatStream(ctx, systemPrompt, msgs, wrapped)
	t.record(ctx, promptChars, totalChars)
	return err
}

func (t *TrackedExecutor) IsAvailable(ctx context.Context) bool {
	return t.inner.IsAvailable(ctx)
}

func (t *TrackedExecutor) record(ctx context.Context, promptChars, completionChars int) {
	// Rough estimate: ~4 chars per token for CJK, ~3 for Latin. Use 3.5 as avg.
	promptTokens := promptChars * 10 / 35  // chars / 3.5
	completionTokens := completionChars * 10 / 35
	if promptTokens < 1 {
		promptTokens = 1
	}
	t.tracker.Record(ctx, t.label, promptTokens, completionTokens)
}

// promptCharCount sums the character count of system prompt + all messages.
func promptCharCount(systemPrompt string, msgs []port.LLMMessage) int {
	n := len(systemPrompt)
	for _, m := range msgs {
		n += len(m.Content)
	}
	return n
}

// NewTrackedGateway is a convenience constructor: gateway + tracker = tracked executor.
func NewTrackedGateway(cfg GatewayConfig, tracker *TokenTracker, label string) *TrackedExecutor {
	gw := NewOpenAIGateway(cfg)
	log.Printf("[TrackedExecutor] %s: wrapped %s @ %s", label, cfg.Model, cfg.BaseURL)
	return WrapExecutor(gw, tracker, label)
}
