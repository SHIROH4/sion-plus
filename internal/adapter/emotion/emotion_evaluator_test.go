package emotion

import (
	"testing"
)

func TestExtractJSON(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want string
	}{
		{"plain json", `{"affection": 0.1, "worry": 0.0}`, `{"affection": 0.1, "worry": 0.0}`},
		{"with markdown fence", "```json\n{\"affection\": 0.1}\n```", `{"affection": 0.1}`},
		{"with markdown no lang", "```\n{\"affection\": 0.1}\n```", `{"affection": 0.1}`},
		{"with surrounding text", "here is the result:\n{\"affection\": 0.1}\nthat's it", "here is the result:\n{\"affection\": 0.1}\nthat's it"},
		{"trailing prefix only", "```json\n{\"affection\": 0.1}", `{"affection": 0.1}`},
		{"whitespace", "  \n  {\"affection\": 0.1}  \n", `{"affection": 0.1}`},
		{"empty", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractJSON(tt.raw)
			if got != tt.want {
				t.Errorf("extractJSON(%q) = %q, want %q", tt.raw, got, tt.want)
			}
		})
	}
}

func TestEvaluateRules(t *testing.T) {
	e := &EmotionEvaluator{}

	t.Run("positive", func(t *testing.T) {
		delta := e.evaluateRules("哈哈好棒！谢谢你帮助我")
		if delta.Affection <= 0 {
			t.Error("expected positive affection delta")
		}
		if delta.Confidence <= 0 {
			t.Error("expected positive confidence delta")
		}
	})

	t.Run("negative", func(t *testing.T) {
		delta := e.evaluateRules("你好烦啊别吵了滚")
		if delta.Annoyance <= 0 {
			t.Error("expected positive annoyance delta")
		}
		if delta.Worry <= 0 {
			t.Error("expected positive worry delta")
		}
	})

	t.Run("negative short amplified", func(t *testing.T) {
		delta := e.evaluateRules("滚 废物")
		if delta.Annoyance <= 0.2 {
			t.Errorf("expected amplified annoyance, got %.2f", delta.Annoyance)
		}
		// Short negative should have negBoost=1.8
		if delta.Annoyance < 0.3 {
			t.Errorf("expected stronger annoyance with boost, got %.2f", delta.Annoyance)
		}
	})

	t.Run("sad", func(t *testing.T) {
		delta := e.evaluateRules("唉，今天好难受，想哭")
		if delta.Worry <= 0 {
			t.Error("expected positive worry delta")
		}
	})

	t.Run("short ellipsis", func(t *testing.T) {
		delta := e.evaluateRules("。。。")
		if delta.Sleepiness <= 0 {
			t.Error("expected positive sleepiness")
		}
		if delta.Loneliness <= 0 {
			t.Error("expected positive loneliness")
		}
	})

	t.Run("question marks", func(t *testing.T) {
		delta := e.evaluateRules("？？？")
		if delta.Worry <= 0 {
			t.Error("expected positive worry for multiple question marks")
		}
		if delta.Curiosity <= 0 {
			t.Error("expected positive curiosity")
		}
	})

	t.Run("neutral", func(t *testing.T) {
		delta := e.evaluateRules("今天天气不错")
		if delta.Affection != 0 || delta.Annoyance != 0 || delta.Worry != 0 || delta.Curiosity != 0 || delta.Sleepiness != 0 || delta.Loneliness != 0 || delta.Playfulness != 0 || delta.Confidence != 0 {
			t.Error("expected all-zero delta for neutral message")
		}
	})

	t.Run("mixed positive and negative", func(t *testing.T) {
		delta := e.evaluateRules("谢谢你但是有时候真的好烦")
		// Both positive and negative keywords, both should register
		if delta.Affection <= 0 || delta.Annoyance <= 0 {
			t.Errorf("expected both affection and annoyance, got aff=%.2f ann=%.2f", delta.Affection, delta.Annoyance)
		}
	})
}

func TestCountKeywords(t *testing.T) {
	tests := []struct {
		name     string
		text     string
		keywords []string
		want     int
	}{
		{"single", "哈哈好棒", []string{"哈哈", "好棒"}, 2},
		{"double count", "谢谢谢谢", []string{"谢谢"}, 2},
		{"zero", "hello world", []string{"哈哈", "好棒"}, 0},
		{"mixed", "哈哈开心谢谢厉害好棒", []string{"哈哈", "开心", "谢谢", "厉害", "好棒"}, 5},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := countKeywords(tt.text, tt.keywords)
			if got != tt.want {
				t.Errorf("countKeywords(%q) = %d, want %d", tt.text, got, tt.want)
			}
		})
	}
}

func TestNeedsLLMEmotionEvaluation(t *testing.T) {
	tests := []struct {
		text string
		want bool
	}{
		{"请解释 Redis Lua 的原子性", false},
		{"只回复 PERF_01_OK", false},
		{"今天任务很多，我有点焦虑", true},
		{"谢谢你陪我", true},
		{"I feel lonely today", true},
	}
	for _, test := range tests {
		if got := needsLLMEmotionEvaluation(test.text); got != test.want {
			t.Errorf("needsLLMEmotionEvaluation(%q)=%v, want %v", test.text, got, test.want)
		}
	}
}
