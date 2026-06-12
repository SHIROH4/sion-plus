package perception

import (
	"testing"
)

func TestDeltaFromSnapshotFocused(t *testing.T) {
	s := &PerceptionEmotionSource{}
	snap := &ActivitySnapshot{State: StateFocused, FocusMin: 90}
	d := s.deltaFromSnapshot(snap)
	if d.Worry <= 0 {
		t.Error("focused for 90min should trigger worry")
	}
	if d.Playfulness >= 0 {
		t.Error("focused should reduce playfulness")
	}
}

func TestDeltaFromSnapshotFocusedLong(t *testing.T) {
	s := &PerceptionEmotionSource{}
	snap := &ActivitySnapshot{State: StateFocused, FocusMin: 150}
	d := s.deltaFromSnapshot(snap)
	if d.Worry < 0.2 {
		t.Errorf("focused for 150min should have strong worry, got %.2f", d.Worry)
	}
}

func TestDeltaFromSnapshotGaming(t *testing.T) {
	s := &PerceptionEmotionSource{}
	snap := &ActivitySnapshot{State: StateGaming}
	d := s.deltaFromSnapshot(snap)
	if d.Playfulness <= 0 {
		t.Error("gaming should increase playfulness")
	}
}

func TestDeltaFromSnapshotAway(t *testing.T) {
	s := &PerceptionEmotionSource{}
	// Short away (40min → above 30min threshold)
	snap := &ActivitySnapshot{State: StateAway, IdleSec: 40 * 60}
	d := s.deltaFromSnapshot(snap)
	if d.Loneliness <= 0 {
		t.Error("away should increase loneliness")
	}
	if d.Sleepiness != 0 {
		t.Error("short away should not increase sleepiness")
	}

	// Long away
	snap2 := &ActivitySnapshot{State: StateAway, IdleSec: 180 * 60}
	d2 := s.deltaFromSnapshot(snap2)
	if d2.Loneliness < d.Loneliness {
		t.Error("longer away should increase loneliness more")
	}
	if d2.Sleepiness <= 0 {
		t.Error("long away should increase sleepiness")
	}
}

func TestDeltaFromSnapshotMeeting(t *testing.T) {
	s := &PerceptionEmotionSource{}
	snap := &ActivitySnapshot{State: StateMeeting}
	d := s.deltaFromSnapshot(snap)
	if d.Playfulness >= 0 {
		t.Error("meeting should reduce playfulness")
	}
}

func TestDeltaFromSnapshotPrivate(t *testing.T) {
	s := &PerceptionEmotionSource{}
	snap := &ActivitySnapshot{State: StatePrivate}
	d := s.deltaFromSnapshot(snap)
	if d.Playfulness >= 0 || d.Curiosity >= 0 {
		t.Error("private should reduce playfulness and curiosity")
	}
}

func TestDeltaFromSnapshotBrowsingIdle(t *testing.T) {
	s := &PerceptionEmotionSource{}
	snap := &ActivitySnapshot{State: StateBrowsing, SwitchCount: 1}
	d := s.deltaFromSnapshot(snap)
	if d.Playfulness <= 0 {
		t.Error("light browsing should slightly increase playfulness")
	}

	snap2 := &ActivitySnapshot{State: StateBrowsing, SwitchCount: 5}
	d2 := s.deltaFromSnapshot(snap2)
	if d2.Playfulness != 0 {
		t.Error("rapid browsing should not affect playfulness")
	}
}

func TestDeltaFromSnapshotAllStates(t *testing.T) {
	s := &PerceptionEmotionSource{}
	states := []ActivityState{
		StateAway, StatePrivate, StateGaming, StateFocused,
		StateBrowsing, StateChatting, StateMeeting, StateIdle, StateUnknown,
	}
	for _, st := range states {
		snap := &ActivitySnapshot{State: st}
		d := s.deltaFromSnapshot(snap)
		if d == nil {
			t.Fatalf("delta should not be nil for state %s", st)
		}
		// All values should be in [-1, 1]
		for _, v := range []float64{d.Affection, d.Worry, d.Curiosity, d.Sleepiness,
			d.Playfulness, d.Loneliness, d.Confidence, d.Annoyance} {
			if v < -1 || v > 1 {
				t.Errorf("delta value %.2f out of range for state %s", v, st)
			}
		}
	}
}
