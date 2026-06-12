// Package memory implements the memory pipeline plugin — fact extraction,
// diary generation, memory retrieval, and compression.
package memory

import (
	"context"
	"sync"

	"github.com/shirohania/sion/plugin/sdk"
)

// Plugin implements sdk.Plugin for the memory module.
type Plugin struct {
	sdk.BasePlugin
	pctx *sdk.PluginContext
	mu   sync.Mutex
}

func New() *Plugin {
	return &Plugin{
		BasePlugin: sdk.NewBasePlugin(sdk.PluginInfo{
			Name:        "memory",
			Version:     "1.0.0",
			Description: "Full memory pipeline: extract, dedup, save, signal detect, diary, compress",
			Author:      "Sion",
			DependsOn:   []string{"chat"},
		}),
	}
}

func (p *Plugin) Init(ctx context.Context, pctx *sdk.PluginContext) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.pctx = pctx

	// Subscribe to chat events to trigger memory extraction
	if p.pctx.EventBus != nil {
		p.pctx.EventBus.Subscribe("chat:received", func(payload any) {
			p.onChatReceived(ctx, payload)
		})
		p.pctx.EventBus.Subscribe("chat:sent", func(payload any) {
			p.onChatSent(ctx, payload)
		})
	}
	return nil
}

func (p *Plugin) onChatReceived(ctx context.Context, payload any) {
	// Trigger memory recall for relevant context
}

func (p *Plugin) onChatSent(ctx context.Context, payload any) {
	// After each chat turn, schedule fact extraction and diary generation
	// This is handled by the MemoryWorker in the main adapter stack.
	// The plugin emits events that the adapter layer responds to.
	if p.pctx.EventBus != nil {
		p.pctx.EventBus.Publish("memory:extract_request", payload)
	}
}

var _ sdk.Plugin = (*Plugin)(nil)
