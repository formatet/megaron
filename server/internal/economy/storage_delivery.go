package economy

import (
	"context"
	"encoding/json"
	"formatet/megaron/server/internal/events"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type StorageNoticeDelivery struct {
	pool  *pgxpool.Pool
	store *events.Store
	hub   Broadcaster
}

func NewStorageNoticeDelivery(pool *pgxpool.Pool, store *events.Store, hub Broadcaster) *StorageNoticeDelivery {
	return &StorageNoticeDelivery{pool, store, hub}
}

// The credit already persisted the outcome and inbox. This job only delivers
// that same identity after commit; it never creates or resurrects a notice.
func (h *StorageNoticeDelivery) Handle(ctx context.Context, e events.ScheduledEvent) error {
	var p struct {
		NotificationID uuid.UUID `json:"notification_id"`
		EventID        int64     `json:"event_id"`
	}
	if err := json.Unmarshal(e.Payload, &p); err != nil {
		return err
	}
	tx, err := h.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	tag, err := tx.Exec(ctx, `INSERT INTO processed_deliveries(event_id) VALUES($1) ON CONFLICT DO NOTHING`, e.ID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return nil
	}
	var owner uuid.UUID
	var level int
	var raw json.RawMessage
	err = tx.QueryRow(ctx, `SELECT player_id,level,body_json FROM notifications WHERE id=$1 AND world_id=$2 AND kind='StorageOverflow'`, p.NotificationID, e.WorldID).Scan(&owner, &level, &raw)
	if err == pgx.ErrNoRows {
		return tx.Commit(ctx)
	}
	if err != nil {
		return err
	}
	var outcome events.Event
	if err := tx.QueryRow(ctx, `SELECT id,stream_id,stream_type,event_type,payload,world_id,created_at FROM events WHERE id=$1 AND world_id=$2 AND event_type='StorageOverflow'`, p.EventID, e.WorldID).Scan(&outcome.ID, &outcome.StreamID, &outcome.StreamType, &outcome.EventType, &outcome.Payload, &outcome.WorldID, &outcome.CreatedAt); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	h.store.RecordCommitted(ctx, &outcome)
	if d, ok := h.hub.(interface {
		DeliverCommittedNotification(context.Context, uuid.UUID, uuid.UUID, string, string, int, any)
	}); ok {
		d.DeliverCommittedNotification(ctx, e.WorldID, owner, p.NotificationID.String(), "StorageOverflow", level, raw)
	}
	return nil
}
