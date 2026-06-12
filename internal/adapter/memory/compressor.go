package memory

import (
	"context"
	"fmt"
	"time"

	"github.com/SHIROH4/sion-plus/internal/domain/types"
)

// Compressor implements the Flush-then-Compress pipeline for L0 working memory.
// Phase 1 (Flush): extracts durable information into L1 before compression.
// Phase 2 (Compress): summarizes middle messages, preserves head and tail.
//
// LLM dependency: Phase 1 and Phase 2 both require an LLM call.
// Until LLMExecutor is wired, the compressor uses a simple concatenation fallback.
type Compressor struct {
	buffer *SessionBuffer

	// LLM callbacks (injected when LLMExecutor is available)
	// Each returns (result, error). nil means "not wired yet, use fallback".
	onFlush    func(ctx context.Context, messages []types.Message) ([]types.FactEntry, error)
	onCompress func(ctx context.Context, messages []types.Message) (string, error)
}

// CompressorConfig tunes compression behaviour.
type CompressorConfig struct {
	Threshold    int           // messages > this triggers compression (default 20)
	HeadKeep     int           // protect first N raw messages (default 3)
	TailKeep     int           // protect last N raw messages (default 4)
	MemoMaxChars int           // compression memo max characters (default 300)
	StaleMemo    time.Duration // memo older than this gets a stale hint (default 1h)
}

// DefaultCompressorConfig returns sensible defaults.
func DefaultCompressorConfig() CompressorConfig {
	return CompressorConfig{
		Threshold:    20,
		HeadKeep:     3,
		TailKeep:     4,
		MemoMaxChars: 300,
		StaleMemo:    1 * time.Hour,
	}
}

// NewCompressor creates a compressor bound to a session buffer.
func NewCompressor(buffer *SessionBuffer, cfg CompressorConfig) *Compressor {
	return &Compressor{
		buffer: buffer,
	}
}

// SetFlushHook injects the LLM-powered flush function.
// Called when LLMExecutor is available.
func (c *Compressor) SetFlushHook(fn func(ctx context.Context, messages []types.Message) ([]types.FactEntry, error)) {
	c.onFlush = fn
}

// SetCompressHook injects the LLM-powered compress function.
func (c *Compressor) SetCompressHook(fn func(ctx context.Context, messages []types.Message) (string, error)) {
	c.onCompress = fn
}

// ── Compression ─────────────────────────────────────────────────────

// CompressResult contains the output of a compression cycle.
type CompressResult struct {
	Memo        string           // the compression summary text
	TrimmedMsgs []types.Message  // messages removed from L0 (for archiving)
	NewFacts    []types.FactEntry // facts extracted during flush (Phase 1)
	Flushed     bool             // whether Phase 1 ran
	Compressed  bool             // whether Phase 2 ran successfully
}

// Run executes a full Flush-then-Compress cycle.
// Returns nil if compression is not needed.
func (c *Compressor) Run(ctx context.Context, cfg CompressorConfig) (*CompressResult, error) {
	if !c.buffer.NeedsCompression(cfg.Threshold) {
		return nil, nil
	}

	result := &CompressResult{}

	// ── Phase 1: Flush ──
	// Extract durable facts from the entire L0 buffer before compression.
	// "Flush before compress" ensures compression never loses permanent knowledge.
	allMsgs := c.buffer.All()
	if len(allMsgs) > 0 && c.onFlush != nil {
		facts, err := c.onFlush(ctx, allMsgs)
		if err != nil {
			// Flush failure is not fatal — proceed with compression anyway.
			// The messages are still in L0 and chat_history.
			result.Flushed = false
		} else {
			result.NewFacts = facts
			result.Flushed = true
		}
	}

	// ── Phase 2: Compress ──
	// Protect head+tail raw messages, compress the middle.
	rawMsgs := c.buffer.Recent(0) // non-expired raw messages, excludes memo
	if len(rawMsgs) <= cfg.HeadKeep+cfg.TailKeep {
		// Not enough messages to compress — just trim excess
		trimmed := c.buffer.TrimTo(cfg.HeadKeep + cfg.TailKeep)
		result.TrimmedMsgs = trimmed
		return result, nil
	}

	head := rawMsgs[:cfg.HeadKeep]
	tail := rawMsgs[len(rawMsgs)-cfg.TailKeep:]
	middle := rawMsgs[cfg.HeadKeep : len(rawMsgs)-cfg.TailKeep]

	memo, err := c.compressMiddle(ctx, middle)
	if err != nil || memo == "" {
		// Compression failed — keep messages, skip this cycle
		result.Compressed = false
		return result, nil
	}
	result.Memo = memo
	result.Compressed = true

	// Rebuild L0: head + memo + tail
	trimmed := c.buffer.TrimTo(0) // remove all raw messages
	result.TrimmedMsgs = trimmed

	// Put head and tail back
	for _, m := range head {
		c.buffer.Append(m)
	}
	c.buffer.SetMemo(memo)
	for _, m := range tail {
		c.buffer.Append(m)
	}

	return result, nil
}

// ── Internal ─────────────────────────────────────────────────────────

func (c *Compressor) compressMiddle(ctx context.Context, messages []types.Message) (string, error) {
	if c.onCompress != nil {
		return c.onCompress(ctx, messages)
	}
	// Fallback: simple concatenation of first N chars per message
	return fallbackCompress(messages, 300), nil
}

// fallbackCompress builds a naive summary by taking the first 40 chars
// of each message. Used when no LLM hook is wired.
func fallbackCompress(msgs []types.Message, maxChars int) string {
	if len(msgs) == 0 {
		return ""
	}
	var result string
	for _, m := range msgs {
		prefix := m.Content
		if len(prefix) > 40 {
			prefix = prefix[:40] + "..."
		}
		role := string(m.Role)
		if role == "user" {
			role = "U"
		} else if role == "assistant" {
			role = "A"
		}
		line := fmt.Sprintf("[%s] %s\n", role, prefix)
		if len(result)+len(line) > maxChars {
			break
		}
		result += line
	}
	return result
}

// ── Stale memo ──────────────────────────────────────────────────────

// StaleHint returns a prompt suffix to inject during next compression
// when the current memo is older than StaleMemo. Moves old content to
// a "past block" section so it doesn't dominate the active context.
func (c *Compressor) StaleHint(cfg CompressorConfig) string {
	if !c.buffer.IsMemoStale(cfg.StaleMemo) {
		return ""
	}
	age := c.buffer.MemoAge()
	hours := int(age / 3600)
	return fmt.Sprintf(
		"\n[注意: 当前上下文摘要已过时约 %dh。请将明显过时的内容移至摘要末尾的\"较久前\"段落。]\n",
		hours,
	)
}
