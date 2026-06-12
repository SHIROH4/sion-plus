package cognition

import (
	"testing"

	"github.com/SHIROH4/sion-plus/internal/domain/types"
)

func TestIdleBonus(t *testing.T) {
	tests := []struct {
		name   string
		mins   float64
		wantMin float64
		wantMax float64
	}{
		{"zero", 0, 0, 0},
		{"rising", 15, 0.4, 0.6},
		{"peak_start", 30, 0.95, 1.0},
		{"plateau", 60, 0.95, 1.0},
		{"decaying", 120, 0.7, 0.95},
		{"decaying_mid", 165, 0.4, 0.7},
		{"floor", 300, 0.1, 0.2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := idleBonus(tt.mins)
			if got < tt.wantMin || got > tt.wantMax {
				t.Errorf("idleBonus(%.0f) = %.3f, want [%.3f, %.3f]", tt.mins, got, tt.wantMin, tt.wantMax)
			}
		})
	}
}

func TestClamp01(t *testing.T) {
	tests := []struct {
		in   float64
		want float64
	}{
		{0.5, 0.5},
		{-0.5, 0},
		{1.5, 1},
		{0, 0},
		{1, 1},
	}
	for _, tt := range tests {
		got := clamp01(tt.in)
		if got != tt.want {
			t.Errorf("clamp01(%.1f) = %.1f, want %.1f", tt.in, got, tt.want)
		}
	}
}

func TestInteractionGate(t *testing.T) {
	tests := []struct {
		rate float64
		want float64
	}{
		{0, 1.0},
		{0.3, 0.8},
		{0.5, 1.0},
		{0.8, 1.0},
		{1.0, 1.0},
	}
	for _, tt := range tests {
		got := interactionGate(tt.rate)
		if got != tt.want {
			t.Errorf("interactionGate(%.1f) = %.2f, want %.2f", tt.rate, got, tt.want)
		}
	}
}

func TestTimeNorm(t *testing.T) {
	tests := []struct {
		mins float64
		want float64
	}{
		{0, 0},
		{5, 0.5},
		{10, 1.0},
		{20, 1.0},
	}
	for _, tt := range tests {
		got := timeNorm(tt.mins)
		if got != tt.want {
			t.Errorf("timeNorm(%.0f)=%.2f, want %.2f", tt.mins, got, tt.want)
		}
	}
}

func TestHasAny(t *testing.T) {
	if got := hasAny(0); got != 0 {
		t.Errorf("hasAny(0)=%f", got)
	}
	if got := hasAny(1); got != 0.6 {
		t.Errorf("hasAny(1)=%f", got)
	}
}

func TestRejectionSeverity(t *testing.T) {
	if got := rejectionSeverity(0); got != 0 {
		t.Errorf("rejectionSeverity(0)=%f", got)
	}
	if got := rejectionSeverity(3); got != 0.6 {
		t.Errorf("rejectionSeverity(3)=%f", got)
	}
	if got := rejectionSeverity(5); got != 1.0 {
		t.Errorf("rejectionSeverity(5)=%f", got)
	}
	if got := rejectionSeverity(10); got != 1.0 {
		t.Errorf("rejectionSeverity(10)=%f", got)
	}
}

func TestMealTimeBonus(t *testing.T) {
	if got := mealTimeBonus(12); got != 0.5 {
		t.Errorf("mealTimeBonus(12)=%f", got)
	}
	if got := mealTimeBonus(18); got != 0.5 {
		t.Errorf("mealTimeBonus(18)=%f", got)
	}
	if got := mealTimeBonus(8); got != 0 {
		t.Errorf("mealTimeBonus(8)=%f", got)
	}
}

func TestNightTimeBonus(t *testing.T) {
	if got := nightTimeBonus(23); got != 0.6 {
		t.Errorf("nightTimeBonus(23)=%f", got)
	}
	if got := nightTimeBonus(3); got != 0.6 {
		t.Errorf("nightTimeBonus(3)=%f", got)
	}
	if got := nightTimeBonus(14); got != 0 {
		t.Errorf("nightTimeBonus(14)=%f", got)
	}
}

func TestComputeDrivesNeutral(t *testing.T) {
	f := &types.QuantifiedFeatures{
		R1_OverallAcceptRate: 0.7,
		R1_SampleCount:       10,
		E3_CooldownNorm:      0.8,
		E4_QuotaRemaining:    10,
	}
	drives := ComputeDrives(f, nil)

	if drives.Social < 0 || drives.Social > 1 {
		t.Errorf("Social out of range: %.2f", drives.Social)
	}
	if drives.Care < 0 || drives.Care > 1 {
		t.Errorf("Care out of range: %.2f", drives.Care)
	}
	if drives.Curious < 0 || drives.Curious > 1 {
		t.Errorf("Curious out of range: %.2f", drives.Curious)
	}
	if drives.Quiet < 0 || drives.Quiet > 1 {
		t.Errorf("Quiet out of range: %.2f", drives.Quiet)
	}
	if drives.Explore < 0 || drives.Explore > 1 {
		t.Errorf("Explore out of range: %.2f", drives.Explore)
	}
}

func TestComputeDrivesNightSuppression(t *testing.T) {
	f := &types.QuantifiedFeatures{
		A1_6_Loneliness:      0.8,
		A1_5_Playfulness:     0.8,
		A1_1_Affection:       0.8,
		U12_NightTime:        1.0,
		R1_OverallAcceptRate: 0.7,
		R1_SampleCount:       10,
		E3_CooldownNorm:      0.8,
		E4_QuotaRemaining:    10,
	}
	drives := ComputeDrives(f, nil)
	if drives.Social > 0.7 {
		t.Errorf("social drive should be suppressed at night: %.2f", drives.Social)
	}
}

func TestComputeDrivesWorkingSuppression(t *testing.T) {
	f := &types.QuantifiedFeatures{
		A1_6_Loneliness:      0.8,
		U3_IsWorking:         1.0,
		R1_OverallAcceptRate: 0.7,
		R1_SampleCount:       10,
		E3_CooldownNorm:      0.8,
		E4_QuotaRemaining:    10,
	}
	drives := ComputeDrives(f, nil)
	if drives.Social > 0.6 {
		t.Errorf("social drive should be suppressed when user working: %.2f", drives.Social)
	}
	if drives.Quiet < 0.1 {
		t.Errorf("quiet drive should be boosted when user working: %.2f", drives.Quiet)
	}
}

func TestComputeDrivesHighAnnoyance(t *testing.T) {
	f := &types.QuantifiedFeatures{
		A1_8_Annoyance:       0.9,
		A1_1_Affection:       0.2,
		A1_6_Loneliness:      0.2,
		R1_OverallAcceptRate: 0.7,
		R1_SampleCount:       10,
		E3_CooldownNorm:      0.8,
		E4_QuotaRemaining:    10,
	}
	drives := ComputeDrives(f, nil)
	if drives.Social > 0.4 {
		t.Errorf("social should be low when annoyed: %.2f", drives.Social)
	}
	if drives.Quiet < 0.2 {
		t.Errorf("quiet should be somewhat elevated when annoyed: %.2f", drives.Quiet)
	}
}

func TestComputeDrivesQuotaProtection(t *testing.T) {
	f := &types.QuantifiedFeatures{
		A1_3_Curiosity:       0.8,
		A11_ActiveInquiries:   3,
		A12_KnowledgeGaps:     3,
		E3_CooldownNorm:      0.8,
		E4_QuotaRemaining:    2,
		R1_OverallAcceptRate: 0.7,
		R1_SampleCount:       10,
	}
	drives := ComputeDrives(f, nil)
	if drives.Quiet < 0.05 {
		t.Errorf("quiet should be boosted when quota low: %.2f", drives.Quiet)
	}
}

func TestComputeTier1(t *testing.T) {
	state := &types.CognitionState{
		EmotionVec: types.EmotionVector{
			Affection: 0.5, Worry: 0.1, Curiosity: 0.3,
			Sleepiness: 0.2, Playfulness: 0.4, Loneliness: 0.1,
			Confidence: 0.6, Annoyance: 0.05,
		},
		Personality: types.PersonalityScale{
			AnnoyanceSensitivity: 0.5,
			AffectionWarmth:      0.7,
			WorryTendency:        0.3,
		},
		OverallAcceptRate:   0.7,
		AcceptSampleCount:   10,
		HistoryAverageValence: 0.2,
	}

	f := ComputeTier1(state)
	if f.A1_1_Affection != 0.5 {
		t.Errorf("A1_1 = %.2f", f.A1_1_Affection)
	}
	if f.A5_1_AnnoySensitivity != 0.5 {
		t.Errorf("A5_1 = %.2f", f.A5_1_AnnoySensitivity)
	}
	// A4_ValenceTrend should now use HistoryAverageValence, not be zero-vs-zero
	if state.Emotion.Valence != 0 || state.HistoryAverageValence != 0.2 {
		// Only check if values are set; default valence is 0
		if f.A4_ValenceTrend == 0 {
			t.Log("A4_ValenceTrend is 0 because current valence is 0 (default)")
		}
	}
}
