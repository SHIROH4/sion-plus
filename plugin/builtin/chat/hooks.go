// Package chat: hook implementations for OnBeforeChat / OnAfterChat.
package chat

import (
	"context"
	"fmt"
	"strings"

	"github.com/shirohania/sion/internal/domain/types"
	"github.com/shirohania/sion/plugin/sdk"
)

// BeforeChatHook builds the system prompt and injects relevant memory
// before each chat turn.
func (p *Plugin) BeforeChatHook(ctx context.Context, userMsg string) (string, error) {
	if p.pctx == nil {
		return "", nil
	}

	var parts []string
	parts = append(parts, "You are Sion, a desktop AI companion.")

	// Inject active strategies
	if p.pctx.MemoryStore != nil {
		strategies, err := p.pctx.MemoryStore.ListActiveStrategies(ctx)
		if err == nil && len(strategies) > 0 {
			parts = append(parts, "\n## Behavioral Strategies")
			for _, s := range strategies {
				parts = append(parts, fmt.Sprintf("- When %s: %s (avoid: %s)",
					s.Situation, s.GoodStrategy, s.BadStrategy))
			}
		}
	}

	// Inject recent facts
	if p.pctx.MemoryRecall != nil {
		results, err := p.pctx.MemoryRecall.HybridSearch(ctx, userMsg, 3)
		if err == nil {
			for _, r := range results {
				parts = append(parts, fmt.Sprintf("[memory] %s", r.Content))
			}
		}
	}

	return strings.Join(parts, "\n"), nil
}

// AfterChatHook is called after each chat turn to update memory.
func (p *Plugin) AfterChatHook(ctx context.Context, userMsg, response string) {
	if p.pctx == nil || p.pctx.MemoryStore == nil {
		return
	}
	// Save to chat history
	msgs := []types.Message{
		{Role: "user", Content: userMsg},
		{Role: "assistant", Content: response},
	}
	_ = p.pctx.MemoryStore.SaveHistory(ctx, msgs)

	// Notify emotion of activity
	if p.pctx.EmotionStateManager != nil {
		p.pctx.EmotionStateManager.NotifyActivity()
	}
}

var _ sdk.Plugin = (*Plugin)(nil)
