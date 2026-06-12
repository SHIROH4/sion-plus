package port

import (
	"context"
	"github.com/SHIROH4/sion-plus/internal/domain/types"
)

// ── Intent Scheduler ──

// IntentScheduler queues, prioritizes, coalesces, and releases proactive intents.
// Reference: N.E.K.O ProactiveDeliveryManager — priority ordering, coalescing by key,
// TTL-based staleness, batch release after playback end + min-gap.
// Implementation: adapter/proactive/intent_scheduler.go
type IntentScheduler interface {
	Submit(ctx context.Context, intent types.ProactiveIntent) error
	Drain() []types.ProactiveIntent
	Peek() []types.ProactiveIntent
	Stats() SchedulerStats
	Reset()
}

type SchedulerStats struct {
	Queued    int `json:"queued"`
	DroppedTTL int `json:"dropped_ttl"`
	Coalesced int `json:"coalesced"`
	Delivered int `json:"delivered"`
}

// ── Delivery Gate ──

// DeliveryGate controls whether the AI can speak/act now.
// Gate conditions: not playing audio, not in inflight, min-gap elapsed,
// no active LLM response, not in silent mode.
type DeliveryGate interface {
	CanRelease(ctx context.Context) bool
	OnPlaybackStart(ctx context.Context)
	OnPlaybackEnd(ctx context.Context)
	Reset()
}

// ── Intent Deliverer ──

// IntentDeliverer executes the delivery of one or more intents.
// A batch of intents is sent to the LLM for rephrasing into the AI's voice,
// then output via the appropriate channel (voice injection, text display, etc.).
type IntentDeliverer interface {
	Deliver(ctx context.Context, intents []types.ProactiveIntent) (*DeliveryResult, error)
}

type DeliveryResult struct {
	Delivered  int    `json:"delivered"`
	Output     string `json:"output"`
	Source     string `json:"source"`
	WasBatched bool   `json:"was_batched"`
	SkippedTTL int    `json:"skipped_ttl"`
}
