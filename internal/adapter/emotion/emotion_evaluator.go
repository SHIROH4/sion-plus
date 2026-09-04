package emotion

import (
	"context"
	"encoding/json"
	"log"
	"strings"

	"github.com/SHIROH4/sion-plus/internal/domain/types"
	"github.com/SHIROH4/sion-plus/internal/port"
)

// EmotionEvaluator implements port.EmotionSignalSource.
// Two-tier: LLM (primary) → rule-based (fallback).
// LLM directly outputs 8D deltas — how Sion herself should feel.
type EmotionEvaluator struct {
	executor port.LLMExecutor
	store    port.EmotionStateManager
}

var _ port.EmotionSignalSource = (*EmotionEvaluator)(nil)

func NewEmotionEvaluator(executor port.LLMExecutor, store port.EmotionStateManager) *EmotionEvaluator {
	return &EmotionEvaluator{executor: executor, store: store}
}

func (e *EmotionEvaluator) Name() string { return "chat" }

// Evaluate implements port.EmotionSignalSource.
func (e *EmotionEvaluator) Evaluate(ctx context.Context, input *port.EmotionEvalInput) (*port.EmotionEvalResult, error) {
	if input == nil || strings.TrimSpace(input.RecentTurns) == "" {
		state, vec := e.store.Current()
		return &port.EmotionEvalResult{State: state, Vector: vec, Source: "cache"}, nil
	}
	if !needsLLMEmotionEvaluation(input.CurrentMsg) {
		delta := e.evaluateRules(input.CurrentMsg)
		delta.Source = "rule_gate"
		e.store.ApplyDelta(delta)
		state, vec := e.store.Current()
		return &port.EmotionEvalResult{Delta: delta, State: state, Vector: vec, Source: "rule_gate"}, nil
	}

	// Tier 1: LLM
	if e.executor != nil && e.executor.IsAvailable(ctx) {
		delta, err := e.evaluateLLM(ctx, input.CurrentMsg, input.RecentTurns)
		if err == nil {
			delta.Source = "chat"
			log.Printf("[EmotionEvaluator] LLM delta: aff=%.2f wor=%.2f cur=%.2f slp=%.2f ply=%.2f lon=%.2f con=%.2f ann=%.2f reason=%q",
				delta.Affection, delta.Worry, delta.Curiosity, delta.Sleepiness,
				delta.Playfulness, delta.Loneliness, delta.Confidence, delta.Annoyance, delta.Reason)
			e.store.ApplyDelta(delta)
			state, vec := e.store.Current()
			return &port.EmotionEvalResult{Delta: delta, State: state, Vector: vec, Source: "llm"}, nil
		}
		log.Printf("[EmotionEvaluator] LLM failed, falling back to rules: %v", err)
	}

	// Tier 2: Rule-based fallback
	delta := e.evaluateRules(input.CurrentMsg)
	delta.Source = "chat"
	e.store.ApplyDelta(delta)
	state, vec := e.store.Current()
	return &port.EmotionEvalResult{Delta: delta, State: state, Vector: vec, Source: "rule"}, nil
}

// needsLLMEmotionEvaluation keeps neutral commands and technical queries on the
// deterministic path. Emotionally salient or relational language still uses
// the LLM, preserving nuance where it matters.
func needsLLMEmotionEvaluation(text string) bool {
	lower := strings.ToLower(text)
	markers := []string{
		"开心", "难过", "焦虑", "生气", "害怕", "孤独", "累", "烦", "压力", "委屈", "喜欢", "讨厌", "爱", "真好", "想哭", "抱歉", "谢谢", "陪我", "安慰", "心情", "感受",
		"happy", "sad", "anxious", "angry", "afraid", "lonely", "tired", "annoyed", "stress", "love", "hate", "sorry", "thanks", "feel",
	}
	for _, marker := range markers {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	return false
}

// ── LLM path ─────────────────────────────────────────────────────

func (e *EmotionEvaluator) evaluateLLM(ctx context.Context, currentMsg, recentTurns string) (*types.EmotionDelta, error) {
	ctx = port.WithLLMCallMetadata(ctx, "emotion", "emotion_eval")
	prompt := buildEmotionDeltaPrompt(LangZH, currentMsg, recentTurns)
	resp, err := e.executor.Chat(ctx, "", []port.LLMMessage{
		{Role: "user", Content: prompt},
	})
	if err != nil {
		return nil, err
	}

	resp = extractJSON(resp)

	var delta types.EmotionDelta
	if err := json.Unmarshal([]byte(resp), &delta); err != nil {
		return nil, err
	}
	delta.ClampDelta()
	return &delta, nil
}

// ── Rule-based fallback ──────────────────────────────────────────

func (e *EmotionEvaluator) evaluateRules(text string) *types.EmotionDelta {
	text = strings.ToLower(text)

	pos := countKeywords(text, []string{"哈哈", "开心", "喜欢", "谢谢", "厉害", "好棒", "可爱",
		"love", "happy", "thanks", "great", "cute", "太棒", "帮了", "帮大", "爱", "对不起", "抱歉", "原谅"})
	neg := countKeywords(text, []string{"烦", "讨厌", "别吵", "闭嘴", "滚", "shut up",
		"annoying", "hate", "go away", "认真回答", "听到没", "废物", "垃圾", "没用", "滚开", "傻", "蠢", "笨"})
	sad := countKeywords(text, []string{"唉", "难受", "想哭", "sad", "depressed", "unhappy",
		"累", "难过", "被裁", "被骂", "不开心"})
	short := len([]rune(text)) <= 8

	delta := &types.EmotionDelta{}

	// Short messages with strong negative keywords → amplified response
	negBoost := 1.0
	if short && neg > 0 {
		negBoost = 1.8
	}

	// Short empty/ellipsis → slight sleepiness, slight loneliness
	if short && pos == 0 && neg == 0 && sad == 0 {
		if strings.Contains(text, "...") || strings.Contains(text, "。。。") {
			delta.Sleepiness = 0.15
			delta.Loneliness = 0.05
			return delta
		}
		// Single "?" or "？" → slight worry
		if strings.Count(text, "?")+strings.Count(text, "？") >= 2 {
			delta.Worry = 0.15
			delta.Curiosity = 0.1
			return delta
		}
	}

	if pos > 0 {
		delta.Affection = float64(pos) * 0.12
		delta.Confidence = float64(pos) * 0.08
		delta.Playfulness = float64(pos) * 0.08
		delta.Annoyance = -float64(pos) * 0.05
	}

	if neg > 0 {
		delta.Annoyance = float64(neg) * 0.18 * negBoost
		delta.Worry = float64(neg) * 0.12 * negBoost
		delta.Confidence = -float64(neg) * 0.1 * negBoost
	}

	if sad > 0 {
		delta.Worry = float64(sad) * 0.15
		delta.Affection = 0.05
		delta.Curiosity = float64(sad) * 0.05
	}

	return delta
}

// ── Helpers ──────────────────────────────────────────────────────

func extractJSON(raw string) string {
	raw = strings.TrimSpace(raw)
	raw = strings.TrimPrefix(raw, "```json")
	raw = strings.TrimPrefix(raw, "```")
	raw = strings.TrimSuffix(raw, "```")
	return strings.TrimSpace(raw)
}

func countKeywords(text string, keywords []string) int {
	n := 0
	for _, kw := range keywords {
		n += strings.Count(text, kw)
	}
	return n
}
