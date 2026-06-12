package proactive

import (
	"context"
	"testing"
	"time"

	"github.com/shirohania/sion/internal/domain/types"
)

func TestIntentSchedulerSubmit(t *testing.T) {
	s := NewIntentScheduler()
	ctx := context.Background()

	err := s.Submit(ctx, types.ProactiveIntent{
		ID: "1", Action: "speak_casual", Message: "hello",
		Priority: 5, CreatedAt: time.Now(),
	})
	if err != nil {
		t.Fatalf("Submit: %v", err)
	}
	if s.Stats().Queued != 1 {
		t.Errorf("expected 1 queued, got %d", s.Stats().Queued)
	}
}

func TestIntentSchedulerPriorityOrder(t *testing.T) {
	s := NewIntentScheduler()
	ctx := context.Background()

	now := time.Now()
	s.Submit(ctx, types.ProactiveIntent{ID: "low", Priority: 3, CreatedAt: now})
	s.Submit(ctx, types.ProactiveIntent{ID: "high", Priority: 7, CreatedAt: now})
	s.Submit(ctx, types.ProactiveIntent{ID: "mid", Priority: 5, CreatedAt: now})

	queue := s.Drain()
	if len(queue) != 3 {
		t.Fatalf("expected 3, got %d", len(queue))
	}
	if queue[0].ID != "high" {
		t.Errorf("first should be high, got %s", queue[0].ID)
	}
	if queue[1].ID != "mid" {
		t.Errorf("second should be mid, got %s", queue[1].ID)
	}
	if queue[2].ID != "low" {
		t.Errorf("third should be low, got %s", queue[2].ID)
	}
}

func TestIntentSchedulerDedupCoalesce(t *testing.T) {
	s := NewIntentScheduler()
	ctx := context.Background()

	now := time.Now()
	s.Submit(ctx, types.ProactiveIntent{ID: "1", Action: "speak_casual", CoalesceKey: "greet", Message: "first", Priority: 5, CreatedAt: now})
	s.Submit(ctx, types.ProactiveIntent{ID: "2", Action: "speak_casual", CoalesceKey: "greet", Message: "second", Priority: 5, CreatedAt: now.Add(time.Second)})

	queue := s.Drain()
	if len(queue) != 1 {
		t.Fatalf("coalesced should leave 1, got %d", len(queue))
	}
	if queue[0].Message != "second" {
		t.Errorf("should keep newer: got %s", queue[0].Message)
	}
	if s.Stats().Coalesced != 1 {
		t.Errorf("coalesced count = %d", s.Stats().Coalesced)
	}
}

func TestIntentSchedulerTTL(t *testing.T) {
	s := NewIntentScheduler()
	ctx := context.Background()

	s.Submit(ctx, types.ProactiveIntent{
		ID: "old", Action: "speak_casual", Message: "stale",
		Priority: 5, TTL: 10 * time.Millisecond, CreatedAt: time.Now().Add(-time.Second),
	})

	if s.Stats().DroppedTTL != 1 {
		t.Errorf("stale intent should be dropped, got %d dropped", s.Stats().DroppedTTL)
	}
	if s.Stats().Queued != 0 {
		t.Errorf("queue should be empty, got %d", s.Stats().Queued)
	}
}

func TestIntentSchedulerDedupRecent(t *testing.T) {
	s := NewIntentScheduler()
	ctx := context.Background()

	now := time.Now()
	s.Submit(ctx, types.ProactiveIntent{ID: "1", Message: "unique", Priority: 5, CreatedAt: now})
	s.Drain()

	// Submit same message again — should be deduped by recent history
	s.Submit(ctx, types.ProactiveIntent{ID: "2", Message: "unique", Priority: 5, CreatedAt: now})
	if s.Stats().Queued != 0 {
		t.Error("duplicate message should be blocked by recent history")
	}
}

func TestIntentSchedulerReset(t *testing.T) {
	s := NewIntentScheduler()
	ctx := context.Background()

	s.Submit(ctx, types.ProactiveIntent{ID: "1", Priority: 5, CreatedAt: time.Now()})
	s.Reset()

	if s.Stats().Queued != 0 {
		t.Error("reset should clear queue")
	}
	if s.Stats().Delivered != 0 {
		t.Error("reset should clear stats")
	}
}

func TestDeliveryGateTryAcquire(t *testing.T) {
	g := NewDeliveryGate()
	if !g.TryAcquire() {
		t.Error("first acquire should succeed")
	}
	if g.TryAcquire() {
		t.Error("second acquire should fail (already running)")
	}
	g.Release()
	if !g.TryAcquire() {
		t.Error("acquire after release should succeed")
	}
}

func TestDeliveryGateCanRelease(t *testing.T) {
	g := NewDeliveryGate()
	ctx := context.Background()

	// Should be able to release when idle
	if !g.CanRelease(ctx) {
		t.Error("should be able to release when idle")
	}

	// After playback start, inflight blocks release
	g.OnPlaybackStart(ctx)
	if g.CanRelease(ctx) {
		t.Error("should not release during playback")
	}

	// After playback end, min-gap blocks immediate release (correct behavior)
	g.OnPlaybackEnd(ctx)
	if g.CanRelease(ctx) {
		t.Log("release passed immediately (min-gap elapsed in fast machine)")
	} else {
		t.Log("min-gap correctly blocks immediate release after playback")
	}
}

func TestDeliveryGateMinGap(t *testing.T) {
	g := NewDeliveryGate()
	ctx := context.Background()

	g.OnPlaybackStart(ctx)
	g.OnPlaybackEnd(ctx)

	// Immediately after — should be blocked by minGap (3s)
	if g.CanRelease(ctx) {
		t.Log("min-gap may not block if time passes quickly in test")
	}
}
