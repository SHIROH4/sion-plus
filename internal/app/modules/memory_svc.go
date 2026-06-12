package modules

import (
	"context"
	"log"

	"github.com/shirohania/sion/internal/adapter/memory"
	"github.com/shirohania/sion/internal/port"
)

// MemoryService wraps the memory stack as a Module.
// Manages MemoryWorker, Compressor, SessionBuffer, and LLMHooks lifecycle.
type MemoryService struct {
	Worker    *memory.MemoryWorker
	Buffer    port.SessionBuffer
	Recall    port.MemoryRecall
	Compress  *memory.Compressor
	Evidence  *memory.EvidenceEngine
	Store     port.MemoryStore
	EventLog  *memory.EventLog

	executor port.LLMExecutor // for LLMHooks injection
}

// NewMemoryService creates the memory service. Call SetExecutor after LLM init.
func NewMemoryService(
	store port.MemoryStore,
	buffer port.SessionBuffer,
	recall port.MemoryRecall,
	evidence *memory.EvidenceEngine,
	worker *memory.MemoryWorker,
	comp *memory.Compressor,
	eventLog *memory.EventLog,
) *MemoryService {
	return &MemoryService{
		Store:    store,
		Buffer:   buffer,
		Recall:   recall,
		Evidence: evidence,
		Worker:   worker,
		Compress: comp,
		EventLog: eventLog,
	}
}

// SetExecutor updates the LLM executor (called after LLMService.Init).
func (s *MemoryService) SetExecutor(exec port.LLMExecutor) {
	s.executor = exec
	// Hooks are installed by runtime.Init() via NewLLMHooksWithRegistry
}

func (s *MemoryService) Name() string { return "memory" }

func (s *MemoryService) Init(ctx context.Context) error {
	// Wire LLM hooks into worker + compressor
	if s.executor != nil {
		hooks := memory.NewLLMHooks(s.executor, s.Worker, s.Compress)
		hooks.Install()
	}
	// Wire EventLog into evidence engine
	if s.EventLog != nil && s.Evidence != nil {
		s.Evidence.SetEventLog(s.EventLog)
	}
	// Wire EventLog into worker
	if s.EventLog != nil {
		s.Worker.SetEventLog(s.EventLog)
	}
	log.Println("[MemoryService] initialized")
	return nil
}

func (s *MemoryService) Start(ctx context.Context) error {
	s.Worker.Start(ctx, memory.DefaultWorkerConfig())
	log.Println("[MemoryService] started")
	return nil
}

func (s *MemoryService) Stop(ctx context.Context) error {
	s.Worker.Stop()
	s.Store.Close()
	log.Println("[MemoryService] stopped")
	return nil
}

func (s *MemoryService) Health(ctx context.Context) error {
	// Check SQLite is accessible
	_, err := s.Store.ListActiveFacts(ctx, 0)
	return err
}
