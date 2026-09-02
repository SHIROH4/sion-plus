// Package domain contains global constants shared across the project.
// All magic numbers live here — never hardcode thresholds in business logic.
package domain

import "time"

// ── Network ──

const (
	DefaultAPIListenAddr = "127.0.0.1:19840"
	DefaultOllamaURL     = "http://localhost:11434"
)

// ── Session Buffer ──

const (
	DefaultSessionSize   = 20
	DefaultSessionMaxAge = 30 * time.Minute
)

// ── Decision Engine ──

const (
	DefaultBaseInterval     = 5 * time.Minute
	MaxDailyActions         = 20
	RouteGapThreshold       = 0.03  // S1/S2 score gap
	DPStepSize              = 0.003 // RL weight update step
	BatchLearnInterval      = 6 * time.Hour
	BatchLearnMinRecords    = 5
	StrategyReflectInterval = 6 * time.Hour
	CuriosityScanInterval   = 2 * time.Hour
	MaxStoredDrives         = 500 // learner ring buffer
	MinOutcomesForDistill   = 10
)

// ── Proactive Delivery ──

const (
	DefaultMinGapSec       = 2.0
	DefaultInflightTimeout = 12 * time.Second
	DefaultIntentTTL       = 90 * time.Second
	MaxPlaybackSec         = 45.0 // watchdog: force-reset stuck playback
	MaxBatchSize           = 5    // max intents per batch release
)

// ── Rejection & Suppression ──

const (
	RejectionTTL                  = 30 * time.Minute
	ConsecutiveUnansweredSuppress = 2
	ConsecutiveDecayInterval      = 30 * time.Minute
	SuppressMentionLimit          = 2 // mentions within window
	SuppressWindowHours           = 5 // hour window
	SuppressCooldownHours         = 5 // cooldown after suppress
)

// ── Diary & Compression ──

const (
	DiaryTriggerInterval     = 4 * time.Hour
	DiaryEmotionThreshold    = 0.3 // valence change threshold for trigger
	CompressionL0Threshold   = 20  // messages
	CompressionHighThreshold = 3   // L1+ summaries
	CompressionMaxLevel      = 3
	CompressionCountL0       = 5 // fold 5 L0 → 1 L1 summary
	CompressionCountHigh     = 3 // fold 3 Ln → 1 Ln+1 summary
)

// ── Retrieval ──

const (
	BM25Budget               = 10
	CosineBudget             = 10
	RRFK                     = 60.0
	FinalBudget              = 5
	FactVectorDedupThreshold = 0.85 // cosine similarity → merge
)

// ── Reflection & Self-Update ──

const (
	SelfUpdateIntervals1 = 24 * time.Hour
	SelfUpdateIntervals2 = 72 * time.Hour  // 3 days
	SelfUpdateIntervals3 = 168 * time.Hour // 7 days
	SelfUpdateSteady     = 720 * time.Hour // 30 days
	DiaryToFactThreshold = 20              // diaries before L1→L2 consolidation
)

// ── Emotion ──

const (
	EmotionDecayInterval    = 5 * time.Minute
	EmotionHistoryMax       = 100
	EmotionCacheMaxSize     = 64
	EmotionCacheTTL         = 30 * time.Second
	ReunionIdleThreshold    = 60 * time.Minute
	ReunionAffectionBoost   = 0.03
	ReunionPlayfulnessBoost = 0.08
	ReunionLonelinessDrop   = 0.3
	ReunionWorryDrop        = 0.1
)

// ── Screen Observation ──

const (
	ObserveIntervalMin    = 30 * time.Second
	ObserveIntervalMax    = 120 * time.Second
	ScreenshotMaxHeight   = 720
	ScreenshotJPEGQuality = 80
)

// ── Memory Evidence ──

const (
	MaxInitialReinforce    = 1.5  // 单次提取最高初始证据分
	OscillationChanges     = 5    // 信号翻转次数 → 标记振荡
	OscillationSignalMul   = 0.3  // 振荡时信号衰减倍率
	SignalHistoryMax       = 20   // 保留最近N条信号用于振荡检测
	StatePastDays          = 7    // state类默认过期天数
	EpisodePastDays        = 3    // episode类默认过期天数
	FactsForceArchiveCount = 5000 // 超过此数强制归档低分fact
	ForceArchivePercent    = 0.10 // 强制归档比例
)

// ── Memory Retrieval ──

const (
	ExploreQuota          = 0.2 // 检索结果中探索配额比例 (20%随机未召回fact)
	MaxSystemPromptTokens = 2000
)

// ── Memory Background Loops ──

const (
	MaintenanceInterval = 30 * time.Minute
	RebuttalInterval    = 3 * time.Hour
)
