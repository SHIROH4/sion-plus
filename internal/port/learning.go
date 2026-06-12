package port

import (
	"context"
	"github.com/SHIROH4/sion-plus/internal/domain/types"
)

// ── Learner ──

// Learner performs DPO-style batch weight updates from action outcomes.
// Stores drive snapshots at decision time, receives rewards later,
// and batch-updates the action weight matrix every 6 hours.
// Implementation: adapter/learning/learner.go
type Learner interface {
	RecordDrive(action string, drives *DriveSnapshot, reward float64) int
	UpdateReward(driveID int, reward float64)
	ShouldLearn() bool
	BatchLearn(ctx context.Context) int
	Audit(ctx context.Context) (*AuditResult, error)
}

type DriveSnapshot struct {
	Social  float64
	Care    float64
	Curious float64
	Quiet   float64
	Explore float64
}

type AuditResult struct {
	StuckActions   []string `json:"stuck_actions"`
	DriftWarning   bool     `json:"drift_warning"`
	Recommendation string   `json:"recommendation"`
}

// ── Strategy Agent ──

// StrategyAgent performs periodic strategic reflection and strategy distillation.
// Runs every 6-24 hours. Feeds recent outcomes + diaries + facts to LLM →
// extracts reusable StrategyPrinciples and thread management recommendations.
// Implementation: adapter/learning/strategy_agent.go
type StrategyAgent interface {
	ShouldRun() bool
	Run(ctx context.Context) (*types.DailyReflectionOutput, error)
	LastRun() int64
	NotifyInteraction()
	NotifyOutcome()
	NotifyEmotion(intensity float64)
}

// ── Curiosity Engine ──

// CuriosityEngine scans knowledge gaps and schedules exploration actions.
// Detects contradictions between facts, dormant threads, incomplete patterns.
// Implementation: adapter/learning/curiosity_engine.go
type CuriosityEngine interface {
	ScanGaps(ctx context.Context) ([]KnowledgeGap, error)
	ShouldExplore(gap KnowledgeGap) bool
	ExecuteExploration(ctx context.Context, gap KnowledgeGap) error
}

type KnowledgeGap struct {
	Topic        string  `json:"topic"`
	Source       string  `json:"source"` // "fact_contradiction"|"thread_dormant"|"pattern_incomplete"
	Priority     float64 `json:"priority"`
	LastExplored int64   `json:"last_explored"`
}
