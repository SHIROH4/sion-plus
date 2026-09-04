package llm

import (
	"context"
	"testing"

	"github.com/SHIROH4/sion-plus/internal/port"
)

type fixedExecutor struct{}

func (fixedExecutor) Chat(context.Context, string, []port.LLMMessage) (string, error) {
	return "ok", nil
}
func (fixedExecutor) ChatWithTools(context.Context, string, []port.LLMMessage, []port.ToolDef, func(string, string) string, int, string) (string, error) {
	return "ok", nil
}
func (fixedExecutor) ChatStream(_ context.Context, _ string, _ []port.LLMMessage, onChunk func(string) error) error {
	return onChunk("ok")
}
func (fixedExecutor) IsAvailable(context.Context) bool { return true }

func TestTrackedExecutorUsesContextCallType(t *testing.T) {
	tracker := NewTokenTracker(t.TempDir())
	executor := WrapExecutor(fixedExecutor{}, tracker, "chat")
	ctx := port.WithLLMCallMetadata(context.Background(), "memory", "fact_extract")

	if _, err := executor.Chat(ctx, "prompt", nil); err != nil {
		t.Fatalf("Chat: %v", err)
	}

	tracker.mu.Lock()
	defer tracker.mu.Unlock()
	if len(tracker.buffer) != 1 {
		t.Fatalf("records=%d, want 1", len(tracker.buffer))
	}
	if got := tracker.buffer[0].CallType; got != "fact_extract" {
		t.Fatalf("call_type=%q, want fact_extract", got)
	}
}

func TestTrackedExecutorFallsBackToDefaultCallType(t *testing.T) {
	tracker := NewTokenTracker(t.TempDir())
	executor := WrapExecutor(fixedExecutor{}, tracker, "chat")

	if _, err := executor.Chat(context.Background(), "prompt", nil); err != nil {
		t.Fatalf("Chat: %v", err)
	}

	tracker.mu.Lock()
	defer tracker.mu.Unlock()
	if got := tracker.buffer[0].CallType; got != "chat" {
		t.Fatalf("call_type=%q, want chat", got)
	}
}
