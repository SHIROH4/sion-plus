package emotion

import (
	"context"
	"encoding/json"
	"log"
	"math"
	"os"
	"sync"
	"time"

	"github.com/SHIROH4/sion-plus/internal/domain/types"
	"github.com/SHIROH4/sion-plus/internal/port"
)

// EmotionStore implements port.EmotionStateManager.
// Maintains a full 8D internal vector + PAD external state with:
//   - EMA smoothing (alpha=0.3) for emotional continuity
//   - Circadian rhythm modulating Sleepiness by time of day
//   - Personality-scaling of reactivity
//   - Background decay toward neutral (5min tick)
//
// v2.2: restored to full 8D from 5D. Added EMA + circadian.
type EmotionStore struct {
	mu sync.RWMutex

	// 8D internal vector — raw (pre-EMA) values
	raw [8]float64

	// 8D EMA-smoothed values (the "real" current state)
	affection   float64 // 0~1, neutral=0.5
	annoyance   float64 // 0~1, neutral=0.0
	loneliness  float64 // 0~1, neutral=0.0
	curiosity   float64 // 0~1, neutral=0.5
	confidence  float64 // 0~1, neutral=0.5
	sleepiness  float64 // 0~1, neutral=0.0, circadian-driven
	playfulness float64 // 0~1, neutral=0.5
	worry       float64 // 0~1, neutral=0.0

	// EMA alpha — how much weight new deltas carry (0~1)
	// Higher = more reactive, lower = smoother (inertia)
	emaAlpha float64 // default 0.3

	// Personality
	personality types.PersonalityScale

	// Decay rates per tick (5min)
	decayRates [8]float64 // indexed by dim index

	// Circadian — hour→sleepiness contribution
	circadian func(hour int) float64

	// History
	history    []emotionSnapshot
	historyMax int

	// Lifecycle
	stopCh    chan struct{}
	wg        sync.WaitGroup
	statePath string

	// User activity
	lastActivity time.Time
}

const (
	dimAffection   = 0
	dimAnnoyance   = 1
	dimLoneliness  = 2
	dimCuriosity   = 3
	dimConfidence  = 4
	dimSleepiness  = 5
	dimPlayfulness = 6
	dimWorry       = 7
)

type emotionSnapshot struct {
	State  types.EmotionState
	Vector types.EmotionVector
	At     int64
}

var _ port.EmotionStateManager = (*EmotionStore)(nil)

// NewEmotionStore creates a store with default 8D values.
func NewEmotionStore(statePath string) *EmotionStore {
	e := &EmotionStore{
		raw:          [8]float64{0.5, 0, 0, 0.5, 0.5, 0, 0.5, 0},
		affection:    0.5,
		annoyance:    0,
		loneliness:   0,
		curiosity:    0.5,
		confidence:   0.5,
		sleepiness:   0,
		playfulness:  0.5,
		worry:        0,
		emaAlpha:     0.3,
		personality:  types.DefaultPersonality(),
		history:      make([]emotionSnapshot, 0, 100),
		historyMax:   100,
		statePath:    statePath,
		lastActivity: time.Now(),
		circadian:    defaultCircadian,
	}
	// Decay rates: neutral=0.02, annoyed=0.05, sleepy growth=0.03
	e.decayRates = [8]float64{
		0.02, // affection
		0.05, // annoyance (faster)
		0.02, // loneliness (growth when absent)
		0.02, // curiosity
		0.02, // confidence
		0.03, // sleepiness (growth at night, decay in day)
		0.02, // playfulness
		0.03, // worry (decays faster than affection)
	}
	return e
}

// ── Lifecycle ──────────────────────────────────────────────────────

func (e *EmotionStore) Start() {
	e.stopCh = make(chan struct{})
	e.wg.Add(1)
	go e.decayLoop()
	log.Println("[EmotionStore] started decay loop (5min tick, 8D+EMA)")
}

func (e *EmotionStore) Stop() {
	if e.stopCh != nil {
		close(e.stopCh)
	}
	e.wg.Wait()
	log.Println("[EmotionStore] stopped")
}

func (e *EmotionStore) decayLoop() {
	defer e.wg.Done()
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-e.stopCh:
			return
		case <-ticker.C:
			e.tick()
		}
	}
}

func (e *EmotionStore) tick() {
	e.mu.Lock()
	defer e.mu.Unlock()

	now := time.Now()
	hour := now.Hour()

	// ── Decay toward neutral ──
	e.affection += (0.5 - e.affection) * e.decayRates[dimAffection]
	e.annoyance += (0.0 - e.annoyance) * e.decayRates[dimAnnoyance]
	e.curiosity += (0.5 - e.curiosity) * e.decayRates[dimCuriosity]
	e.confidence += (0.5 - e.confidence) * e.decayRates[dimConfidence]
	e.playfulness += (0.5 - e.playfulness) * e.decayRates[dimPlayfulness]
	e.worry += (0.0 - e.worry) * e.decayRates[dimWorry]

	// ── Loneliness: time-driven ──
	if time.Since(e.lastActivity) > 30*time.Minute {
		e.loneliness = math.Min(1, e.loneliness+e.decayRates[dimLoneliness])
	} else {
		e.loneliness = math.Max(0, e.loneliness*0.8)
	}

	// ── Sleepiness: circadian-driven ──
	circTarget := e.circadian(hour)
	e.sleepiness += (circTarget - e.sleepiness) * e.decayRates[dimSleepiness]

	e.clamp()
	e.recordHistory()
}

// ── Circadian Rhythm ───────────────────────────────────────────────

func defaultCircadian(hour int) float64 {
	switch {
	case hour >= 2 && hour < 6:
		return 0.8 // deep night — very sleepy
	case hour >= 22 || hour < 2:
		return 0.5 // late night — sleepy
	case hour >= 6 && hour < 8:
		return 0.3 // early morning — waking up
	case hour >= 14 && hour < 16:
		return 0.2 // afternoon dip
	default:
		return 0.0 // awake
	}
}

// ── Current State ──────────────────────────────────────────────────

func (e *EmotionStore) Current() (types.EmotionState, types.EmotionVector) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.computeState(), e.snapshotVector()
}

func (e *EmotionStore) computeState() types.EmotionState {
	vec := e.snapshotVector()
	v := e.padFromVector(vec)

	primary := derivePrimary(vec)
	intensity := (math.Abs(v.Valence) + math.Abs(v.Arousal)) / 2

	return types.EmotionState{
		SchemaVersion: types.SchemaVersionCurrent,
		Valence:       types.Clamp1(v.Valence),
		Arousal:       types.Clamp1(v.Arousal),
		Dominance:     types.Clamp1(v.Dominance),
		Primary:       primary,
		Intensity:     math.Min(1.0, intensity),
	}
}

func (e *EmotionStore) snapshotVector() types.EmotionVector {
	return types.EmotionVector{
		Affection:   e.affection,
		Annoyance:   e.annoyance,
		Loneliness:  e.loneliness,
		Curiosity:   e.curiosity,
		Confidence:  e.confidence,
		Sleepiness:  e.sleepiness,
		Playfulness: e.playfulness,
		Worry:       e.worry,
	}
}

// ── PAD from 8D Vector ────────────────────────────────────────────

type padTriple struct{ Valence, Arousal, Dominance float64 }

func (e *EmotionStore) padFromVector(vec types.EmotionVector) padTriple {
	// Valence: balanced — positive vs negative dimensions with equal weight
	valence := (vec.Affection-0.5)*0.8 + (vec.Confidence-0.5)*0.6 + (vec.Playfulness-0.5)*0.4 -
		vec.Annoyance*0.7 - vec.Loneliness*0.5 - vec.Worry*0.6 - vec.Sleepiness*0.2

	// Arousal: blended excitement minus calming factors
	arousal := (vec.Curiosity+vec.Annoyance+vec.Playfulness)/3.0 -
		vec.Sleepiness*0.5 - vec.Loneliness*0.3

	// Dominance: confidence minus submissive/withdrawn factors
	dominance := (vec.Confidence-0.5)*0.7 - vec.Loneliness*0.5 - vec.Worry*0.6 - vec.Annoyance*0.3

	return padTriple{
		Valence:   types.Clamp1(valence),
		Arousal:   types.Clamp1(arousal),
		Dominance: types.Clamp1(dominance),
	}
}

// ── Evaluate (interface stub) ──────────────────────────────────────
// The real entry point is EmotionEvaluator.Evaluate(), which calls ApplyDelta().
// This method satisfies the port interface but is not used in the current pipeline.

func (e *EmotionStore) Evaluate(ctx context.Context, recentTurns string) error {
	return nil
}

// ── ApplyDelta — the core update path ─────────────────────────────

func (e *EmotionStore) ApplyDelta(delta *types.EmotionDelta) {
	e.mu.Lock()
	defer e.mu.Unlock()

	if delta == nil {
		return
	}
	delta.ClampDelta()

	ps := e.personality

	// Personality modulation
	affectionMul := 1.0 + ps.AffectionWarmth*0.5
	annoyanceMul := 1.0 + ps.AnnoyanceSensitivity
	worryMul := 1.0 + ps.WorryTendency

	// Recovery boost: when delta actively reduces negative emotions,
	// amplify + temporarily raise EMA alpha for faster recovery.
	recoveryBoost := 1.0
	origAlpha := e.emaAlpha
	if delta.Annoyance < 0 || delta.Worry < 0 || delta.Loneliness < 0 {
		recoveryBoost = 2.5
		e.emaAlpha = 0.5 // faster recovery blending
	}

	// Neutral drift: when delta is near-zero across all dims,
	// gently pull sticking dimensions toward neutral.
	deltaSum := math.Abs(delta.Affection) + math.Abs(delta.Worry) + math.Abs(delta.Curiosity) +
		math.Abs(delta.Sleepiness) + math.Abs(delta.Playfulness) + math.Abs(delta.Loneliness) +
		math.Abs(delta.Confidence) + math.Abs(delta.Annoyance)
	if deltaSum < 0.15 {
		if e.affection > 0.6 {
			e.addRaw(dimAffection, -0.04)
		}
		if e.playfulness > 0.6 {
			e.addRaw(dimPlayfulness, -0.04)
		}
		if e.curiosity > 0.6 {
			e.addRaw(dimCuriosity, -0.04)
		}
		if e.worry > 0.1 {
			e.addRaw(dimWorry, -0.03)
		}
	}

	// Apply signed deltas
	e.addRaw(dimAffection, delta.Affection*affectionMul*recoveryBoost)
	e.addRaw(dimAnnoyance, delta.Annoyance*annoyanceMul*recoveryBoost)
	e.addRaw(dimLoneliness, delta.Loneliness*recoveryBoost)
	e.addRaw(dimCuriosity, delta.Curiosity)
	e.addRaw(dimConfidence, delta.Confidence*recoveryBoost)
	e.addRaw(dimSleepiness, delta.Sleepiness)
	e.addRaw(dimPlayfulness, delta.Playfulness*recoveryBoost)
	e.addRaw(dimWorry, delta.Worry*worryMul*recoveryBoost)

	// ── Apply EMA smoothing to all 8 dims ──
	e.applyEMA()
	e.emaAlpha = origAlpha // restore

	// ── Diminishing returns on affection ──
	e.affection = math.Min(1, e.affection)

	e.clamp()
	e.recordHistory()
}

// addRaw adds delta to the raw (pre-EMA) value.
func (e *EmotionStore) addRaw(dim int, delta float64) {
	e.raw[dim] = math.Max(-1, math.Min(1, e.raw[dim]+delta))
}

// applyEMA blends raw deltas into the smoothed state.
// alpha = 0.3 means 30% new signal, 70% existing state.
func (e *EmotionStore) applyEMA() {
	alpha := e.emaAlpha
	e.affection = e.emaBlend(e.affection, e.raw[dimAffection]+0.5, alpha) // raw is delta, recenter to 0~1
	e.annoyance = e.emaBlend(e.annoyance, clamp0(e.raw[dimAnnoyance]), alpha)
	e.loneliness = e.emaBlend(e.loneliness, clamp0(e.raw[dimLoneliness]), alpha)
	e.curiosity = e.emaBlend(e.curiosity, e.raw[dimCuriosity]+0.5, alpha)
	e.confidence = e.emaBlend(e.confidence, e.raw[dimConfidence]+0.5, alpha)
	e.sleepiness = e.emaBlend(e.sleepiness, clamp0(e.raw[dimSleepiness]), alpha)
	e.playfulness = e.emaBlend(e.playfulness, e.raw[dimPlayfulness]+0.5, alpha)
	e.worry = e.emaBlend(e.worry, clamp0(e.raw[dimWorry]), alpha)

	// Reset raw deltas after blending (they accumulate between ticks)
	for i := range e.raw {
		e.raw[i] *= 0.5 // gradual decay of pending delta
	}
}

func (e *EmotionStore) emaBlend(current, target, alpha float64) float64 {
	return current + (target-current)*alpha
}

// ── Direct manipulation ────────────────────────────────────────────

func (e *EmotionStore) ApplyDirect(vec types.EmotionVector) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.affection = types.Clamp01(vec.Affection)
	e.annoyance = types.Clamp01(vec.Annoyance)
	e.loneliness = types.Clamp01(vec.Loneliness)
	e.curiosity = types.Clamp01(vec.Curiosity)
	e.confidence = types.Clamp01(vec.Confidence)
	e.sleepiness = types.Clamp01(vec.Sleepiness)
	e.playfulness = types.Clamp01(vec.Playfulness)
	e.worry = types.Clamp01(vec.Worry)
	e.recordHistory()
}

func (e *EmotionStore) NotifyActivity() {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.lastActivity = time.Now()
	e.loneliness = 0
	e.playfulness = math.Min(1, e.playfulness+0.1) // user is here → more playful
}

// ── Personality ────────────────────────────────────────────────────

func (e *EmotionStore) SetPersonality(p types.PersonalityScale) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.personality = p
}

func (e *EmotionStore) Personality() types.PersonalityScale {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.personality
}

func (e *EmotionStore) LearnPersonality(ctx context.Context) error { return nil }

// ── Need Modulation ────────────────────────────────────────────────

func (e *EmotionStore) SetNeedModulation(mod *types.NeedModulation) {
	if mod == nil {
		return
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	e.decayRates[dimConfidence] = 0.02 * (1.0 / mod.ConfidenceDecayMul)
	e.decayRates[dimCuriosity] = 0.02 * (1.0 / mod.CuriosityDecayMul)
}

// ── History ────────────────────────────────────────────────────────

func (e *EmotionStore) History() ([]types.EmotionState, []types.EmotionVector) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	states := make([]types.EmotionState, len(e.history))
	vectors := make([]types.EmotionVector, len(e.history))
	for i, s := range e.history {
		states[i] = s.State
		vectors[i] = s.Vector
	}
	return states, vectors
}

// ── Persistence ────────────────────────────────────────────────────

func (e *EmotionStore) Load(ctx context.Context) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.statePath == "" {
		return nil
	}
	data, err := os.ReadFile(e.statePath)
	if err != nil {
		return nil
	}
	var saved struct {
		Affection, Annoyance, Loneliness, Curiosity, Confidence float64
		Sleepiness, Playfulness, Worry                          float64
		Personality                                             types.PersonalityScale
	}
	if err := json.Unmarshal(data, &saved); err != nil {
		return nil
	}
	e.affection = saved.Affection
	e.annoyance = saved.Annoyance
	e.loneliness = saved.Loneliness
	e.curiosity = saved.Curiosity
	e.confidence = saved.Confidence
	e.sleepiness = saved.Sleepiness
	e.playfulness = saved.Playfulness
	e.worry = saved.Worry
	e.personality = saved.Personality
	return nil
}

func (e *EmotionStore) Save(ctx context.Context) error {
	e.mu.RLock()
	defer e.mu.RUnlock()
	if e.statePath == "" {
		return nil
	}
	saved := struct {
		Affection, Annoyance, Loneliness, Curiosity, Confidence float64
		Sleepiness, Playfulness, Worry                          float64
		Personality                                             types.PersonalityScale
	}{
		e.affection, e.annoyance, e.loneliness, e.curiosity, e.confidence,
		e.sleepiness, e.playfulness, e.worry,
		e.personality,
	}
	data, _ := json.MarshalIndent(saved, "", "  ")
	return os.WriteFile(e.statePath, data, 0644)
}

// ── Helpers ────────────────────────────────────────────────────────

func (e *EmotionStore) clamp() {
	e.affection = types.Clamp01(e.affection)
	e.annoyance = types.Clamp01(e.annoyance)
	e.loneliness = types.Clamp01(e.loneliness)
	e.curiosity = types.Clamp01(e.curiosity)
	e.confidence = types.Clamp01(e.confidence)
	e.sleepiness = types.Clamp01(e.sleepiness)
	e.playfulness = types.Clamp01(e.playfulness)
	e.worry = types.Clamp01(e.worry)
}

func (e *EmotionStore) recordHistory() {
	snap := emotionSnapshot{
		State:  e.computeState(),
		Vector: e.snapshotVector(),
		At:     time.Now().Unix(),
	}
	e.history = append(e.history, snap)
	if len(e.history) > e.historyMax {
		e.history = e.history[len(e.history)-e.historyMax:]
	}
}

func derivePrimary(vec types.EmotionVector) string {
	// Compute PAD inline for secondary validation
	pad := padTriple{}
	pad.Valence = (vec.Affection-0.5)*0.8 + (vec.Confidence-0.5)*0.6 + (vec.Playfulness-0.5)*0.4 -
		vec.Annoyance*0.7 - vec.Loneliness*0.5 - vec.Worry*0.6 - vec.Sleepiness*0.2

	// Negative emotions first
	switch {
	case vec.Sleepiness > 0.65:
		return "sleepy"
	case vec.Annoyance > 0.35:
		return "anger"
	case vec.Worry > 0.25 && vec.Loneliness > 0.25:
		return "sadness"
	case vec.Worry > 0.12 && pad.Valence < 0.3:
		return "worried"
	case vec.Loneliness > 0.45:
		return "lonely"
	}

	// Positive emotions — require valence not deeply negative
	if pad.Valence > -0.2 {
		switch {
		case vec.Affection > 0.7 && vec.Playfulness > 0.55:
			return "joy"
		case vec.Curiosity > 0.8:
			return "curious"
		case vec.Affection > 0.6:
			return "happy"
		}
	}

	// Low valence with some negative signal
	switch {
	case vec.Worry > 0.12:
		return "worried"
	case vec.Annoyance > 0.15:
		return "irritated"
	case pad.Valence < -0.15:
		return "sadness"
	default:
		return "neutral"
	}
}

func clamp0(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}
