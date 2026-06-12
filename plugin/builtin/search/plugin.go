// Package search implements the web search plugin — Bocha/Bing API
// web search with result formatting.
package search

import (
	"context"
	"sync"

	"github.com/SHIROH4/sion-plus/plugin/sdk"
)

// Plugin implements sdk.Plugin for the search module.
type Plugin struct {
	sdk.BasePlugin
	pctx   *sdk.PluginContext
	mu     sync.Mutex
	client SearchClient
}

// SearchClient abstracts the search backend (Bocha, Bing, etc.).
type SearchClient interface {
	Search(ctx context.Context, query string, topK int) ([]SearchResult, error)
}

// SearchResult is a single web search result.
type SearchResult struct {
	Title   string `json:"title"`
	URL     string `json:"url"`
	Snippet string `json:"snippet"`
}

func New() *Plugin {
	return &Plugin{
		BasePlugin: sdk.NewBasePlugin(sdk.PluginInfo{
			Name:        "search",
			Version:     "1.0.0",
			Description: "Web search via Bocha/Bing API with result formatting",
			Author:      "Sion",
		}),
	}
}

func (p *Plugin) SetClient(client SearchClient) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.client = client
}

func (p *Plugin) Init(ctx context.Context, pctx *sdk.PluginContext) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.pctx = pctx

	// Listen for search requests from other plugins
	if p.pctx.EventBus != nil {
		p.pctx.EventBus.Subscribe("plugin:search:request", func(payload any) {
			p.handleSearchRequest(ctx, payload)
		})
	}
	return nil
}

func (p *Plugin) handleSearchRequest(ctx context.Context, payload any) {
	m, ok := payload.(map[string]string)
	if !ok {
		return
	}
	query := m["query"]
	if query == "" || p.client == nil {
		return
	}
	results, err := p.client.Search(ctx, query, 5)
	if err != nil {
		return
	}
	// Publish results back for any listener
	if p.pctx.EventBus != nil {
		p.pctx.EventBus.Publish("plugin:search:results", results)
	}
}

// Search performs a web search and returns formatted results.
func (p *Plugin) Search(ctx context.Context, query string, topK int) ([]SearchResult, error) {
	if p.client == nil {
		return nil, nil
	}
	return p.client.Search(ctx, query, topK)
}

var _ sdk.Plugin = (*Plugin)(nil)
