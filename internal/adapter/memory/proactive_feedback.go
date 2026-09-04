package memory

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/SHIROH4/sion-plus/internal/domain/types"
)

func (s *SQLiteStore) SaveProactiveDecision(ctx context.Context, d *types.ProactiveDecision) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if d.DecisionID == "" {
		return fmt.Errorf("proactive decision id is required")
	}
	now := time.Now().Unix()
	if d.CreatedAt == 0 {
		d.CreatedAt = now
	}
	if d.State == "" {
		d.State = "delivered"
	}
	if d.DeliveredAt == 0 && d.State == "delivered" {
		d.DeliveredAt = now
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO proactive_decisions
		(decision_id, policy_version, action, source, score, context_json, candidates_json, content, state, created_at, delivered_at, resolved_at)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?)`, d.DecisionID, d.PolicyVersion, d.Action, d.Source, d.Score,
		d.ContextJSON, d.CandidatesJSON, d.Content, d.State, d.CreatedAt, d.DeliveredAt, d.ResolvedAt)
	return err
}

func (s *SQLiteStore) SaveProactiveFeedback(ctx context.Context, f *types.ProactiveFeedback) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if f.DecisionID == "" || f.Kind == "" {
		return fmt.Errorf("decision id and feedback kind are required")
	}
	if f.CreatedAt == 0 {
		f.CreatedAt = time.Now().Unix()
	}
	if f.EventID == "" {
		f.EventID = fmt.Sprintf("feedback:%s:%s:%d", f.DecisionID, f.Kind, f.CreatedAt)
	}
	if f.Confidence == 0 {
		f.Confidence = 1
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var state string
	if err := tx.QueryRowContext(ctx, `SELECT state FROM proactive_decisions WHERE decision_id=?`, f.DecisionID).Scan(&state); err != nil {
		return fmt.Errorf("load proactive decision: %w", err)
	}
	if state != "delivered" && state != "resolved" {
		return fmt.Errorf("feedback requires a delivered decision, got state %q", state)
	}
	request, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO proactive_feedback_requests
		(event_id, decision_id, created_at) VALUES (?,?,?)`, f.EventID, f.DecisionID, f.CreatedAt)
	if err != nil {
		return err
	}
	inserted, err := request.RowsAffected()
	if err != nil {
		return err
	}
	if inserted == 0 {
		return tx.Commit()
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO proactive_feedback
		(decision_id, kind, reward, source, confidence, note, created_at) VALUES (?,?,?,?,?,?,?)`,
		f.DecisionID, f.Kind, f.Reward, f.Source, f.Confidence, f.Note, f.CreatedAt); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO proactive_feedback_current
		(decision_id, event_id, kind, reward, source, confidence, note, created_at)
		VALUES (?,?,?,?,?,?,?,?) ON CONFLICT(decision_id) DO UPDATE SET
		event_id=excluded.event_id, kind=excluded.kind, reward=excluded.reward,
		source=excluded.source, confidence=excluded.confidence, note=excluded.note,
		created_at=excluded.created_at`, f.DecisionID, f.EventID, f.Kind, f.Reward,
		f.Source, f.Confidence, f.Note, f.CreatedAt); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *SQLiteStore) ResolveProactiveDecision(ctx context.Context, decisionID string, at int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if at == 0 {
		at = time.Now().Unix()
	}
	_, err := s.db.ExecContext(ctx, `UPDATE proactive_decisions SET state='resolved', resolved_at=? WHERE decision_id=?`, at, decisionID)
	return err
}

func (s *SQLiteStore) LatestPendingProactiveDecision(ctx context.Context, since int64) (*types.ProactiveDecision, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var d types.ProactiveDecision
	err := s.db.QueryRowContext(ctx, `SELECT decision_id, policy_version, action, source, score,
		context_json, candidates_json, content, state, created_at, delivered_at, resolved_at
		FROM proactive_decisions WHERE state='delivered' AND delivered_at>=?
		ORDER BY delivered_at DESC LIMIT 1`, since).Scan(&d.DecisionID, &d.PolicyVersion, &d.Action,
		&d.Source, &d.Score, &d.ContextJSON, &d.CandidatesJSON, &d.Content, &d.State,
		&d.CreatedAt, &d.DeliveredAt, &d.ResolvedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return &d, nil
}

func (s *SQLiteStore) SaveProactiveReply(ctx context.Context, reply *types.ProactiveReply) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if reply.DecisionID == "" || reply.Attribution == "" {
		return fmt.Errorf("decision id and reply attribution are required")
	}
	if reply.CreatedAt == 0 {
		reply.CreatedAt = time.Now().Unix()
	}
	_, err := s.db.ExecContext(ctx, `INSERT OR IGNORE INTO proactive_replies
		(decision_id, content, attribution, confidence, created_at) VALUES (?,?,?,?,?)`,
		reply.DecisionID, reply.Content, reply.Attribution, reply.Confidence, reply.CreatedAt)
	return err
}

func (s *SQLiteStore) ListProactiveDecisions(ctx context.Context, limit int) ([]types.ProactiveDecision, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if limit <= 0 {
		limit = 50
	}
	rows, err := s.db.QueryContext(ctx, `SELECT decision_id, policy_version, action, source, score, context_json, candidates_json, content, state, created_at, delivered_at, resolved_at FROM proactive_decisions ORDER BY created_at DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []types.ProactiveDecision
	for rows.Next() {
		var d types.ProactiveDecision
		if err := rows.Scan(&d.DecisionID, &d.PolicyVersion, &d.Action, &d.Source, &d.Score, &d.ContextJSON, &d.CandidatesJSON, &d.Content, &d.State, &d.CreatedAt, &d.DeliveredAt, &d.ResolvedAt); err != nil {
			return nil, err
		}
		result = append(result, d)
	}
	return result, rows.Err()
}

func (s *SQLiteStore) ActionFeedbackStats(ctx context.Context) ([]types.ActionFeedbackStats, error) {
	return s.actionFeedbackStats(ctx, "")
}

func (s *SQLiteStore) ActionFeedbackStatsForContext(ctx context.Context, contextKey string) ([]types.ActionFeedbackStats, error) {
	return s.actionFeedbackStats(ctx, contextKey)
}

func (s *SQLiteStore) actionFeedbackStats(ctx context.Context, contextKey string) ([]types.ActionFeedbackStats, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	query := `SELECT d.action, COUNT(f.decision_id), COALESCE(SUM(f.reward), 0),
		SUM(CASE WHEN f.reward > 0 THEN 1 ELSE 0 END), SUM(CASE WHEN f.reward < 0 THEN 1 ELSE 0 END)
		FROM proactive_decisions d JOIN proactive_feedback_current f ON f.decision_id=d.decision_id
	`
	args := []any{}
	if contextKey != "" {
		query += `WHERE json_extract(d.context_json, '$.policy_context') = ? `
		args = append(args, contextKey)
	}
	query += `GROUP BY d.action ORDER BY d.action`
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var stats []types.ActionFeedbackStats
	for rows.Next() {
		var stat types.ActionFeedbackStats
		if err := rows.Scan(&stat.Action, &stat.Samples, &stat.RewardSum, &stat.HelpfulCount, &stat.NegativeCount); err != nil {
			return nil, err
		}
		stats = append(stats, stat)
	}
	return stats, rows.Err()
}

func (s *SQLiteStore) EvaluateProactivePolicy(ctx context.Context, since int64) (*types.ProactivePolicyEvaluation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	eval := &types.ProactivePolicyEvaluation{SinceAt: since}
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*),
		COALESCE(SUM(CASE WHEN state IN ('delivered','resolved') THEN 1 ELSE 0 END),0),
		COALESCE(SUM(CASE WHEN state='blocked' THEN 1 ELSE 0 END),0),
		COALESCE(SUM(CASE WHEN state='silent' THEN 1 ELSE 0 END),0),
		COALESCE(SUM(CASE WHEN state='failed' THEN 1 ELSE 0 END),0),
		COALESCE(SUM(CASE WHEN json_extract(context_json,'$.shadow_policy.action') IS NOT NULL THEN 1 ELSE 0 END),0),
		COALESCE(SUM(CASE WHEN json_extract(context_json,'$.shadow_policy.would_differ')=1 THEN 1 ELSE 0 END),0)
		FROM proactive_decisions WHERE created_at>=?`, since).Scan(&eval.Opportunities,
		&eval.Delivered, &eval.Blocked, &eval.Silent, &eval.Failed,
		&eval.ShadowCompared, &eval.ShadowDifferent); err != nil {
		return nil, err
	}
	var negative int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*), COALESCE(AVG(f.reward),0),
		COALESCE(SUM(CASE WHEN f.reward<0 THEN 1 ELSE 0 END),0)
		FROM proactive_feedback_current f JOIN proactive_decisions d ON d.decision_id=f.decision_id
		WHERE d.created_at>=?`, since).Scan(&eval.ExplicitFeedback, &eval.AverageReward, &negative); err != nil {
		return nil, err
	}
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM proactive_replies r
		JOIN proactive_decisions d ON d.decision_id=r.decision_id WHERE d.created_at>=?`, since).Scan(&eval.ReplyCandidates); err != nil {
		return nil, err
	}
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*), COALESCE(AVG(f.reward),0)
		FROM proactive_decisions d JOIN proactive_feedback_current f ON f.decision_id=d.decision_id
		WHERE d.created_at>=? AND json_extract(d.context_json,'$.shadow_policy.action')=d.action`, since).
		Scan(&eval.ShadowMatchedFeedback, &eval.ShadowMatchedReward); err != nil {
		return nil, err
	}
	if eval.Delivered > 0 {
		eval.FeedbackRate = float64(eval.ExplicitFeedback) / float64(eval.Delivered)
		eval.ReplyCandidateRate = float64(eval.ReplyCandidates) / float64(eval.Delivered)
	}
	if eval.ExplicitFeedback > 0 {
		eval.NegativeRate = float64(negative) / float64(eval.ExplicitFeedback)
	}
	return eval, nil
}

func (s *SQLiteStore) UpsertProactiveControl(ctx context.Context, control *types.ProactiveControl) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if control.Scope != "global" && control.Scope != "category" && control.Scope != "action" {
		return fmt.Errorf("unsupported proactive control scope: %s", control.Scope)
	}
	if control.Mode != "muted" && control.Mode != "snoozed" {
		return fmt.Errorf("unsupported proactive control mode: %s", control.Mode)
	}
	if control.UpdatedAt == 0 {
		control.UpdatedAt = time.Now().Unix()
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO proactive_controls
		(scope, scope_value, mode, until_at, source, updated_at) VALUES (?,?,?,?,?,?)
		ON CONFLICT(scope, scope_value) DO UPDATE SET mode=excluded.mode,
		until_at=excluded.until_at, source=excluded.source, updated_at=excluded.updated_at`,
		control.Scope, control.ScopeValue, control.Mode, control.UntilAt, control.Source, control.UpdatedAt)
	return err
}

func (s *SQLiteStore) ClearProactiveControl(ctx context.Context, scope, scopeValue string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err := s.db.ExecContext(ctx, `DELETE FROM proactive_controls WHERE scope=? AND scope_value=?`, scope, scopeValue)
	return err
}

func (s *SQLiteStore) ProactiveAllowed(ctx context.Context, action, category string, at int64) (bool, string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if at == 0 {
		at = time.Now().Unix()
	}
	rows, err := s.db.QueryContext(ctx, `SELECT scope, scope_value, mode, until_at
		FROM proactive_controls WHERE
		(scope='global' AND scope_value='') OR (scope='category' AND scope_value=?) OR (scope='action' AND scope_value=?)`, category, action)
	if err != nil {
		return false, "", err
	}
	defer rows.Close()
	for rows.Next() {
		var scope, value, mode string
		var until int64
		if err := rows.Scan(&scope, &value, &mode, &until); err != nil {
			return false, "", err
		}
		if until == 0 || until > at {
			return false, scope + ":" + value + ":" + mode, nil
		}
	}
	return true, "", rows.Err()
}
