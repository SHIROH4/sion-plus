package perception

import (
	"testing"

	"github.com/shirohania/sion/internal/port"
)

func TestParseEnrichmentValid(t *testing.T) {
	raw := `{"activity":"debugging Go code","user_mood":"frustrated","user_emotion":"stressed","should_engage":true,"engage_reason":"user seems stuck on a bug","suggested_tone":"supportive","observed_text":"panic: runtime error"}`

	e, err := parseEnrichment(raw)
	if err != nil {
		t.Fatalf("parseEnrichment: %v", err)
	}
	if e.Activity != "debugging Go code" {
		t.Errorf("activity=%q", e.Activity)
	}
	if e.UserMood != "frustrated" {
		t.Errorf("user_mood=%q", e.UserMood)
	}
	if !e.ShouldEngage {
		t.Error("should_engage should be true")
	}
	if e.SuggestedTone != "supportive" {
		t.Errorf("suggested_tone=%q", e.SuggestedTone)
	}
}

func TestParseEnrichmentMarkdownWrapped(t *testing.T) {
	raw := "```json\n{\"activity\":\"watching YouTube\",\"user_mood\":\"relaxed\",\"user_emotion\":\"happy\",\"should_engage\":true,\"engage_reason\":\"watching entertainment\",\"suggested_tone\":\"playful\",\"observed_text\":\"\"}\n```"

	e, err := parseEnrichment(raw)
	if err != nil {
		t.Fatalf("parseEnrichment: %v", err)
	}
	if e.Activity != "watching YouTube" {
		t.Errorf("activity=%q", e.Activity)
	}
}

func TestParseEnrichmentInvalid(t *testing.T) {
	_, err := parseEnrichment("not json at all")
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestToEmotionDeltaFrustrated(t *testing.T) {
	e := &ScreenLLMEnricher{}
	enrich := &ScreenEnrichment{
		UserMood:      "frustrated",
		SuggestedTone: "supportive",
		ShouldEngage:  true,
	}
	d := e.ToEmotionDelta(enrich)
	if d == nil {
		t.Fatal("delta should not be nil")
	}
	if d.Worry <= 0 {
		t.Error("frustrated should increase worry")
	}
	if d.Affection <= 0 {
		t.Error("supportive tone should increase affection")
	}
}

func TestToEmotionDeltaRelaxed(t *testing.T) {
	e := &ScreenLLMEnricher{}
	enrich := &ScreenEnrichment{
		UserMood:      "relaxed",
		SuggestedTone: "playful",
		ShouldEngage:  true,
	}
	d := e.ToEmotionDelta(enrich)
	if d.Playfulness <= 0 {
		t.Error("relaxed + playful should increase playfulness")
	}
}

func TestToEmotionDeltaFocusedNoEngage(t *testing.T) {
	e := &ScreenLLMEnricher{}
	enrich := &ScreenEnrichment{
		UserMood:      "focused",
		SuggestedTone: "quiet",
		ShouldEngage:  false,
	}
	d := e.ToEmotionDelta(enrich)
	if d.Curiosity <= 0 {
		t.Error("focused should increase curiosity")
	}
	if d.Playfulness >= 0 {
		t.Error("quiet tone should reduce playfulness")
	}
}

func TestToEmotionDeltaNil(t *testing.T) {
	e := &ScreenLLMEnricher{}
	d := e.ToEmotionDelta(nil)
	if d != nil {
		t.Error("nil enrichment should return nil delta")
	}
}

func TestBuildVisionPrompt(t *testing.T) {
	obs := &port.ScreenObservation{
		AppName:     "Visual Studio Code",
		WindowTitle: "main.go — sion-v1",
	}
	snap := ActivitySnapshot{State: StateFocused}
	prompt := buildVisionPrompt(obs, snap)
	if prompt == "" {
		t.Error("prompt should not be empty")
	}
	if !contains(prompt, "Visual Studio Code") {
		t.Error("prompt should include app name")
	}
	if !contains(prompt, "main.go") {
		t.Error("prompt should include window title")
	}
	if !contains(prompt, "focused") {
		t.Error("prompt should include state")
	}
}
