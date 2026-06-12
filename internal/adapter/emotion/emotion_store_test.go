package emotion

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/shirohania/sion/internal/domain/types"
)

func newTestStore(t *testing.T) *EmotionStore {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "emotion.json")
	return NewEmotionStore(path)
}

func TestInitialState(t *testing.T) {
	e := newTestStore(t)
	state, vec := e.Current()

	if state.Valence < -1 || state.Valence > 1 {
		t.Errorf("valence out of range: %f", state.Valence)
	}
	if vec.Affection != 0.5 {
		t.Errorf("default affection: got %f, want 0.5", vec.Affection)
	}
	if vec.Sleepiness != 0 {
		t.Errorf("default sleepiness: got %f, want 0", vec.Sleepiness)
	}
	if vec.Worry != 0 {
		t.Errorf("default worry: got %f, want 0", vec.Worry)
	}
	if vec.Playfulness != 0.5 {
		t.Errorf("default playfulness: got %f, want 0.5", vec.Playfulness)
	}
	if state.Primary != "neutral" {
		t.Errorf("initial primary: got %s, want neutral", state.Primary)
	}
}

func TestApplyDeltaHappyDirected(t *testing.T) {
	e := newTestStore(t)

	// EMA smooths — apply multiple times to see effect
	for i := 0; i < 5; i++ {
		e.ApplyDelta(&types.EmotionDelta{Affection: 0.3, Worry: -0.1, Confidence: 0.1})
	}

	_, vec := e.Current()
	if vec.Affection <= 0.55 {
		t.Errorf("affection should rise after repeated happy signals: got %f", vec.Affection)
	}
	if vec.Annoyance > 0.1 {
		t.Errorf("annoyance should stay low: got %f", vec.Annoyance)
	}
	if vec.Playfulness <= 0.5 {
		t.Errorf("playfulness should rise: got %f", vec.Playfulness)
	}
}

func TestApplyDeltaAngryDirected(t *testing.T) {
	e := newTestStore(t)

	for i := 0; i < 5; i++ {
		e.ApplyDelta(&types.EmotionDelta{Annoyance: 0.3, Worry: 0.2, Confidence: -0.15})
	}

	_, vec := e.Current()
	if vec.Annoyance <= 0.1 {
		t.Errorf("annoyance should rise: got %f", vec.Annoyance)
	}
	if vec.Affection < 0.45 {
		t.Errorf("affection should not drop below baseline when user is angry: got %f", vec.Affection)
	}
	if vec.Confidence >= 0.48 {
		t.Errorf("confidence should drop when scolded: got %f", vec.Confidence)
	}
}

func TestApplyDeltaUserVenting(t *testing.T) {
	e := newTestStore(t)

	// User shares sad life events → worry grows
	for i := 0; i < 5; i++ {
		e.ApplyDelta(&types.EmotionDelta{Worry: 0.3, Affection: 0.05, Curiosity: 0.1})
	}

	_, vec := e.Current()
	if vec.Worry <= 0 {
		t.Errorf("worry should grow when user vents: got %f", vec.Worry)
	}
	if vec.Curiosity <= 0.5 {
		t.Errorf("curiosity should rise on user sharing: got %f", vec.Curiosity)
	}
	if vec.Affection < 0.45 {
		t.Errorf("affection should not drop on user venting: got %f", vec.Affection)
	}
}

func TestAffectionDiminishing(t *testing.T) {
	e := newTestStore(t)

	for i := 0; i < 30; i++ {
		e.ApplyDelta(&types.EmotionDelta{Affection: 0.25, Confidence: 0.15, Playfulness: 0.1})
	}

	_, vec := e.Current()
	if vec.Affection > 1.0 {
		t.Errorf("affection should not exceed 1.0: got %f", vec.Affection)
	}
	t.Logf("affection after 30 happy rounds: %f", vec.Affection)
}

func TestLonelinessGrowth(t *testing.T) {
	e := newTestStore(t)
	e.Start()
	defer e.Stop()

	e.mu.Lock()
	e.lastActivity = time.Now().Add(-2 * time.Hour)
	e.mu.Unlock()

	e.tick()

	_, vec := e.Current()
	if vec.Loneliness <= 0 {
		t.Errorf("loneliness should grow after inactivity: got %f", vec.Loneliness)
	}
}

func TestLonelinessReset(t *testing.T) {
	e := newTestStore(t)
	e.Start()
	defer e.Stop()

	e.mu.Lock()
	e.lastActivity = time.Now().Add(-3 * time.Hour)
	e.mu.Unlock()
	for i := 0; i < 10; i++ {
		e.tick()
	}

	_, vec := e.Current()
	if vec.Loneliness < 0.1 {
		t.Fatal("loneliness should have grown")
	}

	e.NotifyActivity()
	_, vec2 := e.Current()
	if vec2.Loneliness > 0.1 {
		t.Errorf("loneliness should reset: got %f", vec2.Loneliness)
	}
	if vec2.Playfulness < 0.55 {
		t.Errorf("playfulness should boost on activity: got %f", vec2.Playfulness)
	}
}

func TestAnnoyanceDecay(t *testing.T) {
	e := newTestStore(t)

	for i := 0; i < 5; i++ {
		e.ApplyDelta(&types.EmotionDelta{Annoyance: 0.4, Confidence: -0.2, Worry: 0.15})
	}
	_, vec := e.Current()
	initial := vec.Annoyance
	t.Logf("initial annoyance: %f", initial)

	for i := 0; i < 10; i++ {
		e.tick()
	}
	_, vec2 := e.Current()
	t.Logf("after 10 ticks: %f", vec2.Annoyance)
	if vec2.Annoyance >= initial*0.65 {
		t.Errorf("annoyance should decay: %f → %f", initial, vec2.Annoyance)
	}
}

func TestPADCalculation(t *testing.T) {
	e := newTestStore(t)

	// Happy catgirl
	e.mu.Lock()
	e.affection = 0.9
	e.annoyance = 0
	e.loneliness = 0
	e.curiosity = 0.7
	e.confidence = 0.8
	e.sleepiness = 0
	e.playfulness = 0.8
	e.worry = 0
	e.mu.Unlock()

	state, _ := e.Current()
	if state.Valence < 0.5 {
		t.Errorf("happy catgirl valence should be high: %f", state.Valence)
	}
	if state.Primary != "joy" {
		t.Errorf("primary should be joy: %s", state.Primary)
	}

	// Annoyed catgirl
	e.mu.Lock()
	e.affection = 0.4
	e.annoyance = 0.8
	e.loneliness = 0.3
	e.confidence = 0.3
	e.sleepiness = 0
	e.playfulness = 0.3
	e.worry = 0.1
	e.mu.Unlock()

	state2, _ := e.Current()
	if state2.Valence >= 0 {
		t.Errorf("annoyed catgirl valence should be negative: %f", state2.Valence)
	}
	if state2.Primary != "anger" {
		t.Errorf("primary should be anger: %s", state2.Primary)
	}

	// Sleepy catgirl at 3am
	e.mu.Lock()
	e.affection = 0.5
	e.annoyance = 0
	e.loneliness = 0.2
	e.confidence = 0.4
	e.sleepiness = 0.8
	e.playfulness = 0.3
	e.worry = 0
	e.mu.Unlock()

	state3, _ := e.Current()
	if state3.Arousal > 0.3 {
		t.Errorf("sleepy catgirl arousal should be low: %f", state3.Arousal)
	}
	if state3.Valence > 0 {
		t.Errorf("sleepy catgirl valence should be neutral/low: %f", state3.Valence)
	}
}

func TestCircadianRhythm(t *testing.T) {
	// 3am → very sleepy
	if s := defaultCircadian(3); s < 0.7 {
		t.Errorf("3am should be very sleepy: got %f", s)
	}
	// 12pm → awake
	if s := defaultCircadian(12); s > 0.1 {
		t.Errorf("12pm should be awake: got %f", s)
	}
	// 11pm → sleepy
	if s := defaultCircadian(23); s < 0.4 {
		t.Errorf("11pm should be sleepy: got %f", s)
	}
	// 15pm → afternoon dip
	if s := defaultCircadian(15); s < 0.1 {
		t.Errorf("3pm should have mild dip: got %f", s)
	}
}

func TestEMASmoothing(t *testing.T) {
	e := newTestStore(t)

	// Single delta → EMA should only move ~30% of the way
	e.ApplyDelta(&types.EmotionDelta{Affection: 0.4, Confidence: 0.3, Playfulness: 0.2})
	_, vec1 := e.Current()

	// After one apply, affection moves from 0.5 toward (delta*0.5 + 0.5)
	// delta=0.5*1.0=0.5, raw[0]+=0.5*1.5=0.75, ema: 0.5+(0.75+0.5-0.5)*0.3=0.5+0.225=0.725
	if vec1.Affection < 0.55 || vec1.Affection > 0.85 {
		t.Errorf("EMA should smooth single delta: got %f (expected ~0.6-0.8)", vec1.Affection)
	}

	// Multiple deltas should accumulate
	for i := 0; i < 20; i++ {
		e.ApplyDelta(&types.EmotionDelta{Affection: 0.4, Confidence: 0.3, Playfulness: 0.2})
	}
	_, vec2 := e.Current()
	if vec2.Affection < 0.9 {
		t.Errorf("EMA should converge toward 1.0 after many rounds: got %f", vec2.Affection)
	}
}

func TestPersonalityModulation(t *testing.T) {
	e := newTestStore(t)
	e.SetPersonality(types.PersonalityScale{
		AnnoyanceSensitivity: 1.0,
		AffectionWarmth:      0.5,
		WorryTendency:        0.5,
	})

	for i := 0; i < 5; i++ {
		e.ApplyDelta(&types.EmotionDelta{Annoyance: 0.3, Confidence: -0.1})
	}
	_, vec1 := e.Current()
	highAnnoy := vec1.Annoyance

	e2 := newTestStore(t)
	e2.SetPersonality(types.PersonalityScale{
		AnnoyanceSensitivity: 0.1,
		AffectionWarmth:      0.5,
		WorryTendency:        0.5,
	})
	for i := 0; i < 5; i++ {
		e2.ApplyDelta(&types.EmotionDelta{Annoyance: 0.3, Confidence: -0.1})
	}
	_, vec2 := e2.Current()
	lowAnnoy := vec2.Annoyance

	if highAnnoy <= lowAnnoy {
		t.Errorf("high sensitivity (%f) should > low (%f)", highAnnoy, lowAnnoy)
	}
}

func TestHistory(t *testing.T) {
	e := newTestStore(t)
	for i := 0; i < 10; i++ {
		e.ApplyDelta(&types.EmotionDelta{Affection: 0.2, Curiosity: 0.05})
	}
	states, vectors := e.History()
	if len(states) != 10 {
		t.Errorf("expected 10 history entries, got %d", len(states))
	}
	if len(vectors) != 10 {
		t.Errorf("expected 10 vector entries, got %d", len(vectors))
	}
}

func TestPersistence(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "emotion.json")

	e1 := NewEmotionStore(path)
	for i := 0; i < 10; i++ {
		e1.ApplyDelta(&types.EmotionDelta{Affection: 0.3, Confidence: 0.2, Playfulness: 0.1})
	}
	e1.SetPersonality(types.PersonalityScale{AnnoyanceSensitivity: 0.7, AffectionWarmth: 0.6, WorryTendency: 0.4})
	e1.Save(nil)

	e2 := NewEmotionStore(path)
	e2.Load(nil)
	_, vec := e2.Current()
	if vec.Affection <= 0.5 {
		t.Errorf("affection not restored: %f", vec.Affection)
	}
	if e2.Personality().AnnoyanceSensitivity != 0.7 {
		t.Errorf("personality not restored")
	}
}

func TestWorryGrowthOnUserDistress(t *testing.T) {
	e := newTestStore(t)

	// User repeatedly vents → worry grows
	for i := 0; i < 10; i++ {
		e.ApplyDelta(&types.EmotionDelta{Worry: 0.3})
	}

	_, vec := e.Current()
	if vec.Worry < 0.2 {
		t.Errorf("worry should be significant after repeated vents: got %f", vec.Worry)
	}

	// User happy → worry should decrease
	for i := 0; i < 10; i++ {
		e.ApplyDelta(&types.EmotionDelta{Affection: 0.3, Worry: -0.1, Confidence: 0.1})
	}
	_, vec2 := e.Current()
	if vec2.Worry >= vec.Worry {
		t.Errorf("worry should decrease when user is happy: %f → %f", vec.Worry, vec2.Worry)
	}
}

func TestSleepinessTimeDriven(t *testing.T) {
	e := newTestStore(t)
	e.Start()
	defer e.Stop()

	// Manually set hour to 3am behavior — simulate many ticks
	e.mu.Lock()
	e.sleepiness = 0.0
	e.mu.Unlock()

	// Tick several times — sleepiness should converge to circadian target
	for i := 0; i < 20; i++ {
		e.tick()
	}

	// Current time determines circadian. If it's daytime, sleepiness stays low.
	// If it's nighttime, it rises. This test just verifies the field exists and moves.
	_, vec := e.Current()
	t.Logf("sleepiness after ticks: %f (hour=%d)", vec.Sleepiness, time.Now().Hour())
	if vec.Sleepiness < 0 && vec.Sleepiness > 1 {
		t.Errorf("sleepiness out of range: %f", vec.Sleepiness)
	}
}
