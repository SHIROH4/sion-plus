// Package chat: function tools exposed to the AI agent loop.
package chat

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/shirohania/sion/internal/domain/types"
	"github.com/shirohania/sion/plugin/sdk"
)

var _ sdk.FunctionProvider = (*Plugin)(nil)

func (p *Plugin) Functions() []sdk.FunctionDef {
	return []sdk.FunctionDef{
		{
			Name:        "web_search",
			Description: "Search the web for information using the configured search backend.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"query": map[string]any{
						"type":        "string",
						"description": "The search query",
					},
				},
				"required": []string{"query"},
			},
			Handler: p.handleWebSearch,
		},
		{
			Name:        "get_memory",
			Description: "Retrieve relevant memories about the user or past conversations.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"query": map[string]any{
						"type":        "string",
						"description": "What to search for in memory",
					},
					"top_k": map[string]any{
						"type":        "integer",
						"description": "Number of results (default 5)",
					},
				},
				"required": []string{"query"},
			},
			Handler: p.handleGetMemory,
		},
		{
			Name:        "memorize",
			Description: "Save a fact about the user or relationship to memory.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"content": map[string]any{
						"type":        "string",
						"description": "The fact to remember",
					},
					"importance": map[string]any{
						"type":        "integer",
						"description": "Importance 1-10 (default 5)",
					},
				},
				"required": []string{"content"},
			},
			Handler: p.handleMemorize,
		},
		{
			Name:        "analyze_screenshot",
			Description: "Request a screenshot capture and analysis of the user's screen.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"prompt": map[string]any{
						"type":        "string",
						"description": "What to look for in the screenshot",
					},
				},
				"required": []string{"prompt"},
			},
			Handler: p.handleAnalyzeScreenshot,
		},
	}
}

func (p *Plugin) handleWebSearch(ctx context.Context, argsJSON string) (string, error) {
	var args struct {
		Query string `json:"query"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return "", fmt.Errorf("web_search: %w", err)
	}
	// Search is handled by a dedicated plugin; emit event and return placeholder.
	if p.pctx.EventBus != nil {
		p.pctx.EventBus.Publish("plugin:search:request", map[string]string{
			"query":  args.Query,
			"source": "chat_tool",
		})
	}
	return fmt.Sprintf("Search for '%s' initiated. Check the search plugin for results.", args.Query), nil
}

func (p *Plugin) handleGetMemory(ctx context.Context, argsJSON string) (string, error) {
	var args struct {
		Query string `json:"query"`
		TopK  int    `json:"top_k"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return "", fmt.Errorf("get_memory: %w", err)
	}
	if args.TopK <= 0 {
		args.TopK = 5
	}
	if p.pctx.MemoryRecall == nil {
		return "Memory recall not available.", nil
	}
	results, err := p.pctx.MemoryRecall.HybridSearch(ctx, args.Query, args.TopK)
	if err != nil {
		return "", fmt.Errorf("get_memory: %w", err)
	}
	if len(results) == 0 {
		return "No relevant memories found.", nil
	}
	var lines []string
	for i, r := range results {
		lines = append(lines, fmt.Sprintf("%d. [%s] %s", i+1, r.Source, r.Content))
	}
	return stringsJoin(lines, "\n"), nil
}

func (p *Plugin) handleMemorize(ctx context.Context, argsJSON string) (string, error) {
	var args struct {
		Content    string `json:"content"`
		Importance int    `json:"importance"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return "", fmt.Errorf("memorize: %w", err)
	}
	if args.Importance <= 0 {
		args.Importance = 5
	}
	if args.Importance > 10 {
		args.Importance = 10
	}
	if p.pctx.MemoryStore == nil {
		return "Memory store not available.", nil
	}
	fact := &types.FactEntry{
		Entity:       "master",
		RelationType: "has_preference",
		Content:      args.Content,
		Importance:   args.Importance,
		Source:       "chat",
		MemCellType:  "fact",
		SourceTier:   types.SourceInferred,
	}
	if err := p.pctx.MemoryStore.SaveFact(ctx, fact); err != nil {
		return "", fmt.Errorf("memorize: %w", err)
	}
	return "Saved to memory.", nil
}

func (p *Plugin) handleAnalyzeScreenshot(ctx context.Context, argsJSON string) (string, error) {
	var args struct {
		Prompt string `json:"prompt"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return "", fmt.Errorf("analyze_screenshot: %w", err)
	}
	if p.pctx.EventBus != nil {
		p.pctx.EventBus.Publish("plugin:vision:request", map[string]string{
			"prompt": args.Prompt,
			"source": "chat_tool",
		})
	}
	return "Screenshot analysis requested. The vision plugin will process it.", nil
}

func stringsJoin(s []string, sep string) string {
	if len(s) == 0 {
		return ""
	}
	result := s[0]
	for i := 1; i < len(s); i++ {
		result += sep + s[i]
	}
	return result
}
