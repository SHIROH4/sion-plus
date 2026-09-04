package modules

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/SHIROH4/sion-plus/internal/adapter/tool"
	"github.com/SHIROH4/sion-plus/internal/domain/types"
	"github.com/SHIROH4/sion-plus/internal/port"
)

// recentRequest holds a cached user request for deduplication.
type recentRequest struct {
	msg      string
	response string
	emotion  types.EmotionState
	source   string
	at       time.Time
}

// ChatOrchestrator orchestrates a single conversation turn end-to-end.
type ChatOrchestrator struct {
	emotionEval    port.EmotionSignalSource
	emotionStore   port.EmotionStateManager
	recall         port.MemoryRecall
	worker         port.ChatMemorySink
	buffer         port.SessionBuffer
	executor       port.LLMExecutor
	promptBldr     *PromptBuilder
	screenObserver port.ScreenObserver
	toolRegistry   port.ChatToolProvider
	postChatHook   func(userMsg, response string)
	preChatHook    func(context.Context, string) // called when user sends a message, before processing

	recentReqs [5]recentRequest // ring buffer for task dedup
	reqIdx     int
}

// SetToolRegistry wires the tool provider for tool-assisted chat.
func (c *ChatOrchestrator) SetToolRegistry(tr port.ChatToolProvider) { c.toolRegistry = tr }

// SetPostChatHook sets a callback invoked after each conversation turn.
// The hook receives (userMsg, assistantResponse) and runs asynchronously.
func (c *ChatOrchestrator) SetPostChatHook(hook func(userMsg, response string)) {
	c.postChatHook = hook
}

// SetPreChatHook sets a callback invoked when the user sends a message,
// before emotion evaluation and LLM processing.
func (c *ChatOrchestrator) SetPreChatHook(hook func(context.Context, string)) { c.preChatHook = hook }

// ChatResult is the output of a single conversation turn.
type ChatResult struct {
	Response      string
	Emotion       types.EmotionState
	EmotionSource string // "llm"|"rule"
	Timing        ChatTiming
}

// ChatTiming exposes user-visible and internal stage latency for repeatable
// evaluation. Values are milliseconds and exclude asynchronous post-chat work.
type ChatTiming struct {
	EmotionMS     float64 `json:"emotion_ms"`
	RecallMS      float64 `json:"recall_ms"`
	PromptMS      float64 `json:"prompt_ms"`
	GenerationMS  float64 `json:"generation_ms"`
	MemoryWriteMS float64 `json:"memory_write_ms"`
	TTFTMS        float64 `json:"ttft_ms"`
	TotalMS       float64 `json:"total_ms"`
}

// NewChatOrchestrator wires all adapters into a conversation pipeline.
func NewChatOrchestrator(
	emotionEval port.EmotionSignalSource,
	emotionStore port.EmotionStateManager,
	recall port.MemoryRecall,
	worker port.ChatMemorySink,
	buffer port.SessionBuffer,
	executor port.LLMExecutor,
	promptBldr *PromptBuilder,
	screenObserver port.ScreenObserver,
) *ChatOrchestrator {
	return &ChatOrchestrator{
		emotionEval:    emotionEval,
		emotionStore:   emotionStore,
		recall:         recall,
		worker:         worker,
		buffer:         buffer,
		executor:       executor,
		promptBldr:     promptBldr,
		screenObserver: screenObserver,
	}
}

// turnContext holds the results of the shared pre-LLM pipeline steps
// (emotion evaluation, memory recall, prompt assembly).
type turnContext struct {
	evalResult   *port.EmotionEvalResult
	facts        []port.MemorySearchResult
	diaries      []port.MemorySearchResult
	boundaries   []port.MemorySearchResult
	promptResult BuildResult
	enrichedMsg  string
	timing       ChatTiming
}

// prepareTurn runs the shared pipeline: emotion → recall → prompt.
func (c *ChatOrchestrator) prepareTurn(ctx context.Context, userMsg string) *turnContext {
	timing := ChatTiming{}
	// Step 1: Emotion evaluation
	stageStart := time.Now()
	recentTurns := c.buildRecentTurns(userMsg)
	evalResult, err := c.emotionEval.Evaluate(ctx, &port.EmotionEvalInput{
		SourceType:  "chat",
		CurrentMsg:  userMsg,
		RecentTurns: recentTurns,
	})
	if err != nil {
		log.Printf("[ChatOrchestrator] emotion eval error: %v", err)
	}
	if evalResult == nil {
		state, vec := c.emotionStore.Current()
		evalResult = &port.EmotionEvalResult{State: state, Vector: vec, Source: "error"}
	}
	c.recall.SetMoodBias(evalResult.State.Valence)
	c.worker.UpdateEmotionState(evalResult.State.Valence, evalResult.State.Arousal)
	timing.EmotionMS = durationMS(time.Since(stageStart))

	// Step 2: Memory recall
	stageStart = time.Now()
	facts, _ := c.recall.HybridSearch(ctx, userMsg, 5)
	diaries, _ := c.recall.SearchDiaries(ctx, userMsg, 2)
	boundaries, _ := c.recall.SearchBoundaries(ctx)
	timing.RecallMS = durationMS(time.Since(stageStart))

	// Step 3: Build prompt (with screen context)
	stageStart = time.Now()
	screenSummary := c.collectScreenSummary(ctx)
	promptResult := c.promptBldr.Build(BuildInput{
		UserMessage:   userMsg,
		L0Messages:    c.buffer.Recent(5),
		L0Memo:        memoContent(c.buffer.Memo()),
		Facts:         facts,
		Diaries:       diaries,
		Boundaries:    boundaries,
		Emotion:       evalResult.State,
		ScreenSummary: screenSummary,
	})
	for _, w := range promptResult.Warnings {
		log.Printf("[ChatOrchestrator] prompt warning: %s", w)
	}
	timing.PromptMS = durationMS(time.Since(stageStart))

	return &turnContext{
		evalResult:   evalResult,
		facts:        facts,
		diaries:      diaries,
		boundaries:   boundaries,
		promptResult: promptResult,
		enrichedMsg:  c.promptBldr.WrapUserMessage(userMsg, promptResult.MemoryContext),
		timing:       timing,
	}
}

// OnUserMessage processes a single conversation turn.
func (c *ChatOrchestrator) OnUserMessage(ctx context.Context, userMsg string) (*ChatResult, error) {
	if c.preChatHook != nil {
		c.preChatHook(ctx, userMsg)
	}
	if cached := c.checkDedup(userMsg); cached != nil {
		log.Printf("[ChatOrchestrator] dedup hit: %q → cached response", truncateForRouting(userMsg))
		return &ChatResult{Response: cached.response, Emotion: cached.emotion, EmotionSource: cached.source}, nil
	}

	start := time.Now()

	tc := c.prepareTurn(ctx, userMsg)

	if c.executor == nil {
		return nil, fmt.Errorf("no LLM executor configured")
	}

	generationStart := time.Now()
	response, err := c.chatWithTools(ctx, tc)
	if err != nil {
		return nil, err
	}
	tc.timing.GenerationMS = durationMS(time.Since(generationStart))
	tc.timing.TTFTMS = durationMS(time.Since(start))

	memoryStart := time.Now()
	c.worker.OnAfterChat(ctx, userMsg, response)
	tc.timing.MemoryWriteMS = durationMS(time.Since(memoryStart))

	c.storeDedup(userMsg, response, tc.evalResult.State, tc.evalResult.Source)

	if c.postChatHook != nil {
		go c.postChatHook(userMsg, response)
	}

	elapsed := time.Since(start)
	tc.timing.TotalMS = durationMS(elapsed)
	logChatTiming(tc.timing)
	log.Printf("[ChatOrchestrator] turn complete in %v (emotion=%s/%s V=%.2f A=%.2f D=%.2f, facts=%d, diaries=%d)",
		elapsed, tc.evalResult.State.Primary, tc.evalResult.Source, tc.evalResult.State.Valence, tc.evalResult.State.Arousal, tc.evalResult.State.Dominance, len(tc.facts), len(tc.diaries))

	return &ChatResult{
		Response:      response,
		Emotion:       tc.evalResult.State,
		EmotionSource: tc.evalResult.Source,
		Timing:        tc.timing,
	}, nil
}

// checkDedup returns a cached result if the message is a near-duplicate of a recent request.
func (c *ChatOrchestrator) checkDedup(msg string) *recentRequest {
	now := time.Now()
	for i := range c.recentReqs {
		r := &c.recentReqs[i]
		if r.msg == "" {
			continue
		}
		if now.Sub(r.at) > 30*time.Second {
			continue
		}
		if r.msg == msg {
			return r
		}
	}
	return nil
}

// storeDedup saves a request+response pair for future deduplication.
func (c *ChatOrchestrator) storeDedup(msg, response string, emotion types.EmotionState, source string) {
	c.recentReqs[c.reqIdx%len(c.recentReqs)] = recentRequest{
		msg: msg, response: response, emotion: emotion, source: source, at: time.Now(),
	}
	c.reqIdx++
}

// OnUserMessageStream processes a conversation turn with streaming LLM output.
func (c *ChatOrchestrator) OnUserMessageStream(ctx context.Context, userMsg string, onChunk func(chunk string) error) (*ChatResult, error) {
	if c.preChatHook != nil {
		c.preChatHook(ctx, userMsg)
	}
	if cached := c.checkDedup(userMsg); cached != nil {
		for _, chunk := range splitChunks(cached.response, 20) {
			onChunk(chunk)
		}
		return &ChatResult{Response: cached.response, Emotion: cached.emotion, EmotionSource: cached.source}, nil
	}

	start := time.Now()

	tc := c.prepareTurn(ctx, userMsg)

	if c.executor == nil {
		return nil, fmt.Errorf("no LLM executor configured")
	}

	// Step 4: If tools are available, route through tool execution first
	// (non-streaming), then pseudo-stream the final response.
	// If no tools, use real streaming directly.
	var response string
	generationStart := time.Now()
	firstChunk := true
	emitChunk := func(chunk string) error {
		if firstChunk {
			firstChunk = false
			tc.timing.TTFTMS = durationMS(time.Since(start))
		}
		return onChunk(chunk)
	}
	if c.toolRegistry != nil && c.toolRegistry.ToolCount() > 0 {
		resp, err := c.chatWithTools(ctx, tc)
		if err != nil {
			return nil, err
		}
		response = resp
		for _, chunk := range splitChunks(response, 20) {
			if err := emitChunk(chunk); err != nil {
				return nil, fmt.Errorf("stream chunk: %w", err)
			}
		}
	} else {
		var fullResponse strings.Builder
		chatCtx := port.WithLLMCallMetadata(ctx, "chat", "chat_response")
		err := c.executor.ChatStream(chatCtx, tc.promptResult.SystemPrompt, []port.LLMMessage{
			{Role: "user", Content: tc.enrichedMsg},
		}, func(chunk string) error {
			fullResponse.WriteString(chunk)
			return emitChunk(chunk)
		})
		if err != nil {
			return nil, fmt.Errorf("llm stream: %w", err)
		}
		response = fullResponse.String()
	}
	tc.timing.GenerationMS = durationMS(time.Since(generationStart))

	// Step 5: Write to memory
	memoryStart := time.Now()
	c.worker.OnAfterChat(ctx, userMsg, response)
	tc.timing.MemoryWriteMS = durationMS(time.Since(memoryStart))
	c.storeDedup(userMsg, response, tc.evalResult.State, tc.evalResult.Source)

	if c.postChatHook != nil {
		go c.postChatHook(userMsg, response)
	}

	elapsed := time.Since(start)
	tc.timing.TotalMS = durationMS(elapsed)
	logChatTiming(tc.timing)
	log.Printf("[ChatOrchestrator] stream turn complete in %v (emotion=%s/%s V=%.2f A=%.2f D=%.2f, facts=%d, diaries=%d)",
		elapsed, tc.evalResult.State.Primary, tc.evalResult.Source, tc.evalResult.State.Valence, tc.evalResult.State.Arousal, tc.evalResult.State.Dominance, len(tc.facts), len(tc.diaries))

	return &ChatResult{
		Response:      response,
		Emotion:       tc.evalResult.State,
		EmotionSource: tc.evalResult.Source,
		Timing:        tc.timing,
	}, nil
}

func durationMS(duration time.Duration) float64 {
	return float64(duration.Microseconds()) / 1000
}

func logChatTiming(timing ChatTiming) {
	log.Printf("[ChatTiming] emotion_ms=%.3f recall_ms=%.3f prompt_ms=%.3f generation_ms=%.3f memory_write_ms=%.3f ttft_ms=%.3f total_ms=%.3f",
		timing.EmotionMS, timing.RecallMS, timing.PromptMS, timing.GenerationMS,
		timing.MemoryWriteMS, timing.TTFTMS, timing.TotalMS)
}

// splitChunks splits text into character-sized chunks for pseudo-streaming.
func splitChunks(s string, charCount int) []string {
	runes := []rune(s)
	if len(runes) <= charCount {
		return []string{s}
	}
	chunks := make([]string, 0, (len(runes)/charCount)+1)
	for i := 0; i < len(runes); i += charCount {
		end := i + charCount
		if end > len(runes) {
			end = len(runes)
		}
		chunks = append(chunks, string(runes[i:end]))
	}
	return chunks
}

// chatWithTools exposes the small built-in tool set directly to the main call.
// A separate LLM routing call added latency and cost to every ordinary chat;
// function calling already lets the provider choose whether a tool is needed.
func (c *ChatOrchestrator) chatWithTools(ctx context.Context, tc *turnContext) (string, error) {
	if c.toolRegistry != nil && c.toolRegistry.ToolCount() > 0 {
		specs := trSpecs(c.toolRegistry)

		resp, resultCount, err := c.executeWithSpecs(ctx, tc, specs)
		if err != nil {
			log.Printf("[ChatOrchestrator] tool call failed: %v, falling back to chat", err)
		} else if resultCount > 0 || resp != "" {
			if resultCount > 0 {
				log.Printf("[ChatOrchestrator] tool results: %d calls (tools=%d)", resultCount, len(specs))
			}
			return resp, nil
		}
	}
	chatCtx := port.WithLLMCallMetadata(ctx, "chat", "chat_response")
	return c.executor.Chat(chatCtx, tc.promptResult.SystemPrompt, []port.LLMMessage{
		{Role: "user", Content: tc.enrichedMsg},
	})
}

// executeWithSpecs calls the LLM with a specific set of tool definitions.
func (c *ChatOrchestrator) executeWithSpecs(ctx context.Context, tc *turnContext, tools []port.ToolDef) (string, int, error) {
	systemPrompt := tc.promptResult.SystemPrompt + "\n\n" + toolAuthorization(tools)
	var toolResults int
	chatCtx := port.WithLLMCallMetadata(ctx, "chat", "chat_response")
	resp, err := c.executor.ChatWithTools(chatCtx, systemPrompt,
		[]port.LLMMessage{{Role: "user", Content: tc.enrichedMsg}},
		tools,
		func(name, argsJSON string) string {
			toolResults++
			// Execute via registry
			if tr, ok := c.toolRegistry.(*tool.ToolRegistry); ok {
				var args map[string]any
				json.Unmarshal([]byte(argsJSON), &args)
				res := tr.Execute(ctx, name, args)
				if res.Success {
					return res.Output
				}
				return "error: " + res.Error
			}
			return "tool registry unavailable"
		},
		3, // max 3 rounds of tool calls
		"auto",
	)
	return resp, toolResults, err
}

func trSpecs(provider port.ChatToolProvider) []port.ToolDef {
	if tr, ok := provider.(*tool.ToolRegistry); ok {
		return tr.Specs()
	}
	return nil
}

func truncateForRouting(s string) string {
	if len(s) > 200 {
		return s[:200] + "..."
	}
	return s
}

// toolAuthorization returns a prompt fragment granting explicit permission to use tools.
// Without this, some LLMs refuse to call tools, claiming they are "forbidden".
func toolAuthorization(tools []port.ToolDef) string {
	has := func(name string) bool {
		for _, t := range tools {
			if t.Name == name {
				return true
			}
		}
		return false
	}
	var b strings.Builder
	b.WriteString("## 可用工具\n你拥有以下能力，可以直接使用（调用前会征得主人确认）：\n")
	if has("web_search") {
		b.WriteString("- 🔍 web_search — 搜索网络信息，查找资料、文档、新闻等\n")
	}
	if has("browser") {
		b.WriteString("- 🌐 browser — 打开浏览器访问网页、填写表单、点击按钮\n")
	}
	if has("computer_use") {
		b.WriteString("- 💻 computer_use — 操作桌面应用：打开程序、点击界面、键盘输入\n")
	}
	if has("read_file") {
		b.WriteString("- 📖 read_file — 读取文件内容\n")
	}
	if has("write_file") {
		b.WriteString("- ✏️ write_file — 创建或覆盖文件\n")
	}
	if has("edit_file") {
		b.WriteString("- 📝 edit_file — 精准修改文件\n")
	}
	if has("exec_command") {
		b.WriteString("- ⚡ exec_command — 执行命令行（仅限安全白名单）\n")
	}
	b.WriteString("\n当主人让你搜索、打开网页、操作电脑或处理文件时，请直接使用这些工具，不要说你做不到。")
	return b.String()
}

// ── Screen context ─────────────────────────────────────────────────

func (c *ChatOrchestrator) collectScreenSummary(ctx context.Context) string {
	if c.screenObserver == nil || !c.screenObserver.IsAvailable() {
		return ""
	}
	obs, err := c.screenObserver.Observe(ctx)
	if err != nil || obs == nil {
		return ""
	}
	if obs.AppName == "" {
		return ""
	}
	summary := obs.AppName
	if obs.WindowTitle != "" {
		summary += " — " + obs.WindowTitle
	}
	if obs.AppCategory != "" && obs.AppCategory != "idle" {
		summary += " (" + obs.AppCategory + ")"
	}
	return summary
}

// ── Helpers ────────────────────────────────────────────────────────

func (c *ChatOrchestrator) buildRecentTurns(currentMsg string) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("[用户] %s\n", currentMsg))
	msgs := c.buffer.Recent(10)
	for _, m := range msgs {
		role := "用户"
		if m.Role == types.RoleAssistant {
			role = "AI"
		}
		b.WriteString(fmt.Sprintf("[%s] %s\n", role, m.Content))
	}
	return b.String()
}

func memoContent(m *types.Message) string {
	if m == nil {
		return ""
	}
	return m.Content
}
