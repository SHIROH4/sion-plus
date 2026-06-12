package event

import (
	"sync"
	"testing"

	"github.com/shirohania/sion/internal/port"
)

func TestPublishSubscribe(t *testing.T) {
	bus := NewEventBus()
	var received string
	var mu sync.Mutex

	bus.Subscribe("test:topic", func(payload any) {
		mu.Lock()
		received = payload.(string)
		mu.Unlock()
	})

	bus.Publish("test:topic", "hello")

	mu.Lock()
	if received != "hello" {
		t.Errorf("expected 'hello', got %q", received)
	}
	mu.Unlock()
}

func TestNoSubscriberNoPanic(t *testing.T) {
	bus := NewEventBus()
	// Publishing to a topic with no subscribers should not panic
	bus.Publish("no_one_listening", "anything")
}

func TestSubscriberPanicRecovery(t *testing.T) {
	bus := NewEventBus()

	// First subscriber panics
	bus.Subscribe("panic:topic", func(payload any) {
		panic("intentional panic")
	})

	// Second subscriber should still receive
	received := false
	bus.Subscribe("panic:topic", func(payload any) {
		received = true
	})

	bus.Publish("panic:topic", "test")

	if !received {
		t.Error("second subscriber should have received despite first panicking")
	}
}

func TestUnsubscribe(t *testing.T) {
	bus := NewEventBus()

	count := 0
	unsub := bus.Subscribe("count", func(payload any) {
		count++
	})

	bus.Publish("count", "first")
	unsub()
	bus.Publish("count", "second")

	if count != 1 {
		t.Errorf("expected count=1 after unsubscribe, got %d", count)
	}
}

func TestUnsubscribeIdempotent(t *testing.T) {
	bus := NewEventBus()
	unsub := bus.Subscribe("x", func(any) {})
	unsub()
	unsub() // second call should not panic
}

func TestSubscribePattern(t *testing.T) {
	bus := NewEventBus()

	var received []string
	var mu sync.Mutex

	bus.SubscribePattern("emotion:", func(topic string, payload any) {
		mu.Lock()
		received = append(received, topic)
		mu.Unlock()
	})

	bus.Publish("emotion:changed", "payload1")
	bus.Publish("emotion:spike", "payload2")
	bus.Publish("not:matching", "payload3")

	mu.Lock()
	if len(received) != 2 {
		t.Errorf("expected 2 pattern-matched events, got %d: %v", len(received), received)
	}
	mu.Unlock()
}

func TestExactAndPatternTogether(t *testing.T) {
	bus := NewEventBus()

	exactCount := 0
	patternCount := 0

	bus.Subscribe("a:b", func(any) { exactCount++ })
	bus.SubscribePattern("a:", func(topic string, payload any) { patternCount++ })

	bus.Publish("a:b", nil)

	if exactCount != 1 {
		t.Errorf("exact subscriber missed: got %d", exactCount)
	}
	if patternCount != 1 {
		t.Errorf("pattern subscriber missed: got %d", patternCount)
	}
}

func TestConcurrentPublish(t *testing.T) {
	bus := NewEventBus()
	count := 0
	var mu sync.Mutex

	bus.Subscribe("concurrent", func(any) {
		mu.Lock()
		count++
		mu.Unlock()
	})

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			bus.Publish("concurrent", nil)
		}()
	}
	wg.Wait()

	mu.Lock()
	if count != 100 {
		t.Errorf("expected 100 concurrent publishes, got %d", count)
	}
	mu.Unlock()
}

func TestPortCompliance(t *testing.T) {
	// Verify EventBusImpl satisfies port.EventBus at compile time
	var _ port.EventBus = NewEventBus()
}
