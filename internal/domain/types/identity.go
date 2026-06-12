package types

// ── Identity Node ──

// IdentityNode is a node in the identity knowledge graph.
type IdentityNode struct {
	ID           int64          `json:"id"`
	SchemaVersion int           `json:"schema_version"`
	Label        string         `json:"label"` // "master"|"sion"|"relationship"|"group:xxx"
	Kind         string         `json:"kind"`  // "user"|"character"|"relationship"
	Properties   map[string]any `json:"properties"`
	Embedding    []float32      `json:"embedding,omitempty"`
	Active       bool           `json:"active"`
	CreatedAt    int64          `json:"created_at"`
	UpdatedAt    int64          `json:"updated_at"`
}

// ── Self Model ──

// SelfModel is the AI's evolving self-description.
type SelfModel struct {
	Content   string `json:"content"`
	Version   int    `json:"version"`
	UpdatedAt int64  `json:"updated_at"`
}

// ── Daily Reflection ──

// DailyReflectionInput is the input to the StrategicAgent's reflection.
type DailyReflectionInput struct {
	CurrentSelfModel    string
	InteractionCount    int
	ProactiveAcceptRate float64
	ActivePrinciples    []StrategyPrinciple
	RecentDiaries       []string
	YesterdayFacts      []string
	ActiveThreads       []ConversationThread
	RecentOutcomes      []ActionOutcome
}

// DailyReflectionOutput is the structured output of strategic reflection.
type DailyReflectionOutput struct {
	SelfModelUpdate        string                `json:"self_model_update"`
	NewPrinciples           []StrategyPrinciple   `json:"new_principles"`
	DeactivatePrincipleIDs  []int64               `json:"deactivate_principle_ids"`
	TacticalDirectives      []string              `json:"tactical_directives"`
	ThreadRecommendations   []ThreadRecommendation `json:"thread_recommendations"`
	NarrativeSummary        string                `json:"narrative_summary"`
}

// ThreadRecommendation is a suggestion for thread lifecycle management.
type ThreadRecommendation struct {
	Action       string  `json:"action"` // "create"|"resolve"|"stale"
	Type         string  `json:"type"`   // "follow_up"|"exploration"|"care"|"entertainment"
	Goal         string  `json:"goal,omitempty"`
	BestApproach string  `json:"best_approach,omitempty"`
	Priority     float64 `json:"priority,omitempty"`
	ThreadID     int64   `json:"thread_id,omitempty"`
	Outcome      string  `json:"outcome,omitempty"`
	Learnings    string  `json:"learnings,omitempty"`
}
