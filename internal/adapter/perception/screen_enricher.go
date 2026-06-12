package perception

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/shirohania/sion/internal/domain/types"
	"github.com/shirohania/sion/internal/port"
)

// ScreenEnrichment is the structured output of screen content analysis.
type ScreenEnrichment struct {
	Activity      string `json:"activity"`       // "debugging Go code", "watching YouTube", etc.
	UserMood      string `json:"user_mood"`      // "focused", "frustrated", "relaxed", "distracted"
	UserEmotion   string `json:"user_emotion"`   // primary emotion: "neutral", "happy", "stressed", "angry"
	ShouldEngage  bool   `json:"should_engage"`   // is this a good time to interact?
	EngageReason  string `json:"engage_reason"`  // why/why not
	SuggestedTone string `json:"suggested_tone"` // "playful", "supportive", "concise", "quiet"
	ObservedText  string `json:"observed_text"`  // any visible text/code the AI noticed
}

// ScreenLLMEnricher uses a vision-capable LLM to analyze screenshots.
// Only used on-demand (proactive triggers), not every chat turn.
type ScreenLLMEnricher struct {
	executor     port.LLMExecutor
	observer     *ScreenObserver
	machine      *ActivityStateMachine
	lastResult   *ScreenEnrichment
	lastCallAt   time.Time
	minInterval  time.Duration
}

func NewScreenLLMEnricher(executor port.LLMExecutor, observer *ScreenObserver, machine *ActivityStateMachine) *ScreenLLMEnricher {
	return &ScreenLLMEnricher{
		executor:    executor,
		observer:    observer,
		machine:     machine,
		minInterval: 30 * time.Second,
	}
}

// Analyze captures a screenshot and asks the vision LLM to analyze it.
// Returns nil if called too frequently or if vision is unavailable.
func (e *ScreenLLMEnricher) Analyze(ctx context.Context) (*ScreenEnrichment, error) {
	if e.executor == nil || !e.observer.IsAvailable() {
		return nil, fmt.Errorf("ScreenLLMEnricher: unavailable")
	}
	if time.Since(e.lastCallAt) < e.minInterval {
		return e.lastResult, nil
	}
	e.lastCallAt = time.Now()

	// 1. Capture screenshot
	jpg, err := e.observer.CaptureScreenshot(ctx)
	if err != nil {
		return nil, fmt.Errorf("screenshot: %w", err)
	}
	b64 := base64.StdEncoding.EncodeToString(jpg)

	// 2. Get rule-based state for context
	obs, _ := e.observer.Observe(ctx)
	var snap ActivitySnapshot
	if obs != nil {
		snap = e.machine.Classify(obs.AppCategory, obs.AppName,
			e.observer.IdleSeconds(ctx), e.observer.SwitchCount(), obs.WindowTitle)
	}

	// 3. Build vision prompt
	prompt := buildVisionPrompt(obs, snap)

	// 4. Call vision LLM
	resp, err := e.executor.Chat(ctx, "", []port.LLMMessage{
		port.NewVisionMessage(prompt, b64),
	})
	if err != nil {
		return nil, fmt.Errorf("vision LLM: %w", err)
	}

	// 5. Parse response
	enrich, err := parseEnrichment(resp)
	if err != nil {
		log.Printf("[ScreenEnricher] parse error: %v (raw: %.200s)", err, resp)
		return nil, err
	}

	e.lastResult = enrich
	return enrich, nil
}

// LastResult returns the most recent analysis (may be nil).
func (e *ScreenLLMEnricher) LastResult() *ScreenEnrichment { return e.lastResult }

// ToEmotionDelta converts enrichment to an emotion delta.
func (e *ScreenLLMEnricher) ToEmotionDelta(enrich *ScreenEnrichment) *types.EmotionDelta {
	if enrich == nil {
		return nil
	}
	d := &types.EmotionDelta{Source: "screen_vision"}

	switch enrich.UserMood {
	case "frustrated":
		d.Worry = 0.25
		d.Playfulness = -0.15
	case "stressed":
		d.Worry = 0.3
	case "relaxed":
		d.Playfulness = 0.15
		d.Curiosity = 0.1
	case "focused":
		d.Curiosity = 0.1
	case "distracted":
		d.Playfulness = 0.2
	}

	switch enrich.SuggestedTone {
	case "quiet":
		d.Playfulness = -0.1
	case "supportive":
		d.Worry = 0.15
		d.Affection = 0.1
	case "playful":
		d.Playfulness = 0.15
	}

	if enrich.ShouldEngage {
		d.Curiosity = 0.15
		d.Playfulness = 0.1
	}

	return d
}

// ── Prompt ─────────────────────────────────────────────────────────

func buildVisionPrompt(obs *port.ScreenObservation, snap ActivitySnapshot) string {
	ctx := ""
	if obs != nil {
		ctx = fmt.Sprintf("已知信息：用户正在使用 %s", obs.AppName)
		if obs.WindowTitle != "" {
			ctx += fmt.Sprintf("，窗口标题：%s", obs.WindowTitle)
		}
		ctx += fmt.Sprintf("。系统判断状态：%s", snap.State)
	}
	return fmt.Sprintf(`你是一个桌面AI伙伴，正在观察主人的屏幕。

%s

请分析这张截图，返回JSON：
{
  "activity": "主人正在做什么（简短描述）",
  "user_mood": "focused/frustrated/relaxed/distracted/other",
  "user_emotion": "neutral/happy/stressed/angry/other",
  "should_engage": true/false,
  "engage_reason": "为什么适合/不适合互动",
  "suggested_tone": "playful/supportive/concise/quiet/other",
  "observed_text": "截图中看到的文本/代码/内容（如果有）"
}

判断规则：
- 正在认真工作/写代码 → focused, should_engage=false
- 正在看视频/娱乐 → relaxed, should_engage=true
- 面对错误/报错信息 → frustrated, should_engage=true（安慰）
- 在聊天/社交 → relaxed, should_engage=false
- 空闲/桌面 → distracted, should_engage=true

返回纯JSON，不要markdown包裹。`, ctx)
}

// ── Response parsing ───────────────────────────────────────────────

func parseEnrichment(raw string) (*ScreenEnrichment, error) {
	raw = strings.TrimSpace(raw)
	raw = strings.TrimPrefix(raw, "```json")
	raw = strings.TrimPrefix(raw, "```")
	raw = strings.TrimSuffix(raw, "```")
	raw = strings.TrimSpace(raw)

	var e ScreenEnrichment
	if err := json.Unmarshal([]byte(raw), &e); err != nil {
		return nil, err
	}
	return &e, nil
}
