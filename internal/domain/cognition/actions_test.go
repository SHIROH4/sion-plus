package cognition

import (
	"testing"
)

func TestBuildActionsCanonical(t *testing.T) {
	actions := BuildActions()
	if len(actions) != 16 {
		t.Errorf("expected 16 canonical actions, got %d", len(actions))
	}

	names := make(map[string]bool)
	for _, a := range actions {
		if names[a.Name] {
			t.Errorf("duplicate action name: %s", a.Name)
		}
		names[a.Name] = true

		if a.Name == "" {
			t.Error("action with empty name")
		}
		if a.Category == "" {
			t.Errorf("%s: missing category", a.Name)
		}
		if a.Source == "" {
			t.Errorf("%s: missing source", a.Name)
		}

		// Check all weights are in [-1, 1] range
		if a.WeightSocial < -1 || a.WeightSocial > 1 ||
			a.WeightCare < -1 || a.WeightCare > 1 ||
			a.WeightCurious < -1 || a.WeightCurious > 1 ||
			a.WeightQuiet < -1 || a.WeightQuiet > 1 ||
			a.WeightExplore < -1 || a.WeightExplore > 1 {
			t.Errorf("%s: weight out of range", a.Name)
		}
	}

	// Verify critical actions exist
	for _, required := range []string{"none", "speak_casual", "speak_care", "speak_inquiry", "search"} {
		if !names[required] {
			t.Errorf("required action %q missing", required)
		}
	}
}

func TestActionByName(t *testing.T) {
	if act := ActionByName("speak_casual"); act == nil {
		t.Error("speak_casual not found")
	}
	if act := ActionByName("nonexistent"); act != nil {
		t.Error("nonexistent should return nil")
	}
	if act := ActionByName(""); act != nil {
		t.Error("empty name should return nil")
	}
}

func TestActionWeightVectors(t *testing.T) {
	// Verify that 'none' is the only action with zero social/care/curious/explore
	// and high quiet weight
	actions := BuildActions()
	for _, a := range actions {
		if a.Name == "none" {
			if a.WeightQuiet != 1.0 {
				t.Error("'none' should have WeightQuiet=1.0")
			}
			if a.WeightSocial != 0 || a.WeightCare != 0 || a.WeightCurious != 0 || a.WeightExplore != 0 {
				t.Error("'none' should have zero non-quiet weights")
			}
		}
	}
}

func TestBuildActionsNightSafeConsistency(t *testing.T) {
	actions := BuildActions()
	for _, a := range actions {
		if a.OutcomeType == "speak" && a.Category == "social" && a.NightSafe {
			t.Errorf("%s: social speak actions should not be night-safe", a.Name)
		}
	}
}
