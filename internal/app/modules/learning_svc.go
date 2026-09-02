package modules

import (
	"context"
	"log"
	"time"

	"github.com/SHIROH4/sion-plus/internal/adapter/learning"
	"github.com/SHIROH4/sion-plus/internal/domain/cognition"
	"github.com/SHIROH4/sion-plus/internal/port"
)

// LearningService wraps the learning stack (Learner + StrategyAgent + CuriosityEngine)
// as a Module with background scheduling.
type LearningService struct {
	Learner   port.Learner
	Strategy  port.StrategyAgent
	Curiosity port.CuriosityEngine

	executor    port.LLMExecutor
	memoryStore port.MemoryStore

	stopCh chan struct{}
}

// NewLearningService creates a learning service with all three adapters.
func NewLearningService(
	executor port.LLMExecutor,
	memoryStore port.MemoryStore,
	memoryRecall port.MemoryRecall,
) *LearningService {
	cfg := cognition.DefaultDPOConfig()
	learner := learning.NewLearner(cfg)

	strategyCfg := cognition.DefaultStrategyConfig()
	strategyAgent := learning.NewStrategyAgent(executor, memoryStore, strategyCfg)

	curiosity := learning.NewCuriosityEngine(memoryStore, memoryRecall)

	return &LearningService{
		Learner:     learner,
		Strategy:    strategyAgent,
		Curiosity:   curiosity,
		executor:    executor,
		memoryStore: memoryStore,
		stopCh:      make(chan struct{}),
	}
}

// SetExecutor updates the LLM executor (called after LLMService.Init).
func (s *LearningService) SetExecutor(exec port.LLMExecutor) {
	s.executor = exec
}

func (s *LearningService) Name() string { return "learning" }

func (s *LearningService) Init(ctx context.Context) error {
	log.Println("[LearningService] initialized (learner + strategy agent + curiosity engine)")
	return nil
}

func (s *LearningService) Start(ctx context.Context) error {
	go s.learningLoop(ctx)
	go s.strategyLoop(ctx)
	go s.curiosityLoop(ctx)
	log.Println("[LearningService] started (3 background loops)")
	return nil
}

func (s *LearningService) Stop(ctx context.Context) error {
	close(s.stopCh)
	log.Println("[LearningService] stopped")
	return nil
}

func (s *LearningService) Health(ctx context.Context) error {
	_, err := s.Learner.Audit(ctx)
	return err
}

// ── Background Loops ──

// learningLoop checks every 30 minutes whether batch learning should run.
func (s *LearningService) learningLoop(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-s.stopCh:
			return
		case <-ctx.Done():
			return
		case <-ticker.C:
			if s.Learner.ShouldLearn() {
				count := s.Learner.BatchLearn(ctx)
				if count > 0 {
					log.Printf("[LearningService] batch DPO update: %d actions", count)
				}
			}
		}
	}
}

// strategyLoop checks every hour whether strategic reflection should run.
func (s *LearningService) strategyLoop(ctx context.Context) {
	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()

	for {
		select {
		case <-s.stopCh:
			return
		case <-ctx.Done():
			return
		case <-ticker.C:
			if s.Strategy.ShouldRun() {
				output, err := s.Strategy.Run(ctx)
				if err != nil {
					log.Printf("[LearningService] strategy reflection: %v", err)
				} else if output != nil {
					log.Printf("[LearningService] strategy reflection: %s", output.NarrativeSummary)
				}
			}
		}
	}
}

// curiosityLoop scans for knowledge gaps every 2 hours.
func (s *LearningService) curiosityLoop(ctx context.Context) {
	ticker := time.NewTicker(2 * time.Hour)
	defer ticker.Stop()

	for {
		select {
		case <-s.stopCh:
			return
		case <-ctx.Done():
			return
		case <-ticker.C:
			gaps, err := s.Curiosity.ScanGaps(ctx)
			if err != nil {
				log.Printf("[LearningService] curiosity scan: %v", err)
				continue
			}
			for _, gap := range gaps {
				if s.Curiosity.ShouldExplore(gap) {
					if err := s.Curiosity.ExecuteExploration(ctx, gap); err != nil {
						log.Printf("[LearningService] curiosity explore: %v", err)
					}
				}
			}
			if len(gaps) > 0 {
				log.Printf("[LearningService] curiosity: %d gaps scanned", len(gaps))
			}
		}
	}
}

// ── Public Hooks (called by other modules) ──

// RecordDriveOutcome records a drive snapshot + reward for DPO learning.
func (s *LearningService) RecordDriveOutcome(action string, drives *port.DriveSnapshot, reward float64) int {
	s.Strategy.NotifyOutcome()
	return s.Learner.RecordDrive(action, drives, reward)
}

// NotifyInteraction notifies the strategy agent of a new interaction.
func (s *LearningService) NotifyInteraction() {
	s.Strategy.NotifyInteraction()
}

// NotifyEmotionSpike notifies the strategy agent of a strong emotional event.
func (s *LearningService) NotifyEmotionSpike(intensity float64) {
	s.Strategy.NotifyEmotion(intensity)
}
