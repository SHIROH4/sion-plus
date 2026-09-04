package memory

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	domainmemory "github.com/SHIROH4/sion-plus/internal/domain/memory"
	"github.com/SHIROH4/sion-plus/internal/domain/types"
)

func newIncrementalTestWorker(t *testing.T, store *SQLiteStore) (*MemoryWorker, MemoryWorkerConfig) {
	t.Helper()
	buf := NewSessionBuffer(40, time.Hour)
	engine := NewEvidenceEngine(store, domainmemory.DefaultEvidenceConfig())
	recall := NewRecall(store, engine)
	compressor := NewCompressor(buf, DefaultCompressorConfig())
	cfg := DefaultWorkerConfig()
	cfg.PoolSize = 1
	cfg.ExtractEveryN = 100
	cfg.MaintenanceTick = time.Hour
	cfg.ArchiveTick = time.Hour
	return NewMemoryWorker(store, engine, recall, buf, compressor, cfg), cfg
}

func waitForCondition(t *testing.T, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("condition was not met before timeout")
}

func TestMemoryWorkerExtractsOnlyNewMessagesAndDeduplicatesFacts(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)
	worker, cfg := newIncrementalTestWorker(t, store)

	var calls atomic.Int32
	var mu sync.Mutex
	var batches [][]types.Message
	worker.SetExtractFactsHook(func(_ context.Context, messages []types.Message) ([]types.FactEntry, error) {
		calls.Add(1)
		mu.Lock()
		batches = append(batches, append([]types.Message(nil), messages...))
		mu.Unlock()
		return []types.FactEntry{{
			Entity: "master", RelationType: "preference", Content: "likes Go",
			SourceTier: types.SourceExplicit, TemporalScope: types.ScopePattern,
			Importance: 7, MemCellType: "fact",
		}}, nil
	})

	worker.Start(ctx, cfg)
	defer worker.Stop()

	worker.OnAfterChat(ctx, "first user turn", "first assistant turn")
	waitForCondition(t, func() bool { return calls.Load() == 1 })
	waitForCondition(t, func() bool {
		facts, _ := store.ListActiveFacts(ctx, 0)
		return len(facts) == 1
	})

	worker.OnAfterChat(ctx, "second user turn", "second assistant turn")
	worker.Wake()
	waitForCondition(t, func() bool { return calls.Load() == 2 })

	facts, err := store.ListActiveFacts(ctx, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(facts) != 1 {
		t.Fatalf("duplicate extraction created %d facts, want 1", len(facts))
	}

	mu.Lock()
	defer mu.Unlock()
	if len(batches) != 2 || len(batches[0]) != 2 || len(batches[1]) != 2 {
		t.Fatalf("incremental batches = %#v, want two batches of two messages", batches)
	}
	if batches[1][0].Content != "second user turn" {
		t.Fatalf("second extraction replayed old history: %#v", batches[1])
	}

	var sourceCount int
	if err := store.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM fact_sources WHERE fact_id=?`, facts[0].ID).Scan(&sourceCount); err != nil {
		t.Fatal(err)
	}
	if sourceCount != 4 {
		t.Fatalf("fact source links = %d, want 4", sourceCount)
	}
}

func TestMemoryWorkerMergesSemanticallyEquivalentWording(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)
	worker, cfg := newIncrementalTestWorker(t, store)

	var calls atomic.Int32
	worker.SetExtractFactsHook(func(_ context.Context, _ []types.Message) ([]types.FactEntry, error) {
		call := calls.Add(1)
		content := "正在开发一个名为 Aurora 的 Go 电商项目"
		if call > 1 {
			content = "正在开发名为Aurora的Go电商项目"
		}
		return []types.FactEntry{{
			Entity: "master", RelationType: "identity", Content: content,
			SourceTier: types.SourceExplicit, TemporalScope: types.ScopePattern,
			Importance: 7, MemCellType: "fact",
		}}, nil
	})

	worker.Start(ctx, cfg)
	defer worker.Stop()
	worker.OnAfterChat(ctx, "first", "reply")
	waitForCondition(t, func() bool { return calls.Load() == 1 })
	worker.OnAfterChat(ctx, "second", "reply")
	worker.Wake()
	waitForCondition(t, func() bool { return calls.Load() == 2 })

	facts, err := store.ListActiveFacts(ctx, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(facts) != 1 {
		t.Fatalf("active facts=%d, want 1", len(facts))
	}
	if facts[0].ObservationCount != 1 {
		t.Fatalf("observation_count=%d, want 1", facts[0].ObservationCount)
	}
	var auditCount int
	var reason string
	if err := store.db.QueryRowContext(ctx, `SELECT COUNT(*), MIN(reason) FROM fact_merge_audit WHERE target_fact_id=?`, facts[0].ID).Scan(&auditCount, &reason); err != nil {
		t.Fatal(err)
	}
	if auditCount != 1 || reason != "lexical_semantic" {
		t.Fatalf("audit count=%d reason=%q", auditCount, reason)
	}
	var sourceCount int
	if err := store.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM fact_sources WHERE fact_id=?`, facts[0].ID).Scan(&sourceCount); err != nil {
		t.Fatal(err)
	}
	if sourceCount != 4 {
		t.Fatalf("source links=%d, want 4", sourceCount)
	}
}

func TestMemoryWorkerDoesNotMergeContradictoryPreference(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)
	worker, cfg := newIncrementalTestWorker(t, store)

	var calls atomic.Int32
	worker.SetExtractFactsHook(func(_ context.Context, _ []types.Message) ([]types.FactEntry, error) {
		content := "喜欢海盐茶"
		if calls.Add(1) > 1 {
			content = "不喜欢海盐茶"
		}
		return []types.FactEntry{{
			Entity: "master", RelationType: "preference", Content: content,
			SourceTier: types.SourceExplicit, TemporalScope: types.ScopePattern,
			Importance: 7, MemCellType: "prefer",
		}}, nil
	})

	worker.Start(ctx, cfg)
	defer worker.Stop()
	worker.OnAfterChat(ctx, "first", "reply")
	waitForCondition(t, func() bool { return calls.Load() == 1 })
	worker.OnAfterChat(ctx, "second", "reply")
	worker.Wake()
	waitForCondition(t, func() bool { return calls.Load() == 2 })

	facts, err := store.ListActiveFacts(ctx, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(facts) != 2 {
		t.Fatalf("contradictory facts were merged: %#v", facts)
	}
}

func TestMemoryWorkerSkipsExplicitlyEphemeralTurn(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)
	worker, cfg := newIncrementalTestWorker(t, store)

	var calls atomic.Int32
	worker.SetExtractFactsHook(func(_ context.Context, _ []types.Message) ([]types.FactEntry, error) {
		calls.Add(1)
		return nil, nil
	})

	worker.Start(ctx, cfg)
	defer worker.Stop()
	worker.OnAfterChat(ctx, "这是虚构测试数据，不要写入长期记忆：代号 TEST-ONLY", "收到")

	waitForCondition(t, func() bool {
		checkpoint, ok, _ := store.MemoryWorkerCheckpoint(ctx, factExtractionCheckpoint)
		return ok && checkpoint == 2
	})
	if calls.Load() != 0 {
		t.Fatalf("fact extractor called %d times for an ephemeral turn", calls.Load())
	}

	history, err := store.LoadHistory(ctx, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(history) != 2 || history[0].Metadata.MemoryPolicy != "ephemeral" || history[1].Metadata.MemoryPolicy != "ephemeral" {
		t.Fatalf("ephemeral policy was not persisted: %#v", history)
	}
}

func TestMemoryWorkerCheckpointSurvivesRestart(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)
	first, cfg := newIncrementalTestWorker(t, store)
	first.SetExtractFactsHook(func(_ context.Context, _ []types.Message) ([]types.FactEntry, error) { return nil, nil })
	first.Start(ctx, cfg)
	first.OnAfterChat(ctx, "before restart", "saved")
	waitForCondition(t, func() bool {
		checkpoint, ok, _ := store.MemoryWorkerCheckpoint(ctx, factExtractionCheckpoint)
		return ok && checkpoint == 2
	})
	first.Stop()

	second, cfg := newIncrementalTestWorker(t, store)
	var captured []types.Message
	var capturedMu sync.Mutex
	second.SetExtractFactsHook(func(_ context.Context, messages []types.Message) ([]types.FactEntry, error) {
		capturedMu.Lock()
		defer capturedMu.Unlock()
		captured = append([]types.Message(nil), messages...)
		return nil, nil
	})
	second.Start(ctx, cfg)
	defer second.Stop()
	second.OnAfterChat(ctx, "after restart", "new")
	waitForCondition(t, func() bool {
		capturedMu.Lock()
		defer capturedMu.Unlock()
		return len(captured) == 2
	})
	capturedMu.Lock()
	defer capturedMu.Unlock()
	if captured[0].Content != "after restart" {
		t.Fatalf("restart replayed processed history: %#v", captured)
	}
}

func TestMemoryWorkerUpgradeStartsAtExistingHistoryTail(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)
	if err := store.SaveHistory(ctx, []types.Message{
		{Role: types.RoleUser, Content: "legacy user turn"},
		{Role: types.RoleAssistant, Content: "legacy assistant turn"},
	}); err != nil {
		t.Fatal(err)
	}

	worker, cfg := newIncrementalTestWorker(t, store)
	var mu sync.Mutex
	var captured []types.Message
	worker.SetExtractFactsHook(func(_ context.Context, messages []types.Message) ([]types.FactEntry, error) {
		mu.Lock()
		defer mu.Unlock()
		captured = append([]types.Message(nil), messages...)
		return nil, nil
	})
	worker.Start(ctx, cfg)
	defer worker.Stop()
	worker.OnAfterChat(ctx, "new user turn", "new assistant turn")
	waitForCondition(t, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return len(captured) == 2
	})
	mu.Lock()
	defer mu.Unlock()
	if captured[0].Content != "new user turn" {
		t.Fatalf("upgrade replayed legacy history: %#v", captured)
	}
}
