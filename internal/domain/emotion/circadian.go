package emotion

import "github.com/SHIROH4/sion-plus/internal/domain/types"

// ── Circadian Rhythm ──

// SleepinessForHour returns the baseline sleepiness for a given hour (0-23).
func SleepinessForHour(hour int) float64 {
	switch {
	case hour >= 23 || hour < 6:
		return 0.75
	case hour >= 6 && hour < 8:
		return 0.50
	case hour >= 8 && hour < 12:
		return 0.20
	case hour >= 12 && hour < 14:
		return 0.15
	case hour >= 14 && hour < 16:
		return 0.35
	case hour >= 16 && hour < 22:
		return 0.20
	default:
		return 0.45
	}
}

// LonelinessFromIdle grows loneliness with idle time.
func LonelinessFromIdle(idleHours float64) float64 {
	return types.Clamp01(0.1 + idleHours*0.04)
}

// ── Sleep Schedule Learning ──

// ActivityHours tracks interaction counts per hour for sleep pattern inference.
type ActivityHours [24]int

// NewActivityHours creates a zeroed tracker.
func NewActivityHours() ActivityHours {
	return ActivityHours{}
}

// RecordActivity increments the counter for the given hour.
func (ah *ActivityHours) RecordActivity(hour int) {
	if hour >= 0 && hour < 24 && ah[hour] < 10000 {
		ah[hour]++
	}
}

// IsQuietHour checks if the hour historically has low activity.
// Falls back to default sleep window (0-7) before enough data is collected.
func IsQuietHour(hour int, activityHours ActivityHours) bool {
	total := 0
	for _, c := range activityHours {
		total += c
	}
	if total < 50 {
		return hour >= 0 && hour < 7
	}
	maxCount := 0
	for _, c := range activityHours {
		if c > maxCount {
			maxCount = c
		}
	}
	if maxCount == 0 {
		return hour >= 0 && hour < 7
	}
	return float64(activityHours[hour]) < float64(maxCount)*0.3
}
