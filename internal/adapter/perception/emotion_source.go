package perception

import (
	"context"

	"github.com/SHIROH4/sion-plus/internal/domain/types"
	"github.com/SHIROH4/sion-plus/internal/port"
)

// PerceptionEmotionSource implements port.EmotionSignalSource.
// Translates screen observations into emotion deltas for the 8D model.
type PerceptionEmotionSource struct {
	observer *ScreenObserver
	machine  *ActivityStateMachine
	lastSnap *ActivitySnapshot
}

var _ port.EmotionSignalSource = (*PerceptionEmotionSource)(nil)

func NewPerceptionEmotionSource(observer *ScreenObserver, machine *ActivityStateMachine) *PerceptionEmotionSource {
	return &PerceptionEmotionSource{observer: observer, machine: machine}
}

func (s *PerceptionEmotionSource) Name() string { return "perception" }

func (s *PerceptionEmotionSource) Evaluate(ctx context.Context, input *port.EmotionEvalInput) (*port.EmotionEvalResult, error) {
	if !s.observer.IsAvailable() {
		return nil, nil
	}

	obs, err := s.observer.Observe(ctx)
	if err != nil {
		return nil, err
	}

	snap := s.machine.Classify(obs.AppCategory, obs.AppName,
		s.observer.IdleSeconds(ctx), s.observer.SwitchCount(), obs.WindowTitle)
	s.lastSnap = &snap

	delta := s.deltaFromSnapshot(&snap)
	delta.Source = "perception"

	return &port.EmotionEvalResult{
		Delta:  delta,
		Source: "perception",
	}, nil
}

func (s *PerceptionEmotionSource) LastSnapshot() *ActivitySnapshot { return s.lastSnap }

// ── Delta mapping ──────────────────────────────────────────────────

func (s *PerceptionEmotionSource) deltaFromSnapshot(snap *ActivitySnapshot) *types.EmotionDelta {
	d := &types.EmotionDelta{}

	switch snap.State {
	case StateFocused:
		if snap.FocusMin > 60 {
			d.Worry = 0.15 // working too long
		}
		if snap.FocusMin > 120 {
			d.Worry = 0.25
		}
		d.Curiosity = 0.1 // what are they working on?
		d.Playfulness = -0.1

	case StateGaming:
		d.Playfulness = 0.2
		d.Curiosity = 0.1

	case StateChatting:
		d.Curiosity = 0.1
		d.Playfulness = 0.05

	case StateAway:
		if snap.IdleSec > 30*60 {
			d.Loneliness = 0.2
		}
		if snap.IdleSec > 120*60 {
			d.Loneliness = 0.35
			d.Sleepiness = 0.15
		}

	case StateMeeting:
		d.Playfulness = -0.15
		d.Curiosity = -0.05

	case StateBrowsing, StateIdle:
		// Light engagement — neutral/mild positive
		if snap.SwitchCount <= 2 {
			d.Playfulness = 0.05
		}

	case StatePrivate:
		// User in private mode — respect, pull back
		d.Playfulness = -0.1
		d.Curiosity = -0.1
	}

	return d
}
