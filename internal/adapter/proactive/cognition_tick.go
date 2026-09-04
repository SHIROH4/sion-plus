package proactive

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/SHIROH4/sion-plus/internal/adapter/tool"
	"github.com/SHIROH4/sion-plus/internal/domain/cognition"
	"github.com/SHIROH4/sion-plus/internal/domain/types"
	"github.com/SHIROH4/sion-plus/internal/port"
	"github.com/SHIROH4/sion-plus/internal/transport/sse"
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
	intervalCh     chan time.Duration
	mode           string
	firstTickDelay time.Duration
	actions        []types.ActionDef
	stopCh         chan struct{}

	consecutiveSame   int
	lastActionName    string
	consecutiveReject int
	lastTickAt        time.Time

	// Silence / backoff tracking
	unansweredProactive  int       // how many proactive speaks went unanswered
	lastUserMessageAt    time.Time // last time user sent a message
	lastProactiveSpeakAt time.Time // last time we spoke proactively
	lastPostChatAt       time.Time
	minSpeakGap          time.Duration // minimum gap between proactive speaks
	feedbackStore        port.ProactiveFeedbackStore
	stateMu              sync.RWMutex
}

func NewCognitionTick(
	gate *deliveryGate,
	scheduler *intentScheduler,
	deliverer *intentDeliverer,
	extractor *FeatureExtractor,
	executor port.LLMExecutor,
) *CognitionTick {
	return &CognitionTick{
		gate:           gate,
		scheduler:      scheduler,
		deliverer:      deliverer,
		extractor:      extractor,
		executor:       executor,
		interval:       60 * time.Second,
		intervalCh:     make(chan time.Duration, 1),
		mode:           "normal",
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
func (t *CognitionTick) SetInterval(d time.Duration) {
	if d <= 0 {
		return
	}
	t.stateMu.Lock()
	t.interval = d
	intervalCh := t.intervalCh
	t.stateMu.Unlock()
	if intervalCh == nil {
		return
	}
	select {
	case intervalCh <- d:
	default:
		select {
		case <-intervalCh:
		default:
		}
		select {
		case intervalCh <- d:
		default:
		}
	}
}

// Interval returns the current tick interval.
func (t *CognitionTick) Interval() time.Duration {
	t.stateMu.RLock()
	defer t.stateMu.RUnlock()
	return t.interval
}

// Mode returns the effective proactive mode. Durable global controls override
// the selected interval mode, including after an application restart.
func (t *CognitionTick) Mode(ctx context.Context) string {
	t.stateMu.RLock()
	mode := t.mode
	t.stateMu.RUnlock()
	if t.feedbackStore != nil {
		allowed, _, err := t.feedbackStore.ProactiveAllowed(ctx, "", "", time.Now().Unix())
		if err == nil && !allowed {
			return "off"
		}
	}
	if mode == "" {
		return "normal"
	}
	return mode
}

// LastAction returns the name of the last chosen action.
func (t *CognitionTick) LastAction() string {
	t.stateMu.RLock()
	defer t.stateMu.RUnlock()
	return t.lastActionName
}

// LastTickAt returns when the last tick ran.
func (t *CognitionTick) LastTickAt() time.Time {
	t.stateMu.RLock()
	defer t.stateMu.RUnlock()
	return t.lastTickAt
}

// Actions returns the available action definitions.
func (t *CognitionTick) Actions() []types.ActionDef { return t.actions }

// SetFirstTickDelay sets the delay before the first cognition tick.
func (t *CognitionTick) SetFirstTickDelay(d time.Duration) { t.firstTickDelay = d }

// SetPersona wires the persona store for structured identity queries.
func (t *CognitionTick) SetPersona(p PersonaQuerier) { t.extractor.SetPersona(p) }

// SetToolRegistry gives the proactive system access to tools (browser, search, etc.).
func (t *CognitionTick) SetToolRegistry(tr *tool.ToolRegistry) { t.toolReg = tr }

// SetMinSpeakGap sets the minimum interval between proactive speaks.
func (t *CognitionTick) SetMinSpeakGap(d time.Duration) {
	t.stateMu.Lock()
	defer t.stateMu.Unlock()
	t.minSpeakGap = d
}

// OnUserMessage records conversational activity only. It must not settle the
// latest proactive decision: a user can start an unrelated chat, so treating
// every message as acceptance would corrupt the preference dataset.
func (t *CognitionTick) OnUserMessage(ctx context.Context, userMsg string) {
	t.stateMu.Lock()
	t.lastUserMessageAt = time.Now()
	t.stateMu.Unlock()
	if t.feedbackStore == nil {
		return
	}
	decision, err := t.feedbackStore.LatestPendingProactiveDecision(ctx, time.Now().Add(-15*time.Minute).Unix())
	if err != nil {
		log.Printf("[CognitionTick] correlate proactive reply: %v", err)
		return
	}
	if decision == nil {
		return
	}
	if err := t.feedbackStore.SaveProactiveReply(ctx, &types.ProactiveReply{
		DecisionID:  decision.DecisionID,
		Content:     userMsg,
		Attribution: "next_user_message",
		Confidence:  0.5,
		CreatedAt:   time.Now().Unix(),
	}); err != nil {
		log.Printf("[CognitionTick] persist proactive reply candidate: %v", err)
	}
}

// SetBroker wires the SSE broker for cross-window proactive message delivery.
func (t *CognitionTick) SetBroker(broker *sse.Broker) { t.deliverer.broker = broker }

// SetHistoryStore wires the memory store so proactive messages are persisted.
func (t *CognitionTick) SetHistoryStore(store port.MemoryStore) {
	t.deliverer.store = store
	if feedbackStore, ok := store.(port.ProactiveFeedbackStore); ok {
		t.feedbackStore = feedbackStore
	}
}

// RecordExplicitFeedback persists a user-controlled preference signal. It is
// intentionally separate from ordinary chat: an unrelated user message must
// not be promoted to a positive reward for the last proactive delivery.
func (t *CognitionTick) RecordExplicitFeedback(ctx context.Context, eventID, decisionID string, kind types.FeedbackKind, note string) error {
	if t.feedbackStore == nil {
		return fmt.Errorf("proactive feedback store is not configured")
	}
	reward := 0.0
	switch kind {
	case types.FeedbackHelpful:
		reward = 1
	case types.FeedbackDismiss, types.FeedbackSnooze:
		reward = -0.35
	case types.FeedbackIrrelevant:
		reward = -0.6
	case types.FeedbackBadTiming:
		reward = -0.45
	case types.FeedbackWrongTone:
		reward = -0.35
	case types.FeedbackStop:
		reward = -1
	default:
		return fmt.Errorf("unsupported feedback kind: %s", kind)
	}
	now := time.Now().Unix()
	if err := t.feedbackStore.SaveProactiveFeedback(ctx, &types.ProactiveFeedback{EventID: eventID, DecisionID: decisionID, Kind: kind, Reward: reward, Source: "ui", Confidence: 1, Note: note, CreatedAt: now}); err != nil {
		return err
	}
	if err := t.feedbackStore.ResolveProactiveDecision(ctx, decisionID, now); err != nil {
		return err
	}
	// Only correlated, explicit feedback changes interruption backoff.
	if kind == types.FeedbackHelpful {
		t.stateMu.Lock()
		t.unansweredProactive = 0
		t.stateMu.Unlock()
	} else if kind == types.FeedbackSnooze {
		if err := t.feedbackStore.UpsertProactiveControl(ctx, &types.ProactiveControl{Scope: "global", Mode: "snoozed", UntilAt: now + int64(time.Hour/time.Second), Source: "explicit_feedback"}); err != nil {
			return err
		}
	} else if kind == types.FeedbackStop {
		if err := t.feedbackStore.UpsertProactiveControl(ctx, &types.ProactiveControl{Scope: "global", Mode: "muted", Source: "explicit_feedback"}); err != nil {
			return err
		}
		t.stateMu.Lock()
		t.unansweredProactive = 3
		t.stateMu.Unlock()
	}
	return nil
}

// SetUserMode persists explicit on/off intent. Selecting an active mode is the
// user's explicit action to clear a previous global mute or snooze.
func (t *CognitionTick) SetUserMode(ctx context.Context, mode string) error {
	if t.feedbackStore == nil {
		return fmt.Errorf("proactive feedback store is not configured")
	}
	var err error
	if mode == "off" {
		err = t.feedbackStore.UpsertProactiveControl(ctx, &types.ProactiveControl{Scope: "global", Mode: "muted", Source: "mode_switch"})
	} else {
		err = t.feedbackStore.ClearProactiveControl(ctx, "global", "")
	}
	if err != nil {
		return err
	}
	t.stateMu.Lock()
	t.mode = mode
	t.stateMu.Unlock()
	return nil
}

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

	ticker := time.NewTicker(t.Interval())
	defer ticker.Stop()

	for {
		select {
		case <-t.stopCh:
			return
		case <-ctx.Done():
			return
		case <-ticker.C:
			t.run(ctx)
		case interval := <-t.intervalCh:
			ticker.Reset(interval)
		}
	}
}

// ── Main tick logic ────────────────────────────────────────────────

func (t *CognitionTick) run(ctx context.Context) {
	if t.feedbackStore != nil {
		allowed, reason, err := t.feedbackStore.ProactiveAllowed(ctx, "", "", time.Now().Unix())
		if err != nil {
			log.Printf("[CognitionTick] load global proactive control: %v", err)
			return
		}
		if !allowed {
			log.Printf("[CognitionTick] suppressed by persistent control: %s", reason)
			return
		}
	}
	// Phase 0: Gate
	if !t.gate.TryAcquire() {
		log.Println("[CognitionTick] skipped — gate busy")
		return
	}
	defer t.gate.Release()
	t.stateMu.Lock()
	t.lastTickAt = time.Now()
	t.stateMu.Unlock()
	log.Println("[CognitionTick] tick started")

	// Phase 0.5: Feature extraction
	features := t.extractor.Extract(ctx)
	if features == nil {
		return
	}

	// Inject real unanswered count into features (overrides hardcoded zero)
	t.stateMu.RLock()
	unanswered := t.unansweredProactive
	interval := t.interval
	lastSpeakAt := t.lastProactiveSpeakAt
	minSpeakGap := t.minSpeakGap
	t.stateMu.RUnlock()
	features.R4_RecentRejections = float64(unanswered)

	// Phase 1: Compute drives
	drives := cognition.ComputeDrives(features, nil)

	// Phase 2: Score actions. Explicit feedback is a bounded preference bias;
	// hard gates remain authoritative and silence is never treated as rejection.
	scored := cognition.ScoreActions(drives, features, t.actions, nil)
	shadowScored := cloneScoredActions(scored)
	shadowReady := false
	shadowSamples := 0
	if t.feedbackStore != nil {
		global, err := t.feedbackStore.ActionFeedbackStats(ctx)
		if err != nil {
			log.Printf("[CognitionTick] load preference stats: %v", err)
		} else {
			contextKey := proactiveContextKey(features)
			local, err := t.feedbackStore.ActionFeedbackStatsForContext(ctx, contextKey)
			if err != nil {
				log.Printf("[CognitionTick] load contextual preference stats: %v", err)
			}
			scored = cognition.ApplyContextualPreferenceBias(scored, local, global, 3)
			// The constrained bandit candidate is observation-only: its recommendation
			// is recorded for replay and never delivered by this policy version.
			shadowResult := cognition.RankConstrainedBanditShadow(shadowScored, local, global, cognition.DefaultConstrainedBanditConfig())
			shadowScored = shadowResult.Actions
			shadowReady = shadowResult.Ready
			shadowSamples = shadowResult.TotalSamples
		}
	}

	if len(scored) == 0 {
		return
	}

	// Night gate: pre-filter to NightSafe so System2 doesn't waste rounds
	// choosing actions that will be hard-blocked anyway.
	isNight := features.U12_NightTime > 0
	if isNight {
		scored = cognition.FilterNightSafe(scored)
		shadowScored = cognition.FilterNightSafe(shadowScored)
		if len(scored) == 0 {
			return
		}
	}
	decisionID := fmt.Sprintf("decision-%d", time.Now().UnixNano())
	var shadow *types.ScoredAction
	if len(shadowScored) > 0 {
		candidate := shadowScored[0]
		shadow = &candidate
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
	record := func(state, detail string) {
		t.recordDecision(ctx, decisionID, features, scored, chosenAction.Name, chosenAction.Source, shadow, shadowReady, shadowSamples, state, detail)
	}

	// Track consecutive same action
	t.stateMu.Lock()
	if chosenAction.Name == t.lastActionName {
		t.consecutiveSame++
	} else {
		t.consecutiveSame = 0
	}
	t.lastActionName = chosenAction.Name
	t.stateMu.Unlock()

	// Phase 4: Hard gate (now sees real unansweredProactive via features)
	if allowed, reason := cognition.HardGate(types.ScoredAction{Action: chosenAction}, features); !allowed {
		log.Printf("[CognitionTick] hard gate blocked %s: %s", chosenAction.Name, reason)
		record("blocked", reason)
		return
	}
	if t.feedbackStore != nil {
		allowed, reason, err := t.feedbackStore.ProactiveAllowed(ctx, chosenAction.Name, chosenAction.Category, time.Now().Unix())
		if err != nil {
			log.Printf("[CognitionTick] load action proactive control: %v", err)
			return
		}
		if !allowed {
			log.Printf("[CognitionTick] hard control blocked %s: %s", chosenAction.Name, reason)
			record("blocked", reason)
			return
		}
	}

	// Phase 5: "none" or silent actions → skip
	if chosenAction.Name == "none" || chosenAction.OutcomeType == "silent" {
		topN := min(3, len(scored))
		topInfo := ""
		for i := 0; i < topN; i++ {
			topInfo += fmt.Sprintf("%s=%.2f ", scored[i].Action.Name, scored[i].FinalScore)
		}
		log.Printf("[CognitionTick] skip %s (aff=%.2f lon=%.2f ply=%.2f unanswered=%d) — drives(S=%.2f C=%.2f E=%.2f) top3: %s",
			chosenAction.Name, features.A1_1_Affection, features.A1_6_Loneliness, features.A1_5_Playfulness, unanswered,
			drives.Social, drives.Care, drives.Explore, topInfo)
		record("silent", "policy selected a silent action")
		return
	}

	// ── Silence backoff gate ──
	// Escalating backoff: 1 unanswered → 2x interval, 2 → 4x, ≥3 → suppress entirely.
	if unanswered >= 3 {
		log.Printf("[CognitionTick] suppressed speak — %d unanswered proactive messages", unanswered)
		record("blocked", "unanswered limit reached")
		return
	}
	if unanswered > 0 {
		backoff := interval * time.Duration(1<<uint(unanswered))
		if time.Since(lastSpeakAt) < backoff {
			log.Printf("[CognitionTick] backoff — last speak %v ago, need %v (unanswered=%d)",
				time.Since(lastSpeakAt).Round(time.Second), backoff, unanswered)
			record("blocked", "unanswered backoff active")
			return
		}
	}

	// ── Min speak gap gate ──
	if time.Since(lastSpeakAt) < minSpeakGap {
		log.Printf("[CognitionTick] min gap — last speak %v ago < %v",
			time.Since(lastSpeakAt).Round(time.Second), minSpeakGap)
		record("blocked", "minimum speak gap active")
		return
	}

	log.Printf("[CognitionTick] SPEAK action=%s score=%.2f (sys2=%v, drives S=%.2f C=%.2f, aff=%.2f, unanswered=%d)",
		chosenAction.Name, scored[0].FinalScore, isSystem2, drives.Social, drives.Care, features.A1_1_Affection, unanswered)

	// Phase 6: Create intent
	intent := t.buildIntent(ctx, decisionID, features, &chosenAction, isSystem2)
	if intent == nil {
		record("silent", "system2 selected none")
		return
	}

	// Phase 7: Schedule
	if err := t.scheduler.Submit(ctx, *intent); err != nil {
		log.Printf("[CognitionTick] scheduler submit: %v", err)
		record("failed", "scheduler: "+err.Error())
		return
	}

	// Phase 8: Schedule intent (release gate so deliver can proceed)
	t.gate.Release()
	// Phase 9: Deliver (outside tick lock)
	if t.gate.CanRelease(ctx) {
		intents := t.scheduler.Drain()
		if len(intents) > 0 {
			t.gate.OnPlaybackStart(ctx)
			result, err := t.deliverer.Deliver(ctx, intents)
			t.gate.OnPlaybackEnd(ctx)
			if err != nil {
				log.Printf("[CognitionTick] deliver: %v", err)
				record("failed", "delivery: "+err.Error())
			} else {
				// Delivery succeeded — mark as unanswered until user replies
				t.stateMu.Lock()
				t.unansweredProactive++
				t.lastProactiveSpeakAt = time.Now()
				unanswered = t.unansweredProactive
				t.stateMu.Unlock()
				record("delivered", result.Output)
				log.Printf("[CognitionTick] unansweredProactive → %d", unanswered)
			}
		} else {
			record("failed", "scheduled intent was not available for delivery")
		}
	} else {
		record("blocked", "delivery gate unavailable")
	}
}

// ── Intent builder ─────────────────────────────────────────────────

func (t *CognitionTick) buildIntent(ctx context.Context, decisionID string, f *types.QuantifiedFeatures, action *types.ActionDef, isSystem2 bool) *types.ProactiveIntent {
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
		ID:          fmt.Sprintf("intent-%d", time.Now().UnixNano()),
		DecisionID:  decisionID,
		Source:      action.Source,
		Action:      action.Name,
		Message:     fmt.Sprintf("%s: %s", action.SkillCard.Trigger, action.SkillCard.Action),
		Priority:    priority,
		CoalesceKey: action.Name,
		TTL:         120 * time.Second,
		CreatedAt:   time.Now(),
	}
}

func (t *CognitionTick) recordDecision(ctx context.Context, decisionID string, features *types.QuantifiedFeatures, scored []types.ScoredAction, chosenAction, source string, shadow *types.ScoredAction, shadowReady bool, shadowSamples int, state, detail string) {
	if t.feedbackStore == nil {
		return
	}
	type shadowTrace struct {
		PolicyVersion  string  `json:"policy_version"`
		Action         string  `json:"action"`
		Score          float64 `json:"score"`
		WouldDiffer    bool    `json:"would_differ"`
		PromotionReady bool    `json:"promotion_ready"`
		TotalSamples   int     `json:"total_samples"`
	}
	var shadowPolicy *shadowTrace
	if shadow != nil {
		shadowPolicy = &shadowTrace{PolicyVersion: "contextual-bandit-v1-shadow", Action: shadow.Action.Name, Score: shadow.FinalScore, WouldDiffer: shadow.Action.Name != chosenAction, PromotionReady: shadowReady, TotalSamples: shadowSamples}
	}
	contextJSON, err := json.Marshal(struct {
		PolicyContext string                    `json:"policy_context"`
		Features      *types.QuantifiedFeatures `json:"features"`
		OutcomeDetail string                    `json:"outcome_detail,omitempty"`
		ShadowPolicy  *shadowTrace              `json:"shadow_policy,omitempty"`
	}{PolicyContext: proactiveContextKey(features), Features: features, OutcomeDetail: detail, ShadowPolicy: shadowPolicy})
	if err != nil {
		log.Printf("[CognitionTick] encode decision context: %v", err)
		return
	}
	type candidate struct {
		Action string  `json:"action"`
		Score  float64 `json:"score"`
	}
	candidates := make([]candidate, 0, min(3, len(scored)))
	for i := 0; i < len(scored) && i < 3; i++ {
		candidates = append(candidates, candidate{Action: scored[i].Action.Name, Score: scored[i].FinalScore})
	}
	candidatesJSON, err := json.Marshal(candidates)
	if err != nil {
		log.Printf("[CognitionTick] encode candidates: %v", err)
		return
	}
	score := 0.0
	for _, item := range scored {
		if item.Action.Name == chosenAction {
			score = item.FinalScore
			break
		}
	}
	content := ""
	if state == "delivered" {
		content = detail
	}
	if err := t.feedbackStore.SaveProactiveDecision(ctx, &types.ProactiveDecision{DecisionID: decisionID, PolicyVersion: "rules-v1-audit", Action: chosenAction, Source: source, Score: score, ContextJSON: string(contextJSON), CandidatesJSON: string(candidatesJSON), Content: content, State: state}); err != nil {
		log.Printf("[CognitionTick] persist proactive decision: %v", err)
	}
}

func proactiveContextKey(features *types.QuantifiedFeatures) string {
	period := "day"
	if features.U12_NightTime > 0 {
		period = "night"
	} else if features.E1_Hour < 12 {
		period = "morning"
	} else if features.E1_Hour >= 18 {
		period = "evening"
	}
	work := "idle"
	if features.U3_IsWorking > 0.5 || features.U1_AppCategory == "work" {
		work = "work"
	}
	return "period:" + period + "|mode:" + work
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
					"type":        "string",
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

	decisionCtx := port.WithLLMCallMetadata(ctx, "chat", "system2_decision")
	_, err := t.executor.ChatWithTools(decisionCtx, "", []port.LLMMessage{
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

// AnalyzePostChat runs the proactive LLM+tool pipeline after a conversation turn.
// Called asynchronously from ChatOrchestrator's postChatHook.
// Guards: rate limit, message substance, negative emotion, min gap after proactive speak,
// and short-ack detection (messages that don't invite follow-up).
func (t *CognitionTick) AnalyzePostChat(userMsg, response string) {
	controlCtx, controlCancel := context.WithTimeout(context.Background(), 2*time.Second)
	if t.feedbackStore != nil {
		allowed, reason, err := t.feedbackStore.ProactiveAllowed(controlCtx, "", "", time.Now().Unix())
		if err != nil || !allowed {
			if err != nil {
				log.Printf("[PostChat] load global proactive control: %v", err)
			} else {
				log.Printf("[PostChat] suppressed by persistent control: %s", reason)
			}
			controlCancel()
			return
		}
	}
	controlCancel()

	// Gate: rate limit
	t.stateMu.Lock()
	if time.Since(t.lastPostChatAt) < 2*time.Minute {
		t.stateMu.Unlock()
		return
	}
	lastSpeakAt := t.lastProactiveSpeakAt
	minSpeakGap := t.minSpeakGap
	unanswered := t.unansweredProactive
	t.stateMu.Unlock()
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
	if time.Since(lastSpeakAt) < minSpeakGap {
		log.Printf("[PostChat] skipped — proactive speak %v ago < min gap %v",
			time.Since(lastSpeakAt).Round(time.Second), minSpeakGap)
		return
	}
	// Gate: don't interject if user hasn't engaged with our last proactive message
	if unanswered >= 2 {
		log.Printf("[PostChat] skipped — %d unanswered proactive, user hasn't engaged yet", unanswered)
		return
	}
	t.stateMu.Lock()
	// Reserve the analysis slot before making a potentially slow LLM call so two
	// concurrent chat completions cannot both pass the rate-limit gate.
	if time.Since(t.lastPostChatAt) < 2*time.Minute {
		t.stateMu.Unlock()
		return
	}
	t.lastPostChatAt = time.Now()
	t.stateMu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	log.Printf("[PostChat] analyzing: %q", truncateStr(userMsg, 50))
	analysisStart := time.Now()
	defer func() {
		log.Printf("[PostChatTiming] total_ms=%.3f", float64(time.Since(analysisStart).Microseconds())/1000)
	}()

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

	postChatCtx := port.WithLLMCallMetadata(ctx, "chat", "post_chat_decision")
	_, err := t.executor.ChatWithTools(postChatCtx, "", []port.LLMMessage{
		{Role: "user", Content: prompt},
	}, allTools, onToolCall, 5, "auto")

	if err != nil {
		log.Printf("[PostChat] LLM failed: %v", err)
		return
	}

	decisionID := fmt.Sprintf("decision-post-chat-%d", time.Now().UnixNano())
	action := cognition.ActionByName(selectedAction)
	if action == nil {
		action = &types.ActionDef{Name: selectedAction, Source: "post_chat", OutcomeType: "speak"}
	}
	scored := []types.ScoredAction{{Action: *action}}
	record := func(state, detail string) {
		t.recordDecision(ctx, decisionID, f, scored, selectedAction, "post_chat", nil, false, 0, state, detail)
	}
	if selectedAction == "" || selectedAction == "none" || selectedMessage == "" {
		if selectedAction == "" {
			selectedAction = "none"
		}
		record("silent", "post-chat policy selected no follow-up")
		return
	}
	if t.feedbackStore != nil {
		allowed, reason, err := t.feedbackStore.ProactiveAllowed(ctx, selectedAction, action.Category, time.Now().Unix())
		if err != nil {
			log.Printf("[PostChat] load action proactive control: %v", err)
			record("failed", "control lookup: "+err.Error())
			return
		}
		if !allowed {
			log.Printf("[PostChat] blocked %s by persistent control: %s", selectedAction, reason)
			record("blocked", reason)
			return
		}
	}

	// Submit as proactive intent via existing scheduler+deliverer pipeline
	intent := types.ProactiveIntent{
		ID:          fmt.Sprintf("intent-post-chat-%d", time.Now().UnixNano()),
		DecisionID:  decisionID,
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
		record("failed", "scheduler: "+err.Error())
		return
	}
	log.Printf("[PostChat] intent submitted: %s — %q", selectedAction, truncateStr(selectedMessage, 60))

	// Deliver immediately (don't wait for next tick)
	if t.gate.CanRelease(ctx) {
		t.gate.OnPlaybackStart(ctx)
		intents := t.scheduler.Drain()
		if len(intents) > 0 {
			result, err := t.deliverer.Deliver(ctx, intents)
			t.gate.OnPlaybackEnd(ctx)
			if err != nil {
				log.Printf("[PostChat] deliver: %v", err)
				record("failed", "delivery: "+err.Error())
			} else {
				t.stateMu.Lock()
				t.unansweredProactive++
				t.lastProactiveSpeakAt = time.Now()
				t.stateMu.Unlock()
				record("delivered", result.Output)
			}
		} else {
			record("failed", "scheduled intent was not available for delivery")
		}
	} else {
		record("blocked", "delivery gate unavailable")
	}
}

func truncateStr(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

func cloneScoredActions(in []types.ScoredAction) []types.ScoredAction {
	out := make([]types.ScoredAction, len(in))
	for i, item := range in {
		out[i] = item
		out[i].Modulators = make(map[string]float64, len(item.Modulators))
		for key, value := range item.Modulators {
			out[i].Modulators[key] = value
		}
	}
	return out
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
