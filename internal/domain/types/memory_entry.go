package types

// ── Evidence Status ──

// EvidenceStatus is the derived credibility tier of a memory entry.
type EvidenceStatus string

const (
	EvPending          EvidenceStatus = "pending"
	EvConfirmed        EvidenceStatus = "confirmed"
	EvPromoted         EvidenceStatus = "promoted"
	EvArchiveCandidate EvidenceStatus = "archive_candidate"
	EvArchived         EvidenceStatus = "archived"
	EvMerged           EvidenceStatus = "merged"
)

// ── Reflection Status ──

// ReflectionStatus tracks the lifecycle of a ReflectionEntry.
type ReflectionStatus string

const (
	ReflPending        ReflectionStatus = "pending"
	ReflConfirmed      ReflectionStatus = "confirmed"
	ReflPromoted       ReflectionStatus = "promoted"
	ReflMerged         ReflectionStatus = "merged"
	ReflDenied         ReflectionStatus = "denied"
	ReflArchived       ReflectionStatus = "archived"
	ReflPromoteBlocked ReflectionStatus = "promote_blocked"
)

// TerminalStatuses returns statuses that are considered terminal (no further transitions).
func TerminalReflectionStatuses() []ReflectionStatus {
	return []ReflectionStatus{ReflPromoted, ReflMerged, ReflDenied, ReflArchived, ReflPromoteBlocked}
}

// CanTransition checks if a reflection can move from oldStatus to newStatus.
func CanTransitionReflection(old, new ReflectionStatus) bool {
	valid := map[ReflectionStatus][]ReflectionStatus{
		ReflPending:        {ReflConfirmed, ReflDenied, ReflArchived, ReflPromoteBlocked},
		ReflConfirmed:      {ReflPromoted, ReflDenied, ReflArchived},
		ReflPromoted:       {ReflMerged},
		ReflPromoteBlocked: {ReflPending}, // can be reset
		ReflDenied:         {ReflArchived},
		ReflArchived:       {}, // terminal
		ReflMerged:         {}, // terminal
	}
	for _, allowed := range valid[old] {
		if new == allowed {
			return true
		}
	}
	return false
}

// ── Event Log ──

// EventLogEntry is an append-only audit record.
type EventLogEntry struct {
	ID        int64  `json:"id"`
	Type      string `json:"type"`      // "fact.added"|"fact.signal_applied"|"fact.archived"|...
	EntityID  int64  `json:"entity_id"` // the affected fact/reflection/diary ID
	EntityKind string `json:"entity_kind"` // "fact"|"reflection"|"diary"|"strategy"
	Payload   string `json:"payload"`   // JSON blob with event details
	CreatedAt int64  `json:"created_at"`
}

// Event type constants.
const (
	EvtFactAdded           = "fact.added"
	EvtFactSignalApplied   = "fact.signal_applied"
	EvtFactArchived        = "fact.archived"
	EvtFactAbsorbed        = "fact.absorbed"
	EvtReflectionSynthesized = "reflection.synthesized"
	EvtReflectionStateChanged = "reflection.state_changed"
	EvtEvidenceUpdated     = "evidence.updated"
)

// ── Evidence Config ──

// EvidenceConfig holds all tunable parameters for the evidence system.
// Shared between domain (calculation) and port (configuration).
type EvidenceConfig struct {
	ReinHalfLifeDays   float64 `json:"rein_half_life_days" yaml:"rein_half_life_days"`
	DispHalfLifeDays   float64 `json:"disp_half_life_days" yaml:"disp_half_life_days"`
	ConfirmedThreshold float64 `json:"confirmed_threshold" yaml:"confirmed_threshold"`
	PromotedThreshold  float64 `json:"promoted_threshold" yaml:"promoted_threshold"`
	ArchiveThreshold   float64 `json:"archive_threshold" yaml:"archive_threshold"`
	ComboThreshold     int     `json:"combo_threshold" yaml:"combo_threshold"`
	ComboBonus         float64 `json:"combo_bonus" yaml:"combo_bonus"`
	BaseReinDelta      float64 `json:"base_rein_delta" yaml:"base_rein_delta"`
}

// ── Evidence Snapshot ──

// EvidenceSnapshot is the computed result after applying an evidence signal.
// Used by both port.EvidenceEngine (return type) and domain/memory (computation).
type EvidenceSnapshot struct {
	Reinforcement float64 `json:"reinforcement"`
	Disputation   float64 `json:"disputation"`
	EvidenceScore float64 `json:"evidence_score"`
	Status        string  `json:"status"` // "pending"|"confirmed"|"promoted"|"archive_candidate"
	ComboCount    int     `json:"combo_count"`
}

// ── Schema Version ──

// SchemaVersionCurrent is the current data schema version.
// Bump when making breaking changes to any persisted struct.
const SchemaVersionCurrent = 1

// ── Memory Evidence ──

// MemoryEvidenceEntry tracks the credibility of a single memory entry.
// Uses dual-signal half-life decay: reinforcement and disputation
// independently decay over time, and evidence_score = rein - disp.
// Protected entries have score = +inf and never archive.
type MemoryEvidenceEntry struct {
	Reinforcement        float64                `json:"reinforcement"`
	Disputation          float64                `json:"disputation"`
	ReinLastSignalAt     int64                  `json:"rein_last_signal_at"`     // unix seconds
	DispLastSignalAt     int64                  `json:"disp_last_signal_at"`     // unix seconds
	ReinComboCount       int                    `json:"rein_combo_count"`
	Protected            bool                   `json:"protected"`
	SubZeroDays          int                    `json:"sub_zero_days"`
	SubZeroLastIncrDate  string                 `json:"sub_zero_last_incr_date"` // "2006-01-02"
	SignalHistory        []EvidenceSignalRecord `json:"signal_history,omitempty"` // 最近N条信号, 用于振荡检测
}

// EvidenceSignalRecord stores a single evidence signal for oscillation detection.
type EvidenceSignalRecord struct {
	Type      string  `json:"type"`      // "reinforce"|"contradict"
	Delta     float64 `json:"delta"`
	Timestamp int64   `json:"timestamp"`
}

// ── FactEntry (L2 Semantic Memory) ──

// FactEntry is a structured atomic fact about the user, the AI, or their relationship.
// Each fact is typed by Entity+RelationType (see ontology.go) and carries evidence
// tracking for credibility management.
type FactEntry struct {
	ID            int64  `json:"id"`
	SchemaVersion int    `json:"schema_version"` // SchemaVersionCurrent
	Entity        string `json:"entity"`         // "master" | "neko" | "relationship"
	RelationType  string `json:"relation_type"`  // see ontology.go
	Content       string `json:"content"`

	// ── 来源层级 (新) ──
	SourceTier       FactSourceTier `json:"source_tier"`       // explicit|observed|inferred
	TemporalScope    TemporalScope  `json:"temporal_scope"`    // pattern|state|episode
	AutoExpireDays   int            `json:"auto_expire_days"`  // state类自动失效天数, 0=永不过时
	ObservationCount int            `json:"observation_count"`  // observed类累计观察次数

	// ── 重要性 ──
	Importance int `json:"importance"` // 1-10

	// ── 证据 ──
	Evidence    MemoryEvidenceEntry `json:"evidence"`
	Oscillating bool                `json:"oscillating"` // 振荡检测标记

	// ── 疲劳控制 (新) ──
	SuppressUntil int64 `json:"suppress_until,omitempty"` // 抑制截止时间(unix秒)
	MentionCount  int   `json:"mention_count"`            // 当前窗口内注入次数

	// ── 向量 ──
	Vector []float32 `json:"vector,omitempty"`

	// ── 召回统计 ──
	RecallCount    int   `json:"recall_count"`
	LastRecalledAt int64 `json:"last_recalled_at"`

	// ── 上下文标签 (Phase 6+) ──
	ContextTags []string `json:"context_tags,omitempty"` // "work"|"play"|"social"

	// ── 元数据 ──
	Source      string `json:"source"`       // "chat"|"reflection"|"warm_start"|"user"|"perception"
	MemCellType string `json:"memcell_type"` // fact|prefer|event|emotion|skill|relation
	EpisodeID   int64  `json:"episode_id,omitempty"`
	Archived    bool   `json:"archived"`

	// ── v2.1: SignalDetection drain + absorption tracking ──
	SignalProcessed bool `json:"signal_processed"` // false=未经过Stage-2信号检测
	Absorbed        bool `json:"absorbed"`          // true=已被reflection消费

	// ── v2.1: Embedding cache ──
	EmbeddingTextSHA256 string `json:"embedding_text_sha256,omitempty"`
	EmbeddingModelID    string `json:"embedding_model_id,omitempty"`

	// ── v2.1: Event timing ──
	EventStartAt int64 `json:"event_start_at,omitempty"` // 事件发生时间(非录入时间)
	EventEndAt   int64 `json:"event_end_at,omitempty"`

	CreatedAt int64 `json:"created_at"`
	UpdatedAt int64 `json:"updated_at"`
}

// ── ReflectionEntry (L1.5: fact synthesis → pending → confirmed → promoted) ──

// ReflectionEntry is a synthesized insight from multiple facts.
// It starts as "pending", gains evidence through SignalDetection,
// and is promoted to persona when confirmed.
type ReflectionEntry struct {
	ID            int64  `json:"id"`
	SchemaVersion int    `json:"schema_version"`
	Text          string `json:"text"`
	Entity        string `json:"entity"`
	RelationType  string `json:"relation_type,omitempty"`
	Status        string `json:"status"` // pending|confirmed|denied|promoted|archived|merged

	// ── Back-link to source facts ──
	SourceFactIDs []int64 `json:"source_fact_ids"`

	// ── Evidence tracking ──
	Reinforcement     float64 `json:"reinforcement"`
	ReinLastSignalAt  string  `json:"rein_last_signal_at,omitempty"`
	Disputation       float64 `json:"disputation"`
	DispLastSignalAt  string  `json:"disp_last_signal_at,omitempty"`

	// ── State machine ──
	Feedback       string `json:"feedback,omitempty"`
	NextEligibleAt string `json:"next_eligible_at,omitempty"` // cooldown end ISO8601
	AbsorbedInto   int64  `json:"absorbed_into,omitempty"`    // persona entry ID after promotion

	// ── Fatigue ──
	Suppress     bool `json:"suppress"`
	MentionCount int  `json:"mention_count"`

	// ── Vector ──
	Vector              []float32 `json:"vector,omitempty"`
	EmbeddingTextSHA256 string    `json:"embedding_text_sha256,omitempty"`
	EmbeddingModelID    string    `json:"embedding_model_id,omitempty"`

	CreatedAt int64 `json:"created_at"`
	UpdatedAt int64 `json:"updated_at"`
}

// ── Fact Source Tier ──

type FactSourceTier string

const (
	SourceExplicit FactSourceTier = "explicit" // 用户亲口说, baseRein=0.50
	SourceObserved FactSourceTier = "observed" // 行为观察, baseRein=0.25
	SourceInferred FactSourceTier = "inferred" // LLM推论, baseRein=0.15
)

// ── Temporal Scope ──

type TemporalScope string

const (
	ScopePattern TemporalScope = "pattern" // 持续模式, 永不过时
	ScopeState   TemporalScope = "state"   // 当前状态, AutoExpireDays后自动过时
	ScopeEpisode TemporalScope = "episode" // 一次性事件, 3天后过时
)

// ── DiaryEntry (L1 Episodic Memory) ──

// DiaryEntry is a timestamped episodic memory entry generated periodically
// or on emotional spikes. Stores an LLM-generated title, summary, and vector.
type DiaryEntry struct {
	ID              int64     `json:"id"`
	SchemaVersion   int       `json:"schema_version"`
	Title           string    `json:"title"`
	Summary         string    `json:"summary"`
	EmotionValence  float64   `json:"emotion_valence"`
	EmotionArousal  float64   `json:"emotion_arousal"`
	EmotionPrimary  string    `json:"emotion_primary"`
	Vector          []float32 `json:"vector,omitempty"`
	TopicID         int64     `json:"topic_id,omitempty"`
	Abstracted      bool      `json:"abstracted"`       // 已被L1→L2抽象
	Archived        bool      `json:"archived"`
	CreatedAt       int64     `json:"created_at"`
}

// ── StrategyPrinciple (L3 Strategic Memory) ──

// StrategyPrinciple is a distilled behavioral strategy from past interactions.
type StrategyPrinciple struct {
	ID            int64     `json:"id"`
	SchemaVersion int       `json:"schema_version"`
	Situation     string    `json:"situation"`
	GoodStrategy  string    `json:"good_strategy"`
	BadStrategy   string    `json:"bad_strategy"`
	Reason        string    `json:"reason"`
	Confidence    float64   `json:"confidence"`   // 0~1
	Source        string    `json:"source"`        // "daily_reflection"|"auto-distill"|"merged"
	Vector        []float32 `json:"vector,omitempty"`
	Active        bool      `json:"active"`
	CreatedAt     int64     `json:"created_at"`
	UpdatedAt     int64     `json:"updated_at"`
}

// ── Episode ──

// Episode groups related facts and conversations around a topic.
type Episode struct {
	ID        int64  `json:"id"`
	Topic     string `json:"topic"`
	Summary   string `json:"summary"`
	Status    string `json:"status"` // "active" | "closed"
	CreatedAt int64  `json:"created_at"`
	UpdatedAt int64  `json:"updated_at"`
}

// ── Topic ──

// Topic is a semantic cluster of conversations with a centroid vector.
type Topic struct {
	ID       int64     `json:"id"`
	Name     string    `json:"name"`
	Centroid []float32 `json:"centroid,omitempty"`
	Count    int       `json:"count"`
}

// ── ConversationThread ──

// ConversationThread tracks an ongoing multi-turn topic.
type ConversationThread struct {
	ID           int64   `json:"id"`
	SchemaVersion int    `json:"schema_version"`
	Type         string  `json:"type"` // "follow_up"|"exploration"|"care"|"entertainment"
	Goal         string  `json:"goal"`
	Status       string  `json:"status"`        // "active"|"stale"|"resolved"
	Priority     float64 `json:"priority"`      // 0~1
	BestApproach string  `json:"best_approach"`
	Outcome      string  `json:"outcome,omitempty"`
	Learnings    string  `json:"learnings,omitempty"`
	CreatedAt    int64   `json:"created_at"`
	UpdatedAt    int64   `json:"updated_at"`
}
