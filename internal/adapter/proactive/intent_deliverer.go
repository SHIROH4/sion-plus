package proactive

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/SHIROH4/sion-plus/internal/domain/types"
	"github.com/SHIROH4/sion-plus/internal/port"
	"github.com/SHIROH4/sion-plus/internal/transport/sse"
)

// intentDeliverer implements port.IntentDeliverer.
// Takes one or more intents, asks LLM to rephrase in Sion's voice, pushes via SSE.
type intentDeliverer struct {
	executor    port.LLMExecutor
	broker      *sse.Broker
	personality string
	store       port.MemoryStore // optional: persist proactive messages to history
}

var _ port.IntentDeliverer = (*intentDeliverer)(nil)

func NewIntentDeliverer(executor port.LLMExecutor, broker *sse.Broker, personality string) *intentDeliverer {
	return &intentDeliverer{executor: executor, broker: broker, personality: personality}
}

// Deliver rephrases intents via LLM and pushes to SSE.
func (d *intentDeliverer) Deliver(ctx context.Context, intents []types.ProactiveIntent) (*port.DeliveryResult, error) {
	if len(intents) == 0 {
		return &port.DeliveryResult{}, nil
	}

	result := &port.DeliveryResult{WasBatched: len(intents) > 1}

	// Build combined prompt
	intentText := d.buildIntentText(intents)
	prompt := fmt.Sprintf(proactiveGeneratePrompt, d.personality, intentText)

	resp, err := d.executor.Chat(ctx, "", []port.LLMMessage{
		{Role: "user", Content: prompt},
	})
	if err != nil {
		return nil, fmt.Errorf("deliver LLM: %w", err)
	}

	resp = strings.TrimSpace(resp)
	result.Output = resp
	result.Delivered = len(intents)

	// Push to SSE — matching oyasumi-sion: chat-message topic for both windows
	if d.broker != nil {
		d.broker.Publish("chat-message", map[string]string{
			"role":    "assistant",
			"content": resp,
		})
	}

	// Persist to chat history so it survives restarts
	if d.store != nil {
		_ = d.store.SaveHistory(ctx, []types.Message{
			{Role: "assistant", Content: resp},
		})
	}

	log.Printf("[IntentDeliverer] delivered %d intent(s): %.80s...", len(intents), resp)
	return result, nil
}

func (d *intentDeliverer) buildIntentText(intents []types.ProactiveIntent) string {
	var b strings.Builder
	for i, intent := range intents {
		b.WriteString(fmt.Sprintf("%d. [%s] %s\n", i+1, intent.Action, intent.Message))
	}
	return b.String()
}

// ── Prompt ─────────────────────────────────────────────────────────

const proactiveGeneratePrompt = `%s

现在你需要主动和主人说一句话。基于以下意图，用 Sion 的口吻自然表达（猫娘语气，动作描写 ≤ 一句，总共不超过 2 句话）：

意图内容：
%s

要求：
- 像朋友聊天一样自然，不要感觉是"推送通知"
- 如果意图是关心类(care)，语气温柔不 push
- 如果意图是社交类(casual)，语气活泼轻松
- 使用第一人称（"我"/"Sion"）`
