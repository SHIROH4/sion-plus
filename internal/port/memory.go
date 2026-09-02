package port

import (
	"context"
	"github.com/SHIROH4/sion-plus/internal/domain/types"
)

// ── Core Store ──

// MemoryStore is the central abstraction for all memory persistence.
// It combines L0-L3 storage + chat history + episodes + topics + threads + outcomes.
// Implementation: adapter/memory/sqlite_store.go
type MemoryStore interface {
	// ── L0 Working Memory ──
	Session() SessionBuffer

	// ── L1 Diary (Episodic) ──
	SaveDiary(ctx context.Context, entry *types.DiaryEntry) error
	ListDiaries(ctx context.Context, limit int) ([]types.DiaryEntry, error)
	SearchDiaries(ctx context.Context, vector []float32, topK int) ([]types.DiaryEntry, error)
	ArchiveDiary(ctx context.Context, id int64) error
	DiaryCount(ctx context.Context) int

	// ── L2 Facts (Semantic) ──
	SaveFact(ctx context.Context, fact *types.FactEntry) error
	GetFact(ctx context.Context, id int64) (*types.FactEntry, error)
	ListActiveFacts(ctx context.Context, minScore float64) ([]types.FactEntry, error)
	ListAllFacts(ctx context.Context) ([]types.FactEntry, error)
	SearchFacts(ctx context.Context, query string, topK int) ([]types.FactEntry, error)
	SearchFactsByVector(ctx context.Context, vector []float32, topK int) ([]types.FactEntry, error)
	ArchiveFact(ctx context.Context, id int64) error
	UpdateFact(ctx context.Context, fact *types.FactEntry) error

	// ── L3 Strategies ──
	SaveStrategy(ctx context.Context, s *types.StrategyPrinciple) (int64, error)
	ListActiveStrategies(ctx context.Context) ([]types.StrategyPrinciple, error)
	DeactivateStrategy(ctx context.Context, id int64) error
	SearchStrategiesByVector(ctx context.Context, vector []float32, topK int) ([]types.StrategyPrinciple, error)

	// ── Chat History ──
	SaveHistory(ctx context.Context, msgs []types.Message) error
	LoadHistory(ctx context.Context, limit int) ([]types.Message, error)
	CleanOldHistory(ctx context.Context, olderThanDays int) error

	// ── Episodes ──
	SaveEpisode(ctx context.Context, ep *types.Episode) (int64, error)
	GetEpisode(ctx context.Context, id int64) (*types.Episode, error)

	// ── Topics ──
	SaveTopic(ctx context.Context, t *types.Topic) (int64, error)
	ListTopics(ctx context.Context) ([]types.Topic, error)

	// ── Threads ──
	SaveThread(ctx context.Context, t *types.ConversationThread) (int64, error)
	ListActiveThreads(ctx context.Context) ([]types.ConversationThread, error)
	ResolveThread(ctx context.Context, id int64, outcome, learnings string) error
	MarkThreadStale(ctx context.Context, id int64) error

	// ── Outcomes ──
	SaveOutcome(ctx context.Context, o *types.ActionOutcome) error
	QueryOutcomes(ctx context.Context, filter OutcomeFilter) ([]types.ActionOutcome, error)

	// ── Stats ──
	CountTodayMessages(ctx context.Context) int

	// ── Maintenance ──
	RunForgetting(ctx context.Context) error
	Close() error
}

// ── Session Buffer (L0) ──

// SessionBuffer is the AI's working memory — a ring buffer with time-based eviction.
type SessionBuffer interface {
	Append(msg types.Message)
	Recent(n int) []types.Message
	All() []types.Message
	Len() int
	Clear()

	// Memo returns the compression memo (summarised old history), or nil.
	Memo() *types.Message

	// SetMemo stores a compression memo after old messages are flushed.
	SetMemo(content string)
}

// ── Outcome Filter ──

// OutcomeFilter narrows outcome queries.
type OutcomeFilter struct {
	Source   string // "proactive"|"reactive"|"" = all
	SinceSec int64  // unix seconds, 0 = no filter
	Limit    int    // 0 = default 100
}

// ── Evidence Engine ──

// EvidenceEngine manages the credibility scores of memory entries.
// Uses dual-signal half-life decay (reinforcement - disputation).
// Implementation: adapter/memory/evidence_engine.go
type EvidenceEngine interface {
	// ApplySignal applies a reinforcement or disputation delta to an entry.
	ApplySignal(ctx context.Context, entryID int64, sig EvidenceSignal) (*types.EvidenceSnapshot, error)

	// Score computes the current decayed evidence score for an entry.
	Score(ctx context.Context, entryID int64) (float64, error)

	// Status derives the status tier from the score.
	Status(ctx context.Context, entryID int64) (types.EvidenceStatus, error)

	// ArchiveSweep marks sub-zero entries for archival. Returns count archived.
	ArchiveSweep(ctx context.Context) (int, error)

	// Protect marks an entry as immune to decay and archival.
	Protect(ctx context.Context, entryID int64) error

	// Unprotect removes the protection flag.
	Unprotect(ctx context.Context, entryID int64) error
}

// ── Evidence Types ──

type EvidenceSignalType string

const (
	SignalUserConfirm   EvidenceSignalType = "user_confirm"
	SignalUserDeny      EvidenceSignalType = "user_deny"
	SignalUserFact      EvidenceSignalType = "user_fact"
	SignalBehaviorMatch EvidenceSignalType = "behavior_match"
	SignalContradiction EvidenceSignalType = "contradiction"
	SignalObservation   EvidenceSignalType = "observation"
)

type EvidenceSignal struct {
	EntryID  int64              `json:"entry_id"`
	Type     EvidenceSignalType `json:"type"`
	Strength float64            `json:"strength"` // -1.0 ~ +1.0
	Source   string             `json:"source"`
}

// ── Memory Retrieval ──

// MemoryRecall provides hybrid memory retrieval combining BM25 + vector + RRF fusion.
// Implementation: adapter/memory/recall.go
type MemoryRecall interface {
	HybridSearch(ctx context.Context, query string, topK int) ([]MemorySearchResult, error)
	VectorSearch(ctx context.Context, vector []float32, topK int) ([]MemorySearchResult, error)
	UnifiedSearch(ctx context.Context, query string, vector []float32, topK int) ([]MemorySearchResult, error)

	// SearchDiaries returns emotionally significant diary entries.
	SearchDiaries(ctx context.Context, query string, topK int) ([]MemorySearchResult, error)

	// SearchBoundaries returns active boundary facts sorted by evidence score.
	SearchBoundaries(ctx context.Context) ([]MemorySearchResult, error)

	// SetMoodBias biases future searches toward mood-congruent facts.
	SetMoodBias(valence float64)
}

// ── Chat Memory Sink ──

// ChatMemorySink receives completed conversation turns and emotion updates.
// ChatOrchestrator calls these after each chat turn so the memory pipeline
// can asynchronously extract facts, apply signals, and generate diaries.
type ChatMemorySink interface {
	OnAfterChat(ctx context.Context, userMsg, response string)
	UpdateEmotionState(valence, arousal float64)
}

type MemorySearchResult struct {
	ID       int64                   `json:"id"`
	Content  string                  `json:"content"`
	Source   string                  `json:"source"` // "fact"|"diary"|"strategy"
	Score    float64                 `json:"score"`
	Evidence *types.EvidenceSnapshot `json:"evidence,omitempty"`
}
