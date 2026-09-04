package types

import "time"

// ── Proactive Intent ──

// ProactiveIntent represents a desire for the AI to take initiative.
// Sources can be internal (cognition tick) or external (plugin events).
type ProactiveIntent struct {
	ID          string        `json:"id"`
	DecisionID  string        `json:"decision_id,omitempty"`
	Source      string        `json:"source"`       // "cognition"|"plugin:qq"|"plugin:timer"...
	Action      string        `json:"action"`       // action name from ActionDef
	Message     string        `json:"message"`      // instruction/prompt for LLM
	Priority    int           `json:"priority"`     // 0-10, higher = more urgent
	CoalesceKey string        `json:"coalesce_key"` // empty = never coalesce
	TTL         time.Duration `json:"-"`
	MediaImages []string      `json:"media_images,omitempty"`
	CreatedAt   time.Time     `json:"created_at"`
}

// ── Priority Constants ──
// Lower = less important. External events outrank internal ticks.

const (
	PriorityCritical   = 9 // external urgent (QQ important message, timer fire)
	PriorityHigh       = 7 // external normal (QQ casual message, search result)
	PriorityNormal     = 5 // internal high-score action (social > 0.7)
	PriorityLow        = 3 // internal normal action
	PriorityBackground = 1 // internal low-score (quiet/none actions)
	PriorityNone       = 0 // do not speak
)

// ── Delivery Mode ──

// DeliveryMode controls how a proactive intent reaches the user.
type DeliveryMode string

const (
	DeliveryProactive DeliveryMode = "proactive" // interrupt and speak now
	DeliveryPassive   DeliveryMode = "passive"   // wait for next user turn
	DeliverySilent    DeliveryMode = "silent"    // update state only, no output
)

// ── Care ──

// UserCareState tracks the user's wellbeing for the care engine.
type UserCareState struct {
	ContinuousWorkMin int   `json:"continuous_work_min"`
	LastMealAt        int64 `json:"last_meal_at"` // unix seconds
	LastRestAt        int64 `json:"last_rest_at"`
	LastHydrationAt   int64 `json:"last_hydration_at"`
	FatigueMentions   int   `json:"fatigue_mentions"` // count in last 24h
	StressMentions    int   `json:"stress_mentions"`
	IsActive          bool  `json:"is_active"`
	UpdatedAt         int64 `json:"updated_at"`
}

// CareAction is a care suggestion emitted by the care engine.
type CareAction struct {
	Message  string `json:"message"`
	Source   string `json:"source"` // "rest"|"meal"|"hydration"|"health"|"encourage"
	Priority int    `json:"priority"`
	Observed string `json:"observed"` // what triggered this action
}
