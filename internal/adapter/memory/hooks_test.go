package memory

import (
	"context"
	"fmt"
	"strings"
	"sync/atomic"
	
	"testing"
	"time"

	"github.com/SHIROH4/sion-plus/internal/domain/memory"
	"github.com/SHIROH4/sion-plus/internal/domain/types"
	"github.com/SHIROH4/sion-plus/internal/port"
)

// mockLLM is a fake LLMExecutor that returns canned responses for testing.
type mockLLM struct {
	chatFn func(ctx context.Context, systemPrompt string, msgs []port.LLMMessage) (string, error)
}

func (m *mockLLM) Chat(ctx context.Context, systemPrompt string, msgs []port.LLMMessage) (string, error) {
	return m.chatFn(ctx, systemPrompt, msgs)
}

func (m *mockLLM) ChatWithTools(ctx context.Context, sp string, msgs []port.LLMMessage, tools []port.ToolDef, onToolCall func(string, string) string, maxRounds int, tc string) (string, error) {
	return "", nil
}

func (m *mockLLM) ChatStream(ctx context.Context, sp string, msgs []port.LLMMessage, onChunk func(string) error) error {
	return nil
}

func (m *mockLLM) IsAvailable(ctx context.Context) bool { return true }

var _ port.LLMExecutor = (*mockLLM)(nil)

func newFullStack(t *testing.T) (*MemoryWorker, *Compressor, *SessionBuffer, *SQLiteStore) {
	t.Helper()

	store := newTestStore(t)
	buf := NewSessionBuffer(40, time.Hour)
	cfg := memory.DefaultEvidenceConfig()
	engine := NewEvidenceEngine(store, cfg)
	recall := NewRecall(store, engine)
	comp := NewCompressor(buf, DefaultCompressorConfig())

	workerCfg := DefaultWorkerConfig()
	workerCfg.ExtractEveryN = 3 // trigger faster for tests
	worker := NewMemoryWorker(store, engine, recall, buf, comp, workerCfg)

	return worker, comp, buf, store
}

func TestLLMHooksExtractFacts(t *testing.T) {
	worker, comp, _, store := newFullStack(t)

	llm := &mockLLM{
		chatFn: func(ctx context.Context, sp string, msgs []port.LLMMessage) (string, error) {
			return `{"facts": [
				{"entity": "master", "relation_type": "preference", "content": "likes Go", "source_tier": "explicit", "importance": 7},
				{"entity": "master", "relation_type": "identity", "content": "backend engineer", "source_tier": "explicit", "importance": 8}
			]}`, nil
		},
	}

	hooks := NewLLMHooks(llm, worker, comp)
	hooks.Install()

	// Start the worker goroutines
	ctx := context.Background()
	worker.Start(ctx, DefaultWorkerConfig())
	defer worker.Stop()

	// Simulate a conversation
	worker.OnAfterChat(ctx, "I'm a backend engineer and I love Go", "That's great!")

	// Trigger extraction
	worker.Wake()
	time.Sleep(500 * time.Millisecond) // let goroutine process

	// Verify facts were persisted
	facts, err := store.ListActiveFacts(context.Background(), 0)
	if err != nil {
		t.Fatalf("ListActiveFacts: %v", err)
	}
	if len(facts) < 2 {
		t.Fatalf("expected >= 2 extracted facts, got %d", len(facts))
	}
}

func TestLLMHooksCompression(t *testing.T) {
	_, comp, buf, _ := newFullStack(t)

	// Fill buffer with messages
	for i := 0; i < 22; i++ {
		buf.Append(types.Message{Role: types.RoleUser, Content: "msg", CreatedAt: time.Now().Unix()})
	}

	llm := &mockLLM{
		chatFn: func(ctx context.Context, sp string, msgs []port.LLMMessage) (string, error) {
			return `{"summary": "用户进行了多轮对话，讨论了各种话题"}`, nil
		},
	}

	hooks := NewLLMHooks(llm, nil, comp)
	hooks.Install()

	ctx := context.Background()
	result, err := comp.Run(ctx, DefaultCompressorConfig())
	if err != nil {
		t.Fatalf("Compressor.Run: %v", err)
	}
	if result == nil {
		t.Fatal("expected compression result, got nil")
	}
	if !result.Compressed {
		t.Error("expected compression to succeed")
	}
	if result.Memo != "用户进行了多轮对话，讨论了各种话题" {
		t.Errorf("unexpected memo: %s", result.Memo)
	}

	// Verify memo is set
	memo := buf.Memo()
	if memo == nil {
		t.Fatal("expected memo to be set")
	}
	if memo.Content != result.Memo {
		t.Errorf("memo content mismatch: %q vs %q", memo.Content, result.Memo)
	}
}

func TestLLMHooksSignalDetection(t *testing.T) {
	worker, comp, _, store := newFullStack(t)

	// Pre-populate an existing fact
	existing := &types.FactEntry{
		Entity: "master", RelationType: "preference", Content: "likes Go",
		SourceTier: types.SourceExplicit, TemporalScope: types.ScopePattern,
		Importance: 7, Source: "chat", MemCellType: "fact",
		Evidence:   types.MemoryEvidenceEntry{Reinforcement: 1.0, ReinLastSignalAt: time.Now().Unix()},
		CreatedAt:  time.Now().Unix(),
	}
	if err := store.SaveFact(context.Background(), existing); err != nil {
		t.Fatalf("SaveFact: %v", err)
	}

	var callCount int32
	llm := &mockLLM{
		chatFn: func(ctx context.Context, sp string, msgs []port.LLMMessage) (string, error) {
			atomic.AddInt32(&callCount, 1)
			userMsg := ""
			if len(msgs) > 0 {
				userMsg = msgs[len(msgs)-1].Content
			}
			// First call: fact extraction
			if strings.Contains(userMsg, "记忆提取器") {
				return `{"facts": [{"entity": "master", "relation_type": "preference", "content": "uses Go daily", "source_tier": "explicit", "importance": 7}]}`, nil
			}
			// Second call: signal detection
			return fmt.Sprintf(`{"signals": [{"entry_id": %d, "type": "reinforce", "source_fact_content": "uses Go daily"}]}`, existing.ID), nil
		},
	}

	hooks := NewLLMHooks(llm, worker, comp)
	hooks.Install()

	ctx := context.Background()
	worker.Start(ctx, DefaultWorkerConfig())
	defer worker.Stop()

	worker.OnAfterChat(ctx, "I use Go every day at work", "Cool!")

	// Trigger processing
	worker.Wake()
	time.Sleep(500 * time.Millisecond)

	// Verify: the existing fact should have been reinforced
	updated, err := store.GetFact(context.Background(), existing.ID)
	if err != nil {
		t.Fatalf("GetFact: %v", err)
	}
	if updated.Evidence.Reinforcement <= 1.0 {
		t.Errorf("expected reinforcement > 1.0 after signal, got %f", updated.Evidence.Reinforcement)
	}
}

func TestLLMHooksJSONExtraction(t *testing.T) {
	tests := []struct {
		raw      string
		expected string
	}{
		{`{"facts": []}`, `{"facts": []}`},
		{"```json\n{\"facts\": []}\n```", `{"facts": []}`},
		{"```\n{\"facts\": []}\n```", `{"facts": []}`},
		{"  {\"key\": \"value\"}  ", `{"key": "value"}`},
	}

	for _, tt := range tests {
		got := extractJSON(tt.raw)
		if got != tt.expected {
			t.Errorf("extractJSON(%q) = %q, want %q", tt.raw, got, tt.expected)
		}
	}
}

func TestMessagesToText(t *testing.T) {
	msgs := []types.Message{
		{Role: types.RoleUser, Content: "hello"},
		{Role: types.RoleAssistant, Content: "hi there"},
	}

	text := messagesToText(msgs)
	if !strings.Contains(text, "hello") || !strings.Contains(text, "hi there") {
		t.Errorf("messagesToText: %s", text)
	}
}

func TestFactsToCompactText(t *testing.T) {
	facts := []types.FactEntry{
		{ID: 1, Entity: "master", RelationType: "preference", SourceTier: types.SourceExplicit, Content: "likes Go"},
	}

	text := factsToCompactText(facts)
	if !strings.Contains(text, "[ID:1]") || !strings.Contains(text, "likes Go") {
		t.Errorf("factsToCompactText: %s", text)
	}
}
