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
	preChatHook    func() // called when user sends a message, before processing

	recentReqs [5]recentRequest // ring buffer for task dedup
	reqIdx     int
}

// SetToolRegistry wires the tool provider for tool-assisted chat.
func (c *ChatOrchestrator) SetToolRegistry(tr port.ChatToolProvider) { c.toolRegistry = tr }

// SetPostChatHook sets a callback invoked after each conversation turn.
// The hook receives (userMsg, assistantResponse) and runs asynchronously.
func (c *ChatOrchestrator) SetPostChatHook(hook func(userMsg, response string)) { c.postChatHook = hook }

// SetPreChatHook sets a callback invoked when the user sends a message,
// before emotion evaluation and LLM processing.
func (c *ChatOrchestrator) SetPreChatHook(hook func()) { c.preChatHook = hook }

// ChatResult is the output of a single conversation turn.
type ChatResult struct {
	Response      string
	Emotion       types.EmotionState
	EmotionSource string // "llm"|"rule"
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
		buffer:       buffer,
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
}

// prepareTurn runs the shared pipeline: emotion → recall → prompt.
func (c *ChatOrchestrator) prepareTurn(ctx context.Context, userMsg string) *turnContext {
	// Step 1: Emotion evaluation
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

	// Step 2: Memory recall
	facts, _ := c.recall.HybridSearch(ctx, userMsg, 5)
	diaries, _ := c.recall.SearchDiaries(ctx, userMsg, 2)
	boundaries, _ := c.recall.SearchBoundaries(ctx)

	// Step 3: Build prompt (with screen context)
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

	return &turnContext{
		evalResult:   evalResult,
		facts:        facts,
		diaries:      diaries,
		boundaries:   boundaries,
		promptResult: promptResult,
		enrichedMsg:  c.promptBldr.WrapUserMessage(userMsg, promptResult.MemoryContext),
	}
}

// OnUserMessage processes a single conversation turn.
func (c *ChatOrchestrator) OnUserMessage(ctx context.Context, userMsg string) (*ChatResult, error) {
	if c.preChatHook != nil {
		c.preChatHook()
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

	response, err := c.chatWithTools(ctx, tc)
	if err != nil {
		return nil, err
	}

	c.worker.OnAfterChat(ctx, userMsg, response)

	c.storeDedup(userMsg, response, tc.evalResult.State, tc.evalResult.Source)

	if c.postChatHook != nil {
		go c.postChatHook(userMsg, response)
	}

	elapsed := time.Since(start)
	log.Printf("[ChatOrchestrator] turn complete in %v (emotion=%s/%s V=%.2f A=%.2f D=%.2f, facts=%d, diaries=%d)",
		elapsed, tc.evalResult.State.Primary, tc.evalResult.Source, tc.evalResult.State.Valence, tc.evalResult.State.Arousal, tc.evalResult.State.Dominance, len(tc.facts), len(tc.diaries))

	return &ChatResult{
		Response:      response,
		Emotion:       tc.evalResult.State,
		EmotionSource: tc.evalResult.Source,
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
		c.preChatHook()
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
	if c.toolRegistry != nil && c.toolRegistry.ToolCount() > 0 {
		resp, err := c.chatWithTools(ctx, tc)
		if err != nil {
			return nil, err
		}
		response = resp
		for _, chunk := range splitChunks(response, 20) {
			if err := onChunk(chunk); err != nil {
				return nil, fmt.Errorf("stream chunk: %w", err)
			}
		}
	} else {
		var fullResponse strings.Builder
		err := c.executor.ChatStream(ctx, tc.promptResult.SystemPrompt, []port.LLMMessage{
			{Role: "user", Content: tc.enrichedMsg},
		}, func(chunk string) error {
			fullResponse.WriteString(chunk)
			return onChunk(chunk)
		})
		if err != nil {
			return nil, fmt.Errorf("llm stream: %w", err)
		}
		response = fullResponse.String()
	}

	// Step 5: Write to memory
	c.worker.OnAfterChat(ctx, userMsg, response)
	c.storeDedup(userMsg, response, tc.evalResult.State, tc.evalResult.Source)

	if c.postChatHook != nil {
		go c.postChatHook(userMsg, response)
	}

	elapsed := time.Since(start)
	log.Printf("[ChatOrchestrator] stream turn complete in %v (emotion=%s/%s V=%.2f A=%.2f D=%.2f, facts=%d, diaries=%d)",
		elapsed, tc.evalResult.State.Primary, tc.evalResult.Source, tc.evalResult.State.Valence, tc.evalResult.State.Arousal, tc.evalResult.State.Dominance, len(tc.facts), len(tc.diaries))

	return &ChatResult{
		Response:      response,
		Emotion:       tc.evalResult.State,
		EmotionSource: tc.evalResult.Source,
	}, nil
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

// chatWithTools routes to the best channel, filters tools, and executes.
func (c *ChatOrchestrator) chatWithTools(ctx context.Context, tc *turnContext) (string, error) {
	if c.toolRegistry != nil && c.toolRegistry.ToolCount() > 0 {
		channel := c.routeChannel(ctx, tc.enrichedMsg)
		log.Printf("[ChatOrchestrator] routed to channel: %s", channel)

		toolNames := channelToolNames(channel)
		var specs []port.ToolDef
		if len(toolNames) > 0 {
			// Try to get filtered specs from registry
			if tr, ok := c.toolRegistry.(*tool.ToolRegistry); ok {
				specs = tr.SpecsByNames(toolNames)
			}
		}
		if len(specs) == 0 {
			specs = trSpecs(c.toolRegistry)
		}

		resp, resultCount, err := c.executeWithSpecs(ctx, tc, specs)
		if err != nil {
			log.Printf("[ChatOrchestrator] tool call failed: %v, falling back to chat", err)
		} else if resultCount > 0 || resp != "" {
			if resultCount > 0 {
				log.Printf("[ChatOrchestrator] tool results: %d calls (channel=%s, tools=%d)", resultCount, channel, len(specs))
			}
			return resp, nil
		}
	}
	return c.executor.Chat(ctx, tc.promptResult.SystemPrompt, []port.LLMMessage{
		{Role: "user", Content: tc.enrichedMsg},
	})
}

// executeWithSpecs calls the LLM with a specific set of tool definitions.
func (c *ChatOrchestrator) executeWithSpecs(ctx context.Context, tc *turnContext, tools []port.ToolDef) (string, int, error) {
	systemPrompt := tc.promptResult.SystemPrompt + "\n\n" + toolAuthorization(tools)
	var toolResults int
	resp, err := c.executor.ChatWithTools(ctx, systemPrompt,
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

// ── Channel Routing ─────────────────────────────────────────────────

// routeChannel asks the LLM which execution channel fits the user's request.
// Returns one of: "browser", "desktop", "file", "shell", "search", "mixed".
func (c *ChatOrchestrator) routeChannel(ctx context.Context, userMsg string) string {
	prompt := fmt.Sprintf(`You are a task router. Classify the user request into ONE channel.

User request: "%s"

Channels:
- browser — web tasks: open URLs, search the web, read web pages, fill forms
- desktop — control desktop apps: open Safari/Chrome, click UI, type in apps
- file — file operations: read/write/edit files
- shell — run commands: git, build, npm, system info
- search — web search only: quick fact lookups, documentation search
- mixed — unclear or needs multiple channels

Respond with ONLY the channel name (one word).`, truncateForRouting(userMsg))

	resp, err := c.executor.Chat(ctx, "", []port.LLMMessage{
		{Role: "user", Content: prompt},
	})
	if err != nil {
		return "mixed" // fallback: use all tools
	}
	resp = strings.TrimSpace(strings.ToLower(resp))
	// Normalize common LLM verbosity
	for _, ch := range []string{"browser", "desktop", "file", "shell", "search", "mixed"} {
		if strings.Contains(resp, ch) {
			return ch
		}
	}
	return "mixed"
}

func truncateForRouting(s string) string {
	if len(s) > 200 {
		return s[:200] + "..."
	}
	return s
}

// channelToolNames returns the tool names for a given channel.
func channelToolNames(channel string) []string {
	switch channel {
	case "browser":
		return []string{"browser", "web_search"}
	case "desktop":
		return []string{"computer_use"}
	case "file":
		return []string{"read_file", "write_file", "edit_file"}
	case "shell":
		return []string{"exec_command"}
	case "search":
		return []string{"web_search"}
	default:
		return nil // nil = use all tools
	}
}

func allToolNames() []string {
	return []string{"browser", "web_search", "computer_use", "read_file", "write_file", "edit_file", "exec_command"}
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

