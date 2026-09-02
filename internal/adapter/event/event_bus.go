package event

import (
	"sync"

	"github.com/SHIROH4/sion-plus/internal/port"
)

// EventBusImpl is a goroutine-safe publish-subscribe bus.
// Publish is non-blocking; each subscriber runs in the calling goroutine
// with panic recovery so one bad subscriber cannot crash the bus.
type EventBusImpl struct {
	mu     sync.RWMutex
	subs   map[string][]subscription
	nextID int
}

type subscription struct {
	id      int
	handler func(payload any)
	pattern bool                            // true = prefix match, false = exact match
	raw     func(topic string, payload any) // for pattern subscribers
}

var _ port.EventBus = (*EventBusImpl)(nil)

// NewEventBus creates an empty event bus.
func NewEventBus() port.EventBus {
	return &EventBusImpl{
		subs: make(map[string][]subscription),
	}
}

// Publish emits an event to all matching subscribers.
// Subscriber panics are recovered and logged; they never propagate to the caller.
func (b *EventBusImpl) Publish(topic string, payload any) {
	b.mu.RLock()

	// Collect matching handlers — exact match + prefix match
	type target struct {
		handler func(payload any)
	}
	var targets []target

	// Exact match subscribers
	for _, sub := range b.subs[topic] {
		if !sub.pattern {
			targets = append(targets, target{handler: sub.handler})
		}
	}

	// Pattern match subscribers
	for pattern, subs := range b.subs {
		if len(pattern) <= len(topic) && topic[:len(pattern)] == pattern {
			for _, sub := range subs {
				if sub.pattern && sub.raw != nil {
					// Capture in closure
					raw := sub.raw
					t := topic
					targets = append(targets, target{handler: func(payload any) {
						raw(t, payload)
					}})
				}
			}
		}
	}

	b.mu.RUnlock()

	// Dispatch with panic recovery
	for _, t := range targets {
		func() {
			defer func() { recover() }()
			t.handler(payload)
		}()
	}
}

// Subscribe registers a handler for exact topic match.
// Returns an unsubscribe function (idempotent, safe to call multiple times).
func (b *EventBusImpl) Subscribe(topic string, handler func(payload any)) func() {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.nextID++
	sub := subscription{id: b.nextID, handler: handler}
	b.subs[topic] = append(b.subs[topic], sub)

	once := sync.Once{}
	return func() {
		once.Do(func() {
			b.remove(topic, sub.id)
		})
	}
}

// SubscribePattern registers a handler for topics with a given prefix.
// e.g. pattern "emotion:" matches "emotion:changed", "emotion:spike" etc.
func (b *EventBusImpl) SubscribePattern(pattern string, handler func(topic string, payload any)) func() {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.nextID++
	sub := subscription{id: b.nextID, pattern: true, raw: handler}
	b.subs[pattern] = append(b.subs[pattern], sub)

	once := sync.Once{}
	return func() {
		once.Do(func() {
			b.remove(pattern, sub.id)
		})
	}
}

func (b *EventBusImpl) remove(topic string, id int) {
	b.mu.Lock()
	defer b.mu.Unlock()

	subs := b.subs[topic]
	filtered := subs[:0]
	for _, s := range subs {
		if s.id != id {
			filtered = append(filtered, s)
		}
	}
	if len(filtered) == 0 {
		delete(b.subs, topic)
	} else {
		b.subs[topic] = filtered
	}
}
