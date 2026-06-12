package modules

import (
	"fmt"
	"strings"

	"github.com/SHIROH4/sion-plus/internal/domain/types"
	"github.com/SHIROH4/sion-plus/internal/port"
)

// PromptBuilder assembles the system prompt and memory-context injection.
// Keeps the system prompt cache-stable (personality only) and injects
// dynamic memory context into the user message.
type PromptBuilder struct {
	personality string
	userModel   string // injected from SelfModelStore on build
	selfModel   string // injected from SelfModelStore on build
}

// NewPromptBuilder creates a builder with the given personality text.
func NewPromptBuilder(personality string) *PromptBuilder {
	return &PromptBuilder{personality: personality}
}

// SetUserModel sets the user model text (from SelfModelStore).
func (b *PromptBuilder) SetUserModel(s string) { b.userModel = s }

// SetSelfModel sets the self model text (from SelfModelStore).
func (b *PromptBuilder) SetSelfModel(s string) { b.selfModel = s }

// BuildResult contains the assembled prompt components.
type BuildResult struct {
	SystemPrompt string   // fixed personality, cache-stable
	MemoryContext string  // injected into user message
	Warnings      []string // budget exceeded warnings
}

// BuildInput is the full context needed to assemble a system prompt.
type BuildInput struct {
	UserMessage   string
	L0Messages    []types.Message
	L0Memo        string
	Facts         []port.MemorySearchResult
	Diaries       []port.MemorySearchResult
	Boundaries    []port.MemorySearchResult
	UserModel     string
	SelfModel     string
	Emotion       types.EmotionState
	ScreenSummary string
}

// Build assembles the system prompt and memory context.
// System prompt is fixed personality only (cache-stable).
// Memory context is injected into the user message via <memory-context> tags.
func (b *PromptBuilder) Build(input BuildInput) BuildResult {
	var ctx strings.Builder
	warnings := make([]string, 0)

	// ── L0: recent conversation ──
	ctx.WriteString("[最近对话]\n")
	for _, m := range input.L0Messages {
		role := roleLabel(m.Role)
		ctx.WriteString(fmt.Sprintf("%s: %s\n", role, m.Content))
	}
	if input.L0Memo != "" {
		ctx.WriteString(fmt.Sprintf("[之前摘要] %s\n", input.L0Memo))
	}

	// ── L1: retrieved facts ──
	if len(input.Facts) > 0 {
		ctx.WriteString("\n[相关记忆]\n")
		for _, f := range input.Facts {
			ctx.WriteString(fmt.Sprintf("- %s", f.Content))
			if f.Evidence != nil && f.Evidence.Status == "confirmed" {
				ctx.WriteString(" (已确认)")
			}
			ctx.WriteString("\n")
		}
	}

	// ── L1: boundaries (always injected) ──
	if len(input.Boundaries) > 0 {
		ctx.WriteString("\n[⚠️ 用户边界 — 绝对不能违反]\n")
		for _, b := range input.Boundaries {
			ctx.WriteString(fmt.Sprintf("- %s\n", b.Content))
		}
	}

	// ── L1: diary ──
	if len(input.Diaries) > 0 {
		ctx.WriteString("\n[近期情景]\n")
		for _, d := range input.Diaries {
			ctx.WriteString(fmt.Sprintf("- %s\n", d.Content))
		}
	}

	// ── L2: user model ──
	if input.UserModel != "" {
		ctx.WriteString(fmt.Sprintf("\n[用户画像]\n%s\n", input.UserModel))
	}

	// ── L2: self model ──
	if input.SelfModel != "" {
		ctx.WriteString(fmt.Sprintf("\n[自我认知]\n%s\n", input.SelfModel))
	}

	// ── Emotion context ──
	ctx.WriteString(fmt.Sprintf("\n[当前心情]\n情绪: %s, 强度: %.1f\n",
		input.Emotion.Primary, input.Emotion.Intensity))

	// ── Screen context ──
	if input.ScreenSummary != "" {
		ctx.WriteString(fmt.Sprintf("\n[当前屏幕]\n%s\n", input.ScreenSummary))
	}

	return BuildResult{
		SystemPrompt: b.personality,
		MemoryContext: ctx.String(),
		Warnings:      warnings,
	}
}

// WrapUserMessage wraps the user message with <memory-context> tags.
// System prompt remains cache-stable; only user message changes per turn.
func (b *PromptBuilder) WrapUserMessage(userMessage, memoryContext string) string {
	return fmt.Sprintf("<memory-context>\n%s\n</memory-context>\n\n%s", memoryContext, userMessage)
}

func roleLabel(role types.MessageRole) string {
	switch role {
	case types.RoleUser:
		return "用户"
	case types.RoleAssistant:
		return "AI"
	case types.RoleSystem:
		return "系统"
	default:
		return string(role)
	}
}
