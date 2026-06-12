package port

import "context"

// ── Token Usage Tracker ──

// TokenUsageTracker records and reports LLM token consumption.
// Non-blocking Record() → in-memory buffer → async JSONL flush.
// Query Summary/TodaySummary for usage dashboards.
// Implementation: adapter/llm/token_tracker.go
type TokenUsageTracker interface {
	// Record logs a single LLM call. Non-blocking.
	// callType examples: "chat"|"emotion"|"memory_extraction"|"memory_summary"
	//   |"signal_detection"|"diary_generation"|"strategy_reflection"
	//   |"search_summary"|"vision_analysis"|"curiosity_exploration"
	Record(ctx context.Context, callType string, promptTokens, completionTokens int)

	// Summary returns aggregated usage since the given timestamp (unix seconds).
	Summary(ctx context.Context, since int64) (*TokenUsageSummary, error)

	// TodaySummary returns today's usage.
	TodaySummary(ctx context.Context) (*TokenUsageSummary, error)

	// Start starts the background flush goroutine.
	Start(ctx context.Context) error

	// Stop gracefully shuts down, flushing remaining buffer.
	Stop(ctx context.Context) error
}

type TokenUsageSummary struct {
	TotalPrompt     int                  `json:"total_prompt"`
	TotalCompletion int                  `json:"total_completion"`
	TotalCalls      int                  `json:"total_calls"`
	ByType          map[string]TypeStats `json:"by_type"`
	Since           int64                `json:"since"`
	GeneratedAt     int64                `json:"generated_at"`
}

type TypeStats struct {
	Calls            int `json:"calls"`
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	Errors           int `json:"errors"`
}
