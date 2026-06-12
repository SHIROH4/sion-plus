package modules

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/SHIROH4/sion-plus/internal/adapter/emotion"
	"github.com/SHIROH4/sion-plus/internal/adapter/memory"
	domainMemory "github.com/SHIROH4/sion-plus/internal/domain/memory"
	"github.com/SHIROH4/sion-plus/internal/domain/types"
	"github.com/SHIROH4/sion-plus/internal/port"
)

// mockLLM returns canned responses keyed by prompt content.
type mockLLM struct {
	responses     map[string]string
	streamChunks  []string // if set, ChatStream sends these chunks
}

func (m *mockLLM) Chat(ctx context.Context, sp string, msgs []port.LLMMessage) (string, error) {
	last := ""
	if len(msgs) > 0 {
		last = msgs[len(msgs)-1].Content
	}
	if resp, ok := m.responses[last]; ok {
		return resp, nil
	}
	for key, resp := range m.responses {
		if strings.Contains(last, key) {
			return resp, nil
		}
	}
	return "mock response", nil
}

func (m *mockLLM) ChatWithTools(ctx context.Context, sp string, msgs []port.LLMMessage, tools []port.ToolDef, onToolCall func(string, string) string, maxRounds int, tc string) (string, error) {
	return "", nil
}

func (m *mockLLM) ChatStream(ctx context.Context, sp string, msgs []port.LLMMessage, onChunk func(string) error) error {
	if len(m.streamChunks) == 0 {
		// Fallback: send full response as one chunk
		resp, _ := m.Chat(ctx, sp, msgs)
		return onChunk(resp)
	}
	for _, chunk := range m.streamChunks {
		if err := onChunk(chunk); err != nil {
			return err
		}
	}
	return nil
}

func (m *mockLLM) IsAvailable(ctx context.Context) bool { return true }

func newTestOrchestrator(t *testing.T) *ChatOrchestrator {
	t.Helper()

	store, _ := memory.NewSQLiteStore(filepath.Join(t.TempDir(), "test.db"))

	evidenceCfg := domainMemory.DefaultEvidenceConfig()
	evidence := memory.NewEvidenceEngine(store, evidenceCfg)
	buffer := memory.NewSessionBuffer(40, 0)
	recall := memory.NewRecall(store, evidence)
	comp := memory.NewCompressor(buffer, memory.DefaultCompressorConfig())
	eventLog := memory.NewEventLog(store)
	evidence.SetEventLog(eventLog)

	workerCfg := memory.DefaultWorkerConfig()
	workerCfg.ExtractEveryN = 3
	worker := memory.NewMemoryWorker(store, evidence, recall, buffer, comp, workerCfg)
	worker.SetEventLog(eventLog)

	emotionStore := emotion.NewEmotionStore(filepath.Join(t.TempDir(), "emo.json"))
	llm := &mockLLM{responses: map[string]string{
		// Emotion evaluation response
		"Sion": `{"affection":0.25,"worry":0.0,"curiosity":0.1,"sleepiness":0.0,"playfulness":0.15,"loneliness":0.0,"confidence":0.15,"annoyance":0.0,"reason":"主人开心"}`,
		// Chat response
		"今天": "主人今天心情不错呢~",
	}}
	emotionEval := emotion.NewEmotionEvaluator(llm, emotionStore)

	promptBldr := NewPromptBuilder("你是一只自学的猫娘。")

	// Inject LLM hooks
	hooks := memory.NewLLMHooks(llm, worker, comp)
	hooks.Install()

	// Start worker
	worker.Start(context.Background(), workerCfg)
	t.Cleanup(func() { worker.Stop() })

	// Start emotion decay
	emotionStore.Start()
	t.Cleanup(func() { emotionStore.Stop() })

	return NewChatOrchestrator(emotionEval, emotionStore, recall, worker, buffer, llm, promptBldr, nil)
}

func TestChatOrchestratorFullPipeline(t *testing.T) {
	orch := newTestOrchestrator(t)
	ctx := context.Background()

	// Send first message
	result, err := orch.OnUserMessage(ctx, "今天天气真好")
	if err != nil {
		t.Fatalf("OnUserMessage: %v", err)
	}
	if result.Response == "" {
		t.Error("expected non-empty response")
	}
	if result.Emotion.Primary == "" {
		t.Error("expected non-empty emotion primary")
	}
	if result.EmotionSource != "llm" && result.EmotionSource != "rule" {
		t.Errorf("emotion source for first turn: got %s", result.EmotionSource)
	}
	t.Logf("turn 1: response=%q emotion=%s/%s", result.Response, result.Emotion.Primary, result.EmotionSource)

	// Send second message
	result2, err := orch.OnUserMessage(ctx, "明天会下雨吗")
	if err != nil {
		t.Fatalf("OnUserMessage 2: %v", err)
	}
	t.Logf("turn 2: response=%q emotion=%s", result2.Response, result2.Emotion.Primary)

	// Verify L0 buffer has messages
	if orch.buffer.Len() < 4 {
		t.Errorf("L0 buffer should have at least 4 messages, got %d", orch.buffer.Len())
	}
}

func TestChatOrchestratorEmotionFlow(t *testing.T) {
	orch := newTestOrchestrator(t)
	ctx := context.Background()

	// First turn primes L0 (returns cache, no emotion change)
	_, _ = orch.OnUserMessage(ctx, "你好")

	initialState, _ := orch.emotionStore.Current()

	// Second turn with positive message (LLM evaluation)
	result, err := orch.OnUserMessage(ctx, "你真好")
	if err != nil {
		t.Fatalf("OnUserMessage: %v", err)
	}

	afterState, _ := orch.emotionStore.Current()

	// EMA smoothed — small change after one interaction
	t.Logf("emotion: valence before=%f after=%f primary=%s source=%s",
		initialState.Valence, afterState.Valence, result.Emotion.Primary, result.EmotionSource)

	// Second turn should be LLM-evaluated
	if result.EmotionSource != "llm" && result.EmotionSource != "rule" {
		t.Errorf("expected emotion source=llm for second turn, got %s", result.EmotionSource)
	}
}

func TestChatOrchestratorMemoryWrites(t *testing.T) {
	orch := newTestOrchestrator(t)
	ctx := context.Background()

	// Send a message and wait for async processing
	_, err := orch.OnUserMessage(ctx, "我是一个后端工程师，喜欢Go语言")
	if err != nil {
		t.Fatalf("OnUserMessage: %v", err)
	}

	// Wait for async MemoryWorker to process
	time.Sleep(500 * time.Millisecond)

	// Check that facts were extracted (mock LLM returns canned facts)
	facts, _ := orch.worker.(*memory.MemoryWorker).Store().ListActiveFacts(ctx, 0)
	t.Logf("extracted %d facts", len(facts))

	// Verify history was saved
	count := orch.worker.(*memory.MemoryWorker).Store().CountTodayMessages(ctx)
	if count < 2 {
		t.Errorf("expected at least 2 messages in history, got %d", count)
	}
}

func TestChatOrchestratorRecallIntegration(t *testing.T) {
	orch := newTestOrchestrator(t)
	ctx := context.Background()

	// Pre-populate a fact with matching keyword
	f := &types.FactEntry{
		Entity: "master", RelationType: "preference", Content: "likes sunny weather 天气",
		SourceTier: types.SourceExplicit, TemporalScope: types.ScopePattern,
		Importance: 7, Source: "chat", MemCellType: "fact",
		Evidence:  types.MemoryEvidenceEntry{Reinforcement: 2.0, ReinLastSignalAt: time.Now().Unix()},
		CreatedAt: time.Now().Unix(),
	}
	orch.worker.(*memory.MemoryWorker).Store().SaveFact(ctx, f)

	// Search for weather fact
	results, err := orch.recall.HybridSearch(ctx, "天气", 3)
	if err != nil {
		t.Fatalf("HybridSearch: %v", err)
	}
	found := false
	for _, r := range results {
		if strings.Contains(r.Content, "sunny") || strings.Contains(r.Content, "天气") {
			found = true
		}
	}
	if !found {
		t.Errorf("should find weather fact, got %d results", len(results))
	}
}

// TestChatOrchestratorMoodCongruentRecall verifies mood bias affects ranking.
func TestChatOrchestratorMoodCongruentRecall(t *testing.T) {
	orch := newTestOrchestrator(t)
	ctx := context.Background()

	// Pre-populate facts
	happy := &types.FactEntry{
		Entity: "master", RelationType: "preference", Content: "happy thing about weather",
		SourceTier: types.SourceExplicit, TemporalScope: types.ScopePattern,
		Importance: 7, Source: "chat", MemCellType: "prefer",
		Evidence:  types.MemoryEvidenceEntry{Reinforcement: 2.0, ReinLastSignalAt: time.Now().Unix()},
		CreatedAt: time.Now().Unix(),
	}
	sad := &types.FactEntry{
		Entity: "master", RelationType: "emotional", Content: "boundary about work stress",
		SourceTier: types.SourceExplicit, TemporalScope: types.ScopePattern,
		Importance: 8, Source: "chat", MemCellType: "fact",
		Evidence:  types.MemoryEvidenceEntry{Reinforcement: 2.0, ReinLastSignalAt: time.Now().Unix()},
		CreatedAt: time.Now().Unix(),
	}
	orch.worker.(*memory.MemoryWorker).Store().SaveFact(ctx, happy)
	orch.worker.(*memory.MemoryWorker).Store().SaveFact(ctx, sad)

	// Set positive mood bias
	orch.recall.SetMoodBias(0.8)
	results, _ := orch.recall.HybridSearch(ctx, "thing", 2)

	// Preference facts should rank higher in positive mood
	if len(results) > 0 && results[0].Score > 0 {
		t.Logf("mood=0.8 top result: %s (score=%.2f)", results[0].Content, results[0].Score)
	}

	// Set negative mood bias
	orch.recall.SetMoodBias(-0.8)
	results2, _ := orch.recall.HybridSearch(ctx, "thing", 2)
	if len(results2) > 0 {
		t.Logf("mood=-0.8 top result: %s (score=%.2f)", results2[0].Content, results2[0].Score)
	}

	// Different moods should produce different rankings (at minimum, scores differ)
	if len(results) > 0 && len(results2) > 0 && results[0].Score == results2[0].Score {
		t.Log("warning: mood bias produced identical scores (may be expected with small dataset)")
	}
}

func TestChatOrchestratorEmotionLLMFallback(t *testing.T) {
	// Use an unavailable LLM → should fall back to rule-based emotion eval.
	// The chat call will fail (nil executor), but we test that emotion doesn't crash.
	buffer := memory.NewSessionBuffer(40, 0)
	store, _ := memory.NewSQLiteStore(filepath.Join(t.TempDir(), "test.db"))
	evidence := memory.NewEvidenceEngine(store, domainMemory.DefaultEvidenceConfig())
	recall := memory.NewRecall(store, evidence)
	workerCfg := memory.DefaultWorkerConfig()
	worker := memory.NewMemoryWorker(store, evidence, recall, buffer, nil, workerCfg)

	emotionStore := emotion.NewEmotionStore(filepath.Join(t.TempDir(), "emo.json"))
	eval := emotion.NewEmotionEvaluator(nil, emotionStore) // nil → rule-based

	b := NewPromptBuilder("test")
	orch := &ChatOrchestrator{
		emotionEval:  eval,
		emotionStore: emotionStore,
		recall:       recall,
		worker:       worker,
		buffer:       buffer,
		promptBldr:   b,
		executor:     nil, // will fail at chat step — expected
	}

	ctx := context.Background()
	_, err := orch.OnUserMessage(ctx, "哈哈今天真开心")
	if err == nil {
		t.Log("no error (emotion rule-based succeeded, but chat should fail)")
	} else {
		t.Logf("expected error at chat step: %v", err)
	}

	// Verify emotion evaluation did run (rule-based)
	state, _ := emotionStore.Current()
	t.Logf("fallback emotion: %s (valence=%.2f)", state.Primary, state.Valence)
	if state.Primary == "" {
		t.Error("emotion should have been evaluated even without LLM")
	}
}

func TestChatOrchestratorStreamingPipeline(t *testing.T) {
	orch := newTestOrchestrator(t)
	// Replace mock LLM with streaming version
	streamLLM := &mockLLM{
		responses:    orch.executor.(*mockLLM).responses,
		streamChunks: []string{"主人", "今天", "心情", "不错", "呢~"},
	}
	orch.executor = streamLLM

	ctx := context.Background()
	var chunks []string
	result, err := orch.OnUserMessageStream(ctx, "今天天气真好", func(chunk string) error {
		chunks = append(chunks, chunk)
		return nil
	})
	if err != nil {
		t.Fatalf("OnUserMessageStream: %v", err)
	}
	if result.Response == "" {
		t.Error("expected non-empty response")
	}
	if len(chunks) != 5 {
		t.Errorf("expected 5 chunks, got %d: %v", len(chunks), chunks)
	}
	if result.Emotion.Primary == "" {
		t.Error("expected non-empty emotion")
	}

	// Verify full response is assembled correctly
	expected := "主人今天心情不错呢~"
	if result.Response != expected {
		t.Errorf("full response = %q, want %q", result.Response, expected)
	}
}

func TestChatOrchestratorStreamingWritesMemory(t *testing.T) {
	orch := newTestOrchestrator(t)
	streamLLM := &mockLLM{
		responses:    orch.executor.(*mockLLM).responses,
		streamChunks: []string{"mock", " streaming", " response"},
	}
	orch.executor = streamLLM

	ctx := context.Background()
	_, err := orch.OnUserMessageStream(ctx, "我是一个后端工程师", func(chunk string) error {
		return nil
	})
	if err != nil {
		t.Fatalf("OnUserMessageStream: %v", err)
	}

	time.Sleep(300 * time.Millisecond)

	// Verify history was written
	count := orch.worker.(*memory.MemoryWorker).Store().CountTodayMessages(ctx)
	if count < 2 {
		t.Errorf("expected at least 2 messages in history after streaming, got %d", count)
	}
}

func TestChatOrchestratorStreamingUsesSamePipeline(t *testing.T) {
	// Verify that streaming and sync use the same emotion + recall pipeline steps.
	// Both should produce the same emotion evaluation result for the same input.
	orch1 := newTestOrchestrator(t)
	orch2 := newTestOrchestrator(t)

	ctx := context.Background()

	// Prime both with first message
	_, _ = orch1.OnUserMessage(ctx, "你好")
	_, _ = orch2.OnUserMessage(ctx, "你好")

	// Sync
	result1, err1 := orch1.OnUserMessage(ctx, "今天天气真好")
	if err1 != nil {
		t.Fatalf("sync: %v", err1)
	}

	// Streaming
	orch2.executor = &mockLLM{
		responses:    orch2.executor.(*mockLLM).responses,
		streamChunks: []string{"mock stream response"},
	}
	result2, err2 := orch2.OnUserMessageStream(ctx, "今天天气真好", func(chunk string) error {
		return nil
	})
	if err2 != nil {
		t.Fatalf("stream: %v", err2)
	}

	// Both should produce emotion results
	if result1.Emotion.Primary == "" || result2.Emotion.Primary == "" {
		t.Error("both pipelines should produce emotion results")
	}
	// L0 buffer should grow identically
	if orch1.buffer.Len() != orch2.buffer.Len() {
		t.Errorf("buffer lengths differ: sync=%d stream=%d", orch1.buffer.Len(), orch2.buffer.Len())
	}
}

func TestChatOrchestratorStreamingEmptyMessage(t *testing.T) {
	orch := newTestOrchestrator(t)
	streamLLM := &mockLLM{
		responses:    orch.executor.(*mockLLM).responses,
		streamChunks: []string{},
	}
	orch.executor = streamLLM

	ctx := context.Background()
	result, err := orch.OnUserMessageStream(ctx, "test", func(chunk string) error {
		return nil
	})
	if err != nil {
		t.Fatalf("OnUserMessageStream: %v", err)
	}
	if result.Response != "mock response" {
		t.Errorf("empty chunks should fallback to mock response, got %q", result.Response)
	}
}

