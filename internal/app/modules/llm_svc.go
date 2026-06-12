package modules

import (
	"context"
	"fmt"
	"log"

	"github.com/shirohania/sion/internal/adapter/llm"
	"github.com/shirohania/sion/internal/port"
)

// LLMService wraps the LLM stack as a Module.
type LLMService struct {
	Registry    *llm.ProviderRegistry
	Tokenizer   *llm.TokenTracker
	Executor    port.LLMExecutor // primary tracked executor ("chat" route)

	providers []port.LLMProviderConfig
	routes    port.LLMRoutes
}

// NewLLMService creates an LLM service with provider registry and token tracking.
func NewLLMService(providers []port.LLMProviderConfig, routes port.LLMRoutes, dataDir string) *LLMService {
	tracker := llm.NewTokenTracker(dataDir)
	return &LLMService{
		Registry:  llm.NewProviderRegistry(),
		Tokenizer: tracker,
		providers: providers,
		routes:    routes,
	}
}

func (s *LLMService) Name() string { return "llm" }

func (s *LLMService) Init(ctx context.Context) error {
	// Load provider configs into registry
	if err := s.Registry.Reload(s.providers, s.routes); err != nil {
		return err
	}

	// Start token tracker
	s.Tokenizer.Start(ctx)

	// Create tracked executor for "chat" route (primary)
	exec, name, err := s.Registry.GetExecutor("chat")
	if err != nil {
		log.Printf("[LLMService] no chat executor: %v (using raw gateway)", err)
	} else {
		s.Executor = llm.WrapExecutor(exec, s.Tokenizer, "chat")
		log.Printf("[LLMService] primary executor: %s", name)
	}

	log.Println("[LLMService] initialized")
	return nil
}

func (s *LLMService) Start(ctx context.Context) error {
	s.Registry.StartHealthCheck(ctx)
	log.Println("[LLMService] started")
	return nil
}

func (s *LLMService) Stop(ctx context.Context) error {
	s.Tokenizer.Stop(ctx)
	log.Println("[LLMService] stopped")
	return nil
}

func (s *LLMService) Health(ctx context.Context) error {
	healthy := s.Registry.ListHealthy()
	if len(healthy) == 0 {
		return fmt.Errorf("no healthy providers")
	}
	return nil
}

func (s *LLMService) Config() ([]port.LLMProviderConfig, port.LLMRoutes) {
	return s.providers, s.routes
}

func (s *LLMService) ReloadConfig(providers []port.LLMProviderConfig, routes port.LLMRoutes) error {
	if err := s.Registry.Reload(providers, routes); err != nil {
		return err
	}
	s.providers = providers
	s.routes = routes
	return nil
}
