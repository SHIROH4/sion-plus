package memory

import (
	"testing"
	"time"

	"github.com/shirohania/sion/internal/domain/types"
	"github.com/shirohania/sion/internal/port"
)

func TestAppendAndRecent(t *testing.T) {
	b := NewSessionBuffer(10, time.Hour)

	b.Append(types.Message{Role: types.RoleUser, Content: "hello"})
	b.Append(types.Message{Role: types.RoleAssistant, Content: "hi there"})

	recent := b.Recent(2)
	if len(recent) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(recent))
	}
	if recent[0].Content != "hello" {
		t.Errorf("first msg: %q", recent[0].Content)
	}
	if recent[1].Content != "hi there" {
		t.Errorf("second msg: %q", recent[1].Content)
	}
	if b.Len() != 2 {
		t.Errorf("Len: got %d, want 2", b.Len())
	}
}

func TestRecentLimit(t *testing.T) {
	b := NewSessionBuffer(10, time.Hour)
	for i := 0; i < 5; i++ {
		b.Append(types.Message{Role: types.RoleUser, Content: "msg"})
	}

	r := b.Recent(3)
	if len(r) != 3 {
		t.Errorf("Recent(3): got %d, want 3", len(r))
	}
}

func TestAll(t *testing.T) {
	b := NewSessionBuffer(10, time.Hour)
	b.Append(types.Message{Role: types.RoleUser, Content: "a"})
	b.Append(types.Message{Role: types.RoleAssistant, Content: "b"})
	b.SetMemo("summary of earlier messages")

	all := b.All()
	// 2 raw + 1 memo
	if len(all) != 3 {
		t.Fatalf("All: got %d, want 3", len(all))
	}
	if all[2].Role != types.RoleSystem {
		t.Errorf("last should be system memo, got %s", all[2].Role)
	}
}

func TestRecentExcludesMemo(t *testing.T) {
	b := NewSessionBuffer(10, time.Hour)
	b.Append(types.Message{Role: types.RoleUser, Content: "hello"})
	b.SetMemo("compressed")

	r := b.Recent(10)
	if len(r) != 1 {
		t.Errorf("Recent should exclude memo: got %d, want 1", len(r))
	}
}

func TestRingBufferWrap(t *testing.T) {
	b := NewSessionBuffer(5, time.Hour)

	// Fill beyond capacity
	for i := 0; i < 8; i++ {
		b.Append(types.Message{Role: types.RoleUser, Content: "msg", CreatedAt: time.Now().Unix()})
	}

	r := b.Recent(0)
	// Buffer capacity is 5, so only last 5 survive
	if len(r) != 5 {
		t.Errorf("wrap: got %d, want 5", len(r))
	}
}

func TestTimeEviction(t *testing.T) {
	b := NewSessionBuffer(10, 1*time.Second)

	now := time.Now()
	b.Append(types.Message{Role: types.RoleUser, Content: "recent", CreatedAt: now.Unix()})
	b.Append(types.Message{Role: types.RoleUser, Content: "old", CreatedAt: now.Add(-time.Hour).Unix()})

	time.Sleep(2 * time.Second) // cross second boundary

	r := b.Recent(0)
	if len(r) != 0 {
		t.Errorf("all messages should have expired after sleep: got %d", len(r))
	}
}

func TestTimeEvictionPreservesRecent(t *testing.T) {
	b := NewSessionBuffer(10, 5*time.Second)

	now := time.Now()
	b.Append(types.Message{Role: types.RoleUser, Content: "recent", CreatedAt: now.Unix()})
	b.Append(types.Message{Role: types.RoleUser, Content: "old", CreatedAt: now.Add(-time.Hour).Unix()})

	r := b.Recent(0)
	if len(r) != 1 {
		t.Fatalf("only 1 should survive: got %d", len(r))
	}
	if r[0].Content != "recent" {
		t.Errorf("survivor: %q", r[0].Content)
	}
}

func TestClear(t *testing.T) {
	b := NewSessionBuffer(10, time.Hour)
	b.Append(types.Message{Role: types.RoleUser, Content: "msg"})
	b.SetMemo("memo")

	b.Clear()

	if b.Len() != 0 {
		t.Errorf("Len after Clear: got %d, want 0", b.Len())
	}
	if b.Memo() != nil {
		t.Error("Memo should be nil after Clear")
	}
}

func TestMemoAge(t *testing.T) {
	b := NewSessionBuffer(10, time.Hour)
	b.SetMemo("compressed summary")

	age := b.MemoAge()
	if age < 0 || age > 1.0 {
		t.Errorf("MemoAge should be ~0, got %f", age)
	}

	// After SetMemo, age should be close to 0
	if b.IsMemoStale(1 * time.Hour) {
		t.Error("fresh memo should not be stale")
	}
}

func TestNeedsCompression(t *testing.T) {
	b := NewSessionBuffer(30, time.Hour)
	threshold := 20

	for i := 0; i < 15; i++ {
		b.Append(types.Message{Role: types.RoleUser, Content: "msg"})
	}
	if b.NeedsCompression(threshold) {
		t.Error("15 messages should not trigger compression")
	}

	for i := 0; i < 10; i++ {
		b.Append(types.Message{Role: types.RoleUser, Content: "msg"})
	}
	if !b.NeedsCompression(threshold) {
		t.Error("25 messages should trigger compression")
	}
}

func TestTrimTo(t *testing.T) {
	b := NewSessionBuffer(30, time.Hour)
	for i := 0; i < 20; i++ {
		b.Append(types.Message{Role: types.RoleUser, Content: "msg", CreatedAt: time.Now().Unix()})
	}
	b.SetMemo("memo")

	trimmed := b.TrimTo(5)
	if len(trimmed) != 15 {
		t.Errorf("TrimTo(5) from 20: trimmed %d, want 15", len(trimmed))
	}
	r := b.Recent(0)
	if len(r) != 5 {
		t.Errorf("after TrimTo: raw %d, want 5", len(r))
	}
	if b.Memo() == nil {
		t.Error("memo should survive TrimTo")
	}
}

func TestSessionBufferConcurrent(t *testing.T) {
	b := NewSessionBuffer(100, time.Hour)

	done := make(chan bool)
	for i := 0; i < 10; i++ {
		go func() {
			for j := 0; j < 100; j++ {
				b.Append(types.Message{Role: types.RoleUser, Content: "msg"})
				b.Recent(5)
				b.Len()
				b.All()
			}
			done <- true
		}()
	}
	for i := 0; i < 10; i++ {
		<-done
	}
	// No crash = pass
}

func TestEmptyBuffer(t *testing.T) {
	b := NewSessionBuffer(10, time.Hour)

	if b.Len() != 0 {
		t.Errorf("empty Len: got %d", b.Len())
	}
	r := b.Recent(5)
	if len(r) != 0 {
		t.Errorf("empty Recent: got %d", len(r))
	}
	all := b.All()
	if len(all) != 0 {
		t.Errorf("empty All: got %d", len(all))
	}
	if b.Memo() != nil {
		t.Error("empty Memo should be nil")
	}
	if b.MemoAge() != -1 {
		t.Errorf("empty MemoAge: got %f", b.MemoAge())
	}
}

func TestStats(t *testing.T) {
	b := NewSessionBuffer(30, time.Hour)
	b.Append(types.Message{Role: types.RoleUser, Content: "a"})
	b.Append(types.Message{Role: types.RoleAssistant, Content: "b"})
	b.SetMemo("summary")

	s := b.Stats()
	if s.Capacity != 30 {
		t.Errorf("Capacity: %d", s.Capacity)
	}
	if s.LiveMessages != 2 {
		t.Errorf("LiveMessages: %d", s.LiveMessages)
	}
	if !s.HasMemo {
		t.Error("HasMemo should be true")
	}
	if s.TotalAppended != 2 {
		t.Errorf("TotalAppended: %d", s.TotalAppended)
	}
}

func TestAutoTimestamp(t *testing.T) {
	b := NewSessionBuffer(10, time.Hour)
	before := time.Now().Unix()
	b.Append(types.Message{Role: types.RoleUser, Content: "no timestamp"})
	after := time.Now().Unix()

	r := b.Recent(1)
	if r[0].CreatedAt < before || r[0].CreatedAt > after+1 {
		t.Errorf("auto timestamp out of range: %d not in [%d, %d]", r[0].CreatedAt, before, after)
	}
}

func TestSessionBufferInterface(t *testing.T) {
	// Compile-time check already done via var _ port.SessionBuffer
	var sb port.SessionBuffer = NewSessionBuffer(10, time.Hour)
	sb.Append(types.Message{Role: types.RoleUser, Content: "test"})
	if sb.Len() != 1 {
		t.Error("interface check failed")
	}
	sb.Clear()
	if sb.Len() != 0 {
		t.Error("clear failed")
	}
}
