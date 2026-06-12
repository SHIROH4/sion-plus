package memory

import (
	"sync"
	"time"

	"github.com/SHIROH4/sion-plus/internal/domain/types"
	"github.com/SHIROH4/sion-plus/internal/port"
)

// SessionBuffer is the AI's working memory (L0).
// Ring buffer with time-based eviction. Thread-safe.
//
// Design (v2.0):
//   - Capacity: 40 slots (headroom for 20 messages + compression overhead)
//   - MaxAge:   30 minutes per message
//   - Compression memo: a SystemMessage injected after compression
//   - Recent(N): returns last N raw messages + active memo
//   - All():     returns everything (used by compressor)
type SessionBuffer struct {
	mu       sync.RWMutex
	buf      []types.Message
	head     int // write position
	size     int // number of messages currently stored
	capacity int

	// Time-based eviction
	maxAge time.Duration

	// Compression memo (SystemMessage, injected after Flush-then-Compress)
	memo      *types.Message
	memoStamp int64 // unix seconds, for stale detection

	// Stats
	totalAppended int64
}

// Default session buffer constants.
const (
	DefaultSessionCapacity = 40
	DefaultSessionMaxAge   = 30 * time.Minute
)

var _ port.SessionBuffer = (*SessionBuffer)(nil)

// NewSessionBuffer creates a ring buffer with the given capacity and max message age.
func NewSessionBuffer(capacity int, maxAge time.Duration) *SessionBuffer {
	if capacity <= 0 {
		capacity = DefaultSessionCapacity
	}
	if maxAge <= 0 {
		maxAge = DefaultSessionMaxAge
	}
	return &SessionBuffer{
		buf:      make([]types.Message, capacity),
		capacity: capacity,
		maxAge:   maxAge,
	}
}

// Append adds a message to the ring buffer. Thread-safe.
// If the buffer is full, the oldest message is overwritten.
func (b *SessionBuffer) Append(msg types.Message) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if msg.CreatedAt == 0 {
		msg.CreatedAt = time.Now().Unix()
	}

	b.buf[b.head] = msg
	b.head = (b.head + 1) % b.capacity
	if b.size < b.capacity {
		b.size++
	}
	b.totalAppended++
}

// Recent returns the last n raw messages (excluding compression memo).
// If n <= 0, returns all. Messages older than maxAge are filtered out.
// Thread-safe.
func (b *SessionBuffer) Recent(n int) []types.Message {
	b.mu.RLock()
	defer b.mu.RUnlock()

	cutoff := time.Now().Add(-b.maxAge).Unix()
	all := b.snapshot(cutoff)

	if n <= 0 || n >= len(all) {
		return all
	}
	start := len(all) - n
	if start < 0 {
		start = 0
	}
	return all[start:]
}

// All returns all non-expired messages including the compression memo.
// Thread-safe.
func (b *SessionBuffer) All() []types.Message {
	b.mu.RLock()
	defer b.mu.RUnlock()

	cutoff := time.Now().Add(-b.maxAge).Unix()
	msgs := b.snapshot(cutoff)

	// Append compression memo if present and not expired.
	if b.memo != nil && b.memo.CreatedAt >= cutoff {
		msgs = append(msgs, *b.memo)
	}
	return msgs
}

// Len returns the number of non-expired raw messages currently in the buffer.
// Thread-safe.
func (b *SessionBuffer) Len() int {
	return len(b.Recent(0))
}

// Clear empties the buffer and removes the compression memo.
// Thread-safe.
func (b *SessionBuffer) Clear() {
	b.mu.Lock()
	defer b.mu.Unlock()

	for i := range b.buf {
		b.buf[i] = types.Message{}
	}
	b.head = 0
	b.size = 0
	b.memo = nil
	b.memoStamp = 0
}

// ── Compression memo ────────────────────────────────────────────────

// SetMemo stores a compression memo (SystemMessage representing summarised history).
// Typically called after Flush-then-Compress completes.
func (b *SessionBuffer) SetMemo(content string) {
	b.mu.Lock()
	defer b.mu.Unlock()

	now := time.Now().Unix()
	b.memo = &types.Message{
		Role:      types.RoleSystem,
		Content:   content,
		CreatedAt: now,
	}
	b.memoStamp = now
}

// Memo returns the current compression memo, or nil if none.
func (b *SessionBuffer) Memo() *types.Message {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.memo
}

// MemoAge returns the age of the compression memo in seconds.
// Returns -1 if no memo is set.
func (b *SessionBuffer) MemoAge() float64 {
	b.mu.RLock()
	defer b.mu.RUnlock()
	if b.memo == nil {
		return -1
	}
	return time.Since(time.Unix(b.memoStamp, 0)).Seconds()
}

// IsMemoStale reports whether the memo is older than the given duration.
// Used to decide whether to inject a stale hint during the next compression.
func (b *SessionBuffer) IsMemoStale(staleAfter time.Duration) bool {
	age := b.MemoAge()
	return age >= 0 && age >= staleAfter.Seconds()
}

// ── Compression trigger ─────────────────────────────────────────────

// NeedsCompression reports whether the buffer has enough raw messages
// to trigger a Flush-then-Compress cycle. Default threshold = 20.
func (b *SessionBuffer) NeedsCompression(threshold int) bool {
	return b.Len() > threshold
}

// TrimTo keeps only the most recent n messages plus the memo.
// Used after compression to rebuild L0.
// Returns the trimmed messages (for archiving to chat_history).
func (b *SessionBuffer) TrimTo(keepRaw int) []types.Message {
	b.mu.Lock()
	defer b.mu.Unlock()

	cutoff := time.Now().Add(-b.maxAge).Unix()

	// Snapshot current raw messages
	raw := b.snapshotLocked(cutoff)

	var trimmed, kept []types.Message
	if len(raw) > keepRaw {
		trimmed = raw[:len(raw)-keepRaw]
		kept = raw[len(raw)-keepRaw:]
	} else {
		kept = raw
	}

	// Rebuild buffer with kept messages + memo
	b.buf = make([]types.Message, b.capacity)
	b.head = 0
	b.size = 0
	for _, m := range kept {
		b.buf[b.head] = m
		b.head = (b.head + 1) % b.capacity
		b.size++
	}

	return trimmed
}

// ── Stats ────────────────────────────────────────────────────────────

// Stats returns buffer statistics for debugging and monitoring.
func (b *SessionBuffer) Stats() SessionStats {
	b.mu.RLock()
	defer b.mu.RUnlock()

	cutoff := time.Now().Add(-b.maxAge).Unix()
	raw := b.snapshotLocked(cutoff)

	return SessionStats{
		Capacity:      b.capacity,
		Size:          b.size,
		LiveMessages:  len(raw),
		HasMemo:       b.memo != nil,
		MemoAge:       b.memoAgeLocked(),
		TotalAppended: b.totalAppended,
	}
}

type SessionStats struct {
	Capacity      int
	Size          int
	LiveMessages  int
	HasMemo       bool
	MemoAge       float64
	TotalAppended int64
}

// ── Internal helpers ────────────────────────────────────────────────

// snapshot returns non-expired raw messages in insertion order.
// Caller must hold at least RLock.
func (b *SessionBuffer) snapshot(cutoff int64) []types.Message {
	return b.snapshotLocked(cutoff)
}

func (b *SessionBuffer) snapshotLocked(cutoff int64) []types.Message {
	if b.size == 0 {
		return nil
	}

	out := make([]types.Message, 0, b.size)
	// Walk from oldest to newest
	start := b.head - b.size
	if start < 0 {
		start += b.capacity
	}
	for i := 0; i < b.size; i++ {
		idx := (start + i) % b.capacity
		m := b.buf[idx]
		if m.CreatedAt >= cutoff {
			out = append(out, m)
		}
	}
	return out
}

func (b *SessionBuffer) memoAgeLocked() float64 {
	if b.memo == nil {
		return -1
	}
	return time.Since(time.Unix(b.memoStamp, 0)).Seconds()
}
