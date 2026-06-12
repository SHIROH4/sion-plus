// Package chat implements the chat plugin — system prompt construction,
// pre/post-chat hooks, and function tools exposed to the AI.
package chat

import (
	"context"
	"sync"

	"github.com/SHIROH4/sion-plus/plugin/sdk"
)

// Plugin implements sdk.Plugin for the chat module.
type Plugin struct {
	sdk.BasePlugin
	pctx *sdk.PluginContext
	mu   sync.Mutex
}

func New() *Plugin {
	return &Plugin{
		BasePlugin: sdk.NewBasePlugin(sdk.PluginInfo{
			Name:        "chat",
			Version:     "1.0.0",
			Description: "System prompt builder, chat hooks, and AI function tools",
			Author:      "Sion",
		}),
	}
}

func (p *Plugin) Init(ctx context.Context, pctx *sdk.PluginContext) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.pctx = pctx
	return nil
}
