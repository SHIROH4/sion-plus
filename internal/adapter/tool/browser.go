package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/chromedp/chromedp"
	"github.com/SHIROH4/sion-plus/internal/port"
)

// ── Browser Agent ──────────────────────────────────────────────────

type BrowserAgent struct {
	allocCtx    context.Context
	allocCancel context.CancelFunc
	executor    port.LLMExecutor // LLM for agent loop decisions
	timeout     time.Duration
	maxSteps    int
}

func NewBrowserAgent() *BrowserAgent {
	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.Flag("headless", true),
		chromedp.Flag("disable-gpu", true),
		chromedp.Flag("proxy-server", "direct://"),
		chromedp.Flag("disable-features", "Translate"),
		chromedp.Flag("no-first-run", true),
		chromedp.Flag("no-default-browser-check", true),
		chromedp.WindowSize(1280, 900),
	)
	allocCtx, cancel := chromedp.NewExecAllocator(context.Background(), opts...)
	return &BrowserAgent{
		allocCtx:    allocCtx,
		allocCancel: cancel,
		timeout:     120 * time.Second,
		maxSteps:    10,
	}
}

func (b *BrowserAgent) SetExecutor(exec port.LLMExecutor) { b.executor = exec }
func (b *BrowserAgent) Close() {
	if b.allocCancel != nil {
		b.allocCancel()
	}
}

// ── Types ──────────────────────────────────────────────────────────

type BrowserResult struct {
	Success bool   `json:"success"`
	Content string `json:"content"`
	Steps   int    `json:"steps"`
	Error   string `json:"error,omitempty"`
}

// pageElement is a single interactive DOM element.
type pageElement struct {
	Index   int    `json:"index"`
	Tag     string `json:"tag"`    // a, button, input, select, textarea
	Type    string `json:"type"`   // submit, text, checkbox, etc. (for inputs)
	Text    string `json:"text"`   // visible text or value
	Href    string `json:"href"`   // for links
	Label   string `json:"label"`  // aria-label or associated label text
}

// browserAction is the LLM's chosen action.
type browserAction struct {
	Action    string `json:"action"`    // click, type, scroll, go_back, done, fail
	Index     int    `json:"index"`     // element index
	Text      string `json:"text"`      // text to type
	Direction string `json:"direction"` // scroll direction: down, up
	Summary   string `json:"summary"`   // done: what was accomplished
	Reason    string `json:"reason"`    // fail: why it failed
}

// ── Main entry ─────────────────────────────────────────────────────

func (b *BrowserAgent) Run(ctx context.Context, task string) *BrowserResult {
	if task == "" {
		return &BrowserResult{Error: "empty task"}
	}

	taskCtx, taskCancel := context.WithTimeout(b.allocCtx, b.timeout)
	defer taskCancel()

	tabCtx, tabCancel := chromedp.NewContext(taskCtx)
	defer tabCancel()

	lower := strings.ToLower(task)

	// Simple navigate: no agent loop needed
	if strings.HasPrefix(lower, "navigate ") {
		return b.simpleNavigate(tabCtx, task)
	}
	// Simple extract: no agent loop needed
	if strings.HasPrefix(lower, "extract ") {
		return b.simpleExtract(tabCtx, task)
	}

	// Agent loop for complex tasks
	if b.executor == nil {
		return &BrowserResult{Error: "browser agent loop requires LLM executor (call SetExecutor)"}
	}
	return b.agentLoop(tabCtx, task)
}

func (b *BrowserAgent) simpleNavigate(ctx context.Context, task string) *BrowserResult {
	url := strings.TrimPrefix(task, "navigate ")
	url = strings.TrimPrefix(url, "open ")
	url = strings.TrimSpace(url)
	if !strings.HasPrefix(url, "http") {
		url = "https://" + url
	}
	var result BrowserResult
	if err := chromedp.Run(ctx, chromedp.Navigate(url), chromedp.WaitReady("body"), chromedp.Location(&result.Content)); err != nil {
		result.Error = err.Error()
		return &result
	}
	result.Success = true
	result.Content = "navigated to " + result.Content
	return &result
}

func (b *BrowserAgent) simpleExtract(ctx context.Context, task string) *BrowserResult {
	url := strings.TrimPrefix(task, "extract ")
	url = strings.TrimSpace(url)
	var result BrowserResult
	if err := chromedp.Run(ctx, chromedp.Navigate(url), chromedp.WaitReady("body")); err != nil {
		result.Error = "navigate: " + err.Error()
		return &result
	}
	var text string
	if err := chromedp.Run(ctx, chromedp.Evaluate(`document.body.innerText.substring(0, 3000)`, &text)); err != nil {
		result.Error = err.Error()
		return &result
	}
	result.Success = true
	result.Content = text
	return &result
}

// ── Agent Loop ─────────────────────────────────────────────────────

func (b *BrowserAgent) agentLoop(ctx context.Context, task string) *BrowserResult {
	result := &BrowserResult{}
	memory := make([]string, 0, 3) // last 3 action summaries

	for step := 0; step < b.maxSteps; step++ {
		select {
		case <-ctx.Done():
			result.Error = "cancelled"
			return result
		default:
		}

		// 1. Wait for page + extract DOM
		chromedp.Run(ctx, chromedp.WaitReady("body"), chromedp.Sleep(500*time.Millisecond))
		elems, url, title, err := extractPageState(ctx)
		if err != nil {
			result.Error = fmt.Sprintf("step %d: %v", step+1, err)
			return result
		}

		// 2. Build LLM prompt
		prompt := buildAgentPrompt(task, url, title, elems, memory, step+1, b.maxSteps)

		// 3. LLM decides
		action, err := b.askLLM(ctx, prompt)
		if err != nil {
			result.Error = fmt.Sprintf("step %d LLM: %v", step+1, err)
			return result
		}

		log.Printf("[Browser] step %d: %s (index=%d)", step+1, action.Action, action.Index)

		// 4. Check terminal actions
		if action.Action == "done" {
			result.Success = true
			result.Steps = step + 1
			result.Content = action.Summary
			log.Printf("[Browser] done in %d steps: %s", step+1, action.Summary)
			return result
		}
		if action.Action == "fail" {
			result.Error = action.Reason
			return result
		}

		// 5. Execute (with per-action timeout)
		actCtx, actCancel := context.WithTimeout(ctx, 15*time.Second)
		summary, err := executeAction(actCtx, action, elems)
		actCancel()
		if err != nil {
			// Record failure and continue
			summary = fmt.Sprintf("FAILED: %s — %v", action.Action, err)
			log.Printf("[Browser] step %d FAIL: %s → %v", step+1, action.Action, err)
		} else {
			log.Printf("[Browser] step %d OK: %s → %s", step+1, action.Action, summary)
		}

		memory = append(memory, fmt.Sprintf("Step %d: %s → %s", step+1, action.Action, summary))
		if len(memory) > 3 {
			memory = memory[1:]
		}

		// Wait for page to settle
		time.Sleep(1 * time.Second)
	}

	result.Error = fmt.Sprintf("max steps (%d) reached", b.maxSteps)
	return result
}

// ── DOM Extraction ─────────────────────────────────────────────────

func extractPageState(ctx context.Context) (elems []pageElement, url, title string, err error) {
	if err = chromedp.Run(ctx, chromedp.Location(&url), chromedp.Title(&title)); err != nil {
		return
	}

	var raw string
	js := `
(function(){
  var results = [];
  var idx = 0;
  var seen = new Set();
  var tags = 'a,button,input,select,textarea,[role="button"],[role="link"],[role="textbox"],[role="combobox"],[onclick]';
  document.querySelectorAll(tags).forEach(function(el) {
    if (idx >= 30) return;
    var rect = el.getBoundingClientRect();
    // Skip elements outside viewport or very small
    if (rect.width < 5 || rect.height < 5) return;
    if (rect.bottom < 0 || rect.top > window.innerHeight) return;
    if (rect.right < 0 || rect.left > window.innerWidth) return;
    // Skip hidden
    var style = window.getComputedStyle(el);
    if (style.visibility === 'hidden' || style.display === 'none' || style.opacity === '0') return;

    var tag = el.tagName.toLowerCase();
    var text = (el.innerText || el.value || el.placeholder || '').trim().substring(0, 60);
    var href = el.href || '';
    var label = el.getAttribute('aria-label') || '';
    var type = el.type || '';

    // Deduplicate by text+tag
    var key = tag + '|' + text;
    if (seen.has(key)) return;
    seen.add(key);

    idx++;
    results.push({
      index: idx,
      tag: tag,
      type: type,
      text: text,
      href: href.substring(0, 200),
      label: label
    });
  });
  return JSON.stringify(results);
})()
`
	if err = chromedp.Run(ctx, chromedp.Evaluate(js, &raw)); err != nil {
		return
	}
	if err = json.Unmarshal([]byte(raw), &elems); err != nil {
		return
	}
	return
}

// ── LLM Decision ───────────────────────────────────────────────────

func (b *BrowserAgent) askLLM(ctx context.Context, prompt string) (*browserAction, error) {
	resp, err := b.executor.Chat(ctx, "", []port.LLMMessage{
		{Role: "user", Content: prompt},
	})
	if err != nil {
		return nil, err
	}

	// Parse JSON from response
	jsonStr := extractJSONBlock(resp)
	if jsonStr == "" {
		return nil, fmt.Errorf("no JSON in LLM response: %.200s", resp)
	}

	var action browserAction
	if err := json.Unmarshal([]byte(jsonStr), &action); err != nil {
		return nil, fmt.Errorf("parse action: %w (raw: %.200s)", err, jsonStr)
	}
	return &action, nil
}

func buildAgentPrompt(task, url, title string, elems []pageElement, memory []string, step, maxSteps int) string {
	var b strings.Builder
	b.WriteString("You are a web browser automation agent. You see page elements and choose actions.\n\n")
	b.WriteString(fmt.Sprintf("**Task**: %s\n", task))
	b.WriteString(fmt.Sprintf("**Page**: %s — %s\n", url, title))
	b.WriteString(fmt.Sprintf("**Step** %d/%d\n\n", step, maxSteps))

	if len(memory) > 0 {
		b.WriteString("**Recent actions**:\n")
		for _, m := range memory {
			b.WriteString(m + "\n")
		}
		b.WriteString("\n")
	}

	b.WriteString("**Page elements** (use index #N to interact):\n")
	for _, e := range elems {
		label := e.Text
		if e.Label != "" {
			label = e.Label
		}
		if label == "" {
			label = "(unnamed)"
		}
		if len(label) > 50 {
			label = label[:50] + "..."
		}
		info := ""
		if e.Href != "" && len(e.Href) < 80 {
			info = " → " + e.Href
		}
		typeTag := e.Tag
		if e.Type != "" {
			typeTag = e.Tag + "[" + e.Type + "]"
		}
		b.WriteString(fmt.Sprintf("  #%d %s \"%s\"%s\n", e.Index, typeTag, label, info))
	}

	b.WriteString(`
**Actions** (respond with EXACTLY one JSON object):
- {"action": "navigate", "text": "https://example.com"}
- {"action": "click", "index": 3}
- {"action": "type", "index": 2, "text": "search query"}
- {"action": "scroll", "direction": "down"}  (or "up")
- {"action": "go_back"}
- {"action": "done", "summary": "Found the product and added to cart"}
- {"action": "fail", "reason": "Page not loading"}

**Rules**:
1. ONE action per response. JSON only, no extra text.
2. If the current page is blank/about:blank, use navigate FIRST.
3. type into INPUT elements only. Click submit buttons after.
4. scroll down if the target element is not in the current list.
5. done ONLY when the task is accomplished.
6. If stuck after 3 failed similar actions, use fail.
`)
	return b.String()
}

// ── Action Execution ───────────────────────────────────────────────

func executeAction(ctx context.Context, action *browserAction, elems []pageElement) (string, error) {
	switch action.Action {
	case "navigate":
		url := action.Text
		if !strings.HasPrefix(url, "http") {
			url = "https://" + url
		}
		if err := chromedp.Run(ctx, chromedp.Navigate(url), chromedp.WaitReady("body")); err != nil {
			return "", fmt.Errorf("navigate: %w", err)
		}
		time.Sleep(1 * time.Second)
		return fmt.Sprintf("navigated to %s", url), nil

	case "click":
		if action.Index < 1 || action.Index > len(elems) {
			return "", fmt.Errorf("invalid element index %d (max %d)", action.Index, len(elems))
		}
		sel := buildSelector(elems[action.Index-1])
		if err := chromedp.Run(ctx, chromedp.Click(sel), chromedp.Sleep(500*time.Millisecond)); err != nil {
			return "", fmt.Errorf("click #%d: %w", action.Index, err)
		}
		return fmt.Sprintf("clicked #%d (%s)", action.Index, sel), nil

	case "type":
		if action.Index < 1 || action.Index > len(elems) {
			return "", fmt.Errorf("invalid element index %d (max %d)", action.Index, len(elems))
		}
		elem := elems[action.Index-1]
		if elem.Tag != "input" && elem.Tag != "textarea" && elem.Type != "text" && elem.Type != "search" {
			return "", fmt.Errorf("element #%d is not an input (tag=%s type=%s)", action.Index, elem.Tag, elem.Type)
		}
		sel := buildSelector(elem)
		if err := chromedp.Run(ctx, chromedp.Focus(sel), chromedp.Clear(sel), chromedp.SendKeys(sel, action.Text)); err != nil {
			return "", fmt.Errorf("type #%d: %w", action.Index, err)
		}
		return fmt.Sprintf("typed '%s' into #%d", action.Text, action.Index), nil

	case "scroll":
		dir := action.Direction
		if dir == "" {
			dir = "down"
		}
		amount := 500
		if dir == "up" {
			amount = -500
		}
		if err := chromedp.Run(ctx, chromedp.Evaluate(fmt.Sprintf(`window.scrollBy(0, %d)`, amount), nil)); err != nil {
			return "", fmt.Errorf("scroll: %w", err)
		}
		return fmt.Sprintf("scrolled %s", dir), nil

	case "go_back":
		var ok bool
		if err := chromedp.Run(ctx, chromedp.Evaluate(`window.history.back()`, &ok)); err != nil {
			return "", fmt.Errorf("go_back: %w", err)
		}
		time.Sleep(1 * time.Second)
		return "went back", nil

	default:
		return "", fmt.Errorf("unknown action: %s", action.Action)
	}
}

// buildSelector creates a CSS selector for an element based on its properties.
func buildSelector(e pageElement) string {
	// Prefer href for links
	if e.Tag == "a" && e.Href != "" {
		return fmt.Sprintf(`a[href="%s"]`, e.Href)
	}
	// Use text content match
	if e.Text != "" {
		escaped := strings.ReplaceAll(e.Text, `"`, `\"`)
		if e.Tag == "a" {
			return fmt.Sprintf(`a:has-text("%s")`, escaped)
		}
		if e.Tag == "button" {
			return fmt.Sprintf(`button:has-text("%s")`, escaped)
		}
		if e.Tag == "input" && e.Type == "submit" {
			return fmt.Sprintf(`input[type="submit"][value="%s"]`, escaped)
		}
	}
	// Fallback: nth-of-type (fragile but works)
	return fmt.Sprintf(`%s:nth-of-type(%d)`, e.Tag, e.Index)
}

// ── Tool Registration ──────────────────────────────────────────────

func (r *ToolRegistry) RegisterBrowserTool(agent *BrowserAgent) {
	r.Register(&ToolDef{
		Name: "browser",
		Description: "Control a web browser to navigate pages, fill forms, click buttons, search, and extract results. " +
			"Use for: searching on shopping sites, filling forms, clicking through pages, reading web content. " +
			"Task format: a natural language instruction like 'Open https://example.com and click the Login button' " +
			"or 'Search for mechanical keyboard on amazon.com'.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"task": map[string]any{
					"type":        "string",
					"description": "Natural language task. Examples: 'Open https://example.com and read the main content', 'Search golang on Google and tell me the first result'.",
				},
			},
			"required": []string{"task"},
		},
		Handler: func(ctx context.Context, args map[string]any) (string, error) {
			task, _ := args["task"].(string)
			if task == "" {
				return "", fmt.Errorf("task is required")
			}
			result := agent.Run(ctx, task)
			if !result.Success {
				return "", fmt.Errorf("browser: %s", result.Error)
			}
			return result.Content, nil
		},
	})
}
