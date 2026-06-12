package modules

import (
	"strings"
	"testing"

	"github.com/shirohania/sion/internal/domain/types"
	"github.com/shirohania/sion/internal/port"
)

func TestPromptBuilderBasic(t *testing.T) {
	b := NewPromptBuilder("你是一只猫娘。")

	input := BuildInput{
		UserMessage: "今天天气真好",
		L0Messages: []types.Message{
			{Role: types.RoleUser, Content: "你好"},
			{Role: types.RoleAssistant, Content: "主人好~"},
		},
		Facts: []port.MemorySearchResult{
			{Content: "喜欢晴天", Evidence: &types.EvidenceSnapshot{Status: "confirmed"}},
		},
		Diaries: []port.MemorySearchResult{
			{Content: "上午一起散步"},
		},
		Boundaries: []port.MemorySearchResult{
			{Content: "工作时不要打扰"},
		},
		UserModel: "主人是后端工程师",
		SelfModel: "我是Sion，一只猫娘",
		Emotion:   types.EmotionState{Primary: "joy", Intensity: 0.6},
	}

	result := b.Build(input)

	// System prompt should just be the personality
	if result.SystemPrompt != "你是一只猫娘。" {
		t.Errorf("SystemPrompt: got %q", result.SystemPrompt)
	}

	// Memory context should contain all sections
	mctx := result.MemoryContext
	checks := []string{
		"[最近对话]", "你好", "主人好~",
		"[相关记忆]", "喜欢晴天", "已确认",
		"用户边界", "工作时不要打扰",
		"[近期情景]", "上午一起散步",
		"[用户画像]", "后端工程师",
		"[自我认知]", "Sion",
		"[当前心情]", "joy",
	}
	for _, c := range checks {
		if !strings.Contains(mctx, c) {
			t.Errorf("MemoryContext missing %q", c)
		}
	}
}

func TestPromptBuilderEmptyInput(t *testing.T) {
	b := NewPromptBuilder("base")
	result := b.Build(BuildInput{})

	if result.SystemPrompt != "base" {
		t.Error("system prompt should be base")
	}
	if result.MemoryContext == "" {
		t.Error("memory context should not be empty (at least emotion)")
	}
}

func TestWrapUserMessage(t *testing.T) {
	b := NewPromptBuilder("x")
	wrapped := b.WrapUserMessage("你好", "今天是晴天\n主人很开心")
	if !strings.Contains(wrapped, "<memory-context>") {
		t.Error("wrapped message should contain <memory-context> tag")
	}
	if !strings.Contains(wrapped, "你好") {
		t.Error("wrapped message should contain original user message")
	}
	if !strings.Contains(wrapped, "今天是晴天") {
		t.Error("wrapped message should contain memory context")
	}
}

func TestRoleLabel(t *testing.T) {
	if roleLabel(types.RoleUser) != "用户" {
		t.Errorf("user: %s", roleLabel(types.RoleUser))
	}
	if roleLabel(types.RoleAssistant) != "AI" {
		t.Errorf("assistant: %s", roleLabel(types.RoleAssistant))
	}
	if roleLabel(types.RoleSystem) != "系统" {
		t.Errorf("system: %s", roleLabel(types.RoleSystem))
	}
}

func TestPromptBuilderNoFacts(t *testing.T) {
	b := NewPromptBuilder("x")
	result := b.Build(BuildInput{
		Emotion: types.EmotionState{Primary: "neutral"},
	})
	if strings.Contains(result.MemoryContext, "[相关记忆]") {
		t.Error("should not have facts section when empty")
	}
	if strings.Contains(result.MemoryContext, "用户边界") {
		t.Error("should not have boundary section when empty")
	}
}
