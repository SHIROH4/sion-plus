package tool

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/chromedp/chromedp"
)

func TestBrowserNavigate(t *testing.T) {
	agent := NewBrowserAgent()
	defer agent.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	result := agent.Run(ctx, "navigate https://example.com")
	if !result.Success {
		t.Fatalf("navigate failed: %s", result.Error)
	}
	if result.Content == "" {
		t.Error("expected non-empty content")
	}
	t.Logf("navigate: %s", result.Content)
}

func TestBrowserExtract(t *testing.T) {
	agent := NewBrowserAgent()
	defer agent.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	result := agent.Run(ctx, "extract https://example.com")
	if !result.Success {
		t.Fatalf("extract failed: %s", result.Error)
	}
	if !strings.Contains(result.Content, "Example Domain") {
		t.Errorf("expected 'Example Domain' in content, got: %.100s", result.Content)
	}
	t.Logf("extract: %.200s", result.Content)
}

func TestBrowserElementExtraction(t *testing.T) {
	agent := NewBrowserAgent()
	defer agent.Close()

	// Navigate first
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	result := agent.Run(ctx, "navigate https://example.com")
	if !result.Success {
		t.Fatalf("navigate failed: %s", result.Error)
	}

	// Open a fresh tab and extract elements
	taskCtx, tc := context.WithTimeout(agent.allocCtx, 30*time.Second)
	defer tc()
	tabCtx, tc2 := chromedp.NewContext(taskCtx)
	defer tc2()

	if err := chromedp.Run(tabCtx,
		chromedp.Navigate("https://example.com"),
		chromedp.WaitReady("body"),
	); err != nil {
		t.Fatalf("re-navigate: %v", err)
	}

	elems, url, title, err := extractPageState(tabCtx)
	if err != nil {
		t.Fatalf("extractPageState: %v", err)
	}
	t.Logf("page: %s — %s", url, title)
	t.Logf("elements: %d", len(elems))
	for _, e := range elems {
		t.Logf("  #%d %s[%s] text=%q href=%q",
			e.Index, e.Tag, e.Type, truncateStr(e.Text, 50), truncateStr(e.Href, 50))
	}
	if len(elems) == 0 {
		t.Error("expected at least 1 element on example.com")
	}
}
