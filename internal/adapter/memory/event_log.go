package memory

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/shirohania/sion/internal/domain/types"
)

// EventLog is an append-only audit trail backed by SQLite.
// Records every fact creation, signal application, reflection state change,
// and evidence update. Used for debugging, funnel analytics, and crash recovery.
type EventLog struct {
	store *SQLiteStore
}

// NewEventLog creates an event logger bound to a SQLite store.
func NewEventLog(store *SQLiteStore) *EventLog {
	return &EventLog{store: store}
}

// ── Emit ───────────────────────────────────────────────────────────

func (el *EventLog) emit(ctx context.Context, eventType string, entityID int64, entityKind string, payload any) error {
	payloadJSON, _ := json.Marshal(payload)
	_, err := el.store.db.ExecContext(ctx,
		`INSERT INTO events (type, entity_id, entity_kind, payload, created_at)
		 VALUES (?,?,?,?,?)`,
		eventType, entityID, entityKind, string(payloadJSON), time.Now().Unix(),
	)
	return err
}

// ── Fact events ────────────────────────────────────────────────────

type factAddedPayload struct {
	Content      string `json:"content"`
	Entity       string `json:"entity"`
	RelationType string `json:"relation_type"`
	SourceTier   string `json:"source_tier"`
	Importance   int    `json:"importance"`
}

func (el *EventLog) LogFactAdded(ctx context.Context, fact *types.FactEntry) error {
	return el.emit(ctx, types.EvtFactAdded, fact.ID, "fact", factAddedPayload{
		Content:      fact.Content,
		Entity:       fact.Entity,
		RelationType: fact.RelationType,
		SourceTier:   string(fact.SourceTier),
		Importance:   fact.Importance,
	})
}

type factSignalPayload struct {
	SignalType string  `json:"signal_type"`
	ReinBefore float64 `json:"rein_before"`
	ReinAfter  float64 `json:"rein_after"`
	DispBefore float64 `json:"disp_before"`
	DispAfter  float64 `json:"disp_after"`
	ScoreAfter float64 `json:"score_after"`
}

func (el *EventLog) LogFactSignalApplied(ctx context.Context, fact *types.FactEntry, sigType string, snap *types.EvidenceSnapshot) error {
	return el.emit(ctx, types.EvtFactSignalApplied, fact.ID, "fact", factSignalPayload{
		SignalType: sigType,
		ReinAfter:  snap.Reinforcement,
		ScoreAfter: snap.EvidenceScore,
	})
}

func (el *EventLog) LogFactArchived(ctx context.Context, factID int64) error {
	return el.emit(ctx, types.EvtFactArchived, factID, "fact", nil)
}

func (el *EventLog) LogFactAbsorbed(ctx context.Context, factID int64, reflectionID int64) error {
	return el.emit(ctx, types.EvtFactAbsorbed, factID, "fact", map[string]int64{"reflection_id": reflectionID})
}

// ── Reflection events ──────────────────────────────────────────────

type reflectionStatePayload struct {
	FromStatus string `json:"from_status"`
	ToStatus   string `json:"to_status"`
	Feedback   string `json:"feedback,omitempty"`
}

func (el *EventLog) LogReflectionSynthesized(ctx context.Context, reflectionID int64, sourceCount int) error {
	return el.emit(ctx, types.EvtReflectionSynthesized, reflectionID, "reflection",
		map[string]int{"source_fact_count": sourceCount})
}

func (el *EventLog) LogReflectionStateChanged(ctx context.Context, reflectionID int64, from, to string, feedback string) error {
	return el.emit(ctx, types.EvtReflectionStateChanged, reflectionID, "reflection", reflectionStatePayload{
		FromStatus: from,
		ToStatus:   to,
		Feedback:   feedback,
	})
}

// ── Evidence events ────────────────────────────────────────────────

type evidenceUpdatedPayload struct {
	Reinforcement float64 `json:"reinforcement"`
	Disputation   float64 `json:"disputation"`
	Score         float64 `json:"score"`
	Status        string  `json:"status"`
	Oscillating   bool    `json:"oscillating"`
}

func (el *EventLog) LogEvidenceUpdated(ctx context.Context, fact *types.FactEntry, snap *types.EvidenceSnapshot) error {
	return el.emit(ctx, types.EvtEvidenceUpdated, fact.ID, "fact", evidenceUpdatedPayload{
		Reinforcement: snap.Reinforcement,
		Disputation:   snap.Disputation,
		Score:         snap.EvidenceScore,
		Status:        snap.Status,
		Oscillating:   fact.Oscillating,
	})
}

// ── Query ──────────────────────────────────────────────────────────

// FunnelCounts returns the count of events by type since a given timestamp.
// Used for the evidence funnel analytics dashboard.
func (el *EventLog) FunnelCounts(ctx context.Context, since int64) (map[string]int, error) {
	rows, err := el.store.db.QueryContext(ctx,
		`SELECT type, COUNT(*) FROM events WHERE created_at >= ? GROUP BY type`, since)
	if err != nil {
		return nil, fmt.Errorf("FunnelCounts: %w", err)
	}
	defer rows.Close()

	counts := make(map[string]int)
	for rows.Next() {
		var typ string
		var n int
		if err := rows.Scan(&typ, &n); err != nil {
			return nil, err
		}
		counts[typ] = n
	}
	return counts, rows.Err()
}

// QueryEvents returns recent events of the given types.
func (el *EventLog) QueryEvents(ctx context.Context, eventTypes []string, limit int) ([]types.EventLogEntry, error) {
	if limit <= 0 {
		limit = 100
	}

	query := `SELECT id, type, entity_id, entity_kind, payload, created_at
		 FROM events ORDER BY created_at DESC LIMIT ?`
	rows, err := el.store.db.QueryContext(ctx, query, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entries []types.EventLogEntry
	for rows.Next() {
		var e types.EventLogEntry
		if err := rows.Scan(&e.ID, &e.Type, &e.EntityID, &e.EntityKind, &e.Payload, &e.CreatedAt); err != nil {
			return nil, err
		}
		entries = append(entries, e)
	}
	return entries, rows.Err()
}

// QueryEventsByEntity returns all events for a specific entity.
func (el *EventLog) QueryEventsByEntity(ctx context.Context, entityKind string, entityID int64, limit int) ([]types.EventLogEntry, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := el.store.db.QueryContext(ctx,
		`SELECT id, type, entity_id, entity_kind, payload, created_at
		 FROM events WHERE entity_kind=? AND entity_id=?
		 ORDER BY created_at DESC LIMIT ?`, entityKind, entityID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entries []types.EventLogEntry
	for rows.Next() {
		var e types.EventLogEntry
		if err := rows.Scan(&e.ID, &e.Type, &e.EntityID, &e.EntityKind, &e.Payload, &e.CreatedAt); err != nil {
			return nil, err
		}
		entries = append(entries, e)
	}
	return entries, rows.Err()
}
