package llm

import (
	"context"
	"log"
	"sync"
	"time"

	"github.com/SHIROH4/sion-plus/internal/port"
)

// RateLimiter is a simple token bucket that limits LLM calls per interval.
type RateLimiter struct {
	mu        sync.Mutex
	tokens    float64
	capacity  float64
	rate      float64 // tokens per second
	lastCheck time.Time
}

// NewRateLimiter creates a token bucket with the given rate and burst capacity.
//   - callsPerMinute: sustained rate (e.g. 10 = 10 calls/minute)
//   - burst: max burst size (e.g. 3 = allow 3 immediate calls)
func NewRateLimiter(callsPerMinute int, burst int) *RateLimiter {
	return &RateLimiter{
		tokens:    float64(burst),
		capacity:  float64(burst),
		rate:      float64(callsPerMinute) / 60.0,
		lastCheck: time.Now(),
	}
}

// Allow returns true if a call is allowed right now. Consumes 1 token.
func (r *RateLimiter) Allow() bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	now := time.Now()
	elapsed := now.Sub(r.lastCheck).Seconds()
	r.lastCheck = now

	// Refill
	r.tokens += elapsed * r.rate
	if r.tokens > r.capacity {
		r.tokens = r.capacity
	}

	if r.tokens >= 1.0 {
		r.tokens -= 1.0
		return true
	}
	return false
}

// Wait blocks until a token is available or ctx is cancelled.
func (r *RateLimiter) Wait(ctx context.Context) error {
	for {
		if r.Allow() {
			return nil
		}
		waitTime := time.Duration((1.0-r.tokens)/r.rate) * time.Second
		if waitTime > 30*time.Second {
			waitTime = 30 * time.Second
		}
		if waitTime < 100*time.Millisecond {
			waitTime = 100 * time.Millisecond
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(waitTime):
		}
	}
}

// RateLimitedExecutor wraps an LLMExecutor with per-call rate limiting.
type RateLimitedExecutor struct {
	inner   port.LLMExecutor
	limiter *RateLimiter
	label   string
}

var _ port.LLMExecutor = (*RateLimitedExecutor)(nil)

// WrapRateLimited wraps an executor with rate limiting.
func WrapRateLimited(inner port.LLMExecutor, callsPerMinute, burst int, label string) *RateLimitedExecutor {
	log.Printf("[RateLimiter] %s: %d calls/min, burst=%d", label, callsPerMinute, burst)
	return &RateLimitedExecutor{
		inner:   inner,
		limiter: NewRateLimiter(callsPerMinute, burst),
		label:   label,
	}
}

func (e *RateLimitedExecutor) IsAvailable(ctx context.Context) bool {
	return e.inner.IsAvailable(ctx)
}

func (e *RateLimitedExecutor) Chat(ctx context.Context, systemPrompt string, msgs []port.LLMMessage) (string, error) {
	if err := e.limiter.Wait(ctx); err != nil {
		return "", err
	}
	return e.inner.Chat(ctx, systemPrompt, msgs)
}

func (e *RateLimitedExecutor) ChatWithTools(ctx context.Context, systemPrompt string, msgs []port.LLMMessage, tools []port.ToolDef, onToolCall func(string, string) string, maxRounds int, toolChoice string) (string, error) {
	if err := e.limiter.Wait(ctx); err != nil {
		return "", err
	}
	return e.inner.ChatWithTools(ctx, systemPrompt, msgs, tools, onToolCall, maxRounds, toolChoice)
}

func (e *RateLimitedExecutor) ChatStream(ctx context.Context, systemPrompt string, msgs []port.LLMMessage, onChunk func(string) error) error {
	if err := e.limiter.Wait(ctx); err != nil {
		return err
	}
	return e.inner.ChatStream(ctx, systemPrompt, msgs, onChunk)
}
