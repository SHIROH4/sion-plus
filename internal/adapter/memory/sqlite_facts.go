package memory

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/shirohania/sion/internal/domain/types"
	"github.com/shirohania/sion/internal/port"
)

// ── L1 Diary ────────────────────────────────────────────────────────

func (s *SQLiteStore) SaveDiary(ctx context.Context, entry *types.DiaryEntry) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now().Unix()
	if entry.CreatedAt == 0 {
		entry.CreatedAt = now
	}
	entry.SchemaVersion = types.SchemaVersionCurrent

	res, err := s.db.ExecContext(ctx,
		`INSERT INTO diaries (schema_version, title, summary, emotion_valence,
		 emotion_arousal, emotion_primary, vector, topic_id, reflection_id,
		 abstracted, archived, created_at)
		 VALUES (?,?,?,?,?,?,?,?,?,?,?,?)`,
		entry.SchemaVersion, entry.Title, entry.Summary,
		entry.EmotionValence, entry.EmotionArousal, entry.EmotionPrimary,
		float32ToBlob(entry.Vector),
		nullableInt64(entry.TopicID), nullableInt64(0), // reflection_id placeholder
		boolToInt(entry.Abstracted), boolToInt(entry.Archived),
		entry.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("SaveDiary: %w", err)
	}
	id, _ := res.LastInsertId()
	entry.ID = id

	_, _ = s.db.ExecContext(ctx,
		`INSERT INTO diaries_fts(rowid, title, summary) VALUES (?,?,?)`,
		id, entry.Title, entry.Summary,
	)
	return nil
}

func (s *SQLiteStore) ListDiaries(ctx context.Context, limit int) ([]types.DiaryEntry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if limit <= 0 {
		limit = 50
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, schema_version, title, summary, emotion_valence, emotion_arousal,
		        emotion_primary, vector, topic_id, abstracted, archived, created_at
		 FROM diaries WHERE archived=0 ORDER BY created_at DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanDiaries(rows)
}

func (s *SQLiteStore) SearchDiaries(ctx context.Context, vector []float32, topK int) ([]types.DiaryEntry, error) {
	return s.ListDiaries(ctx, topK)
}

// SearchDiariesByTimeRange returns diaries within [start, end] (v2.0).
func (s *SQLiteStore) SearchDiariesByTimeRange(ctx context.Context, start, end int64, limit int) ([]types.DiaryEntry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if limit <= 0 {
		limit = 50
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, schema_version, title, summary, emotion_valence, emotion_arousal,
		        emotion_primary, vector, topic_id, abstracted, archived, created_at
		 FROM diaries WHERE archived=0 AND created_at BETWEEN ? AND ?
		 ORDER BY created_at DESC LIMIT ?`, start, end, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanDiaries(rows)
}

func (s *SQLiteStore) ArchiveDiary(ctx context.Context, id int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	_, err := s.db.ExecContext(ctx, `UPDATE diaries SET archived=1 WHERE id=?`, id)
	return err
}

func (s *SQLiteStore) DiaryCount(ctx context.Context) int {
	s.mu.Lock()
	defer s.mu.Unlock()

	var n int
	_ = s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM diaries WHERE archived=0`).Scan(&n)
	return n
}

// ── L2 Facts ────────────────────────────────────────────────────────

func (s *SQLiteStore) SaveFact(ctx context.Context, fact *types.FactEntry) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now().Unix()
	if fact.CreatedAt == 0 {
		fact.CreatedAt = now
	}
	fact.UpdatedAt = now
	fact.SchemaVersion = types.SchemaVersionCurrent

	evidenceJSON, _ := json.Marshal(fact.Evidence)
	contextJSON, _ := json.Marshal(fact.ContextTags)

	res, err := s.db.ExecContext(ctx,
		`INSERT INTO facts (schema_version, entity, relation_type, content,
		 source_tier, temporal_scope, auto_expire_days, observation_count,
		 importance, evidence, oscillating, suppress_until, mention_count,
		 vector, recall_count, last_recalled_at, context_tags,
		 source, memcell_type, episode_id, archived,
		 signal_processed, absorbed,
		 embedding_text_sha256, embedding_model_id,
		 event_start_at, event_end_at,
		 created_at, updated_at)
		 VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		fact.SchemaVersion, fact.Entity, fact.RelationType, fact.Content,
		fact.SourceTier, fact.TemporalScope, fact.AutoExpireDays, fact.ObservationCount,
		fact.Importance, string(evidenceJSON), boolToInt(fact.Oscillating),
		fact.SuppressUntil, fact.MentionCount,
		float32ToBlob(fact.Vector),
		fact.RecallCount, fact.LastRecalledAt, string(contextJSON),
		fact.Source, fact.MemCellType, nullableInt64(fact.EpisodeID),
		boolToInt(fact.Archived),
		0, 0, // signal_processed=0 (pending), absorbed=0
		"", "", // embedding cache not yet filled
		0, 0, // event_start_at, event_end_at
		fact.CreatedAt, fact.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("SaveFact: %w", err)
	}
	id, _ := res.LastInsertId()
	fact.ID = id

	_, _ = s.db.ExecContext(ctx,
		`INSERT INTO facts_fts(rowid, content, entity, relation_type) VALUES (?,?,?,?)`,
		id, fact.Content, fact.Entity, fact.RelationType,
	)
	return nil
}

func (s *SQLiteStore) GetFact(ctx context.Context, id int64) (*types.FactEntry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	row := s.db.QueryRowContext(ctx, factSelectCols+" FROM facts WHERE id=?", id)
	return scanFact(row)
}

func (s *SQLiteStore) ListActiveFacts(ctx context.Context, minScore float64) ([]types.FactEntry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	rows, err := s.db.QueryContext(ctx,
		factSelectCols+" FROM facts WHERE archived=0 ORDER BY created_at DESC LIMIT 1000")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanFacts(rows)
}

func (s *SQLiteStore) ListAllFacts(ctx context.Context) ([]types.FactEntry, error) {
	return s.ListActiveFacts(ctx, 0)
}

// ListUnprocessedFacts returns facts that have not yet been through Stage-2 SignalDetection.
func (s *SQLiteStore) ListUnprocessedFacts(ctx context.Context, limit int) ([]types.FactEntry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if limit <= 0 {
		limit = 100
	}
	rows, err := s.db.QueryContext(ctx,
		factSelectCols+" FROM facts WHERE signal_processed=0 AND archived=0 ORDER BY created_at ASC LIMIT ?", limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanFacts(rows)
}

// MarkFactsProcessed marks a batch of facts as signal_processed=1.
func (s *SQLiteStore) MarkFactsProcessed(ctx context.Context, ids []int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	for _, id := range ids {
		if _, err := tx.ExecContext(ctx,
			`UPDATE facts SET signal_processed=1, updated_at=? WHERE id=?`,
			time.Now().Unix(), id); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// MarkFactsAbsorbed marks facts as consumed by a reflection.
func (s *SQLiteStore) MarkFactsAbsorbed(ctx context.Context, ids []int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	for _, id := range ids {
		if _, err := tx.ExecContext(ctx,
			`UPDATE facts SET absorbed=1, updated_at=? WHERE id=?`,
			time.Now().Unix(), id); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *SQLiteStore) SearchFacts(ctx context.Context, query string, topK int) ([]types.FactEntry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if topK <= 0 {
		topK = 10
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT f.id, f.schema_version, f.entity, f.relation_type, f.content,
		 f.source_tier, f.temporal_scope, f.auto_expire_days, f.observation_count,
		 f.importance, f.evidence, f.oscillating, f.suppress_until, f.mention_count,
		 f.vector, f.recall_count, f.last_recalled_at, f.context_tags,
		 f.source, f.memcell_type, f.episode_id, f.archived,
		 f.signal_processed, f.absorbed,
		 f.embedding_text_sha256, f.embedding_model_id,
		 f.event_start_at, f.event_end_at,
		 f.created_at, f.updated_at
		 FROM facts f
		 JOIN facts_fts ft ON f.id = ft.rowid
		 WHERE facts_fts MATCH ? AND f.archived=0
		 ORDER BY rank LIMIT ?`, escapeFTS5(query), topK)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanFacts(rows)
}

func (s *SQLiteStore) SearchFactsByVector(ctx context.Context, vector []float32, topK int) ([]types.FactEntry, error) {
	return s.SearchFacts(ctx, "", topK)
}

func (s *SQLiteStore) SearchFactsByTimeRange(ctx context.Context, start, end int64, limit int) ([]types.FactEntry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if limit <= 0 {
		limit = 50
	}
	rows, err := s.db.QueryContext(ctx,
		factSelectCols+` FROM facts WHERE archived=0 AND created_at BETWEEN ? AND ?
		 ORDER BY created_at DESC LIMIT ?`, start, end, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanFacts(rows)
}

func (s *SQLiteStore) ArchiveFact(ctx context.Context, id int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	_, err := s.db.ExecContext(ctx, `UPDATE facts SET archived=1, updated_at=? WHERE id=?`,
		time.Now().Unix(), id)
	return err
}

func (s *SQLiteStore) UpdateFact(ctx context.Context, fact *types.FactEntry) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	fact.UpdatedAt = time.Now().Unix()
	evidenceJSON, _ := json.Marshal(fact.Evidence)
	contextJSON, _ := json.Marshal(fact.ContextTags)

	_, err := s.db.ExecContext(ctx,
		`UPDATE facts SET entity=?, relation_type=?, content=?,
		 source_tier=?, temporal_scope=?, auto_expire_days=?, observation_count=?,
		 importance=?, evidence=?, oscillating=?, suppress_until=?, mention_count=?,
		 vector=?, recall_count=?, last_recalled_at=?, context_tags=?,
		 source=?, memcell_type=?, episode_id=?, archived=?,
		 signal_processed=?, absorbed=?,
		 event_start_at=?, event_end_at=?,
		 updated_at=?
		 WHERE id=?`,
		fact.Entity, fact.RelationType, fact.Content,
		fact.SourceTier, fact.TemporalScope, fact.AutoExpireDays, fact.ObservationCount,
		fact.Importance, string(evidenceJSON), boolToInt(fact.Oscillating),
		fact.SuppressUntil, fact.MentionCount,
		float32ToBlob(fact.Vector),
		fact.RecallCount, fact.LastRecalledAt, string(contextJSON),
		fact.Source, fact.MemCellType, nullableInt64(fact.EpisodeID),
		boolToInt(fact.Archived),
		1, boolToInt(false), // signal_processed=1 (manually updated), absorbed kept from existing
		0, 0, // event_start_at, event_end_at (preserve existing)
		fact.UpdatedAt,
		fact.ID,
	)
	if err != nil {
		return fmt.Errorf("UpdateFact: %w", err)
	}

	_, _ = s.db.ExecContext(ctx, `DELETE FROM facts_fts WHERE rowid=?`, fact.ID)
	_, _ = s.db.ExecContext(ctx,
		`INSERT INTO facts_fts(rowid, content, entity, relation_type) VALUES (?,?,?,?)`,
		fact.ID, fact.Content, fact.Entity, fact.RelationType,
	)
	return nil
}

// ── Reflections (L1.5: fact synthesis) ──────────────────────────────

func (s *SQLiteStore) SaveReflection(ctx context.Context, r *types.ReflectionEntry) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now().Unix()
	if r.CreatedAt == 0 {
		r.CreatedAt = now
	}
	r.UpdatedAt = now
	r.SchemaVersion = types.SchemaVersionCurrent

	sourceIDsJSON, _ := json.Marshal(r.SourceFactIDs)

	res, err := s.db.ExecContext(ctx,
		`INSERT INTO reflections (schema_version, text, entity, relation_type,
		 status, source_fact_ids, reinforcement, rein_last_signal_at,
		 disputation, disp_last_signal_at,
		 feedback, next_eligible_at, absorbed_into,
		 suppress, mention_count, vector,
		 embedding_text_sha256, embedding_model_id,
		 created_at, updated_at)
		 VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		r.SchemaVersion, r.Text, r.Entity, r.RelationType,
		r.Status, string(sourceIDsJSON),
		r.Reinforcement, nullableInt64String(r.ReinLastSignalAt),
		r.Disputation, nullableInt64String(r.DispLastSignalAt),
		r.Feedback, r.NextEligibleAt, nullableInt64(r.AbsorbedInto),
		boolToInt(r.Suppress), r.MentionCount,
		float32ToBlob(r.Vector),
		r.EmbeddingTextSHA256, r.EmbeddingModelID,
		r.CreatedAt, r.UpdatedAt,
	)
	if err != nil {
		return 0, fmt.Errorf("SaveReflection: %w", err)
	}
	id, _ := res.LastInsertId()

	_, _ = s.db.ExecContext(ctx,
		`INSERT INTO reflections_fts(rowid, text, entity, relation_type) VALUES (?,?,?,?)`,
		id, r.Text, r.Entity, r.RelationType,
	)
	return id, nil
}

func (s *SQLiteStore) ListReflectionsByStatus(ctx context.Context, statuses []string, limit int) ([]types.ReflectionEntry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if limit <= 0 {
		limit = 50
	}
	// Build IN clause
	placeholders := make([]string, len(statuses))
	args := make([]any, 0, len(statuses))
	for i, st := range statuses {
		placeholders[i] = "?"
		args = append(args, st)
	}
	query := fmt.Sprintf(
		`SELECT id, schema_version, text, entity, relation_type,
		        status, source_fact_ids, reinforcement, rein_last_signal_at,
		        disputation, disp_last_signal_at,
		        feedback, next_eligible_at, absorbed_into,
		        suppress, mention_count, vector,
		        embedding_text_sha256, embedding_model_id,
		        created_at, updated_at
		 FROM reflections WHERE status IN (%s) ORDER BY created_at DESC LIMIT ?`,
		placeholdersStr(statuses))
	args = append(args, limit)

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanReflections(rows)
}

func (s *SQLiteStore) UpdateReflectionStatus(ctx context.Context, id int64, status string, feedback string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	_, err := s.db.ExecContext(ctx,
		`UPDATE reflections SET status=?, feedback=?, updated_at=? WHERE id=?`,
		status, feedback, time.Now().Unix(), id)
	return err
}

// ── L3 Strategies ───────────────────────────────────────────────────

func (s *SQLiteStore) SaveStrategy(ctx context.Context, st *types.StrategyPrinciple) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now().Unix()
	if st.CreatedAt == 0 {
		st.CreatedAt = now
	}
	st.UpdatedAt = now
	st.SchemaVersion = types.SchemaVersionCurrent

	res, err := s.db.ExecContext(ctx,
		`INSERT INTO strategies (schema_version, situation, good_strategy, bad_strategy,
		 reason, confidence, source, vector, active, created_at, updated_at)
		 VALUES (?,?,?,?,?,?,?,?,?,?,?)`,
		st.SchemaVersion, st.Situation, st.GoodStrategy, st.BadStrategy,
		st.Reason, st.Confidence, st.Source, float32ToBlob(st.Vector),
		boolToInt(st.Active), st.CreatedAt, st.UpdatedAt,
	)
	if err != nil {
		return 0, fmt.Errorf("SaveStrategy: %w", err)
	}
	id, _ := res.LastInsertId()
	return id, nil
}

func (s *SQLiteStore) ListActiveStrategies(ctx context.Context) ([]types.StrategyPrinciple, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	rows, err := s.db.QueryContext(ctx,
		`SELECT id, schema_version, situation, good_strategy, bad_strategy,
		        reason, confidence, source, vector, active, created_at, updated_at
		 FROM strategies WHERE active=1 ORDER BY confidence DESC LIMIT 50`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanStrategies(rows)
}

func (s *SQLiteStore) DeactivateStrategy(ctx context.Context, id int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	_, err := s.db.ExecContext(ctx,
		`UPDATE strategies SET active=0, updated_at=? WHERE id=?`,
		time.Now().Unix(), id)
	return err
}

func (s *SQLiteStore) SearchStrategiesByVector(ctx context.Context, vector []float32, topK int) ([]types.StrategyPrinciple, error) {
	return s.ListActiveStrategies(ctx)
}

// ── Scanner helpers ─────────────────────────────────────────────────

const factSelectCols = `SELECT id, schema_version, entity, relation_type, content,
		source_tier, temporal_scope, auto_expire_days, observation_count,
		importance, evidence, oscillating, suppress_until, mention_count,
		vector, recall_count, last_recalled_at, context_tags,
		source, memcell_type, episode_id, archived,
		signal_processed, absorbed,
		embedding_text_sha256, embedding_model_id,
		event_start_at, event_end_at,
		created_at, updated_at`

func scanFact(row *sql.Row) (*types.FactEntry, error) {
	var f types.FactEntry
	var evidenceJSON, contextJSON string
	var vectorBlob []byte
	var oscillatingInt int
	var episodeID sql.NullInt64
	var signalProc, absorbedInt int
	var embSHA, embModel string
	if err := row.Scan(
		&f.ID, &f.SchemaVersion, &f.Entity, &f.RelationType, &f.Content,
		&f.SourceTier, &f.TemporalScope, &f.AutoExpireDays, &f.ObservationCount,
		&f.Importance, &evidenceJSON, &oscillatingInt,
		&f.SuppressUntil, &f.MentionCount,
		&vectorBlob, &f.RecallCount, &f.LastRecalledAt, &contextJSON,
		&f.Source, &f.MemCellType, &episodeID,
		&f.Archived,
		&signalProc, &absorbedInt,
		&embSHA, &embModel,
		&f.EventStartAt, &f.EventEndAt,
		&f.CreatedAt, &f.UpdatedAt,
	); err != nil {
		return nil, err
	}
	json.Unmarshal([]byte(evidenceJSON), &f.Evidence)
	json.Unmarshal([]byte(contextJSON), &f.ContextTags)
	f.Oscillating = oscillatingInt != 0
	if episodeID.Valid {
		f.EpisodeID = episodeID.Int64
	}
	f.Vector = blobToFloat32(vectorBlob)
	return &f, nil
}

func scanFacts(rows *sql.Rows) ([]types.FactEntry, error) {
	var facts []types.FactEntry
	for rows.Next() {
		var f types.FactEntry
		var evidenceJSON, contextJSON string
		var vectorBlob []byte
		var oscillatingInt int
		var episodeID sql.NullInt64
		var signalProc, absorbedInt int
		var embSHA, embModel string
		if err := rows.Scan(
			&f.ID, &f.SchemaVersion, &f.Entity, &f.RelationType, &f.Content,
			&f.SourceTier, &f.TemporalScope, &f.AutoExpireDays, &f.ObservationCount,
			&f.Importance, &evidenceJSON, &oscillatingInt,
			&f.SuppressUntil, &f.MentionCount,
			&vectorBlob, &f.RecallCount, &f.LastRecalledAt, &contextJSON,
			&f.Source, &f.MemCellType, &episodeID,
			&f.Archived,
			&signalProc, &absorbedInt,
			&embSHA, &embModel,
			&f.EventStartAt, &f.EventEndAt,
			&f.CreatedAt, &f.UpdatedAt,
		); err != nil {
			return nil, err
		}
		json.Unmarshal([]byte(evidenceJSON), &f.Evidence)
		json.Unmarshal([]byte(contextJSON), &f.ContextTags)
		f.Oscillating = oscillatingInt != 0
		if episodeID.Valid {
			f.EpisodeID = episodeID.Int64
		}
		f.Vector = blobToFloat32(vectorBlob)
		facts = append(facts, f)
	}
	return facts, rows.Err()
}

func scanDiaries(rows *sql.Rows) ([]types.DiaryEntry, error) {
	var diaries []types.DiaryEntry
	for rows.Next() {
		var d types.DiaryEntry
		var vectorBlob []byte
		var topicID sql.NullInt64
		var reflectionID sql.NullInt64
		if err := rows.Scan(
			&d.ID, &d.SchemaVersion, &d.Title, &d.Summary,
			&d.EmotionValence, &d.EmotionArousal, &d.EmotionPrimary,
			&vectorBlob, &topicID, &d.Abstracted, &d.Archived, &d.CreatedAt,
		); err != nil {
			return nil, err
		}
		if topicID.Valid {
			d.TopicID = topicID.Int64
		}
		_ = reflectionID // reserved for future use
		d.Vector = blobToFloat32(vectorBlob)
		diaries = append(diaries, d)
	}
	return diaries, rows.Err()
}

func scanReflections(rows *sql.Rows) ([]types.ReflectionEntry, error) {
	var refs []types.ReflectionEntry
	for rows.Next() {
		var r types.ReflectionEntry
		var sourceIDsJSON string
		var vectorBlob []byte
		var reinLast, dispLast sql.NullString
		var absorbedInto sql.NullInt64
		if err := rows.Scan(
			&r.ID, &r.SchemaVersion, &r.Text, &r.Entity, &r.RelationType,
			&r.Status, &sourceIDsJSON,
			&r.Reinforcement, &reinLast,
			&r.Disputation, &dispLast,
			&r.Feedback, &r.NextEligibleAt, &absorbedInto,
			&r.Suppress, &r.MentionCount,
			&vectorBlob,
			&r.EmbeddingTextSHA256, &r.EmbeddingModelID,
			&r.CreatedAt, &r.UpdatedAt,
		); err != nil {
			return nil, err
		}
		json.Unmarshal([]byte(sourceIDsJSON), &r.SourceFactIDs)
		if reinLast.Valid {
			r.ReinLastSignalAt = reinLast.String
		}
		if dispLast.Valid {
			r.DispLastSignalAt = dispLast.String
		}
		if absorbedInto.Valid {
			r.AbsorbedInto = absorbedInto.Int64
		}
		r.Vector = blobToFloat32(vectorBlob)
		refs = append(refs, r)
	}
	return refs, rows.Err()
}

func scanStrategies(rows *sql.Rows) ([]types.StrategyPrinciple, error) {
	var strategies []types.StrategyPrinciple
	for rows.Next() {
		var s types.StrategyPrinciple
		var vectorBlob []byte
		var activeInt int
		if err := rows.Scan(
			&s.ID, &s.SchemaVersion, &s.Situation, &s.GoodStrategy, &s.BadStrategy,
			&s.Reason, &s.Confidence, &s.Source, &vectorBlob, &activeInt,
			&s.CreatedAt, &s.UpdatedAt,
		); err != nil {
			return nil, err
		}
		s.Vector = blobToFloat32(vectorBlob)
		s.Active = activeInt != 0
		strategies = append(strategies, s)
	}
	return strategies, rows.Err()
}

// ── Helpers ─────────────────────────────────────────────────────────

func placeholdersStr(items []string) string {
	if len(items) == 0 {
		return ""
	}
	s := ""
	for i := range items {
		if i > 0 {
			s += ","
		}
		s += "?"
	}
	return s
}

func nullableInt64String(v string) any {
	if v == "" {
		return nil
	}
	return v
}

// ── Reflected Facts query ───────────────────────────────────────────

// ListUnabsorbedFacts returns facts that haven't been consumed into any reflection.
func (s *SQLiteStore) ListUnabsorbedFacts(ctx context.Context, minImportance int, limit int) ([]types.FactEntry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if limit <= 0 {
		limit = 200
	}
	rows, err := s.db.QueryContext(ctx,
		factSelectCols+` FROM facts
		 WHERE absorbed=0 AND archived=0 AND importance >= ?
		 ORDER BY created_at ASC LIMIT ?`, minImportance, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanFacts(rows)
}

// ── Embedding cache ─────────────────────────────────────────────────

// UpdateFactEmbedding stores the embedding vector and model metadata for a fact.
func (s *SQLiteStore) UpdateFactEmbedding(ctx context.Context, id int64, vector []float32, textSHA256, modelID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	_, err := s.db.ExecContext(ctx,
		`UPDATE facts SET vector=?, embedding_text_sha256=?, embedding_model_id=?, updated_at=?
		 WHERE id=?`,
		float32ToBlob(vector), textSHA256, modelID, time.Now().Unix(), id)
	return err
}

// FindCachedEmbedding returns the cached vector if the same text was embedded with the same model.
func (s *SQLiteStore) FindCachedEmbedding(ctx context.Context, textSHA256, modelID string) ([]float32, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	var blob []byte
	err := s.db.QueryRowContext(ctx,
		`SELECT vector FROM facts
		 WHERE embedding_text_sha256=? AND embedding_model_id=? AND vector IS NOT NULL
		 LIMIT 1`, textSHA256, modelID).Scan(&blob)
	if err != nil {
		return nil, err
	}
	return blobToFloat32(blob), nil
}

// ── Compile-time check ──────────────────────────────────────────────

var _ port.MemoryStore = (*SQLiteStore)(nil)
