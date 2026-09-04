package proactive

import (
	"context"
	"time"

	"github.com/SHIROH4/sion-plus/internal/domain/types"
	"github.com/SHIROH4/sion-plus/internal/port"
)

// PersonaQuerier is the subset of PersonaStore used by proactive.
type PersonaQuerier interface {
	Boundaries(ctx context.Context) []PersonaEntry
	WorkPatterns(ctx context.Context) []PersonaEntry
	Preferences(ctx context.Context) []PersonaEntry
}

// PersonaEntry mirrors memory.PersonaEntry without importing the package.
type PersonaEntry struct {
	Entity       string
	RelationType string
	Value        string
	Evidence     PersonaEvidence
}

// PersonaEvidence mirrors memory.PersonaEvidence.
type PersonaEvidence struct {
	Score float64
}

// FeatureExtractor consolidates emotion, memory, perception, and clock
// into the QuantifiedFeatures vector used for drive/action scoring.
type FeatureExtractor struct {
	emotionStore port.EmotionStateManager
	memoryStore  port.MemoryStore
	screenObs    port.ScreenObserver
	persona      PersonaQuerier
}

func NewFeatureExtractor(
	emotionStore port.EmotionStateManager,
	memoryStore port.MemoryStore,
	screenObs port.ScreenObserver,
) *FeatureExtractor {
	return &FeatureExtractor{
		emotionStore: emotionStore,
		memoryStore:  memoryStore,
		screenObs:    screenObs,
	}
}

// SetPersona wires the persona store for structured identity queries.
func (e *FeatureExtractor) SetPersona(p PersonaQuerier) { e.persona = p }

// Extract fills the QuantifiedFeatures from current system state.
// Only fills features actually used by ComputeDrives / computeModulators.
func (e *FeatureExtractor) Extract(ctx context.Context) *types.QuantifiedFeatures {
	now := time.Now()
	hour := now.Hour()

	f := &types.QuantifiedFeatures{}

	// ── A组: Agent状态 (Emotion 8D → 直接映射) ──
	state, vec := e.emotionStore.Current()
	f.A1_1_Affection = vec.Affection
	f.A1_2_Worry = vec.Worry
	f.A1_3_Curiosity = vec.Curiosity
	f.A1_4_Sleepiness = vec.Sleepiness
	f.A1_5_Playfulness = vec.Playfulness
	f.A1_6_Loneliness = vec.Loneliness
	f.A1_7_Confidence = vec.Confidence
	f.A1_8_Annoyance = vec.Annoyance
	f.A2_PrimaryEmotion = state.Primary
	f.A3_Intensity = state.Intensity

	// Personality (from store)
	ps := e.emotionStore.Personality()
	f.A5_1_AnnoySensitivity = ps.AnnoyanceSensitivity
	f.A5_2_AffectWarmth = ps.AffectionWarmth
	f.A5_3_WorryTendency = ps.WorryTendency

	// Daily action count (from memory)
	f.A6_DailyActionCount = e.memoryStore.CountTodayMessages(ctx)

	// ── U组: User状态 (Perception + Clock) ──
	if e.screenObs != nil && e.screenObs.IsAvailable() {
		obs, err := e.screenObs.Observe(ctx)
		if err == nil && obs != nil {
			f.U1_AppCategory = obs.AppCategory
			f.U2_WindowSubtype = obs.AppCategory // simplification: subtype from app type
		}
	}

	// Night detection
	if hour >= 23 || hour < 6 {
		f.U12_NightTime = 1.0
	} else if hour >= 22 || hour < 7 {
		f.U12_NightTime = 0.5
	}

	// ── E组: Environment (Clock) ──
	f.E1_Hour = hour
	f.E2_DayOfWeek = int(now.Weekday())
	f.E3_CooldownNorm = 0.8  // default: mostly available
	f.E4_QuotaRemaining = 10 // default: plenty of quota

	// ── R组: Relationship (explicit proactive feedback) ──
	// No feedback means unknown, not positive. Use neutral priors until enough
	// explicit labels are collected; silence and unrelated chat are excluded.
	f.R1_OverallAcceptRate = 0.5
	f.R1_SampleCount = 0
	f.R4_RecentRejections = 0
	f.R3_SourceAcceptRate = map[string]float64{"proactive": 0.5}
	f.U8_EngagementNorm = 0.5
	f.U10_TimeWindowPref = 0.5
	if feedbackStore, ok := e.memoryStore.(port.ProactiveFeedbackStore); ok {
		if stats, err := feedbackStore.ActionFeedbackStats(ctx); err == nil {
			samples, rewardSum := 0, 0.0
			for _, stat := range stats {
				samples += stat.Samples
				rewardSum += stat.RewardSum
			}
			if samples > 0 {
				// Map mean reward [-1,1] to a conservative acceptance proxy [0,1].
				rate := (rewardSum/float64(samples) + 1) / 2
				f.R1_OverallAcceptRate = rate
				f.R1_SampleCount = float64(samples)
				f.R3_SourceAcceptRate["proactive"] = rate
			}
		}
	}

	// ── Persona-derived features ──
	if e.persona != nil {
		// Work patterns → compute continuous_work_min
		patterns := e.persona.WorkPatterns(ctx)
		if len(patterns) > 0 {
			f.U4_ContinuousWorkMin = float64(len(patterns)) * 15 // rough estimate
		}

		// Boundaries → lower engagement if many boundaries exist
		boundaries := e.persona.Boundaries(ctx)
		if len(boundaries) > 3 {
			f.U8_EngagementNorm *= 0.7 // user is sensitive → reduce engagement
		}

		// Preferences → adjust time window preference
		prefs := e.persona.Preferences(ctx)
		if len(prefs) > 5 {
			f.U10_TimeWindowPref = 1.0 // well-known user → confident timing
		}
	}

	return f
}
