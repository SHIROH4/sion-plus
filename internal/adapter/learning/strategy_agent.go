package learning

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/SHIROH4/sion-plus/internal/domain/cognition"
	"github.com/SHIROH4/sion-plus/internal/domain/types"
	"github.com/SHIROH4/sion-plus/internal/port"
)

// StrategyAgentImpl implements port.StrategyAgent.
// Periodically runs LLM-driven strategic reflection to distill
// StrategyPrinciples and thread management recommendations from
// recent outcomes + diaries + facts.
type StrategyAgentImpl struct {
	mu sync.Mutex

	executor    port.LLMExecutor
	memoryStore port.MemoryStore

	cfg        cognition.StrategyConfig
	lastRunAt  int64
	interactionCount int
	outcomeCount     int
	maxEmotionIntensity float64
}

var _ port.StrategyAgent = (*StrategyAgentImpl)(nil)

// NewStrategyAgent creates a StrategyAgent.
func NewStrategyAgent(
	executor port.LLMExecutor,
	memoryStore port.MemoryStore,
	cfg cognition.StrategyConfig,
) *StrategyAgentImpl {
	if cfg.MinIntervalHours == 0 {
		cfg = cognition.DefaultStrategyConfig()
	}
	return &StrategyAgentImpl{
		executor:    executor,
		memoryStore: memoryStore,
		cfg:         cfg,
	}
}

// NotifyInteraction increments the interaction counter for scheduling.
func (s *StrategyAgentImpl) NotifyInteraction() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.interactionCount++
}

// NotifyOutcome increments the new-outcome counter.
func (s *StrategyAgentImpl) NotifyOutcome() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.outcomeCount++
}

// NotifyEmotion records the highest emotion intensity seen since last run.
func (s *StrategyAgentImpl) NotifyEmotion(intensity float64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if intensity > s.maxEmotionIntensity {
		s.maxEmotionIntensity = intensity
	}
}

// ShouldRun checks scheduling criteria.
func (s *StrategyAgentImpl) ShouldRun() bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	ok, _ := cognition.ShouldReflect(
		s.cfg, s.lastRunAt, s.interactionCount,
		s.outcomeCount, s.maxEmotionIntensity,
	)
	return ok
}

// LastRun returns the Unix timestamp of the last completed run.
func (s *StrategyAgentImpl) LastRun() int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.lastRunAt
}

// Run executes a strategic reflection cycle:
// 1. Gather inputs (diaries, facts, outcomes, threads)
// 2. Build a prompt for the LLM
// 3. Parse the structured output
// 4. Save new StrategyPrinciples to L3 memory
func (s *StrategyAgentImpl) Run(ctx context.Context) (*types.DailyReflectionOutput, error) {
	s.mu.Lock()
	interactionCount := s.interactionCount
	outcomeCount := s.outcomeCount
	maxIntensity := s.maxEmotionIntensity
	s.mu.Unlock()

	if s.executor == nil {
		return nil, fmt.Errorf("strategy agent: no LLM executor")
	}

	// Gather inputs
	input := s.buildInput(ctx)

	// Build prompt
	prompt := s.buildPrompt(input, interactionCount, outcomeCount, maxIntensity)

	// Call LLM
	response, err := s.executor.Chat(ctx, prompt, nil)
	if err != nil {
		return nil, fmt.Errorf("strategy agent LLM: %w", err)
	}

	// Parse output
	output, err := s.parseReflectionOutput(response)
	if err != nil {
		return nil, fmt.Errorf("strategy agent parse: %w", err)
	}

	// Persist new principles
	for _, p := range output.NewPrinciples {
		p.Source = "daily_reflection"
		p.Active = true
		if _, err := s.memoryStore.SaveStrategy(ctx, &p); err != nil {
			log.Printf("[StrategyAgent] save principle: %v", err)
		}
	}

	// Deactivate stale principles
	for _, id := range output.DeactivatePrincipleIDs {
		if err := s.memoryStore.DeactivateStrategy(ctx, id); err != nil {
			log.Printf("[StrategyAgent] deactivate principle %d: %v", id, err)
		}
	}

	// Apply thread recommendations
	for _, rec := range output.ThreadRecommendations {
		s.applyThreadRecommendation(ctx, rec)
	}

	// Reset counters
	s.mu.Lock()
	s.lastRunAt = time.Now().Unix()
	s.interactionCount = 0
	s.outcomeCount = 0
	s.maxEmotionIntensity = 0
	s.mu.Unlock()

	log.Printf("[StrategyAgent] reflection complete: %d new principles, %d thread recs",
		len(output.NewPrinciples), len(output.ThreadRecommendations))
	return output, nil
}

func (s *StrategyAgentImpl) buildInput(ctx context.Context) *types.DailyReflectionInput {
	input := &types.DailyReflectionInput{}

	// Active strategies
	strategies, err := s.memoryStore.ListActiveStrategies(ctx)
	if err == nil {
		input.ActivePrinciples = strategies
	}

	// Recent diaries
	diaries, err := s.memoryStore.ListDiaries(ctx, 10)
	if err == nil {
		for _, d := range diaries {
			input.RecentDiaries = append(input.RecentDiaries, d.Summary)
		}
	}

	// Active threads
	threads, err := s.memoryStore.ListActiveThreads(ctx)
	if err == nil {
		input.ActiveThreads = threads
	}

	// Recent outcomes
	outcomes, err := s.memoryStore.QueryOutcomes(ctx, port.OutcomeFilter{
		Limit: 50,
	})
	if err == nil {
		input.RecentOutcomes = outcomes
	}

	return input
}

func (s *StrategyAgentImpl) buildPrompt(input *types.DailyReflectionInput, interactionCount, outcomeCount int, maxIntensity float64) string {
	var sb strings.Builder
	sb.WriteString("You are Sion's strategic reflection system. Analyze recent interaction data and produce structured recommendations.\n\n")

	sb.WriteString("## Recent Activity\n")
	sb.WriteString(fmt.Sprintf("- Interactions since last reflection: %d\n", interactionCount))
	sb.WriteString(fmt.Sprintf("- New outcomes: %d\n", outcomeCount))
	sb.WriteString(fmt.Sprintf("- Max emotion intensity: %.2f\n\n", maxIntensity))

	if len(input.RecentDiaries) > 0 {
		sb.WriteString("## Recent Diaries\n")
		for _, d := range input.RecentDiaries {
			sb.WriteString(fmt.Sprintf("- %s\n", d))
		}
		sb.WriteString("\n")
	}

	if len(input.ActivePrinciples) > 0 {
		sb.WriteString("## Active Strategies\n")
		for _, p := range input.ActivePrinciples {
			sb.WriteString(fmt.Sprintf("- [%d] When %s: %s (avoid: %s)\n", p.ID, p.Situation, p.GoodStrategy, p.BadStrategy))
		}
		sb.WriteString("\n")
	}

	if len(input.ActiveThreads) > 0 {
		sb.WriteString("## Active Threads\n")
		for _, t := range input.ActiveThreads {
			sb.WriteString(fmt.Sprintf("- [%d] %s: %s (status: %s)\n", t.ID, t.Type, t.Goal, t.Status))
		}
		sb.WriteString("\n")
	}

	sb.WriteString("## Instructions\n")
	sb.WriteString("Respond with a JSON object containing:\n")
	sb.WriteString(`{
  "self_model_update": "updated self-description or empty string",
  "new_principles": [{"situation": "...", "good_strategy": "...", "bad_strategy": "...", "reason": "...", "confidence": 0.8}],
  "deactivate_principle_ids": [1, 2],
  "tactical_directives": ["directive 1", "directive 2"],
  "thread_recommendations": [{"action": "create|resolve|stale", "type": "...", "goal": "...", "priority": 0.7, "thread_id": 0}],
  "narrative_summary": "one paragraph summary of what you learned"
}`)
	sb.WriteString("\n\nBe concise. Only include principles you're confident about (confidence >= 0.6).")

	return sb.String()
}

func (s *StrategyAgentImpl) parseReflectionOutput(response string) (*types.DailyReflectionOutput, error) {
	// Extract JSON from the response (may be wrapped in markdown)
	jsonStr := extractJSON(response)

	var output types.DailyReflectionOutput
	if err := json.Unmarshal([]byte(jsonStr), &output); err != nil {
		return nil, fmt.Errorf("json unmarshal: %w (response: %.200s)", err, response)
	}
	return &output, nil
}

func (s *StrategyAgentImpl) applyThreadRecommendation(ctx context.Context, rec types.ThreadRecommendation) {
	switch rec.Action {
	case "resolve":
		if rec.ThreadID > 0 {
			_ = s.memoryStore.ResolveThread(ctx, rec.ThreadID, rec.Outcome, rec.Learnings)
		}
	case "stale":
		if rec.ThreadID > 0 {
			_ = s.memoryStore.MarkThreadStale(ctx, rec.ThreadID)
		}
	case "create":
		thread := &types.ConversationThread{
			Type:         rec.Type,
			Goal:         rec.Goal,
			Status:       "active",
			Priority:     rec.Priority,
			BestApproach: rec.BestApproach,
		}
		_, _ = s.memoryStore.SaveThread(ctx, thread)
	}
}

// extractJSON finds the first JSON object or array in a string.
func extractJSON(s string) string {
	start := -1
	depth := 0
	for i, c := range s {
		if c == '{' || c == '[' {
			if start == -1 {
				start = i
			}
			depth++
		} else if c == '}' || c == ']' {
			depth--
			if depth == 0 && start >= 0 {
				return s[start : i+1]
			}
		}
	}
	return s
}
