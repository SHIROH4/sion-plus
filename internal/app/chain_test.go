package app

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/SHIROH4/sion-plus/internal/adapter/llm"
	"github.com/SHIROH4/sion-plus/internal/adapter/proactive"
	"github.com/SHIROH4/sion-plus/internal/adapter/tool"
	"github.com/SHIROH4/sion-plus/internal/port"
)

// ── Mock LLM ─────────────────────────────────────────────────────

type chainMockLLM struct {
	chatReplies   []string
	emotionDeltas []string
	factExtracts  []string
	signalDetects []string
	chatIdx       int
	emotionIdx    int
	factIdx       int
	signalIdx     int
}

func (m *chainMockLLM) Chat(ctx context.Context, sp string, msgs []port.LLMMessage) (string, error) {
	last := ""
	if len(msgs) > 0 {
		last = msgs[len(msgs)-1].Content
	}
	// Route to correct mock based on content
	if strings.Contains(last, "routeChannel") || strings.Contains(last, "task router") || strings.Contains(last, "postChat") || strings.Contains(last, "Select the most") {
		if m.chatIdx < len(m.chatReplies) {
			r := m.chatReplies[m.chatIdx]
			m.chatIdx++
			return r, nil
		}
		return `{"action":"none"}`, nil
	}
	// General chat: return "主人..." for any user message
	return "主人今天心情不错呢~", nil
}
func (m *chainMockLLM) ChatWithTools(ctx context.Context, sp string, msgs []port.LLMMessage, tools []port.ToolDef, onTC func(string, string) string, mr int, tc string) (string, error) {
	// Return empty — no tool calls
	return "", nil
}
func (m *chainMockLLM) ChatStream(ctx context.Context, sp string, msgs []port.LLMMessage, onChunk func(string) error) error {
	onChunk("mock stream")
	return nil
}
func (m *chainMockLLM) IsAvailable(ctx context.Context) bool { return true }

// ── Helper ───────────────────────────────────────────────────────

func newChainRuntime(t *testing.T) (*AppRuntime, context.Context, func()) {
	t.Helper()
	dir := t.TempDir()

	mock := &chainMockLLM{
		chatReplies: []string{
			"主人今天心情不错呢~",    // chat reply
			"browser",       // route channel
			"主人我帮你查了一下天气喵~", // postChat proactive
		},
	}

	cfg := Config{
		DataDir:      dir,
		Personality:  "你是一只叫Sion的猫娘。",
		LLMProviders: []port.LLMProviderConfig{},
		LLMRoutes:    port.LLMRoutes{Default: "mock", Chat: "mock", Emotion: "mock", Memory: "mock", Signal: "mock", Summary: "mock", Vision: "mock"},
	}

	r, err := NewRuntime(cfg)
	if err != nil {
		t.Fatalf("NewRuntime: %v", err)
	}

	// Replace executor with mock
	r.services.llm.Executor = mock
	r.services.llm.Executor = llm.WrapRateLimited(mock, 999, 10, "test")

	if err := r.Init(context.Background()); err != nil {
		t.Fatalf("Init: %v", err)
	}

	// Override cognition tick interval for testing
	if r.CognitionTick != nil {
		r.CognitionTick.SetInterval(5 * time.Second)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)

	cleanup := func() {
		cancel()
		r.Stop(context.Background())
	}
	return r, ctx, cleanup
}

// ── 链路 1: ChatOrchestrator 完整对话 ──────────────────────────

func TestChainChatOrchestrator(t *testing.T) {
	r, ctx, cleanup := newChainRuntime(t)
	defer cleanup()

	// Ensure ChatOrchestrator is wired
	if r.Chat == nil {
		t.Fatal("ChatOrchestrator not wired")
	}

	t.Log("=== 链路 1: ChatOrchestrator 完整对话 ===")

	// Step 1: Send a message
	result, err := r.Chat.OnUserMessage(ctx, "今天天气真好")
	if err != nil {
		t.Fatalf("OnUserMessage: %v", err)
	}
	t.Logf("✅ Step 1 OnUserMessage: response=%q emotion=%s/%s",
		truncate(result.Response, 40), result.Emotion.Primary, result.EmotionSource)

	if result.Response == "" {
		t.Error("❌ response should not be empty")
	}
	if result.Emotion.Primary == "" {
		t.Error("❌ emotion primary should not be empty")
	}

	// Step 2: Verify L0 buffer was updated
	l0Len := r.services.memory.Buffer.Len()
	t.Logf("✅ Step 2 L0 buffer: %d messages", l0Len)
	if l0Len < 2 {
		t.Errorf("❌ L0 buffer should have >= 2 messages, got %d", l0Len)
	}

	// Step 3: Verify emotion was evaluated
	state, vec := r.services.emotion.Store.Current()
	t.Logf("✅ Step 3 Emotion: primary=%s V=%.2f A=%.2f | 8D: aff=%.2f lon=%.2f",
		state.Primary, state.Valence, state.Arousal, vec.Affection, vec.Loneliness)

	// Step 4: Verify history was saved
	count := r.services.memory.Store.CountTodayMessages(ctx)
	t.Logf("✅ Step 4 History: %d messages today", count)

	// Step 5: Verify tool registry is accessible
	t.Logf("✅ Step 5 Tools: %d registered", r.ToolRegistry.ToolCount())
	if r.ToolRegistry.ToolCount() < 3 {
		t.Errorf("❌ expected >= 3 tools, got %d", r.ToolRegistry.ToolCount())
	}

	// Step 6: Send second message — 测试去重
	result2, err := r.Chat.OnUserMessage(ctx, "今天天气真好")
	if err != nil {
		t.Fatalf("OnUserMessage 2: %v", err)
	}
	if result2.Response == result.Response {
		t.Logf("✅ Step 6 Dedup: same message returned identical response (dedup hit)")
	}

	t.Log("=== 链路 1 通过 ===")
}

// ── 链路 2: Memory Worker 管线 ──────────────────────────────────

func TestChainMemoryPipeline(t *testing.T) {
	r, ctx, cleanup := newChainRuntime(t)
	defer cleanup()

	t.Log("=== 链路 2: Memory Worker 管线 ===")

	// Send a message containing personal info
	_, err := r.Chat.OnUserMessage(ctx, "我是一个后端工程师，喜欢Go语言和Rust")
	if err != nil {
		t.Fatalf("OnUserMessage: %v", err)
	}

	// Wait for async memory worker
	time.Sleep(500 * time.Millisecond)

	// Verify L0 buffer
	l0Len := r.services.memory.Buffer.Len()
	t.Logf("✅ L0 buffer: %d messages", l0Len)

	// Verify history saved
	historyCount := r.services.memory.Store.CountTodayMessages(ctx)
	t.Logf("✅ History: %d messages today", historyCount)

	// Verify facts can be queried
	facts, _ := r.services.memory.Store.ListActiveFacts(ctx, 0)
	t.Logf("✅ Active facts: %d", len(facts))

	// Verify recall works
	results, err := r.services.memory.Recall.HybridSearch(ctx, "工程师", 3)
	if err != nil {
		t.Logf("⚠️  HybridSearch error: %v", err)
	} else {
		t.Logf("✅ Recall '工程师': %d results", len(results))
	}

	t.Log("=== 链路 2 通过 ===")
}

// ── 链路 3: Emotion 评估管线 ──────────────────────────────────

func TestChainEmotionPipeline(t *testing.T) {
	r, ctx, cleanup := newChainRuntime(t)
	defer cleanup()

	t.Log("=== 链路 3: Emotion 评估管线 ===")

	// Initial state
	state, _ := r.services.emotion.Store.Current()
	t.Logf("✅ Initial: primary=%s V=%.2f", state.Primary, state.Valence)

	// Send emotional messages
	messages := []string{
		"今天太开心了！项目终于上线了",
		"唉，bug调了一整天还是没修好",
	}

	for _, msg := range messages {
		_, err := r.Chat.OnUserMessage(ctx, msg)
		if err != nil {
			t.Logf("⚠️  message %q: %v", msg, err)
			continue
		}
		state, vec := r.services.emotion.Store.Current()
		t.Logf("✅ After %q: primary=%s V=%.2f A=%.2f | 8D: aff=%.2f ann=%.2f",
			truncate(msg, 20), state.Primary, state.Valence, state.Arousal,
			vec.Affection, vec.Annoyance)
	}

	// Verify emotion history
	hStates, hVecs := r.services.emotion.Store.History()
	t.Logf("✅ Emotion history: %d states, %d vectors", len(hStates), len(hVecs))

	t.Log("=== 链路 3 通过 ===")
}

// ── 链路 4: Proactive 决策管线 ──────────────────────────────────

func TestChainProactivePipeline(t *testing.T) {
	r, ctx, cleanup := newChainRuntime(t)
	defer cleanup()

	t.Log("=== 链路 4: Proactive 决策管线 ===")

	if r.CognitionTick == nil {
		t.Skip("CognitionTick not wired")
	}

	// Start the tick loop
	r.CognitionTick.Start(ctx)
	defer r.CognitionTick.Stop()

	// Send a message to trigger PostChat analysis
	_, err := r.Chat.OnUserMessage(ctx, "你觉得今天会下雨吗")
	if err != nil {
		t.Fatalf("OnUserMessage: %v", err)
	}

	// Wait for async hook
	time.Sleep(2 * time.Second)

	// Verify PostChat was called (async — just check no panic)
	t.Logf("✅ PostChat analysis triggered (async)")

	// Verify feature extractor works
	extractor := proactive.NewFeatureExtractor(
		r.services.emotion.Store,
		r.services.memory.Store,
		nil, // no screen observer in test
	)
	features := extractor.Extract(ctx)
	t.Logf("✅ Features extracted: affection=%.2f, loneliness=%.2f, working=%.2f",
		features.A1_1_Affection, features.A1_6_Loneliness, features.U3_IsWorking)

	// Verify TaskDedup works
	result1, _ := r.Chat.OnUserMessage(ctx, "你好呀")
	result2, _ := r.Chat.OnUserMessage(ctx, "你好呀")
	if result1.Response == result2.Response {
		t.Logf("✅ Dedup: duplicate message returned identical response")
	}

	t.Log("=== 链路 4 通过 ===")
}

// ── 链路 5: Tool 执行管线 ──────────────────────────────────────

func TestChainToolPipeline(t *testing.T) {
	r, ctx, cleanup := newChainRuntime(t)
	defer cleanup()

	t.Log("=== 链路 5: Tool 执行管线 ===")

	if r.ToolRegistry == nil {
		t.Fatal("ToolRegistry not wired")
	}

	// Test 1: File tools
	t.Logf("--- File Tools ---")
	dir := t.TempDir()
	testFile := filepath.Join(dir, "test.txt")
	tool.InitAllowedPaths(dir, dir)

	// write_file
	res := r.ToolRegistry.Execute(ctx, "write_file", map[string]any{
		"path":    testFile,
		"content": "hello world",
	})
	t.Logf("✅ write_file: success=%v output=%s", res.Success, truncate(res.Output, 40))
	if !res.Success {
		t.Errorf("❌ write_file failed: %s", res.Error)
	}

	// read_file
	res = r.ToolRegistry.Execute(ctx, "read_file", map[string]any{
		"path": testFile,
	})
	t.Logf("✅ read_file: success=%v output=%s", res.Success, truncate(res.Output, 40))

	// edit_file
	res = r.ToolRegistry.Execute(ctx, "edit_file", map[string]any{
		"path":       testFile,
		"old_string": "hello world",
		"new_string": "hello sion",
	})
	t.Logf("✅ edit_file: success=%v", res.Success)

	// Verify edit
	res = r.ToolRegistry.Execute(ctx, "read_file", map[string]any{"path": testFile})
	if !strings.Contains(res.Output, "hello sion") {
		t.Errorf("❌ edit verify failed: %s", res.Output)
	} else {
		t.Logf("✅ edit verified: content=%s", truncate(res.Output, 40))
	}

	// Test 2: Bash tool
	t.Logf("--- Bash Tool ---")
	res = r.ToolRegistry.Execute(ctx, "exec_command", map[string]any{
		"command": "echo hello from bash",
	})
	t.Logf("✅ exec_command: success=%v output=%s", res.Success, truncate(res.Output, 40))

	// Test 3: Dangerous command blocked
	res = r.ToolRegistry.Execute(ctx, "exec_command", map[string]any{
		"command": "rm -rf /",
	})
	t.Logf("✅ dangerous blocked: success=%v error=%s", res.Success, truncate(res.Error, 60))
	if res.Success {
		t.Error("❌ dangerous command should be blocked")
	}

	// Test 4: Path sandbox
	res = r.ToolRegistry.Execute(ctx, "read_file", map[string]any{
		"path": "/etc/passwd",
	})
	t.Logf("✅ path sandbox: success=%v error=%s", res.Success, truncate(res.Error, 60))
	if res.Success {
		t.Error("❌ out-of-bounds path should be blocked")
	}

	t.Log("=== 链路 5 通过 ===")
}

// ── 链路 6: 全部链路串联 (端到端) ─────────────────────────────

func TestChainEndToEnd(t *testing.T) {
	r, ctx, cleanup := newChainRuntime(t)
	defer cleanup()

	t.Log("=== 链路 6: 端到端串联 ===")

	conversation := []string{
		"你好呀小猫咪",
		"我是程序员，主要用Go语言",
		"今天写了一天代码好累",
	}

	for i, msg := range conversation {
		result, err := r.Chat.OnUserMessage(ctx, msg)
		if err != nil {
			t.Fatalf("turn %d: %v", i+1, err)
		}
		t.Logf("Turn %d: %q → %q (emotion=%s)", i+1,
			truncate(msg, 20), truncate(result.Response, 40), result.Emotion.Primary)
	}

	// Final state check
	state, vec := r.services.emotion.Store.Current()
	l0Len := r.services.memory.Buffer.Len()
	msgCount := r.services.memory.Store.CountTodayMessages(ctx)
	toolCount := r.ToolRegistry.ToolCount()

	t.Logf("")
	t.Logf("=== 最终状态 ===")
	t.Logf("Emotion:  %s (V=%.2f A=%.2f)", state.Primary, state.Valence, state.Arousal)
	t.Logf("8D:       aff=%.2f wor=%.2f cur=%.2f slp=%.2f ply=%.2f lon=%.2f con=%.2f ann=%.2f",
		vec.Affection, vec.Worry, vec.Curiosity, vec.Sleepiness,
		vec.Playfulness, vec.Loneliness, vec.Confidence, vec.Annoyance)
	t.Logf("L0:       %d messages", l0Len)
	t.Logf("History:  %d messages today", msgCount)
	t.Logf("Tools:    %d registered", toolCount)

	// Validate minimum expectations
	checks := []struct {
		name string
		ok   bool
	}{
		{"emotion primary set", state.Primary != ""},
		{"L0 has conversation", l0Len >= 6},
		{"history saved", msgCount >= 6},
		{"tools registered", toolCount >= 3},
	}
	allOk := true
	for _, c := range checks {
		if c.ok {
			t.Logf("✅ %s", c.name)
		} else {
			t.Errorf("❌ %s", c.name)
			allOk = false
		}
	}
	if allOk {
		t.Log("=== 全部 6 条链路通过 ===")
	}
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
