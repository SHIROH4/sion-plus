package proactive

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/shirohania/sion/internal/adapter/tool"
	"github.com/shirohania/sion/internal/domain/cognition"
	"github.com/shirohania/sion/internal/domain/types"
	"github.com/shirohania/sion/internal/port"
	"github.com/shirohania/sion/internal/transport/sse"
)

// CognitionTick orchestrates the proactive decision loop.
type CognitionTick struct {
	gate      *deliveryGate
	scheduler *intentScheduler
	deliverer *intentDeliverer
	extractor *FeatureExtractor
	executor  port.LLMExecutor
	toolReg   *tool.ToolRegistry // optional: enables proactive tool use (search, browser, etc.)

	interval       time.Duration
	firstTickDelay time.Duration
	actions        []types.ActionDef
	stopCh         chan struct{}

	consecutiveSame   int
	lastActionName    string
	consecutiveReject int
	lastTickAt        time.Time

	// Silence / backoff tracking
	unansweredProactive  int           // how many proactive speaks went unanswered
	lastUserMessageAt    time.Time     // last time user sent a message
	lastProactiveSpeakAt time.Time     // last time we spoke proactively
	minSpeakGap          time.Duration // minimum gap between proactive speaks
}

func NewCognitionTick(
	gate *deliveryGate,
	scheduler *intentScheduler,
	deliverer *intentDeliverer,
	extractor *FeatureExtractor,
	executor port.LLMExecutor,
) *CognitionTick {
	return &CognitionTick{
		gate:      gate,
		scheduler: scheduler,
		deliverer: deliverer,
		extractor: extractor,
		executor:  executor,
		interval:       60 * time.Second,
		firstTickDelay: 30 * time.Second,
		actions:        cognition.BuildActions(),
		stopCh:         make(chan struct{}),
		minSpeakGap:    5 * time.Minute,
	}
}

func (t *CognitionTick) Start(ctx context.Context) {
	log.Printf("[CognitionTick] starting with interval=%v", t.interval)
	go t.loop(ctx)
}

func (t *CognitionTick) Stop() {
	select {
	case <-t.stopCh:
		// already closed
	default:
		close(t.stopCh)
	}
	log.Println("[CognitionTick] stopped")
}

// SetInterval changes the tick frequency (for mode switching).
func (t *CognitionTick) SetInterval(d time.Duration) { t.interval = d }

// Interval returns the current tick interval.
func (t *CognitionTick) Interval() time.Duration { return t.interval }

// LastAction returns the name of the last chosen action.
func (t *CognitionTick) LastAction() string { return t.lastActionName }

// LastTickAt returns when the last tick ran.
func (t *CognitionTick) LastTickAt() time.Time { return t.lastTickAt }

// Actions returns the available action definitions.
func (t *CognitionTick) Actions() []types.ActionDef { return t.actions }

// SetFirstTickDelay sets the delay before the first cognition tick.
func (t *CognitionTick) SetFirstTickDelay(d time.Duration) { t.firstTickDelay = d }

// SetPersona wires the persona store for structured identity queries.
func (t *CognitionTick) SetPersona(p PersonaQuerier) { t.extractor.SetPersona(p) }

// SetToolRegistry gives the proactive system access to tools (browser, search, etc.).
func (t *CognitionTick) SetToolRegistry(tr *tool.ToolRegistry) { t.toolReg = tr }

// SetMinSpeakGap sets the minimum interval between proactive speaks.
func (t *CognitionTick) SetMinSpeakGap(d time.Duration) { t.minSpeakGap = d }

// OnUserMessage is called when the user sends a message. It resets the
// unanswered-proactive counter so that user engagement lifts the backoff.
func (t *CognitionTick) OnUserMessage() {
	t.unansweredProactive = 0
	t.lastUserMessageAt = time.Now()
}

// SetBroker wires the SSE broker for cross-window proactive message delivery.
func (t *CognitionTick) SetBroker(broker *sse.Broker) { t.deliverer.broker = broker }

// SetHistoryStore wires the memory store so proactive messages are persisted.
func (t *CognitionTick) SetHistoryStore(store port.MemoryStore) { t.deliverer.store = store }

func (t *CognitionTick) loop(ctx context.Context) {
	select {
	case <-time.After(t.firstTickDelay):
	case <-t.stopCh:
		return
	case <-ctx.Done():
		return
	}

	// Run first tick immediately after warmup delay
	t.run(ctx)

	ticker := time.NewTicker(t.interval)
	defer ticker.Stop()

	for {
		select {
		case <-t.stopCh:
			return
		case <-ctx.Done():
			return
		case <-ticker.C:
			t.run(ctx)
		}
	}
}

// ── Main tick logic ────────────────────────────────────────────────

func (t *CognitionTick) run(ctx context.Context) {
	// Phase 0: Gate
	if !t.gate.TryAcquire() {
		log.Println("[CognitionTick] skipped — gate busy")
		return
	}
	defer t.gate.Release()
	log.Println("[CognitionTick] tick started")

	// Phase 0.5: Feature extraction
	features := t.extractor.Extract(ctx)

	// Inject real unanswered count into features (overrides hardcoded zero)
	features.R4_RecentRejections = float64(t.unansweredProactive)

	// Phase 1: Compute drives
	drives := cognition.ComputeDrives(features, nil)

	// Phase 2: Score actions
	scored := cognition.ScoreActions(drives, features, t.actions, nil)

	if len(scored) == 0 {
		return
	}

		// Night gate: pre-filter to NightSafe so System2 doesn't waste rounds
		// choosing actions that will be hard-blocked anyway.
		isNight := features.U12_NightTime > 0
		if isNight {
			scored = cognition.FilterNightSafe(scored)
			if len(scored) == 0 {
				return
			}
		}

	// Phase 3: Decision routing
	decision := cognition.Route(scored, features)

	var chosenAction types.ActionDef
	var isSystem2 bool

	if decision.FastPath {
		chosenAction = scored[0].Action
	} else {
		topK := min(3, len(scored))
		selected, ok := t.llmDecideWithTools(ctx, features, scored[:topK], isNight)
		if ok && selected != nil {
			chosenAction = *selected
			isSystem2 = true
		} else {
			chosenAction = scored[0].Action
		}
	}

	// Track consecutive same action
	if chosenAction.Name == t.lastActionName {
		t.consecutiveSame++
	} else {
		t.consecutiveSame = 0
	}
	t.lastActionName = chosenAction.Name

	// Phase 4: Hard gate (now sees real unansweredProactive via features)
	if allowed, reason := cognition.HardGate(types.ScoredAction{Action: chosenAction}, features); !allowed {
		log.Printf("[CognitionTick] hard gate blocked %s: %s", chosenAction.Name, reason)
		return
	}

	// Phase 5: "none" or silent actions → skip
	if chosenAction.Name == "none" || chosenAction.OutcomeType == "silent" {
		topN := min(3, len(scored))
		topInfo := ""
		for i := 0; i < topN; i++ {
			topInfo += fmt.Sprintf("%s=%.2f ", scored[i].Action.Name, scored[i].FinalScore)
		}
		log.Printf("[CognitionTick] skip %s (aff=%.2f lon=%.2f ply=%.2f unanswered=%d) — drives(S=%.2f C=%.2f E=%.2f) top3: %s",
			chosenAction.Name, features.A1_1_Affection, features.A1_6_Loneliness, features.A1_5_Playfulness, t.unansweredProactive,
			drives.Social, drives.Care, drives.Explore, topInfo)
		return
	}

	// ── Silence backoff gate ──
	// Escalating backoff: 1 unanswered → 2x interval, 2 → 4x, ≥3 → suppress entirely.
	if t.unansweredProactive >= 3 {
		log.Printf("[CognitionTick] suppressed speak — %d unanswered proactive messages", t.unansweredProactive)
		return
	}
	if t.unansweredProactive > 0 {
		backoff := t.interval * time.Duration(1<<uint(t.unansweredProactive))
		if time.Since(t.lastProactiveSpeakAt) < backoff {
			log.Printf("[CognitionTick] backoff — last speak %v ago, need %v (unanswered=%d)",
				time.Since(t.lastProactiveSpeakAt).Round(time.Second), backoff, t.unansweredProactive)
			return
		}
	}

	// ── Min speak gap gate ──
	if time.Since(t.lastProactiveSpeakAt) < t.minSpeakGap {
		log.Printf("[CognitionTick] min gap — last speak %v ago < %v",
			time.Since(t.lastProactiveSpeakAt).Round(time.Second), t.minSpeakGap)
		return
	}

	log.Printf("[CognitionTick] SPEAK action=%s score=%.2f (sys2=%v, drives S=%.2f C=%.2f, aff=%.2f, unanswered=%d)",
		chosenAction.Name, scored[0].FinalScore, isSystem2, drives.Social, drives.Care, features.A1_1_Affection, t.unansweredProactive)

	// Phase 6: Create intent
	intent := t.buildIntent(ctx, features, &chosenAction, isSystem2)
	if intent == nil {
		return
	}

	// Phase 7: Schedule
	if err := t.scheduler.Submit(ctx, *intent); err != nil {
		log.Printf("[CognitionTick] scheduler submit: %v", err)
		return
	}

	// Phase 8: Schedule intent (release gate so deliver can proceed)
	t.gate.Release()
	t.lastTickAt = time.Now()

	// Phase 9: Deliver (outside tick lock)
	if t.gate.CanRelease(ctx) {
		intents := t.scheduler.Drain()
		if len(intents) > 0 {
			t.gate.OnPlaybackStart(ctx)
			_, err := t.deliverer.Deliver(ctx, intents)
			t.gate.OnPlaybackEnd(ctx)
			if err != nil {
				log.Printf("[CognitionTick] deliver: %v", err)
			} else {
				// Delivery succeeded — mark as unanswered until user replies
				t.unansweredProactive++
				t.lastProactiveSpeakAt = time.Now()
				log.Printf("[CognitionTick] unansweredProactive → %d", t.unansweredProactive)
			}
		}
	}
}

// ── Intent builder ─────────────────────────────────────────────────

func (t *CognitionTick) buildIntent(ctx context.Context, f *types.QuantifiedFeatures, action *types.ActionDef, isSystem2 bool) *types.ProactiveIntent {
	// If System2 decided "none" via LLM, respect it
	if isSystem2 && action.Name == "none" {
		return nil
	}

	priority := types.PriorityLow
	if action.Category == "care" {
		priority = types.PriorityNormal
	}
	if action.Name == "speak_inquiry" {
		priority = types.PriorityHigh
	}

	return &types.ProactiveIntent{
		Source:      action.Source,
		Action:      action.Name,
		Message:     fmt.Sprintf("%s: %s", action.SkillCard.Trigger, action.SkillCard.Action),
		Priority:    priority,
		CoalesceKey: action.Name,
		TTL:         120 * time.Second,
		CreatedAt:   time.Now(),
	}
}

// ── System2: LLM decision (function calling) ─────────────────────

// llmDecideWithTools asks the LLM to select an action via function calling.
// If ToolRegistry is wired, also exposes tools (web_search, browser, etc.) so the
// proactive system can gather information before deciding what to say/do.
func (t *CognitionTick) llmDecideWithTools(ctx context.Context, f *types.QuantifiedFeatures, topActions []types.ScoredAction, isNight bool) (*types.ActionDef, bool) {
	if t.executor == nil {
		return nil, false
	}

	actionNames := make([]string, len(topActions))
	for i, a := range topActions {
		actionNames[i] = a.Action.Name
	}
	actionNames = append(actionNames, "none")

	allTools := []port.ToolDef{{
		Name:        "select_action",
		Description: "Pick the best proactive action for Sion (catgirl AI companion) to take right now. Use AFTER gathering any needed info.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"action": map[string]any{
					"type": "string", "enum": actionNames,
					"description": "The action to take.",
				},
				"reason": map[string]any{
					"type": "string",
					"description": "Brief reason.",
				},
			},
			"required": []string{"action", "reason"},
		},
	}}

	// Add tool registry tools if available (for info gathering before deciding)
	if t.toolReg != nil {
		allTools = append(allTools, t.toolReg.Specs()...)

	// At night, skip heavy tools — computer_use is privacy-invasive and browser is unnecessary.
	// Keep lightweight tools: web_search, exec_command, read_file.
	if isNight {
		filtered := make([]port.ToolDef, 0, len(allTools))
		for _, tool := range allTools {
			if tool.Name == "computer_use" || tool.Name == "browser" {
				continue
			}
			filtered = append(filtered, tool)
		}
		allTools = filtered
	}
	}

	prompt := buildS2DecisionPrompt(f, topActions, isNight)

	var selectedAction string
	onToolCall := func(name, argsJSON string) string {
		if name == "select_action" {
			var args struct {
				Action string `json:"action"`
				Reason string `json:"reason"`
			}
			if err := json.Unmarshal([]byte(argsJSON), &args); err == nil {
				selectedAction = args.Action
				log.Printf("[CognitionTick] System2: %s (reason: %s)", args.Action, args.Reason)
			}
			return `{"status":"ok"}`
		}
		// Execute via tool registry
		if t.toolReg != nil {
			var args map[string]any
			json.Unmarshal([]byte(argsJSON), &args)
			res := t.toolReg.Execute(ctx, name, args)
			if res != nil {
				log.Printf("[CognitionTick] tool %s: %s", name, truncateStr(res.Output, 80))
				return res.Output
			}
		}
		return `{"error":"tool not available"}`
	}

	_, err := t.executor.ChatWithTools(ctx, "", []port.LLMMessage{
		{Role: "user", Content: prompt},
	}, allTools, onToolCall, 5, "auto")

	if err != nil {
		log.Printf("[CognitionTick] System2 LLM failed: %v", err)
		return nil, false
	}

	if selectedAction == "" || selectedAction == "none" {
		return nil, false
	}

	act := cognition.ActionByName(selectedAction)
	return act, act != nil
}

func buildS2DecisionPrompt(f *types.QuantifiedFeatures, topActions []types.ScoredAction, isNight bool) string {
	actionsDesc := ""
	for _, a := range topActions {
		nightTag := ""
		if a.Action.NightSafe {
			nightTag = " [夜间安全]"
		}
		actionsDesc += fmt.Sprintf("- %s (score: %.2f)%s\n", a.Action.Name, a.FinalScore, nightTag)
	}

	nightNote := ""
	roundNote := ""
	if isNight {
		nightNote = "\n⚠️ 现在是深夜时段。只能选择标记了 [夜间安全] 的行动。"
		roundNote = "\n⚠️ 注意：工具探索最多5轮。确保在第4轮之前调用 select_action，否则会超时回退到默认选择。"
	} else {
		roundNote = "\n提示：工具探索最多5轮。确保在第4轮之前调用 select_action，否则会超时回退到默认选择。"
	}

	return fmt.Sprintf(`你是 Sion 的决策中枢。从候选列表中选一个最合适的主动行动。

当前状态:
- 亲近感: %.2f, 担心: %.2f, 孤独感: %.2f
- 情绪: %s (强度 %.2f)
- 用户应用: %s, 深夜: %.0f
%s

候选行动:
%s
%s
选择一个行动并说明理由。`, f.A1_1_Affection, f.A1_2_Worry, f.A1_6_Loneliness,
		f.A2_PrimaryEmotion, f.A3_Intensity,
		f.U1_AppCategory, f.U12_NightTime,
		nightNote, actionsDesc, roundNote)
}

// ── Helpers ────────────────────────────────────────────────────────

// ── PostChat Analysis ──────────────────────────────────────────────

// lastPostChatAt guards against running post-chat analysis too frequently.
var lastPostChatAt time.Time

// AnalyzePostChat runs the proactive LLM+tool pipeline after a conversation turn.
// Called asynchronously from ChatOrchestrator's postChatHook.
// Guards: rate limit, message substance, negative emotion, min gap after proactive speak,
// and short-ack detection (messages that don't invite follow-up).
func (t *CognitionTick) AnalyzePostChat(userMsg, response string) {
	// Gate: rate limit
	if time.Since(lastPostChatAt) < 2*time.Minute {
		return
	}
	// Gate: message must be substantial (>5 CJK chars or >20 ascii)
	runes := []rune(userMsg)
	if len(runes) < 5 && len(userMsg) < 20 {
		return
	}
	// Gate: skip short acknowledgments that don't invite follow-up
	if isShortAck(runes) {
		log.Printf("[PostChat] skipped — short ack: %q", truncateStr(userMsg, 30))
		return
	}
	// Gate: don't pile on if we just spoke proactively
	if time.Since(t.lastProactiveSpeakAt) < t.minSpeakGap {
		log.Printf("[PostChat] skipped — proactive speak %v ago < min gap %v",
			time.Since(t.lastProactiveSpeakAt).Round(time.Second), t.minSpeakGap)
		return
	}
	// Gate: don't interject if user hasn't engaged with our last proactive message
	if t.unansweredProactive >= 2 {
		log.Printf("[PostChat] skipped — %d unanswered proactive, user hasn't engaged yet", t.unansweredProactive)
		return
	}
	lastPostChatAt = time.Now()

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	log.Printf("[PostChat] analyzing: %q", truncateStr(userMsg, 50))

	// Build lightweight feature snapshot
	f := t.extractor.Extract(ctx)
	if f == nil {
		return
	}

	// Gate: skip if user seems annoyed or very negative
	if f.A1_8_Annoyance > 0.6 || f.A2_PrimaryEmotion == "sadness" && f.A3_Intensity > 0.7 {
		log.Printf("[PostChat] skipped — negative emotion (annoy=%.2f, emotion=%s)", f.A1_8_Annoyance, f.A2_PrimaryEmotion)
		return
	}

	// Gate: skip social post-chat at night — only NightSafe actions are appropriate
	if f.U12_NightTime > 0 {
		log.Printf("[PostChat] skipped — night time, no social follow-up")
		return
	}

	actionNames := []string{"speak_casual", "speak_care", "speak_inquiry", "none"}

	allTools := []port.ToolDef{{
		Name:        "select_action",
		Description: "Choose a proactive action for Sion after the user's message. Respond AFTER optionally gathering info with tools.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"action": map[string]any{
					"type": "string", "enum": actionNames,
					"description": "Action to take.",
				},
				"message": map[string]any{
					"type":        "string",
					"description": "What to say. Include the actual line of dialogue.",
				},
				"reason": map[string]any{
					"type": "string", "description": "Brief reason.",
				},
			},
			"required": []string{"action", "message", "reason"},
		},
	}}

	// Add tools: web_search + browser (not computer_use — too heavy for post-chat)
	if t.toolReg != nil {
		allTools = append(allTools, t.toolReg.SpecsByNames([]string{"web_search", "browser"})...)
	}

	prompt := fmt.Sprintf(`You are Sion, a proactive catgirl AI companion. The user just said something. Decide if you should say something helpful or interesting in response.

User said: %q
You replied: %q

User state: emotion=%s intensity=%.2f, app=%s, night=%.0f

If you want to search the web or browse a page for info before responding, use web_search or browser first.
If you decide to speak, use select_action with the dialogue line in "message".
If nothing to say, use select_action with action="none".

Respond with tool calls only. Do not output text outside of tool calls.`,
		userMsg, truncateStr(response, 200),
		f.A2_PrimaryEmotion, f.A3_Intensity, f.U1_AppCategory, f.U12_NightTime)

	var selectedAction, selectedMessage string
	onToolCall := func(name, argsJSON string) string {
		if name == "select_action" {
			var args struct {
				Action  string `json:"action"`
				Message string `json:"message"`
				Reason  string `json:"reason"`
			}
			if err := json.Unmarshal([]byte(argsJSON), &args); err == nil {
				selectedAction = args.Action
				selectedMessage = args.Message
				log.Printf("[PostChat] decided: %s (reason: %s)", args.Action, args.Reason)
			}
			return `{"status":"ok"}`
		}
		if t.toolReg != nil {
			var args map[string]any
			json.Unmarshal([]byte(argsJSON), &args)
			res := t.toolReg.Execute(ctx, name, args)
			if res != nil {
				log.Printf("[PostChat] tool %s: %s", name, truncateStr(res.Output, 80))
				return res.Output
			}
		}
		return `{"error":"tool not available"}`
	}

	_, err := t.executor.ChatWithTools(ctx, "", []port.LLMMessage{
		{Role: "user", Content: prompt},
	}, allTools, onToolCall, 5, "auto")

	if err != nil {
		log.Printf("[PostChat] LLM failed: %v", err)
		return
	}

	if selectedAction == "" || selectedAction == "none" || selectedMessage == "" {
		return
	}

	// Submit as proactive intent via existing scheduler+deliverer pipeline
	intent := types.ProactiveIntent{
		Source:      "post_chat",
		Action:      selectedAction,
		Message:     selectedMessage,
		Priority:    types.PriorityNormal,
		CoalesceKey: "post_chat",
		TTL:         60 * time.Second,
		CreatedAt:   time.Now(),
	}
	if err := t.scheduler.Submit(ctx, intent); err != nil {
		log.Printf("[PostChat] scheduler submit: %v", err)
		return
	}
	log.Printf("[PostChat] intent submitted: %s — %q", selectedAction, truncateStr(selectedMessage, 60))

	// Deliver immediately (don't wait for next tick)
	if t.gate.CanRelease(ctx) {
		t.gate.OnPlaybackStart(ctx)
		intents := t.scheduler.Drain()
		if len(intents) > 0 {
			_, err := t.deliverer.Deliver(ctx, intents)
			t.gate.OnPlaybackEnd(ctx)
			if err != nil {
				log.Printf("[PostChat] deliver: %v", err)
			}
		}
	}
}

func truncateStr(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

// isShortAck detects messages that are mere acknowledgments, not inviting follow-up.
// Examples: "ok", "好的", "嗯", "知道了", "哈哈", "666", etc.
func isShortAck(runes []rune) bool {
	if len(runes) > 5 {
		return false
	}
	acks := []string{"ok", "okay", "好", "嗯", "哦", "啊", "行", "对", "是", "知道了", "哈哈", "嘿嘿",
		"666", "牛", "厉害", "没错", "是的", "好的", "好吧", "可以", "fine", "k", "yes", "no", "yep", "nope", "嗯嗯", "噢噢"}
	s := string(runes)
	for _, a := range acks {
		if s == a {
			return true
		}
	}
	return false
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
