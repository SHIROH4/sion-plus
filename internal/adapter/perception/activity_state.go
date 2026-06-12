package perception

import (
	"strings"
	"time"
)

// ── Activity State ──────────────────────────────────────────────────

type ActivityState string

const (
	StateAway       ActivityState = "away"
	StatePrivate    ActivityState = "private"
	StateGaming     ActivityState = "gaming"
	StateFocused    ActivityState = "focused"
	StateBrowsing   ActivityState = "browsing"
	StateChatting   ActivityState = "chatting"
	StateMeeting    ActivityState = "meeting"
	StateIdle       ActivityState = "idle"
	StateUnknown    ActivityState = "unknown"
)

// ── Propensity ──────────────────────────────────────────────────────

type Propensity string

const (
	PropensityClosed             Propensity = "closed"
	PropensityRestricted         Propensity = "restricted"
	PropensityOpen               Propensity = "open"
	PropensityGreeting           Propensity = "greeting"
)

// ── Tone ────────────────────────────────────────────────────────────

type Tone string

const (
	ToneTerse   Tone = "terse"
	ToneHushed  Tone = "hushed"
	ToneMellow  Tone = "mellow"
	TonePlayful Tone = "playful"
	ToneWarm    Tone = "warm"
	ToneConcise Tone = "concise"
)

// ── ActivitySnapshot ────────────────────────────────────────────────

type ActivitySnapshot struct {
	State       ActivityState `json:"state"`
	Propensity  Propensity    `json:"propensity"`
	Tone        Tone          `json:"tone"`
	AppCategory string        `json:"app_category"`
	AppName     string        `json:"app_name"`
	WindowTitle string        `json:"window_title"`
	IdleSec     float64       `json:"idle_sec"`
	SwitchCount int           `json:"switch_count"`
	FocusMin    float64       `json:"focus_min"` // minutes in focused state
	StaleSec    float64       `json:"stale_sec"`  // seconds since left away state
}

// ── ActivityStateMachine ────────────────────────────────────────────

type ActivityStateMachine struct {
	currentState   ActivityState
	focusStart     time.Time
	lastState      ActivityState
	lastStateSince time.Time
	staleReturning bool
	leftAwayAt     time.Time
}

func NewActivityStateMachine() *ActivityStateMachine {
	return &ActivityStateMachine{
		currentState:   StateUnknown,
		lastStateSince: time.Now(),
	}
}

// Classify determines the activity state from observation data.
// Pure rules — no LLM calls.
func (m *ActivityStateMachine) Classify(
	appCategory string,
	appName string,
	idleSec float64,
	switchCount int,
	_ string, // windowTitle (reserved)
) ActivitySnapshot {
	now := time.Now()

	// Track state duration
	prevState := m.currentState
	if prevState != m.currentState {
		m.lastState = prevState
		m.lastStateSince = now
	}

	// ── Rule 1: Away (idle > 15 min) ──
	if idleSec > 15*60 {
		m.currentState = StateAway
		m.staleReturning = true
		snap := m.buildSnapshot(appCategory, appName, idleSec, switchCount)
		snap.Propensity = PropensityClosed
		snap.Tone = ToneTerse
		return snap
	}

	// If just returned from away, open greeting window
	if m.staleReturning && idleSec < 30 {
		m.staleReturning = false
		m.leftAwayAt = now
		m.currentState = StateIdle
		snap := m.buildSnapshot(appCategory, appName, idleSec, switchCount)
		snap.Propensity = PropensityGreeting
		snap.Tone = ToneWarm
		snap.StaleSec = 0
		return snap
	}

	// Still in greeting window (60s after return)
	if !m.leftAwayAt.IsZero() && now.Sub(m.leftAwayAt) < 60*time.Second && idleSec < 30 {
		m.currentState = StateIdle
		snap := m.buildSnapshot(appCategory, appName, idleSec, switchCount)
		snap.Propensity = PropensityGreeting
		snap.Tone = ToneWarm
		snap.StaleSec = now.Sub(m.leftAwayAt).Seconds()
		return snap
	}
	m.leftAwayAt = time.Time{}

	// ── Rule 2: Private (sensitive apps) ──
	if isPrivateApp(appName) {
		m.currentState = StatePrivate
		snap := m.buildSnapshot(appCategory, appName, idleSec, switchCount)
		snap.Propensity = PropensityClosed
		snap.Tone = ToneTerse
		return snap
	}

	// ── Rule 3: Gaming ──
	if appCategory == "play" && idleSec < 60 {
		m.currentState = StateGaming
		snap := m.buildSnapshot(appCategory, appName, idleSec, switchCount)
		snap.Propensity = PropensityRestricted
		snap.Tone = TonePlayful
		return snap
	}

	// ── Rule 4: Meeting ──
	if appCategory == "social" && isMeetingApp(appName) {
		m.currentState = StateMeeting
		snap := m.buildSnapshot(appCategory, appName, idleSec, switchCount)
		snap.Propensity = PropensityClosed
		snap.Tone = ToneTerse
		return snap
	}

	// ── Rule 5: Chatting ──
	if appCategory == "social" {
		m.currentState = StateChatting
		snap := m.buildSnapshot(appCategory, appName, idleSec, switchCount)
		snap.Propensity = PropensityRestricted
		snap.Tone = ToneWarm
		return snap
	}

	// ── Rule 6: Focused (work app + dwell > 90s) ──
	if appCategory == "work" {
		if m.currentState != StateFocused {
			m.focusStart = now
		}
		dwell := now.Sub(m.focusStart).Seconds()
		if dwell > 90 {
			m.currentState = StateFocused
			snap := m.buildSnapshot(appCategory, appName, idleSec, switchCount)
			snap.Propensity = PropensityRestricted
			snap.Tone = ToneConcise
			snap.FocusMin = dwell / 60
			return snap
		}
	}

	// ── Rule 7: Rapid switching → transitioning → idle ──
	if switchCount >= 5 {
		m.currentState = StateIdle
		snap := m.buildSnapshot(appCategory, appName, idleSec, switchCount)
		snap.Propensity = PropensityOpen
		snap.Tone = ToneMellow
		return snap
	}

	// ── Rule 8: Browsing ──
	if appCategory == "idle" && idleSec < 60 {
		m.currentState = StateBrowsing
		snap := m.buildSnapshot(appCategory, appName, idleSec, switchCount)
		snap.Propensity = PropensityOpen
		snap.Tone = ToneMellow
		return snap
	}

	// ── Rule 9: Default → Idle ──
	m.currentState = StateIdle
	snap := m.buildSnapshot(appCategory, appName, idleSec, switchCount)
	snap.Propensity = PropensityOpen
	snap.Tone = ToneMellow
	return snap
}

func (m *ActivityStateMachine) buildSnapshot(cat, name string, idle float64, sw int) ActivitySnapshot {
	focusMin := float64(0)
	if m.currentState == StateFocused {
		focusMin = time.Since(m.focusStart).Minutes()
	}
	return ActivitySnapshot{
		State:       m.currentState,
		AppCategory: cat,
		AppName:     name,
		IdleSec:     idle,
		SwitchCount: sw,
		FocusMin:    focusMin,
	}
}

// ── Helpers ────────────────────────────────────────────────────────

func isPrivateApp(name string) bool {
	n := strings.ToLower(name)
	priv := []string{"1password", "bitwarden", "keychain", "lastpass", "dashlane",
		"bank", "银行", "paypal", "stripe", "venmo", "cash"}
	for _, p := range priv {
		if strings.Contains(n, p) {
			return true
		}
	}
	return false
}

func isMeetingApp(name string) bool {
	n := strings.ToLower(name)
	meet := []string{"zoom", "teams", "meet", "腾讯会议", "facetime", "webex", "voov"}
	for _, m := range meet {
		if strings.Contains(n, m) {
			return true
		}
	}
	return false
}
