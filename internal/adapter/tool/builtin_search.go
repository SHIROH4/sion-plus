package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// RegisterSearchTool adds web_search to the registry.
func (r *ToolRegistry) RegisterSearchTool() {
	r.Register(&ToolDef{
		Name:        "web_search",
		Description: "Search the web using DuckDuckGo. Returns titles, URLs, and snippets. Use for finding current information, documentation, or answers.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"query": map[string]any{
					"type":        "string",
					"description": "Search query",
				},
				"max_results": map[string]any{
					"type":        "integer",
					"description": "Maximum number of results (optional, default 5, max 10)",
				},
			},
			"required": []string{"query"},
		},
		Handler: handleWebSearch,
	})
}

type ddgResult struct {
	Title   string `json:"title"`
	URL     string `json:"url"`
	Snippet string `json:"snippet"`
}

func handleWebSearch(ctx context.Context, args map[string]any) (string, error) {
	query, _ := args["query"].(string)
	if query == "" {
		return "", fmt.Errorf("query is required")
	}

	maxResults := intArg(args, "max_results", 5)
	if maxResults > 10 {
		maxResults = 10
	}

	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	apiURL := fmt.Sprintf("https://api.duckduckgo.com/?q=%s&format=json&no_html=1&skip_disambig=1",
		url.QueryEscape(query))

	req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
	if err != nil {
		return "", fmt.Errorf("search request: %w", err)
	}
	req.Header.Set("User-Agent", "Sion/2.2")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		// Fallback: return a helpful message
		return fmt.Sprintf("Search for %q could not reach DuckDuckGo API: %v. Try again or use a different query.", query, err), nil
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 64*1024))

	var ddgResp struct {
		AbstractText string       `json:"AbstractText"`
		AbstractURL  string       `json:"AbstractURL"`
		Results      []ddgResult  `json:"Results"`
		RelatedTopics []struct {
			Text     string `json:"Text"`
			FirstURL string `json:"FirstURL"`
		} `json:"RelatedTopics"`
	}
	json.Unmarshal(body, &ddgResp)

	var b strings.Builder
	b.WriteString(fmt.Sprintf("Search results for: %s\n\n", query))

	if ddgResp.AbstractText != "" {
		b.WriteString(fmt.Sprintf("📌 %s\n   %s\n\n", ddgResp.AbstractText, ddgResp.AbstractURL))
	}

	count := 0
	for _, r := range ddgResp.Results {
		if count >= maxResults {
			break
		}
		if r.URL != "" {
			b.WriteString(fmt.Sprintf("%d. %s\n   %s\n   %s\n\n", count+1, r.Title, r.URL, r.Snippet))
			count++
		}
	}

	if count == 0 {
		for _, t := range ddgResp.RelatedTopics {
			if count >= maxResults {
				break
			}
			if t.FirstURL != "" {
				b.WriteString(fmt.Sprintf("%d. %s\n   %s\n\n", count+1, t.Text, t.FirstURL))
				count++
			}
		}
	}

	if count == 0 && ddgResp.AbstractText == "" {
		b.WriteString("No results found. Try a different query.\n")
	}

	return strings.TrimSpace(b.String()), nil
}
