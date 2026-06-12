package llm

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/SHIROH4/sion-plus/internal/port"
)

func TestRecordAndFlush(t *testing.T) {
	dir := t.TempDir()
	tracker := NewTokenTracker(dir)

	ctx := context.Background()
	tracker.Start(ctx)
	defer tracker.Stop(ctx)

	// Record 5 entries
	for i := 0; i < 5; i++ {
		tracker.Record(ctx, "chat", 100, 50)
	}

	// Force flush by stopping
	tracker.Stop(ctx)

	// Verify file was written
	tokenDir := filepath.Join(dir, "token_usage")
	files, err := os.ReadDir(tokenDir)
	if err != nil {
		t.Fatalf("token_usage dir not created: %v", err)
	}
	if len(files) == 0 {
		t.Fatal("no jsonl files created")
	}

	// Verify summary
	summary, err := tracker.Summary(ctx, 0)
	if err != nil {
		t.Fatalf("Summary: %v", err)
	}
	if summary.TotalCalls != 5 {
		t.Errorf("expected 5 calls, got %d", summary.TotalCalls)
	}
	if summary.TotalPrompt != 500 {
		t.Errorf("expected 500 prompt tokens, got %d", summary.TotalPrompt)
	}
	if summary.TotalCompletion != 250 {
		t.Errorf("expected 250 completion tokens, got %d", summary.TotalCompletion)
	}
}

func TestBufferFlushesAtCapacity(t *testing.T) {
	dir := t.TempDir()
	tracker := NewTokenTracker(dir)

	ctx := context.Background()
	tracker.Start(ctx)
	defer tracker.Stop(ctx)

	// Record more than maxBufferSize (100)
	for i := 0; i < 150; i++ {
		tracker.Record(ctx, "memory_extraction", 10, 5)
	}

	// Stop (forces final flush)
	tracker.Stop(ctx)

	summary, _ := tracker.Summary(ctx, 0)
	if summary.TotalCalls != 150 {
		t.Errorf("expected 150 calls, got %d", summary.TotalCalls)
	}
}

func TestTodaySummaryFiltersByDate(t *testing.T) {
	dir := t.TempDir()
	tracker := NewTokenTracker(dir)

	ctx := context.Background()
	tracker.Start(ctx)

	// Record now
	tracker.Record(ctx, "chat", 100, 50)
	tracker.Stop(ctx)

	// Query with "since tomorrow" → should get 0
	tomorrowStart := time.Now().Add(24 * time.Hour).Truncate(24 * time.Hour).Unix()
	summary, err := tracker.Summary(ctx, tomorrowStart)
	if err != nil {
		t.Fatalf("Summary: %v", err)
	}
	if summary.TotalCalls != 0 {
		t.Errorf("expected 0 calls since tomorrow, got %d", summary.TotalCalls)
	}

	// TodaySummary should return today's data
	todaySummary, err := tracker.TodaySummary(ctx)
	if err != nil {
		t.Fatalf("TodaySummary: %v", err)
	}
	if todaySummary.TotalCalls != 1 {
		t.Errorf("expected 1 call today, got %d", todaySummary.TotalCalls)
	}
}

func TestMultipleCallTypes(t *testing.T) {
	dir := t.TempDir()
	tracker := NewTokenTracker(dir)

	ctx := context.Background()
	tracker.Start(ctx)
	defer tracker.Stop(ctx)

	tracker.Record(ctx, "chat", 100, 50)
	tracker.Record(ctx, "chat", 200, 100)
	tracker.Record(ctx, "emotion", 50, 20)
	tracker.Record(ctx, "memory_extraction", 80, 30)

	tracker.Stop(ctx)

	summary, _ := tracker.Summary(ctx, 0)

	if summary.TotalCalls != 4 {
		t.Errorf("expected 4 calls, got %d", summary.TotalCalls)
	}

	// By type
	chatStats := summary.ByType["chat"]
	if chatStats.Calls != 2 {
		t.Errorf("expected 2 chat calls, got %d", chatStats.Calls)
	}
	if chatStats.PromptTokens != 300 {
		t.Errorf("expected 300 chat prompt tokens, got %d", chatStats.PromptTokens)
	}

	emotionStats := summary.ByType["emotion"]
	if emotionStats.Calls != 1 {
		t.Errorf("expected 1 emotion call, got %d", emotionStats.Calls)
	}

	memStats := summary.ByType["memory_extraction"]
	if memStats.Calls != 1 {
		t.Errorf("expected 1 memory_extraction call, got %d", memStats.Calls)
	}
}

func TestRecordNonBlocking(t *testing.T) {
	dir := t.TempDir()
	tracker := NewTokenTracker(dir)

	ctx := context.Background()
	tracker.Start(ctx)

	// Record without starting goroutine → still works (buffer grows)
	done := make(chan bool)
	go func() {
		for i := 0; i < 1000; i++ {
			tracker.Record(ctx, "chat", 10, 5)
		}
		done <- true
	}()

	select {
	case <-done:
	// OK
	case <-time.After(2 * time.Second):
		t.Fatal("Record blocked unexpectedly")
	}

	tracker.Stop(ctx)
}

func TestEmptySummary(t *testing.T) {
	dir := t.TempDir()
	tracker := NewTokenTracker(dir)

	ctx := context.Background()
	summary, err := tracker.Summary(ctx, 0)
	if err != nil {
		t.Fatalf("Summary: %v", err)
	}
	if summary.TotalCalls != 0 {
		t.Errorf("expected 0 calls for empty tracker, got %d", summary.TotalCalls)
	}
}

func TestPortCompliance(t *testing.T) {
	var _ port.TokenUsageTracker = NewTokenTracker(t.TempDir())
}
