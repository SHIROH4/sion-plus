package modules

import (
	"context"
	"log"

	"github.com/shirohania/sion/internal/adapter/emotion"
	"github.com/shirohania/sion/internal/port"
)

// EmotionService wraps the emotion stack as a Module.
type EmotionService struct {
	Store     port.EmotionStateManager
	Evaluator port.EmotionSignalSource

	executor port.LLMExecutor
	statePath string
}

// NewEmotionService creates an emotion service.
func NewEmotionService(statePath string, executor port.LLMExecutor) *EmotionService {
	store := emotion.NewEmotionStore(statePath)
	evaluator := emotion.NewEmotionEvaluator(executor, store)
	return &EmotionService{
		Store:     store,
		Evaluator: evaluator,
		executor:  executor,
		statePath: statePath,
	}
}

// SetExecutor updates the LLM executor (called after LLMService.Init).
func (s *EmotionService) SetExecutor(exec port.LLMExecutor) {
	s.executor = exec
	s.Evaluator = emotion.NewEmotionEvaluator(exec, s.Store)
}

func (s *EmotionService) Name() string { return "emotion" }

func (s *EmotionService) Init(ctx context.Context) error {
	// Restore persisted state
	if err := s.Store.Load(ctx); err != nil {
		log.Printf("[EmotionService] load state: %v (using defaults)", err)
	}
	log.Println("[EmotionService] initialized")
	return nil
}

func (s *EmotionService) Start(ctx context.Context) error {
	s.Store.Start() // background decay loop
	log.Println("[EmotionService] started")
	return nil
}

func (s *EmotionService) Stop(ctx context.Context) error {
	s.Store.Stop()
	// Persist state before shutdown
	if err := s.Store.Save(ctx); err != nil {
		log.Printf("[EmotionService] save state: %v", err)
	}
	log.Println("[EmotionService] stopped")
	return nil
}

func (s *EmotionService) Health(ctx context.Context) error {
	_, _ = s.Store.Current()
	return nil
}
