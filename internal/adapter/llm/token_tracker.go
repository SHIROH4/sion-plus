package llm

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/shirohania/sion/internal/port"
)

var _ port.TokenUsageTracker = (*TokenTracker)(nil)

// tokenRecord is a single LLM call entry, written as one JSON line.
type tokenRecord struct {
	CallType         string `json:"call_type"`
	PromptTokens     int    `json:"prompt_tokens"`
	CompletionTokens int    `json:"completion_tokens"`
	Timestamp        int64  `json:"timestamp"`
}

// TokenTracker records LLM token usage with non-blocking writes and async JSONL flush.
type TokenTracker struct {
	mu      sync.Mutex
	buffer  []tokenRecord
	flushCh chan struct{}
	stopCh  chan struct{}
	done    chan struct{}
	dataDir string
	logger  *slog.Logger
}

const (
	maxBufferSize = 100
	flushInterval = 60 * time.Second
)

// NewTokenTracker creates a token tracker that writes to <dataDir>/token_usage/.
func NewTokenTracker(dataDir string) *TokenTracker {
	return &TokenTracker{
		buffer:  make([]tokenRecord, 0, maxBufferSize),
		flushCh: make(chan struct{}, 1),
		stopCh:  make(chan struct{}),
		done:    make(chan struct{}),
		dataDir: dataDir,
		logger:  slog.Default().With("module", "token_tracker"),
	}
}

// Record logs an LLM call. Non-blocking — writes to buffer, signals flush if full.
func (t *TokenTracker) Record(ctx context.Context, callType string, promptTokens, completionTokens int) {
	r := tokenRecord{
		CallType:         callType,
		PromptTokens:     promptTokens,
		CompletionTokens: completionTokens,
		Timestamp:        time.Now().Unix(),
	}

	t.mu.Lock()
	t.buffer = append(t.buffer, r)
	n := len(t.buffer)
	t.mu.Unlock()

	if n >= maxBufferSize {
		// Non-blocking signal
		select {
		case t.flushCh <- struct{}{}:
		default:
		}
	}
}

// Start begins the background flush goroutine.
func (t *TokenTracker) Start(ctx context.Context) error {
	go t.flushLoop()
	t.logger.Info("started")
	return nil
}

// Stop gracefully shuts down, flushing remaining records. Idempotent.
func (t *TokenTracker) Stop(ctx context.Context) error {
	select {
	case <-t.stopCh:
		// Already stopped
		return nil
	default:
		close(t.stopCh)
	}
	<-t.done // wait for flushLoop to exit
	t.logger.Info("stopped")
	return nil
}

// Summary returns aggregated usage since the given timestamp (unix seconds, 0=all).
func (t *TokenTracker) Summary(ctx context.Context, since int64) (*port.TokenUsageSummary, error) {
	summary := &port.TokenUsageSummary{
		ByType:      make(map[string]port.TypeStats),
		Since:       since,
		GeneratedAt: time.Now().Unix(),
	}

	// Scan all files in token_usage directory
	tokenDir := filepath.Join(t.dataDir, "token_usage")
	entries, err := os.ReadDir(tokenDir)
	if err != nil {
		if os.IsNotExist(err) {
			return summary, nil
		}
		return nil, err
	}

	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".jsonl" {
			continue
		}
		t.scanFile(filepath.Join(tokenDir, entry.Name()), since, summary)
	}

	return summary, nil
}

// TodaySummary returns today's usage only.
func (t *TokenTracker) TodaySummary(ctx context.Context) (*port.TokenUsageSummary, error) {
	todayStart := time.Now().Truncate(24 * time.Hour).Unix()
	return t.Summary(ctx, todayStart)
}

// ── Internal ──

func (t *TokenTracker) flushLoop() {
	defer close(t.done)

	ticker := time.NewTicker(flushInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			t.flush()
		case <-t.flushCh:
			t.flush()
		case <-t.stopCh:
			t.flush() // final flush before exit
			return
		}
	}
}

func (t *TokenTracker) flush() {
	t.mu.Lock()
	if len(t.buffer) == 0 {
		t.mu.Unlock()
		return
	}
	records := t.buffer
	t.buffer = make([]tokenRecord, 0, maxBufferSize)
	t.mu.Unlock()

	path := t.todayPath()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.logger.Warn("failed to create token_usage dir", "err", err)
		return
	}

	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		t.logger.Warn("failed to open token_usage file", "err", err)
		return
	}
	defer f.Close()

	enc := json.NewEncoder(f)
	for _, r := range records {
		if err := enc.Encode(r); err != nil {
			t.logger.Warn("failed to write token record", "err", err)
		}
	}

	t.logger.Debug("flushed", "records", len(records))
}

func (t *TokenTracker) todayPath() string {
	return filepath.Join(t.dataDir, "token_usage", time.Now().Format("2006-01-02")+".jsonl")
}

func (t *TokenTracker) scanFile(path string, since int64, summary *port.TokenUsageSummary) {
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}

	lines := splitJSONLLines(data)
	for _, line := range lines {
		if len(line) == 0 {
			continue
		}
		var r tokenRecord
		if err := json.Unmarshal(line, &r); err != nil {
			continue
		}
		if since > 0 && r.Timestamp < since {
			continue
		}

		summary.TotalPrompt += r.PromptTokens
		summary.TotalCompletion += r.CompletionTokens
		summary.TotalCalls++

		ts := summary.ByType[r.CallType]
		ts.Calls++
		ts.PromptTokens += r.PromptTokens
		ts.CompletionTokens += r.CompletionTokens
		summary.ByType[r.CallType] = ts
	}
}

func splitJSONLLines(data []byte) [][]byte {
	var lines [][]byte
	start := 0
	for i := 0; i < len(data); i++ {
		if data[i] == '\n' {
			if i > start {
				lines = append(lines, data[start:i])
			}
			start = i + 1
		}
	}
	if start < len(data) {
		lines = append(lines, data[start:])
	}
	return lines
}
