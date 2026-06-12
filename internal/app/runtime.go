package app

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"runtime"
	"path/filepath"
	"time"

	"github.com/shirohania/sion/internal/adapter/llm"
	"github.com/shirohania/sion/internal/adapter/memory"
	"github.com/shirohania/sion/internal/adapter/perception"
	"github.com/shirohania/sion/internal/adapter/proactive"
	"github.com/shirohania/sion/internal/adapter/tool"
	"github.com/shirohania/sion/internal/app/modules"
	"github.com/shirohania/sion/internal/domain/types"
	domainMemory "github.com/shirohania/sion/internal/domain/memory"
	"github.com/shirohania/sion/internal/port"
)

type AppRuntime struct {
	modules  []modules.Module
	services struct {
		memory   *modules.MemoryService
		emotion  *modules.EmotionService
		llm      *modules.LLMService
		learning *modules.LearningService
	}

	Chat *modules.ChatOrchestrator

	CognitionTick *proactive.CognitionTick
	ToolRegistry  *tool.ToolRegistry
	personality   string
	dataDir       string
	startTime     time.Time

	promptBldr      *modules.PromptBuilder
	recallRef       port.MemoryRecall
	workerRef       *memory.MemoryWorker
	bufferRef       port.SessionBuffer
	compRef         *memory.Compressor
	perceptObserver port.ScreenObserver
}

type Config struct {
	DataDir      string
	Personality  string
	LLMProviders []port.LLMProviderConfig
	LLMRoutes    port.LLMRoutes
	VisionURL    string
	VisionKey    string
	VisionModel  string
}

func NewRuntime(cfg Config) (*AppRuntime, error) {
	if cfg.DataDir == "" {
		home, _ := os.UserHomeDir()
		cfg.DataDir = filepath.Join(home, ".sion")
	}
	r := &AppRuntime{dataDir: cfg.DataDir, personality: cfg.Personality, startTime: time.Now()}

	r.services.llm = modules.NewLLMService(cfg.LLMProviders, cfg.LLMRoutes, cfg.DataDir)

	dbPath := filepath.Join(r.dataDir, "sion.db")
	store, err := memory.NewSQLiteStore(dbPath)
	if err != nil {
		return nil, fmt.Errorf("sqlite: %w", err)
	}

	evidenceCfg := domainMemory.DefaultEvidenceConfig()
	evidence := memory.NewEvidenceEngine(store, evidenceCfg)
	buffer := memory.NewSessionBuffer(40, 0)
	recall := memory.NewRecall(store, evidence)
	comp := memory.NewCompressor(buffer, memory.DefaultCompressorConfig())
	eventLog := memory.NewEventLog(store)

	workerCfg := memory.DefaultWorkerConfig()
	worker := memory.NewMemoryWorker(store, evidence, recall, buffer, comp, workerCfg)

	r.services.memory = modules.NewMemoryService(store, buffer, recall, evidence, worker, comp, eventLog)

	emotionPath := filepath.Join(r.dataDir, "emotion.json")
	r.services.emotion = modules.NewEmotionService(emotionPath, r.services.llm.Executor)

	r.ToolRegistry = tool.NewToolRegistry()
	homeDir, _ := os.UserHomeDir()
	tool.InitAllowedPaths(homeDir, cfg.DataDir)
	r.ToolRegistry.RegisterFileTools()
	r.ToolRegistry.RegisterBashTool()
	r.ToolRegistry.RegisterSearchTool()

	cuaExecutor := r.services.llm.Executor
	if cfg.VisionURL != "" && cfg.VisionKey != "" {
		cuaExecutor = llm.NewOpenAIGateway(llm.GatewayConfig{
			BaseURL: cfg.VisionURL, APIKey: cfg.VisionKey, Model: cfg.VisionModel,
		})
		log.Printf("[Runtime] computer_use using dedicated vision gateway: %s", cfg.VisionModel)
	}
	cuaAgent := tool.NewComputerUseAgent(cuaExecutor, tool.NewMacOSObserver())
	r.ToolRegistry.RegisterComputerUseTool(cuaAgent)
	browserAgent := tool.NewBrowserAgent()
	browserAgent.SetExecutor(r.services.llm.Executor)
	r.ToolRegistry.RegisterBrowserTool(browserAgent)
	log.Printf("[Runtime] tool registry: %d tools registered", len(r.ToolRegistry.List()))

	perceptClassifier := perception.NewAppClassifier()
	r.perceptObserver = perception.NewScreenObserver(perceptClassifier)

	r.promptBldr = modules.NewPromptBuilder(cfg.Personality)
	r.recallRef = recall
	r.workerRef = worker
	r.bufferRef = buffer
	r.compRef = comp

	r.services.learning = modules.NewLearningService(
		r.services.llm.Executor, store, recall,
	)

	r.modules = []modules.Module{r.services.llm, r.services.emotion, r.services.memory, r.services.learning}
	return r, nil
}

func (r *AppRuntime) Init(ctx context.Context) error {
	for _, m := range r.modules {
		log.Printf("[Runtime] init %s", m.Name())
		if err := m.Init(ctx); err != nil {
			return fmt.Errorf("%s init: %w", m.Name(), err)
		}
	}

	// Wrap primary executor with global rate limiter (20 calls/min, burst=5)
	r.services.llm.Executor = llm.WrapRateLimited(r.services.llm.Executor, 20, 5, "global")

	r.services.memory.SetExecutor(r.services.llm.Executor)
	r.services.emotion.SetExecutor(r.services.llm.Executor)
	r.services.learning.SetExecutor(r.services.llm.Executor)

	memory.NewLLMHooksWithRegistry(r.services.llm.Registry, r.workerRef, r.compRef).Install()
	r.Chat = modules.NewChatOrchestrator(
		r.services.emotion.Evaluator, r.services.emotion.Store,
		r.recallRef, r.workerRef, r.bufferRef,
		r.services.llm.Executor, r.promptBldr, r.perceptObserver,
	)
	r.Chat.SetToolRegistry(r.ToolRegistry)

	{
		extractor := proactive.NewFeatureExtractor(
			r.services.emotion.Store, r.services.memory.Store, r.perceptObserver,
		)
		r.CognitionTick = proactive.NewCognitionTick(
			proactive.NewDeliveryGate(), proactive.NewIntentScheduler(),
			proactive.NewIntentDeliverer(r.services.llm.Executor, nil, r.personality),
			extractor, r.services.llm.Executor,
		)
		r.CognitionTick.SetFirstTickDelay(30 * time.Second)
		r.CognitionTick.SetToolRegistry(r.ToolRegistry)
		r.Chat.SetPostChatHook(r.CognitionTick.AnalyzePostChat)
			r.Chat.SetPreChatHook(r.CognitionTick.OnUserMessage)
		log.Println("[Runtime] proactive cognition wired")

		{
			selfModelStore := memory.NewSelfModelStore(r.dataDir)
			selfModelStore.Load(ctx)
			r.promptBldr.SetUserModel(selfModelStore.UserModel())
			r.promptBldr.SetSelfModel(selfModelStore.SelfModel())

			identityBuilder := memory.NewIdentityBuilder(selfModelStore, r.services.llm.Executor, r.services.memory.Store.(*memory.SQLiteStore))
			r.workerRef.SetIdentityBuilder(identityBuilder)

			personaStore := memory.NewPersonaStore(r.services.memory.Store.(*memory.SQLiteStore))
			r.CognitionTick.SetPersona(&personaBridge{store: personaStore})
			log.Printf("[Runtime] identity + persona wired")
		}
	}

	return nil
}

func (r *AppRuntime) Start(ctx context.Context) error {
	for _, m := range r.modules {
		log.Printf("[Runtime] start %s", m.Name())
		if err := m.Start(ctx); err != nil {
			return fmt.Errorf("%s start: %w", m.Name(), err)
		}
	}
	if r.CognitionTick != nil {
		r.CognitionTick.Start(ctx)
	}
	log.Println("[Runtime] all services started")
	return nil
}

func (r *AppRuntime) Stop(ctx context.Context) error {
	if r.CognitionTick != nil {
		r.CognitionTick.Stop()
	}
	for i := len(r.modules) - 1; i >= 0; i-- {
		m := r.modules[i]
		log.Printf("[Runtime] stop %s", m.Name())
		if err := m.Stop(ctx); err != nil {
			log.Printf("[Runtime] %s stop error: %v", m.Name(), err)
		}
	}
	return nil
}

// PersonalityConfig is the full editable persona configuration.
type PersonalityConfig struct {
	Name          string         `json:"name"`
	SystemPrompt  string         `json:"system_prompt"`
	Traits        map[string]int `json:"traits"`
	SpeakingStyle string         `json:"speaking_style"`
	Background    string         `json:"background"`              
}

// LoadPersonalityConfig reads the personality config from disk, falling back to defaults.
func (r *AppRuntime) LoadPersonalityConfig() (*PersonalityConfig, error) {
	path := filepath.Join(r.dataDir, "personality.json")
	data, err := os.ReadFile(path)
	cfg := &PersonalityConfig{
		Name:   "Sion",
		SystemPrompt: r.personality,
		Traits: map[string]int{"warmth": 8, "playfulness": 7, "formality": 3, "curiosity": 6, "empathy": 8},
		SpeakingStyle: "",
		Background:    "",
	}
	if err != nil {
		return cfg, nil // return defaults
	}
	json.Unmarshal(data, cfg)
	if cfg.Traits == nil {
		cfg.Traits = map[string]int{"warmth": 8, "playfulness": 7, "formality": 3, "curiosity": 6, "empathy": 8}
	}
	return cfg, nil
}

// SavePersonalityConfig writes the personality config to disk.
func (r *AppRuntime) LLMConfig() ([]port.LLMProviderConfig, port.LLMRoutes) {
	return r.services.llm.Config()
}

func (r *AppRuntime) ReloadLLMConfig(providers []port.LLMProviderConfig, routes port.LLMRoutes) error {
	return r.services.llm.ReloadConfig(providers, routes)
}

func (r *AppRuntime) SavePersonalityConfig(cfg *PersonalityConfig) error {
	path := filepath.Join(r.dataDir, "personality.json")
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

func (r *AppRuntime) EmotionState() (types.EmotionState, types.EmotionVector) {
	return r.services.emotion.Store.Current()
}

func (r *AppRuntime) EmotionHistory() ([]types.EmotionState, []types.EmotionVector) {
	return r.services.emotion.Store.History()
}

func (r *AppRuntime) TodayMessageCount(ctx context.Context) int {
	return r.services.memory.Store.CountTodayMessages(ctx)
}

func (r *AppRuntime) LoadChatHistory(ctx context.Context, limit int) ([]types.Message, error) {
	return r.services.memory.Store.LoadHistory(ctx, limit)
}

func (r *AppRuntime) MemoryStore() port.MemoryStore {
	return r.services.memory.Store
}

func (r *AppRuntime) ListActiveFacts(ctx context.Context, minScore float64) ([]types.FactEntry, error) {
	return r.services.memory.Store.ListActiveFacts(ctx, minScore)
}

func (r *AppRuntime) ListTopics(ctx context.Context) ([]types.Topic, error) {
	return r.services.memory.Store.ListTopics(ctx)
}

func (r *AppRuntime) ListActiveThreads(ctx context.Context) ([]types.ConversationThread, error) {
	return r.services.memory.Store.ListActiveThreads(ctx)
}

func (r *AppRuntime) ProactiveStatus() (time.Duration, string, time.Time) {
	if r.CognitionTick == nil {
		return 0, "", time.Time{}
	}
	return r.CognitionTick.Interval(), r.CognitionTick.LastAction(), r.CognitionTick.LastTickAt()
}

func (r *AppRuntime) ProactiveActions() []types.ActionDef {
	if r.CognitionTick == nil {
		return nil
	}
	return r.CognitionTick.Actions()
}

func (r *AppRuntime) ListTools() []*tool.ToolDef {
	return r.ToolRegistry.List()
}

func (r *AppRuntime) Health(ctx context.Context) map[string]error {
	results := make(map[string]error)
	for _, m := range r.modules {
		results[m.Name()] = m.Health(ctx)
	}
	return results
}

// SystemStats returns basic system resource usage.
func (r *AppRuntime) SystemStats() (cpu float64, memUsedMB int64, memTotalMB int64, goroutines int, uptimeSec int64) {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	memUsedMB = int64(m.Alloc / 1024 / 1024)
	memTotalMB = int64(m.Sys / 1024 / 1024)
	goroutines = runtime.NumGoroutine()
	cpu = float64(runtime.NumCPU())
	uptimeSec = int64(time.Since(r.startTime).Seconds())
	return
}
