package memory

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/shirohania/sion/internal/domain/types"
	"github.com/shirohania/sion/internal/port"
)

// ── Chat History ────────────────────────────────────────────────────

func (s *SQLiteStore) SaveHistory(ctx context.Context, msgs []types.Message) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now().Unix()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.PrepareContext(ctx,
		`INSERT INTO messages (role, content, images, meta_json, created_at)
		 VALUES (?,?,?,?,?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, msg := range msgs {
		ts := msg.CreatedAt
		if ts == 0 {
			ts = now
		}
		imagesJSON, _ := json.Marshal(msg.Images)
		metaJSON, _ := json.Marshal(msg.Metadata)
		if _, err := stmt.ExecContext(ctx, msg.Role, msg.Content, string(imagesJSON), string(metaJSON), ts); err != nil {
			return fmt.Errorf("SaveHistory: %w", err)
		}
	}
	return tx.Commit()
}

func (s *SQLiteStore) LoadHistory(ctx context.Context, limit int) ([]types.Message, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if limit <= 0 {
		limit = 100
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, role, content, images, meta_json, created_at
		 FROM messages ORDER BY id DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var msgs []types.Message
	for rows.Next() {
		var m types.Message
		var imagesJSON, metaJSON string
		var id int64
		if err := rows.Scan(&id, &m.Role, &m.Content, &imagesJSON, &metaJSON, &m.CreatedAt); err != nil {
			return nil, err
		}
		json.Unmarshal([]byte(imagesJSON), &m.Images)
		json.Unmarshal([]byte(metaJSON), &m.Metadata)
		msgs = append(msgs, m)
	}
	// Reverse to chronological order (queried DESC)
	for i, j := 0, len(msgs)-1; i < j; i, j = i+1, j-1 {
		msgs[i], msgs[j] = msgs[j], msgs[i]
	}
	return msgs, rows.Err()
}

// SearchMessagesByTimeRange returns messages within [start, end] (v2.0).
func (s *SQLiteStore) SearchMessagesByTimeRange(ctx context.Context, start, end int64, limit int) ([]types.Message, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if limit <= 0 {
		limit = 100
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, role, content, images, meta_json, created_at
		 FROM messages WHERE created_at BETWEEN ? AND ?
		 ORDER BY created_at DESC LIMIT ?`, start, end, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var msgs []types.Message
	for rows.Next() {
		var m types.Message
		var imagesJSON, metaJSON string
		var id int64
		if err := rows.Scan(&id, &m.Role, &m.Content, &imagesJSON, &metaJSON, &m.CreatedAt); err != nil {
			return nil, err
		}
		json.Unmarshal([]byte(imagesJSON), &m.Images)
		json.Unmarshal([]byte(metaJSON), &m.Metadata)
		msgs = append(msgs, m)
	}
	return msgs, rows.Err()
}

func (s *SQLiteStore) CleanOldHistory(ctx context.Context, olderThanDays int) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	cutoff := time.Now().AddDate(0, 0, -olderThanDays).Unix()
	_, err := s.db.ExecContext(ctx, `DELETE FROM messages WHERE created_at < ?`, cutoff)
	return err
}

// ── Episodes ────────────────────────────────────────────────────────

func (s *SQLiteStore) SaveEpisode(ctx context.Context, ep *types.Episode) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now().Unix()
	if ep.CreatedAt == 0 {
		ep.CreatedAt = now
	}
	ep.UpdatedAt = now

	res, err := s.db.ExecContext(ctx,
		`INSERT INTO episodes (topic, summary, status, created_at, updated_at)
		 VALUES (?,?,?,?,?)`,
		ep.Topic, ep.Summary, ep.Status, ep.CreatedAt, ep.UpdatedAt)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (s *SQLiteStore) GetEpisode(ctx context.Context, id int64) (*types.Episode, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	row := s.db.QueryRowContext(ctx,
		`SELECT id, topic, summary, status, created_at, updated_at
		 FROM episodes WHERE id=?`, id)
	var ep types.Episode
	err := row.Scan(&ep.ID, &ep.Topic, &ep.Summary, &ep.Status, &ep.CreatedAt, &ep.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &ep, nil
}

// ── Topics ──────────────────────────────────────────────────────────

func (s *SQLiteStore) SaveTopic(ctx context.Context, t *types.Topic) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	res, err := s.db.ExecContext(ctx,
		`INSERT INTO topics (name, centroid, count) VALUES (?,?,?)`,
		t.Name, float32ToBlob(t.Centroid), t.Count)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (s *SQLiteStore) ListTopics(ctx context.Context) ([]types.Topic, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	rows, err := s.db.QueryContext(ctx,
		`SELECT id, name, centroid, count FROM topics ORDER BY count DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var topics []types.Topic
	for rows.Next() {
		var t types.Topic
		var blob []byte
		if err := rows.Scan(&t.ID, &t.Name, &blob, &t.Count); err != nil {
			return nil, err
		}
		t.Centroid = blobToFloat32(blob)
		topics = append(topics, t)
	}
	return topics, rows.Err()
}

// ── Threads ─────────────────────────────────────────────────────────

func (s *SQLiteStore) SaveThread(ctx context.Context, t *types.ConversationThread) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now().Unix()
	if t.CreatedAt == 0 {
		t.CreatedAt = now
	}
	t.UpdatedAt = now
	t.SchemaVersion = types.SchemaVersionCurrent

	res, err := s.db.ExecContext(ctx,
		`INSERT INTO conversation_threads (schema_version, type, goal, status,
		 priority, best_approach, outcome, learnings, created_at, updated_at)
		 VALUES (?,?,?,?,?,?,?,?,?,?)`,
		t.SchemaVersion, t.Type, t.Goal, t.Status, t.Priority,
		t.BestApproach, t.Outcome, t.Learnings, t.CreatedAt, t.UpdatedAt)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (s *SQLiteStore) ListActiveThreads(ctx context.Context) ([]types.ConversationThread, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	rows, err := s.db.QueryContext(ctx,
		`SELECT id, schema_version, type, goal, status, priority,
		        best_approach, outcome, learnings, created_at, updated_at
		 FROM conversation_threads WHERE status='active' ORDER BY priority DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var threads []types.ConversationThread
	for rows.Next() {
		var t types.ConversationThread
		if err := rows.Scan(&t.ID, &t.SchemaVersion, &t.Type, &t.Goal, &t.Status,
			&t.Priority, &t.BestApproach, &t.Outcome, &t.Learnings,
			&t.CreatedAt, &t.UpdatedAt); err != nil {
			return nil, err
		}
		threads = append(threads, t)
	}
	return threads, rows.Err()
}

func (s *SQLiteStore) ResolveThread(ctx context.Context, id int64, outcome, learnings string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	_, err := s.db.ExecContext(ctx,
		`UPDATE conversation_threads SET status='resolved', outcome=?, learnings=?,
		 updated_at=? WHERE id=?`,
		outcome, learnings, time.Now().Unix(), id)
	return err
}

func (s *SQLiteStore) MarkThreadStale(ctx context.Context, id int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	_, err := s.db.ExecContext(ctx,
		`UPDATE conversation_threads SET status='stale', updated_at=? WHERE id=?`,
		time.Now().Unix(), id)
	return err
}

// ── Outcomes ────────────────────────────────────────────────────────

func (s *SQLiteStore) SaveOutcome(ctx context.Context, o *types.ActionOutcome) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now().Unix()
	if o.CreatedAt == 0 {
		o.CreatedAt = now
	}
	o.SchemaVersion = types.SchemaVersionCurrent

	_, err := s.db.ExecContext(ctx,
		`INSERT INTO outcomes (schema_version, action_source, action_type,
		 hour_of_day, day_of_week, app_context, emotion_bucket, escalation_lvl,
		 outcome, response_delay, created_at)
		 VALUES (?,?,?,?,?,?,?,?,?,?,?)`,
		o.SchemaVersion, o.ActionSource, o.ActionType,
		o.HourOfDay, o.DayOfWeek, o.AppContext, o.EmotionBucket,
		o.EscalationLvl, int(o.Outcome), o.ResponseDelay, o.CreatedAt)
	return err
}

func (s *SQLiteStore) QueryOutcomes(ctx context.Context, filter port.OutcomeFilter) ([]types.ActionOutcome, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if filter.Limit <= 0 {
		filter.Limit = 100
	}

	var clauses []string
	var args []any
	if filter.Source != "" {
		clauses = append(clauses, "action_source = ?")
		args = append(args, filter.Source)
	}
	if filter.SinceSec > 0 {
		clauses = append(clauses, "created_at >= ?")
		args = append(args, filter.SinceSec)
	}
	where := ""
	if len(clauses) > 0 {
		where = "WHERE " + strings.Join(clauses, " AND ")
	}
	query := fmt.Sprintf(
		`SELECT id, schema_version, action_source, action_type, hour_of_day,
		        day_of_week, app_context, emotion_bucket, escalation_lvl,
		        outcome, response_delay, created_at
		 FROM outcomes %s ORDER BY created_at DESC LIMIT ?`, where)
	args = append(args, filter.Limit)

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var outcomes []types.ActionOutcome
	for rows.Next() {
		var o types.ActionOutcome
		var rawOutcome int
		if err := rows.Scan(&o.ID, &o.SchemaVersion, &o.ActionSource, &o.ActionType,
			&o.HourOfDay, &o.DayOfWeek, &o.AppContext, &o.EmotionBucket,
			&o.EscalationLvl, &rawOutcome, &o.ResponseDelay, &o.CreatedAt); err != nil {
			return nil, err
		}
		o.Outcome = types.OutcomeResult(rawOutcome)
		outcomes = append(outcomes, o)
	}
	return outcomes, rows.Err()
}

// ── Drive Records ───────────────────────────────────────────────────

func (s *SQLiteStore) SaveDriveRecord(ctx context.Context, r *types.DriveRecord) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now().Unix()
	if r.At == 0 {
		r.At = now
	}
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO drive_records (action, social, care, curious, quiet, explore, reward, at)
		 VALUES (?,?,?,?,?,?,?,?)`,
		r.Action, r.Social, r.Care, r.Curious, r.Quiet, r.Explore, r.Reward, r.At)
	if err != nil {
		return 0, err
	}
	id, _ := res.LastInsertId()
	return int(id), nil
}

func (s *SQLiteStore) ListDriveRecords(ctx context.Context, since int64, limit int) ([]types.DriveRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if limit <= 0 {
		limit = 200
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, action, social, care, curious, quiet, explore, reward, at
		 FROM drive_records WHERE at >= ? ORDER BY at ASC LIMIT ?`, since, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var recs []types.DriveRecord
	for rows.Next() {
		var r types.DriveRecord
		if err := rows.Scan(&r.ID, &r.Action, &r.Social, &r.Care, &r.Curious, &r.Quiet, &r.Explore, &r.Reward, &r.At); err != nil {
			return nil, err
		}
		recs = append(recs, r)
	}
	return recs, rows.Err()
}

// ── Stats ───────────────────────────────────────────────────────────

func (s *SQLiteStore) CountTodayMessages(ctx context.Context) int {
	s.mu.Lock()
	defer s.mu.Unlock()

	todayStart := time.Now().Truncate(24 * time.Hour).Unix()
	var n int
	_ = s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM messages WHERE created_at >= ?`, todayStart).Scan(&n)
	return n
}

// ── Maintenance ─────────────────────────────────────────────────────

func (s *SQLiteStore) RunForgetting(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now().Unix()

	// Archive expired state/episode facts
	_, _ = s.db.ExecContext(ctx,
		`UPDATE facts SET archived=1, updated_at=?
		 WHERE archived=0
		   AND temporal_scope='state'
		   AND auto_expire_days > 0
		   AND created_at + auto_expire_days*86400 < ?`,
		now, now,
	)

	// Archive facts with sub_zero_days >= 7 (extracted from evidence JSON)
	// Note: this is a heuristic scan; the evidence engine handles precision.
	_, _ = s.db.ExecContext(ctx,
		`UPDATE facts SET archived=1, updated_at=?
		 WHERE archived=0
		   AND json_extract(evidence, '$.sub_zero_days') >= 7`,
		now,
	)

	// Force-archive oldest facts when total exceeds threshold
	var total int
	_ = s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM facts WHERE archived=0`).Scan(&total)
	if total > 5000 {
		_, _ = s.db.ExecContext(ctx,
			`UPDATE facts SET archived=1, updated_at=?
			 WHERE id IN (
			   SELECT id FROM facts WHERE archived=0
			   ORDER BY created_at ASC LIMIT ?
			 )`, now, total/10,
		)
	}

	// Archive denied/archived reflections older than 90 days
	cutoff := time.Now().AddDate(0, 0, -90).Unix()
	_, _ = s.db.ExecContext(ctx,
		`DELETE FROM reflections WHERE status IN ('denied','archived') AND created_at < ?`, cutoff)

	return nil
}
