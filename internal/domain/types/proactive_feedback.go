package types

// FeedbackKind is an explicit user signal about a proactive delivery.
// A missing signal remains pending; silence is not treated as rejection.
type FeedbackKind string

const (
	FeedbackHelpful    FeedbackKind = "helpful"
	FeedbackDismiss    FeedbackKind = "dismiss"
	FeedbackIrrelevant FeedbackKind = "irrelevant"
	FeedbackBadTiming  FeedbackKind = "bad_timing"
	FeedbackWrongTone  FeedbackKind = "wrong_tone"
	FeedbackSnooze     FeedbackKind = "snooze"
	FeedbackStop       FeedbackKind = "stop"
)

// ProactiveDecision is the auditable record of one delivered proactive action.
// ContextJSON and CandidatesJSON intentionally preserve the exact policy inputs
// needed to explain and later evaluate a decision without coupling persistence to
// a particular feature schema.
type ProactiveDecision struct {
	DecisionID     string  `json:"decision_id"`
	PolicyVersion  string  `json:"policy_version"`
	Action         string  `json:"action"`
	Source         string  `json:"source"`
	Score          float64 `json:"score"`
	ContextJSON    string  `json:"context_json"`
	CandidatesJSON string  `json:"candidates_json"`
	Content        string  `json:"content"`
	State          string  `json:"state"` // silent|blocked|failed|delivered|resolved|expired
	CreatedAt      int64   `json:"created_at"`
	DeliveredAt    int64   `json:"delivered_at"`
	ResolvedAt     int64   `json:"resolved_at"`
}

// ProactiveReply is a temporal attribution candidate, not a reward. It records
// that a user message followed a proactive delivery within a bounded window;
// explicit feedback remains the only input to policy learning.
type ProactiveReply struct {
	DecisionID  string  `json:"decision_id"`
	Content     string  `json:"content"`
	Attribution string  `json:"attribution"` // next_user_message
	Confidence  float64 `json:"confidence"`
	CreatedAt   int64   `json:"created_at"`
}

// ProactiveFeedback keeps the raw, explicit preference separate from the
// decision. This makes feedback correction and future reward-model changes safe.
type ProactiveFeedback struct {
	ID         int64        `json:"id"`
	EventID    string       `json:"event_id"`
	DecisionID string       `json:"decision_id"`
	Kind       FeedbackKind `json:"kind"`
	Reward     float64      `json:"reward"`
	Source     string       `json:"source"` // ui|api|classifier
	Confidence float64      `json:"confidence"`
	Note       string       `json:"note"`
	CreatedAt  int64        `json:"created_at"`
}

// ProactiveControl is a durable user boundary evaluated before policy scores.
// UntilAt=0 means the control is permanent until explicitly cleared.
type ProactiveControl struct {
	Scope      string `json:"scope"`       // global|category|action
	ScopeValue string `json:"scope_value"` // empty for global
	Mode       string `json:"mode"`        // muted|snoozed
	UntilAt    int64  `json:"until_at"`
	Source     string `json:"source"`
	UpdatedAt  int64  `json:"updated_at"`
}

// ActionFeedbackStats is an aggregated, explicit-feedback-only view used by
// the online policy. It deliberately excludes silence and raw chat messages.
type ActionFeedbackStats struct {
	Action        string  `json:"action"`
	Samples       int     `json:"samples"`
	RewardSum     float64 `json:"reward_sum"`
	HelpfulCount  int     `json:"helpful_count"`
	NegativeCount int     `json:"negative_count"`
}

// ProactivePolicyEvaluation summarizes logged outcomes without treating reply
// candidates as rewards. It supports offline policy review and shadow rollout.
type ProactivePolicyEvaluation struct {
	SinceAt               int64   `json:"since_at"`
	Opportunities         int     `json:"opportunities"`
	Delivered             int     `json:"delivered"`
	Blocked               int     `json:"blocked"`
	Silent                int     `json:"silent"`
	Failed                int     `json:"failed"`
	ExplicitFeedback      int     `json:"explicit_feedback"`
	FeedbackRate          float64 `json:"feedback_rate"`
	AverageReward         float64 `json:"average_reward"`
	NegativeRate          float64 `json:"negative_rate"`
	ReplyCandidates       int     `json:"reply_candidates"`
	ReplyCandidateRate    float64 `json:"reply_candidate_rate"`
	ShadowCompared        int     `json:"shadow_compared"`
	ShadowDifferent       int     `json:"shadow_different"`
	ShadowMatchedFeedback int     `json:"shadow_matched_feedback"`
	ShadowMatchedReward   float64 `json:"shadow_matched_reward"`
}
