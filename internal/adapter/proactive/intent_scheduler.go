package proactive

import (
	"context"
	"sort"
	"sync"
	"time"

	"github.com/SHIROH4/sion-plus/internal/domain/types"
	"github.com/SHIROH4/sion-plus/internal/port"
)

// intentScheduler implements port.IntentScheduler.
// Simplified queue: priority ordering, TTL expiry, basic dedup by coalesce key.
type intentScheduler struct {
	mu        sync.Mutex
	queue     []types.ProactiveIntent
	recent    [10]string // recent delivered messages for dedup
	recentIdx int

	// Stats
	delivered  int
	droppedTTL int
	coalesced  int
}

var _ port.IntentScheduler = (*intentScheduler)(nil)

func NewIntentScheduler() *intentScheduler {
	return &intentScheduler{}
}

// Submit adds an intent to the queue with dedup + TTL checks.
func (s *intentScheduler) Submit(ctx context.Context, intent types.ProactiveIntent) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// TTL check
	if intent.TTL > 0 && time.Since(intent.CreatedAt) > intent.TTL {
		s.droppedTTL++
		return nil
	}

	// Dedup by coalesce key
	if intent.CoalesceKey != "" {
		for i, q := range s.queue {
			if q.CoalesceKey == intent.CoalesceKey {
				s.queue[i] = intent // replace with newer
				s.coalesced++
				return nil
			}
		}
	}

	// Dedup by recent messages
	for _, r := range s.recent {
		if r != "" && r == intent.Message {
			return nil // duplicate
		}
	}

	s.queue = append(s.queue, intent)

	// Sort by priority descending, then FIFO
	sort.Slice(s.queue, func(i, j int) bool {
		if s.queue[i].Priority != s.queue[j].Priority {
			return s.queue[i].Priority > s.queue[j].Priority
		}
		return s.queue[i].CreatedAt.Before(s.queue[j].CreatedAt)
	})

	return nil
}

// Drain returns all queued intents and clears the queue.
func (s *intentScheduler) Drain() []types.ProactiveIntent {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := s.queue
	s.queue = nil

	// Record in recent history
	for _, intent := range out {
		s.recent[s.recentIdx%10] = intent.Message
		s.recentIdx++
		s.delivered++
	}
	return out
}

// Peek returns queued intents without removing them.
func (s *intentScheduler) Peek() []types.ProactiveIntent {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]types.ProactiveIntent, len(s.queue))
	copy(out, s.queue)
	return out
}

// Stats returns scheduler statistics.
func (s *intentScheduler) Stats() port.SchedulerStats {
	s.mu.Lock()
	defer s.mu.Unlock()
	return port.SchedulerStats{
		Queued:     len(s.queue),
		DroppedTTL: s.droppedTTL,
		Coalesced:  s.coalesced,
		Delivered:  s.delivered,
	}
}

func (s *intentScheduler) Reset() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.queue = nil
	s.recent = [10]string{}
	s.recentIdx = 0
	s.delivered = 0
	s.droppedTTL = 0
	s.coalesced = 0
}
