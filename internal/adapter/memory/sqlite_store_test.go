package memory

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/SHIROH4/sion-plus/internal/domain/types"
	"github.com/SHIROH4/sion-plus/internal/port"
)

func newTestStore(t *testing.T) *SQLiteStore {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "test.db")
	store, err := NewSQLiteStore(path)
	if err != nil {
		t.Fatalf("NewSQLiteStore: %v", err)
	}
	t.Cleanup(func() { store.Close() })
	return store
}

func TestFactsCRUD(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)

	f := &types.FactEntry{
		Entity:       "master",
		RelationType: "preference",
		Content:      "likes Rust programming language",
		SourceTier:   types.SourceExplicit,
		TemporalScope: types.ScopePattern,
		Importance:   8,
		Source:       "chat",
		MemCellType:  "prefer",
		Evidence: types.MemoryEvidenceEntry{
			Reinforcement:    0.5,
			ReinLastSignalAt: time.Now().Unix(),
		},
	}

	err := store.SaveFact(ctx, f)
	if err != nil {
		t.Fatalf("SaveFact: %v", err)
	}
	if f.ID == 0 {
		t.Error("expected auto-generated ID after save")
	}

	// GetFact
	got, err := store.GetFact(ctx, f.ID)
	if err != nil {
		t.Fatalf("GetFact: %v", err)
	}
	if got.Content != f.Content {
		t.Errorf("Content: got %q, want %q", got.Content, f.Content)
	}
	if got.SourceTier != types.SourceExplicit {
		t.Errorf("SourceTier: got %q, want %q", got.SourceTier, types.SourceExplicit)
	}

	// UpdateFact
	got.Importance = 9
	got.Content = "loves Rust"
	err = store.UpdateFact(ctx, got)
	if err != nil {
		t.Fatalf("UpdateFact: %v", err)
	}
	updated, err := store.GetFact(ctx, f.ID)
	if err != nil {
		t.Fatalf("GetFact after update: %v", err)
	}
	if updated.Importance != 9 {
		t.Errorf("Importance: got %d, want 9", updated.Importance)
	}
	if updated.Content != "loves Rust" {
		t.Errorf("Content: got %q, want %q", updated.Content, "loves Rust")
	}

	// ListActiveFacts
	facts, err := store.ListActiveFacts(ctx, 0)
	if err != nil {
		t.Fatalf("ListActiveFacts: %v", err)
	}
	if len(facts) != 1 {
		t.Fatalf("expected 1 fact, got %d", len(facts))
	}

	// ArchiveFact
	err = store.ArchiveFact(ctx, f.ID)
	if err != nil {
		t.Fatalf("ArchiveFact: %v", err)
	}
	active, err := store.ListActiveFacts(ctx, 0)
	if err != nil {
		t.Fatalf("ListActiveFacts after archive: %v", err)
	}
	if len(active) != 0 {
		t.Errorf("expected 0 active facts after archive, got %d", len(active))
	}

	// SearchFactsByTimeRange: create facts at different times
	now := time.Now().Unix()
	f2 := &types.FactEntry{
		Entity:       "master",
		RelationType: "identity",
		Content:      "backend engineer",
		SourceTier:   types.SourceExplicit,
		TemporalScope: types.ScopePattern,
		Importance:   7,
		Source:       "chat",
		MemCellType:  "fact",
		Evidence: types.MemoryEvidenceEntry{
			Reinforcement:    0.5,
			ReinLastSignalAt: time.Now().Unix(),
		},
		CreatedAt: now,
	}
	err = store.SaveFact(ctx, f2)
	if err != nil {
		t.Fatalf("SaveFact f2: %v", err)
	}

	// Should find f2 (created just now) in the last hour
	recent, err := store.SearchFactsByTimeRange(ctx, now-3600, now+1, 10)
	if err != nil {
		t.Fatalf("SearchFactsByTimeRange: %v", err)
	}
	if len(recent) != 1 {
		t.Errorf("expected 1 recent fact, got %d", len(recent))
	}

	// Should NOT find f2 in a window 48h ago
	old, err := store.SearchFactsByTimeRange(ctx, now-172800, now-86400, 10)
	if err != nil {
		t.Fatalf("SearchFactsByTimeRange old: %v", err)
	}
	if len(old) != 0 {
		t.Errorf("expected 0 facts in old window, got %d", len(old))
	}
}

func TestFTS5Search(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)

	f := &types.FactEntry{
		Entity:       "master",
		RelationType: "preference",
		Content:      "likes Go generics",
		SourceTier:   types.SourceExplicit,
		TemporalScope: types.ScopePattern,
		Importance:   6,
		Source:       "chat",
		MemCellType:  "prefer",
		Evidence: types.MemoryEvidenceEntry{
			Reinforcement:    0.5,
			ReinLastSignalAt: time.Now().Unix(),
		},
	}
	_ = store.SaveFact(ctx, f)

	results, err := store.SearchFacts(ctx, "Go", 5)
	if err != nil {
		t.Fatalf("SearchFacts: %v", err)
	}
	if len(results) == 0 {
		t.Error("expected FTS5 to find 'Go' match")
	}
}

func TestDiaryCRUD(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)

	d := &types.DiaryEntry{
		Title:          "morning coding session",
		Summary:        "user worked on Rust project for 3 hours",
		EmotionValence: 0.3,
		EmotionArousal: -0.2,
		EmotionPrimary: "focused",
	}
	err := store.SaveDiary(ctx, d)
	if err != nil {
		t.Fatalf("SaveDiary: %v", err)
	}
	if d.ID == 0 {
		t.Error("expected auto-generated ID")
	}

	diaries, err := store.ListDiaries(ctx, 10)
	if err != nil {
		t.Fatalf("ListDiaries: %v", err)
	}
	if len(diaries) != 1 {
		t.Fatalf("expected 1 diary, got %d", len(diaries))
	}
	if diaries[0].Title != d.Title {
		t.Errorf("Title: got %q, want %q", diaries[0].Title, d.Title)
	}

	// Archive
	err = store.ArchiveDiary(ctx, d.ID)
	if err != nil {
		t.Fatalf("ArchiveDiary: %v", err)
	}
	remaining, _ := store.ListDiaries(ctx, 10)
	if len(remaining) != 0 {
		t.Errorf("expected 0 after archive, got %d", len(remaining))
	}
}

func TestChatHistory(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)

	msgs := []types.Message{
		{Role: types.RoleUser, Content: "hello", CreatedAt: time.Now().Unix()},
		{Role: types.RoleAssistant, Content: "hi there!", CreatedAt: time.Now().Unix() + 1},
	}
	err := store.SaveHistory(ctx, msgs)
	if err != nil {
		t.Fatalf("SaveHistory: %v", err)
	}

	loaded, err := store.LoadHistory(ctx, 10)
	if err != nil {
		t.Fatalf("LoadHistory: %v", err)
	}
	if len(loaded) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(loaded))
	}
	if loaded[0].Role != types.RoleUser || loaded[0].Content != "hello" {
		t.Errorf("first msg: role=%q content=%q", loaded[0].Role, loaded[0].Content)
	}
	if loaded[1].Role != types.RoleAssistant || loaded[1].Content != "hi there!" {
		t.Errorf("second msg: role=%q content=%q", loaded[1].Role, loaded[1].Content)
	}

	// Clean old
	err = store.CleanOldHistory(ctx, 365)
	if err != nil {
		t.Fatalf("CleanOldHistory: %v", err)
	}
	after, _ := store.LoadHistory(ctx, 10)
	if len(after) != 2 {
		t.Errorf("messages <365d should survive: got %d", len(after))
	}
}

func TestThreads(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)

	id, err := store.SaveThread(ctx, &types.ConversationThread{
		Type:    "follow_up",
		Goal:    "ask about Rust project progress",
		Status:  "active",
		Priority: 0.8,
	})
	if err != nil {
		t.Fatalf("SaveThread: %v", err)
	}
	if id == 0 {
		t.Error("expected auto-generated ID")
	}

	threads, err := store.ListActiveThreads(ctx)
	if err != nil {
		t.Fatalf("ListActiveThreads: %v", err)
	}
	if len(threads) != 1 {
		t.Fatalf("expected 1 thread, got %d", len(threads))
	}

	// Resolve
	err = store.ResolveThread(ctx, id, "user completed the project", "follow up next sprint")
	if err != nil {
		t.Fatalf("ResolveThread: %v", err)
	}
	active, _ := store.ListActiveThreads(ctx)
	if len(active) != 0 {
		t.Errorf("expected 0 active threads after resolve, got %d", len(active))
	}
}

func TestOutcomes(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)

	o := &types.ActionOutcome{
		ActionSource: "proactive",
		ActionType:   "care_check",
		HourOfDay:    14,
		DayOfWeek:    3,
		Outcome:      types.OutcomeEngaged,
	}
	err := store.SaveOutcome(ctx, o)
	if err != nil {
		t.Fatalf("SaveOutcome: %v", err)
	}

	outcomes, err := store.QueryOutcomes(ctx, port.OutcomeFilter{Source: "proactive", Limit: 10})
	if err != nil {
		t.Fatalf("QueryOutcomes: %v", err)
	}
	if len(outcomes) != 1 {
		t.Fatalf("expected 1 outcome, got %d", len(outcomes))
	}
}

func TestDriveRecords(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)

	id, err := store.SaveDriveRecord(ctx, &types.DriveRecord{
		Action:  "care_check",
		Social:  0.5,
		Care:    0.8,
		Curious: 0.2,
		Quiet:   0.1,
		Explore: 0.3,
		Reward:  1.0,
		At:      time.Now().Unix(),
	})
	if err != nil {
		t.Fatalf("SaveDriveRecord: %v", err)
	}
	if id == 0 {
		t.Error("expected auto-generated ID")
	}

	recs, err := store.ListDriveRecords(ctx, 0, 10)
	if err != nil {
		t.Fatalf("ListDriveRecords: %v", err)
	}
	if len(recs) != 1 {
		t.Fatalf("expected 1 record, got %d", len(recs))
	}
}

func TestCountTodayMessages(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)

	count := store.CountTodayMessages(ctx)
	if count != 0 {
		t.Errorf("expected 0, got %d", count)
	}

	_ = store.SaveHistory(ctx, []types.Message{
		{Role: types.RoleUser, Content: "hi", CreatedAt: time.Now().Unix()},
	})

	count = store.CountTodayMessages(ctx)
	if count != 1 {
		t.Errorf("expected 1, got %d", count)
	}
}

func TestConcurrentAccess(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)

	done := make(chan bool, 10)
	for i := 0; i < 10; i++ {
		go func(n int) {
			_ = store.SaveFact(ctx, &types.FactEntry{
				Entity:       "master",
				RelationType: "preference",
				Content:      "test",
				SourceTier:   types.SourceExplicit,
				Source:       "test",
				MemCellType:  "fact",
				Evidence: types.MemoryEvidenceEntry{
					Reinforcement:    0.5,
					ReinLastSignalAt: time.Now().Unix(),
				},
				CreatedAt: time.Now().Unix(),
			})
			done <- true
		}(i)
	}
	for i := 0; i < 10; i++ {
		<-done
	}

	facts, _ := store.ListActiveFacts(ctx, 0)
	if len(facts) != 10 {
		t.Errorf("expected 10 facts from concurrent writes, got %d", len(facts))
	}
}

func TestFloat32BlobRoundtrip(t *testing.T) {
	original := []float32{0.1, -0.5, 1.0, 0.0}
	blob := float32ToBlob(original)
	restored := blobToFloat32(blob)

	if len(restored) != len(original) {
		t.Fatalf("len: got %d, want %d", len(restored), len(original))
	}
	for i := range original {
		if abs(restored[i]-original[i]) > 0.001 {
			t.Errorf("idx %d: got %f, want %f", i, restored[i], original[i])
		}
	}
}

func abs(x float32) float32 {
	if x < 0 {
		return -x
	}
	return x
}

func TestSaveStoresTimestamp(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)

	f := &types.FactEntry{
		Entity:       "master",
		RelationType: "identity",
		Content:      "timestamp test",
		SourceTier:   types.SourceExplicit,
		Source:       "test",
		MemCellType:  "fact",
		Evidence: types.MemoryEvidenceEntry{
			Reinforcement:    0.5,
			ReinLastSignalAt: time.Now().Unix(),
		},
	}
	_ = store.SaveFact(ctx, f)

	if f.CreatedAt == 0 {
		t.Error("CreatedAt should be auto-populated")
	}
	if f.UpdatedAt == 0 {
		t.Error("UpdatedAt should be auto-populated")
	}
	if f.SchemaVersion != types.SchemaVersionCurrent {
		t.Errorf("SchemaVersion: got %d, want %d", f.SchemaVersion, types.SchemaVersionCurrent)
	}
}

func TestRunForgetting(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)

	// Create a state fact that should expire
	f := &types.FactEntry{
		Entity:       "master",
		RelationType: "emotional",
		Content:      "temporary state",
		SourceTier:   types.SourceObserved,
		TemporalScope: types.ScopeState,
		AutoExpireDays: 1,
		Source:       "test",
		MemCellType:  "emotion",
		Evidence: types.MemoryEvidenceEntry{
			Reinforcement:    0.5,
			ReinLastSignalAt: time.Now().Unix(),
		},
		CreatedAt: time.Now().Add(-48 * time.Hour).Unix(),
	}
	_ = store.SaveFact(ctx, f)

	err := store.RunForgetting(ctx)
	if err != nil {
		t.Fatalf("RunForgetting: %v", err)
	}

	// The expired fact should now be archived
	got, err := store.GetFact(ctx, f.ID)
	if err != nil {
		t.Fatalf("GetFact: %v", err)
	}
	if !got.Archived {
		t.Error("expired state fact should be archived")
	}
}

func TestEpisodesAndTopics(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)

	epID, err := store.SaveEpisode(ctx, &types.Episode{
		Topic:   "Rust migration",
		Summary: "discussed migrating from Python to Rust",
		Status:  "active",
	})
	if err != nil {
		t.Fatalf("SaveEpisode: %v", err)
	}

	ep, err := store.GetEpisode(ctx, epID)
	if err != nil {
		t.Fatalf("GetEpisode: %v", err)
	}
	if ep.Topic != "Rust migration" {
		t.Errorf("Topic: got %q, want %q", ep.Topic, "Rust migration")
	}

	topicID, err := store.SaveTopic(ctx, &types.Topic{
		Name:  "rust",
		Count: 5,
	})
	if err != nil {
		t.Fatalf("SaveTopic: %v", err)
	}
	if topicID == 0 {
		t.Error("expected auto-generated topic ID")
	}

	topics, err := store.ListTopics(ctx)
	if err != nil {
		t.Fatalf("ListTopics: %v", err)
	}
	if len(topics) != 1 {
		t.Fatalf("expected 1 topic, got %d", len(topics))
	}
}

func TestReopenPersistence(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	path := filepath.Join(dir, "persist.db")

	// Create and write
	s1, err := NewSQLiteStore(path)
	if err != nil {
		t.Fatalf("NewSQLiteStore 1: %v", err)
	}
	_ = s1.SaveFact(ctx, &types.FactEntry{
		Entity:       "master",
		RelationType: "identity",
		Content:      "persists across opens",
		SourceTier:   types.SourceExplicit,
		Source:       "test",
		MemCellType:  "fact",
		Evidence: types.MemoryEvidenceEntry{
			Reinforcement:    0.5,
			ReinLastSignalAt: time.Now().Unix(),
		},
	})
	s1.Close()

	// Reopen and read
	s2, err := NewSQLiteStore(path)
	if err != nil {
		t.Fatalf("NewSQLiteStore 2: %v", err)
	}
	defer s2.Close()

	facts, err := s2.ListActiveFacts(ctx, 0)
	if err != nil {
		t.Fatalf("ListActiveFacts: %v", err)
	}
	if len(facts) != 1 {
		t.Fatalf("expected 1 fact after reopen, got %d", len(facts))
	}
	if facts[0].Content != "persists across opens" {
		t.Errorf("Content: got %q", facts[0].Content)
	}
}

func TestBackupDirCreation(t *testing.T) {
	dir := t.TempDir()
	backupDir := filepath.Join(dir, "deeply", "nested", "sion")
	path := filepath.Join(backupDir, "test.db")

	store, err := NewSQLiteStore(path)
	if err != nil {
		t.Fatalf("NewSQLiteStore with nested dir: %v", err)
	}
	defer store.Close()

	if _, err := os.Stat(path); err != nil {
		t.Errorf("db file not created: %v", err)
	}
}
