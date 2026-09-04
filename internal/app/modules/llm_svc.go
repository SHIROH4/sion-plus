package modules

import (
	"context"
	"fmt"
	"log"

	"github.com/SHIROH4/sion-plus/internal/adapter/llm"
	"github.com/SHIROH4/sion-plus/internal/port"
)

// LLMService wraps the LLM stack as a Module.
type LLMService struct {
	Registry  *llm.ProviderRegistry
	Tokenizer *llm.TokenTracker
	Executor  port.LLMExecutor // primary tracked executor ("chat" route)

	providers []port.LLMProviderConfig
	routes    port.LLMRoutes
}

// routedExecutor resolves the configured provider immediately before each
// request. Keeping this indirection is important because the dashboard can
// reload provider credentials while chat, emotion and proactive modules are
// already holding a reference to LLMService.Executor.
type routedExecutor struct {
	registry port.LLMProviderRegistry
	tracker  *llm.TokenTracker
	taskType string
	label    string
}

var _ port.LLMExecutor = (*routedExecutor)(nil)

func (e *routedExecutor) resolve(ctx context.Context) (port.LLMExecutor, error) {
	route := port.LLMRouteFromContext(ctx, e.taskType)
	exec, _, err := e.registry.GetExecutor(route)
	if err != nil {
		return nil, err
	}
	return llm.WrapExecutor(exec, e.tracker, e.label), nil
}

func (e *routedExecutor) Chat(ctx context.Context, systemPrompt string, msgs []port.LLMMessage) (string, error) {
	exec, err := e.resolve(ctx)
	if err != nil {
		return "", err
	}
	return exec.Chat(ctx, systemPrompt, msgs)
}

func (e *routedExecutor) ChatWithTools(ctx context.Context, systemPrompt string, msgs []port.LLMMessage, tools []port.ToolDef, onToolCall func(string, string) string, maxRounds int, toolChoice string) (string, error) {
	exec, err := e.resolve(ctx)
	if err != nil {
		return "", err
	}
	return exec.ChatWithTools(ctx, systemPrompt, msgs, tools, onToolCall, maxRounds, toolChoice)
}

func (e *routedExecutor) ChatStream(ctx context.Context, systemPrompt string, msgs []port.LLMMessage, onChunk func(string) error) error {
	exec, err := e.resolve(ctx)
	if err != nil {
		return err
	}
	return exec.ChatStream(ctx, systemPrompt, msgs, onChunk)
}

func (e *routedExecutor) IsAvailable(ctx context.Context) bool {
	exec, err := e.resolve(ctx)
	return err == nil && exec.IsAvailable(ctx)
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

	_, name, err := s.Registry.GetExecutor("chat")
	if err != nil {
		log.Printf("[LLMService] no chat executor: %v", err)
	} else {
		// Keep a stable executor reference for modules, but resolve its concrete
		// provider per call so ReloadConfig updates credentials without a restart.
		// When no provider is configured, retain an explicitly injected executor
		// for tests and embedding hosts.
		s.Executor = &routedExecutor{registry: s.Registry, tracker: s.Tokenizer, taskType: "chat", label: "chat"}
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
