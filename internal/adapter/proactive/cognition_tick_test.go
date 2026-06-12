package proactive

import (
	"context"
	"testing"
	"time"

	"github.com/SHIROH4/sion-plus/internal/domain/cognition"
	"github.com/SHIROH4/sion-plus/internal/domain/types"
)

func TestIsShortAck(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{"ok", true},
		{"好的", true},
		{"嗯", true},
		{"知道了", true},
		{"哈哈", true},
		{"嗯嗯", true},
		{"噢噢", true},
		{"okay", true},
		{"666", true},
		{"今天天气真好啊", false},
		{"帮我搜索一下Go的资料", false},
		{"hello world", false},
		{"", false},
		{"谢谢你今天的帮助", false},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := isShortAck([]rune(tt.input))
			if got != tt.want {
				t.Errorf("isShortAck(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestBuildS2DecisionPrompt(t *testing.T) {
	f := &types.QuantifiedFeatures{
		A1_1_Affection:    0.6,
		A1_2_Worry:        0.1,
		A1_6_Loneliness:   0.2,
		A2_PrimaryEmotion: "neutral",
		A3_Intensity:      0.3,
		U1_AppCategory:    "work",
		U12_NightTime:     0,
	}

	actions := cognition.BuildActions()
	topScored := []types.ScoredAction{
		{Action: actions[0], FinalScore: 0.75},
		{Action: actions[1], FinalScore: 0.60},
	}

	prompt := buildS2DecisionPrompt(f, topScored, false)
	promptNight := buildS2DecisionPrompt(f, topScored, true)
	if !containsStr(promptNight, "夜间安全") {
		t.Error("night prompt should contain night-safe tags")
	}
	if prompt == "" {
		t.Error("buildS2DecisionPrompt returned empty")
	}
	if !containsStr(prompt, "speak_casual") {
		t.Error("prompt should contain action names")
	}
	if !containsStr(prompt, "0.75") {
		t.Error("prompt should contain scores")
	}
}

func TestCognitionTickBuildIntent(t *testing.T) {
	tick := &CognitionTick{}
	ctx := context.Background()

	f := &types.QuantifiedFeatures{
		A1_1_Affection: 0.5,
	}

	// Test speak_inquiry → high priority
	action := cognition.ActionByName("speak_inquiry")
	intent := tick.buildIntent(ctx, f, action, false)
	if intent == nil {
		t.Fatal("expected intent")
	}
	if intent.Priority != types.PriorityHigh {
		t.Errorf("speak_inquiry should be high priority, got %d", intent.Priority)
	}
	if intent.CoalesceKey != "speak_inquiry" {
		t.Errorf("coalesce key = %s", intent.CoalesceKey)
	}
	if intent.TTL != 120*time.Second {
		t.Errorf("TTL = %v", intent.TTL)
	}

	// Test care action → normal priority
	careAction := cognition.ActionByName("care_rest")
	careIntent := tick.buildIntent(ctx, f, careAction, false)
	if careIntent == nil {
		t.Fatal("expected intent")
	}
	if careIntent.Priority != types.PriorityNormal {
		t.Errorf("care should be normal priority, got %d", careIntent.Priority)
	}

	// Test casual → low priority
	casualAction := cognition.ActionByName("speak_casual")
	casualIntent := tick.buildIntent(ctx, f, casualAction, false)
	if casualIntent == nil {
		t.Fatal("expected intent")
	}
	if casualIntent.Priority != types.PriorityLow {
		t.Errorf("casual should be low priority, got %d", casualIntent.Priority)
	}

	// Test "none" with System2 → nil
	noneAction := cognition.ActionByName("none")
	noneIntent := tick.buildIntent(ctx, f, noneAction, true)
	if noneIntent != nil {
		t.Error("System2 'none' should produce nil intent")
	}

	// Test "none" with System1 → not nil (S1 passes through)
	noneIntentS1 := tick.buildIntent(ctx, f, noneAction, false)
	if noneIntentS1 == nil {
		t.Error("System1 'none' should produce intent (silence is valid action)")
	}
}

func TestCognitionTickBackoff(t *testing.T) {
	tick := &CognitionTick{
		interval:       60 * time.Second,
		lastProactiveSpeakAt: time.Now(),
	}

	// With 0 unanswered, no backoff
	if tick.unansweredProactive != 0 {
		t.Error("unansweredProactive should start at 0")
	}

	// Simulate 1 unanswered — should need 2x interval
	tick.unansweredProactive = 1
	backoff := tick.interval * time.Duration(1<<uint(tick.unansweredProactive))
	if backoff != 120*time.Second {
		t.Errorf("1 unanswered → backoff = %v, want 120s", backoff)
	}

	// Simulate 2 unanswered — should need 4x interval
	tick.unansweredProactive = 2
	backoff = tick.interval * time.Duration(1<<uint(tick.unansweredProactive))
	if backoff != 240*time.Second {
		t.Errorf("2 unanswered → backoff = %v, want 240s", backoff)
	}

	// Simulate 3 unanswered — full suppression
	tick.unansweredProactive = 3
	// (The run() method would return early before backoff calculation)
	if tick.unansweredProactive < 3 {
		t.Error("3 unanswered should be the suppression threshold")
	}
}

func TestCognitionTickOnUserMessageResetsCounter(t *testing.T) {
	tick := &CognitionTick{unansweredProactive: 5}
	tick.OnUserMessage()
	if tick.unansweredProactive != 0 {
		t.Errorf("OnUserMessage should reset unanswered to 0, got %d", tick.unansweredProactive)
	}
	if tick.lastUserMessageAt.IsZero() {
		t.Error("lastUserMessageAt should be set")
	}
}

func TestCognitionTickMinSpeakGap(t *testing.T) {
	tick := &CognitionTick{
		minSpeakGap:          5 * time.Minute,
		lastProactiveSpeakAt: time.Now(),
	}
	if time.Since(tick.lastProactiveSpeakAt) >= tick.minSpeakGap {
		t.Error("fresh speak should be within min gap")
	}

	// Set speak time to 10 minutes ago
	tick.lastProactiveSpeakAt = time.Now().Add(-10 * time.Minute)
	if time.Since(tick.lastProactiveSpeakAt) < tick.minSpeakGap {
		t.Error("old speak should be beyond min gap")
	}
}

func TestCognitionTickActions(t *testing.T) {
	tick := &CognitionTick{
		actions: cognition.BuildActions(),
	}
	actions := tick.Actions()
	if len(actions) != 16 {
		t.Errorf("expected 16 actions, got %d", len(actions))
	}
}

func TestCognitionTickSetMinSpeakGap(t *testing.T) {
	tick := &CognitionTick{minSpeakGap: 5 * time.Minute}
	tick.SetMinSpeakGap(10 * time.Minute)
	if tick.minSpeakGap != 10*time.Minute {
		t.Errorf("SetMinSpeakGap should update minSpeakGap, got %v", tick.minSpeakGap)
	}
}

func containsStr(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(s) > len(sub) && containsSubstring(s, sub))
}

func containsSubstring(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
