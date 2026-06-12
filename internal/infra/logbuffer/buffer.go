// Package logbuffer provides an in-memory ring buffer for capturing log output.
package logbuffer

import (
	"io"
	"strings"
	"sync"
	"time"
)

// Entry is a single captured log line.
type Entry struct {
	Level   string `json:"level"`   // debug|info|warn|error
	Source  string `json:"source"`  // extracted prefix like "[Runtime]"
	Message string `json:"message"` // the full log line
	Time    int64  `json:"time"`    // unix milliseconds
}

// Buffer is a thread-safe ring buffer of log entries.
type Buffer struct {
	mu      sync.RWMutex
	entries []Entry
	max     int
	pos     int
	total   int64
}

// New creates a buffer holding up to max entries.
func New(max int) *Buffer {
	return &Buffer{entries: make([]Entry, max), max: max}
}

// Write implements io.Writer for use with log.SetOutput.
func (b *Buffer) Write(p []byte) (n int, err error) {
	line := strings.TrimSpace(string(p))
	if line == "" {
		return len(p), nil
	}
	level := "info"
	lineLower := strings.ToLower(line)
	if strings.Contains(lineLower, "error") || strings.Contains(lineLower, "fail") {
		level = "error"
	} else if strings.Contains(lineLower, "warn") {
		level = "warn"
	} else if strings.Contains(lineLower, "debug") {
		level = "debug"
	}

	source := ""
	if idx := strings.Index(line, "] "); idx > 0 && strings.HasPrefix(line, "[") {
		source = line[1:idx]
	}

	entry := Entry{
		Level:   level,
		Source:  source,
		Message: line,
		Time:    time.Now().UnixMilli(),
	}

	b.mu.Lock()
	b.entries[b.pos%b.max] = entry
	b.pos++
	b.total++
	b.mu.Unlock()

	return len(p), nil
}

// Entries returns all buffered entries in chronological order.
func (b *Buffer) Entries() []Entry {
	b.mu.RLock()
	defer b.mu.RUnlock()

	n := b.pos
	if n > b.max {
		n = b.max
	}
	out := make([]Entry, n)
	for i := 0; i < n; i++ {
		idx := (b.pos - n + i) % b.max
		if idx < 0 {
			idx += b.max
		}
		out[i] = b.entries[idx]
	}
	return out
}

// Writer returns an io.Writer that writes to this buffer (for log.SetOutput).
func (b *Buffer) Writer() io.Writer {
	return b
}

var _ io.Writer = (*Buffer)(nil)
