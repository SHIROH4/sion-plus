package types

// ── 52-Dimension Quantified Features ──
//
// This is the complete state vector for the decision engine.
// It is organized into five groups:
//   A (Agent, 15 dims) — the AI's own state
//   U (User, 14 dims)  — what the user is doing
//   E (Environment, 7 dims) — time, quota, cooldown
//   R (Relationship, 10 dims) — how the user treats the AI
//   T (Task Context, 3 dims) — what the AI is working on

type QuantifiedFeatures struct {
	// === A组: Agent自身状态 ===
	A1_1_Affection        float64 `json:"a1_1_affection"`
	A1_2_Worry            float64 `json:"a1_2_worry"`
	A1_3_Curiosity        float64 `json:"a1_3_curiosity"`
	A1_4_Sleepiness       float64 `json:"a1_4_sleepiness"`
	A1_5_Playfulness      float64 `json:"a1_5_playfulness"`
	A1_6_Loneliness       float64 `json:"a1_6_loneliness"`
	A1_7_Confidence       float64 `json:"a1_7_confidence"`
	A1_8_Annoyance        float64 `json:"a1_8_annoyance"`
	A2_PrimaryEmotion     string  `json:"a2_primary_emotion"`
	A3_Intensity          float64 `json:"a3_intensity"`
	A4_ValenceTrend       float64 `json:"a4_valence_trend"`
	A5_1_AnnoySensitivity float64 `json:"a5_1_annoy_sensitivity"`
	A5_2_AffectWarmth     float64 `json:"a5_2_affect_warmth"`
	A5_3_WorryTendency    float64 `json:"a5_3_worry_tendency"`
	A6_DailyActionCount   int     `json:"a6_daily_action_count"`
	A14_ConsecutiveCount  int     `json:"a14_consecutive_count"`
	A11_ActiveInquiries   int     `json:"a11_active_inquiries"`
	A12_KnowledgeGaps     int     `json:"a12_knowledge_gaps"`
	A13_LearningMomentum  float64 `json:"a13_learning_momentum"`

	// === U组: User状态 ===
	U1_AppCategory       string  `json:"u1_app_category"`
	U2_WindowSubtype     string  `json:"u2_window_subtype"`
	U3_IsWorking         float64 `json:"u3_is_working"`
	U4_ContinuousWorkMin float64 `json:"u4_continuous_work_min"`
	U5_AppSwitchCount    float64 `json:"u5_app_switch_count"`
	U7_LengthTrend       float64 `json:"u7_length_trend"`
	U8_EngagementNorm    float64 `json:"u8_engagement_norm"`
	U10_TimeWindowPref   float64 `json:"u10_time_window_pref"`
	U11_MealTime         float64 `json:"u11_meal_time"`
	U12_NightTime        float64 `json:"u12_night_time"`
	U13_IsWeekend        float64 `json:"u13_is_weekend"`
	U14_TimeSinceChatMin float64 `json:"u14_time_since_chat_min"`
	U15_FatigueMentionHr float64 `json:"u15_fatigue_mention_hr"`
	U16_PrefDiversity    float64 `json:"u16_pref_diversity"`

	// === E组: Environment ===
	E1_Hour             int     `json:"e1_hour"`
	E2_DayOfWeek        int     `json:"e2_day_of_week"`
	E3_CooldownNorm     float64 `json:"e3_cooldown_norm"`
	E4_QuotaRemaining   int     `json:"e4_quota_remaining"`
	E5_MinSinceDecision float64 `json:"e5_min_since_decision"`
	E6_LLMAvailable     bool    `json:"e6_llm_available"`
	E7_ReflectionDue    float64 `json:"e7_reflection_due"`

	// === R组: Relationship ===
	R1_OverallAcceptRate float64            `json:"r1_overall_accept_rate"`
	R1_SampleCount       float64            `json:"r1_sample_count"`
	R2_TimeWindowAccept  float64            `json:"r2_time_window_accept"`
	R3_SourceAcceptRate  map[string]float64 `json:"r3_source_accept_rate"`
	R4_RecentRejections  float64            `json:"r4_recent_rejections"`
	R4_RejectionSeverity float64            `json:"r4_rejection_severity"`
	R5_NeglectHours      float64            `json:"r5_neglect_hours"`
	R6_DepthTrend        float64            `json:"r6_depth_trend"`
	R7_UserInitiative24h float64            `json:"r7_user_initiative_24h"`
	R8_IntimacyTrend     float64            `json:"r8_intimacy_trend"`

	// === T组: Task Context ===
	T1_PrincipleCount     float64 `json:"t1_principle_count"`
	T2_PatternCount       float64 `json:"t2_pattern_count"`
	T3_ReflexionLogCount  float64 `json:"t3_reflexion_log_count"`
	T5_TodayActivityCount float64 `json:"t5_today_activity_count"`

	ComputedAt int64 `json:"computed_at"` // unix seconds
}

// ── Cognition State (unified input for feature computation) ──

// CognitionState is the complete input snapshot consumed by FeatureComputer.
// Built by the adapter/cognition layer from current emotion, screen observation,
// and outcome history. Passed to both Tier 1 (pure memory) and Tier 2 (SQL-backed)
// feature computation.
type CognitionState struct {
	// Core state
	Emotion     EmotionState
	EmotionVec  EmotionVector
	Personality PersonalityScale
	Needs       IntrinsicNeeds
	CareState   UserCareState

	// Screen observation (nil if unavailable)
	ScreenObs *ScreenObsInput

	// User activity
	IsWorking      bool
	WorkStartAt    int64
	AppSwitchCount int

	// Conversation context
	LengthTrend    float64
	EngagementNorm float64

	// Timestamps
	LastChatAt       int64
	LastActionAt     int64
	LastDecisionAt   int64
	LastReflectionAt int64

	// Counters
	ActionCount       int
	ConsecutiveAction string
	ConsecutiveCount  int
	QuotaRemaining    int

	// Derived signals
	FatigueMentionHours   float64
	HistoryAverageValence float64 // EMA of past valence values, for A4_ValenceTrend
	PrefDiversity         int
	LLMAvailable          bool

	// Relationship stats
	OverallAcceptRate float64
	AcceptSampleCount int
	TimeWindowAccept  float64
	SourceAcceptRate  map[string]float64
	RecentRejections  int
	DepthTrend        float64
	UserInitiative24h int
	IntimacyTrend     float64

	// Task stats
	ActivePrincipleCount int
	ActivePatternCount   int
	ReflexionLogCount    int
}

// ScreenObsInput is a lightweight screen observation for feature computation.
// Only the category is needed — raw screenshot bytes are handled separately.
type ScreenObsInput struct {
	AppCategory string
}

// ── 5-Dimension Drive Vector ──

// DriveVector is the output of ComputeDrives: five internal motivational drives
// that determine which action the AI should take.
type DriveVector struct {
	Social  float64 `json:"social"`  // [0,1] — desire to interact
	Care    float64 `json:"care"`    // [0,1] — desire to nurture
	Curious float64 `json:"curious"` // [0,1] — desire to learn
	Quiet   float64 `json:"quiet"`   // [0,1] — desire to stay silent
	Explore float64 `json:"explore"` // [0,1] — desire to discover
}

// Zero returns true if all drives are at zero.
func (d DriveVector) Zero() bool {
	return d.Social == 0 && d.Care == 0 && d.Curious == 0 && d.Quiet == 0 && d.Explore == 0
}

// ── 16 Actions ──

// ActionDef defines a single behavior the AI can choose to perform.
// Each action has a weight vector for drive dot-product scoring and
// a SkillCard that explains the action to the LLM (for System 2 fallback).
type ActionDef struct {
	Name        string `json:"name"`
	Category    string `json:"category"` // "social"|"care"|"learning"|"none"
	NightSafe   bool   `json:"night_safe"`
	OutcomeType string `json:"outcome_type"` // "speak"|"action"|"silent"
	Source      string `json:"source"`       // "proactive"|"reactive"

	WeightSocial  float64 `json:"weight_social"`
	WeightCare    float64 `json:"weight_care"`
	WeightCurious float64 `json:"weight_curious"`
	WeightQuiet   float64 `json:"weight_quiet"`
	WeightExplore float64 `json:"weight_explore"`

	SkillCard SkillCard `json:"skill_card"`
}

// SkillCard describes an action to the LLM for System 2 fallback decisions.
type SkillCard struct {
	Trigger  string `json:"trigger"`  // "When to use this action"
	Action   string `json:"action"`   // "What to do"
	Delivery string `json:"delivery"` // "How to present it"
}

// ScoredAction is an action with its computed scores.
type ScoredAction struct {
	Action     ActionDef          `json:"action"`
	RawScore   float64            `json:"raw_score"`
	FinalScore float64            `json:"final_score"`
	Modulators map[string]float64 `json:"modulators"` // multiplier breakdown
}

// ── Decision Result ──

// DecisionResult is the output of the System1/System2 decision router.
type DecisionResult struct {
	FastPath  bool          `json:"fast_path"`
	Action    *ScoredAction `json:"action,omitempty"`
	Reason    string        `json:"reason"`
	LLMPrompt string        `json:"llm_prompt,omitempty"` // only when FastPath=false
}
