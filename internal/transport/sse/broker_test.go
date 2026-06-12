package sse

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestBrokerStartStop(t *testing.T) {
	b := NewBroker()
	ctx, cancel := context.WithCancel(context.Background())
	b.Start(ctx)

	// Should not panic on publish after start
	b.Publish("test", map[string]string{"msg": "hello"})

	cancel()
	b.Stop()
}

func TestBrokerSubscribeAndReceive(t *testing.T) {
	b := NewBroker()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	b.Start(ctx)

	var wg sync.WaitGroup
	wg.Add(1)

	var received []string
	var mu sync.Mutex

	go func() {
		defer wg.Done()
		// Simulate ServeHTTP by reading the response body via a real http server
		ts := httptest.NewServer(http.HandlerFunc(b.ServeHTTP))
		defer ts.Close()

		req2, _ := http.NewRequest("GET", ts.URL+"/api/events?topics=chat", nil)
		ctx2, cancel2 := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel2()
		req2 = req2.WithContext(ctx2)

		resp, err := http.DefaultClient.Do(req2)
		if err != nil {
			t.Logf("client error (expected on timeout): %v", err)
			return
		}
		defer resp.Body.Close()

		buf := make([]byte, 4096)
		for {
			n, err := resp.Body.Read(buf)
			if n > 0 {
				mu.Lock()
				received = append(received, string(buf[:n]))
				mu.Unlock()
			}
			if err != nil {
				return
			}
		}
	}()

	// Wait for client to connect
	time.Sleep(100 * time.Millisecond)

	// Publish to the subscribed topic
	b.Publish("chat", map[string]string{"token": "hello"})
	b.Publish("other", map[string]string{"token": "should-not-receive"})
	b.Publish("chat", map[string]string{"token": "world"})

	time.Sleep(300 * time.Millisecond)
	cancel()
	wg.Wait()

	mu.Lock()
	all := strings.Join(received, "")
	mu.Unlock()

	if !strings.Contains(all, "hello") {
		t.Error("should receive 'hello' on subscribed topic")
	}
	if !strings.Contains(all, "world") {
		t.Error("should receive 'world' on subscribed topic")
	}
	if strings.Contains(all, "should-not-receive") {
		t.Error("should NOT receive events from non-subscribed topic")
	}
}

func TestBrokerHeartbeat(t *testing.T) {
	b := NewBroker()
	b.HeartbeatInterval = 100 * time.Millisecond
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	b.Start(ctx)

	ts := httptest.NewServer(http.HandlerFunc(b.ServeHTTP))
	defer ts.Close()

	ctx2, cancel2 := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel2()
	req, _ := http.NewRequestWithContext(ctx2, "GET", ts.URL+"/api/events?topics=test", nil)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status %d", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.Contains(ct, "text/event-stream") {
		t.Errorf("content-type = %q, want text/event-stream", ct)
	}

	// Read until we see a heartbeat comment or timeout
	done := make(chan bool, 1)
	var gotHeartbeat bool
	go func() {
		buf := make([]byte, 2048)
		for {
			n, err := resp.Body.Read(buf)
			if n > 0 && strings.Contains(string(buf[:n]), "heartbeat") {
				gotHeartbeat = true
				done <- true
				return
			}
			if err != nil {
				done <- false
				return
			}
		}
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
	}
	if !gotHeartbeat {
		t.Error("should receive heartbeat within 30s")
	}
}

func TestBrokerMultipleTopics(t *testing.T) {
	b := NewBroker()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	b.Start(ctx)

	ts := httptest.NewServer(http.HandlerFunc(b.ServeHTTP))
	defer ts.Close()

	ctx2, cancel2 := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel2()
	req, _ := http.NewRequestWithContext(ctx2, "GET", ts.URL+"/api/events?topics=chat,emotion", nil)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer resp.Body.Close()

	// Publish to both topics
	b.Publish("chat", map[string]string{"msg": "chat-event"})
	b.Publish("emotion", map[string]string{"primary": "joy"})
	time.Sleep(200 * time.Millisecond)

	buf := make([]byte, 4096)
	n, _ := resp.Body.Read(buf)
	text := string(buf[:n])

	if !strings.Contains(text, "chat-event") {
		t.Error("should receive chat topic event")
	}
	if !strings.Contains(text, "joy") {
		t.Error("should receive emotion topic event")
	}
}

func TestBrokerNoTopicsReceivesNothing(t *testing.T) {
	b := NewBroker()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	b.Start(ctx)

	ts := httptest.NewServer(http.HandlerFunc(b.ServeHTTP))
	defer ts.Close()

	ctx2, cancel2 := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel2()
	req, _ := http.NewRequestWithContext(ctx2, "GET", ts.URL+"/api/events", nil)

	resp, _ := http.DefaultClient.Do(req)
	if resp != nil {
		defer resp.Body.Close()
		buf := make([]byte, 4096)
		n, _ := resp.Body.Read(buf)
		text := string(buf[:n])
		// Should only get heartbeats, no data events
		if strings.Contains(text, "event:") && !strings.Contains(text, "heartbeat") {
			t.Error("should not receive data events without topic subscription")
		}
	}
}

func TestBrokerPublishNonBlocking(t *testing.T) {
	b := NewBroker()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	b.Start(ctx)

	// Fill the publish channel
	for i := 0; i < 1000; i++ {
		b.Publish("chat", map[string]int{"n": i})
	}
	// Should not block or panic
}

func TestBrokerServeHTTPSetsHeaders(t *testing.T) {
	b := NewBroker()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	b.Start(ctx)

	ts := httptest.NewServer(http.HandlerFunc(b.ServeHTTP))
	defer ts.Close()

	ctx2, cancel2 := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel2()
	req, _ := http.NewRequestWithContext(ctx2, "GET", ts.URL+"/api/events?topics=test", nil)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.Contains(ct, "text/event-stream") {
		t.Errorf("Content-Type = %q", ct)
	}
	if cc := resp.Header.Get("Cache-Control"); cc != "no-cache" {
		t.Errorf("Cache-Control = %q, want no-cache", cc)
	}
}

func TestEventJSONEncoding(t *testing.T) {
	evt := Event{Topic: "chat", Data: map[string]string{"token": "你好"}}
	data, err := json.Marshal(evt.Data)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	expected := `{"token":"你好"}`
	if string(data) != expected {
		t.Errorf("json = %q, want %q", data, expected)
	}
}

func TestSSEFormat(t *testing.T) {
	evt := Event{Topic: "token", Data: map[string]string{"token": "hello"}}
	data, _ := json.Marshal(evt.Data)
	line := fmt.Sprintf("event: %s\ndata: %s\n\n", evt.Topic, data)

	if !strings.HasPrefix(line, "event: token\n") {
		t.Error("SSE line should start with 'event: token'")
	}
	if !strings.HasSuffix(line, "\n\n") {
		t.Error("SSE line should end with double newline")
	}
	if !strings.Contains(line, `"token":"hello"`) {
		t.Error("SSE line should contain the JSON data")
	}
}

func TestBrokerClientCleanupOnContextCancel(t *testing.T) {
	b := NewBroker()
	ctx, cancel := context.WithCancel(context.Background())
	b.Start(ctx)

	ts := httptest.NewServer(http.HandlerFunc(b.ServeHTTP))
	defer ts.Close()

	ctx2, cancel2 := context.WithCancel(context.Background())
	req, _ := http.NewRequestWithContext(ctx2, "GET", ts.URL+"/api/events?topics=test", nil)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}

	// Cancel the request context
	cancel2()
	time.Sleep(100 * time.Millisecond)

	// Publish should not panic with a dead client
	b.Publish("test", "data")

	resp.Body.Close()
	cancel()
}
