// Package types defines all shared domain types for the Sion project.
// These types have zero external dependencies — they are pure data structures
// and simple helper functions used by domain, port, adapter, and application layers.
package types

import "time"

// ── Message Roles ──

// MessageRole classifies a chat message by its speaker.
type MessageRole string

const (
	RoleSystem    MessageRole = "system"
	RoleUser      MessageRole = "user"
	RoleAssistant MessageRole = "assistant"
	RoleTool      MessageRole = "tool"
)

// ── Message ──

// Message is a single chat message with optional compression metadata.
type Message struct {
	Role      MessageRole `json:"role"`
	Content   string      `json:"content"`
	Images    []string    `json:"images,omitempty"` // base64 encoded
	Metadata  MessageMeta `json:"meta,omitempty"`
	CreatedAt int64       `json:"created_at"` // unix seconds
}

// MessageMeta carries compression-level metadata for inline archive markers.
// Level 0 = raw message, Level 1-3 = LLM summaries of increasing compression.
type MessageMeta struct {
	Level     int       `json:"level"`
	StartTime time.Time `json:"start_time"`
	EndTime   time.Time `json:"end_time"`
	Name      string    `json:"name"` // archive marker label, e.g. "L1-20240101120000"
}

// ChatContext carries the full pipeline context through a chat turn.
type ChatContext struct {
	Messages         []Message
	UserMessage      string
	SystemPrompt     string
	RecentTurns      string // formatted recent conversation for LLM
	RetrievedFacts   []FactEntry
	RetrievedDiaries []DiaryEntry
	Emotion          EmotionState
	EmotionVec       EmotionVector
	SelfModel        string
	CareSnapshot     UserCareState
	ScreenSummary    string
	TurnCount        int
}
