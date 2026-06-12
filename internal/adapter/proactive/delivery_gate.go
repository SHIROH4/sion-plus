package proactive

import (
	"context"
	"log"
	"sync"
	"sync/atomic"
	"time"

	"github.com/shirohania/sion/internal/port"
)

// ProactivePhase tracks the current cognition phase for concurrency control.
type ProactivePhase int32

const (
	PhaseIdle    ProactivePhase = 0
	PhaseRunning ProactivePhase = 1
)

// deliveryGate implements port.DeliveryGate.
// Controls when the AI can speak: min-gap between deliveries, inflight guard, phase CAS.
type deliveryGate struct {
	phase         atomic.Int32        // ProactivePhase
	inflight      atomic.Bool
	lastReleaseAt time.Time
	mu            sync.Mutex
	minGap        time.Duration
}

var _ port.DeliveryGate = (*deliveryGate)(nil)

func NewDeliveryGate() *deliveryGate {
	return &deliveryGate{minGap: 3 * time.Second}
}

// TryAcquire attempts to start a cognition tick. Returns false if already running.
func (g *deliveryGate) TryAcquire() bool {
	return g.phase.CompareAndSwap(int32(PhaseIdle), int32(PhaseRunning))
}

// Release marks the tick as complete.
func (g *deliveryGate) Release() {
	g.phase.Store(int32(PhaseIdle))
}

// CanRelease checks whether the AI is allowed to speak right now.
func (g *deliveryGate) CanRelease(ctx context.Context) bool {
	if g.phase.Load() != int32(PhaseIdle) {
		return false
	}
	if g.inflight.Load() {
		return false
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	if time.Since(g.lastReleaseAt) < g.minGap {
		return false
	}
	return true
}

// OnPlaybackStart marks the start of audio/visual delivery.
func (g *deliveryGate) OnPlaybackStart(ctx context.Context) {
	g.inflight.Store(true)
	log.Println("[DeliveryGate] playback started")
}

// OnPlaybackEnd marks the end of delivery and resets the gap timer.
func (g *deliveryGate) OnPlaybackEnd(ctx context.Context) {
	g.inflight.Store(false)
	g.mu.Lock()
	g.lastReleaseAt = time.Now()
	g.mu.Unlock()
	log.Println("[DeliveryGate] playback ended")
}

func (g *deliveryGate) Reset() {
	g.phase.Store(int32(PhaseIdle))
	g.inflight.Store(false)
	g.mu.Lock()
	g.lastReleaseAt = time.Time{}
	g.mu.Unlock()
}
