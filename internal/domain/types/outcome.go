package types

// OutcomeResult classifies the user's response to a proactive action.
type OutcomeResult int

const (
	OutcomePending  OutcomeResult = 0  // no response yet
	OutcomeReplied  OutcomeResult = 1  // user replied
	OutcomeEngaged  OutcomeResult = 2  // user engaged positively (multi-turn)
	OutcomeIgnored  OutcomeResult = 3  // user saw but ignored
	OutcomeRejected OutcomeResult = -1 // user explicitly rejected / told to stop
)

// ActionOutcome records a proactive action and its outcome.
type ActionOutcome struct {
	ID            int64         `json:"id"`
	SchemaVersion int           `json:"schema_version"`
	ActionSource  string        `json:"action_source"`
	ActionType    string        `json:"action_type"`
	HourOfDay     int           `json:"hour_of_day"`
	DayOfWeek     int           `json:"day_of_week"`
	AppContext    string        `json:"app_context"`
	EmotionBucket string        `json:"emotion_bucket"`
	EscalationLvl int           `json:"escalation_lvl"`
	Outcome       OutcomeResult `json:"outcome"`
	ResponseDelay int           `json:"response_delay_sec"`
	CreatedAt     int64         `json:"created_at"`
}

// DriveRecord stores drives at decision time for later DPO batch learning.
type DriveRecord struct {
	ID      int
	Action  string
	Social  float64
	Care    float64
	Curious float64
	Quiet   float64
	Explore float64
	Reward  float64 // +1 accepted, 0 ignored, -1 rejected
	At      int64
}
