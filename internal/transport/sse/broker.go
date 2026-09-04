package sse

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"
)

// Event is a single SSE event published to a topic.
type Event struct {
	Topic string
	Data  any
}

type client struct {
	ch     chan Event
	topics map[string]struct{}
}

// Broker manages SSE client connections with topic-based fanout.
type Broker struct {
	mu                sync.RWMutex
	clients           map[*client]struct{}
	register          chan *client
	remove            chan *client
	pub               chan Event
	ctx               context.Context
	cancel            context.CancelFunc
	HeartbeatInterval time.Duration // default 30s if zero
}

// NewBroker creates a broker ready to start.
func NewBroker() *Broker {
	return &Broker{
		clients:  make(map[*client]struct{}),
		register: make(chan *client),
		remove:   make(chan *client),
		pub:      make(chan Event, 256),
	}
}

// Start begins the broker's event loop. Call before serving connections.
func (b *Broker) Start(ctx context.Context) {
	b.ctx, b.cancel = context.WithCancel(ctx)
	go b.loop()
	log.Println("[SSE] broker started")
}

// Stop shuts down the broker and disconnects all clients.
func (b *Broker) Stop() {
	if b.cancel != nil {
		b.cancel()
	}
}

// Publish sends an event to all clients subscribed to the topic.
func (b *Broker) Publish(topic string, data any) {
	select {
	case b.pub <- Event{Topic: topic, Data: data}:
	default:
	}
}

func (b *Broker) loop() {
	interval := b.HeartbeatInterval
	if interval <= 0 {
		interval = 30 * time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-b.ctx.Done():
			b.mu.Lock()
			for c := range b.clients {
				close(c.ch)
			}
			b.clients = make(map[*client]struct{})
			b.mu.Unlock()
			return

		case c := <-b.register:
			b.mu.Lock()
			b.clients[c] = struct{}{}
			b.mu.Unlock()

		case c := <-b.remove:
			b.mu.Lock()
			if _, ok := b.clients[c]; ok {
				close(c.ch)
				delete(b.clients, c)
			}
			b.mu.Unlock()

		case evt := <-b.pub:
			b.mu.RLock()
			for c := range b.clients {
				if _, ok := c.topics[evt.Topic]; ok {
					select {
					case c.ch <- evt:
					default:
					}
				}
			}
			b.mu.RUnlock()

		case <-ticker.C:
			b.mu.RLock()
			for c := range b.clients {
				select {
				case c.ch <- Event{Topic: "heartbeat", Data: nil}:
				default:
				}
			}
			b.mu.RUnlock()
		}
	}
}

// ServeHTTP handles an SSE connection. Reads ?topics=chat,emotion from query.
func (b *Broker) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// SSE connections are intentionally long-lived. Clear the server-wide
	// WriteTimeout for this response so an otherwise healthy subscription is
	// not terminated while waiting for future events.
	if err := http.NewResponseController(w).SetWriteDeadline(time.Time{}); err != nil && !errors.Is(err, http.ErrNotSupported) {
		log.Printf("[SSE] clear write deadline: %v", err)
	}

	topicSet := make(map[string]struct{})
	if raw := r.URL.Query().Get("topics"); raw != "" {
		for _, t := range strings.Split(raw, ",") {
			t = strings.TrimSpace(t)
			if t != "" {
				topicSet[t] = struct{}{}
			}
		}
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	c := &client{
		ch:     make(chan Event, 64),
		topics: topicSet,
	}

	select {
	case b.register <- c:
	case <-b.ctx.Done():
		return
	}

	defer func() {
		select {
		case b.remove <- c:
		case <-b.ctx.Done():
		}
	}()

	ctx := r.Context()
	for {
		select {
		case <-ctx.Done():
			return
		case <-b.ctx.Done():
			return
		case evt, ok := <-c.ch:
			if !ok {
				return
			}
			if evt.Topic == "heartbeat" {
				fmt.Fprintf(w, ": heartbeat\n\n")
				flusher.Flush()
				continue
			}
			data, err := json.Marshal(evt.Data)
			if err != nil {
				continue
			}
			fmt.Fprintf(w, "event: %s\ndata: %s\n\n", evt.Topic, data)
			flusher.Flush()
		}
	}
}
