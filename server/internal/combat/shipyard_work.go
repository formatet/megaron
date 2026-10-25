package combat

import (
	"context"
	"encoding/json"
	"formatet/megaron/server/internal/economy"
	"formatet/megaron/server/internal/events"
	"formatet/megaron/server/internal/unit"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"log/slog"
)

// New event type: legacy TrainComplete/ShipRepairComplete semantics stay frozen.
type ShipyardWorkCompletePayload struct {
	UnitID   uuid.UUID `json:"unit_id"`
	Revision int       `json:"revision"`
}
type ShipyardWorkCompleteHandler struct {
	pool  *pgxpool.Pool
	store *events.Store
	hub   Broadcaster
}

func NewShipyardWorkCompleteHandler(pool *pgxpool.Pool, store *events.Store, hub Broadcaster) *ShipyardWorkCompleteHandler {
	return &ShipyardWorkCompleteHandler{pool, store, hub}
}

// Handle settles saved progress and locks the unit across completion + audit.
// A stale timer, a paused yard or a retry cannot finish/notify a job twice.
func (h *ShipyardWorkCompleteHandler) Handle(ctx context.Context, e events.ScheduledEvent) error {
	var p ShipyardWorkCompletePayload
	if err := json.Unmarshal(e.Payload, &p); err != nil {
		return err
	}
	tx, err := h.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var revision int
	var remaining *int
	if err = tx.QueryRow(ctx, `SELECT shipyard_work_revision,shipyard_work_remaining FROM units WHERE id=$1 AND world_id=$2 FOR UPDATE`, p.UnitID, e.WorldID).Scan(&revision, &remaining); err == pgx.ErrNoRows {
		return nil
	} else if err != nil {
		return err
	}
	if remaining == nil || revision != p.Revision {
		return nil
	}
	if _, err = tx.Exec(ctx, `SELECT sync_shipyard_work($1)`, p.UnitID); err != nil {
		return err
	}
	var running bool
	var status, typ string
	var city, owner uuid.UUID
	if err = tx.QueryRow(ctx, `SELECT shipyard_work_remaining,shipyard_work_running,status,type,settlement_id,owner_id FROM units WHERE id=$1`, p.UnitID).Scan(&remaining, &running, &status, &typ, &city, &owner); err != nil {
		return err
	}
	if remaining == nil || *remaining > 0 || !running {
		return tx.Commit(ctx)
	}
	if _, err = tx.Exec(ctx, `UPDATE units SET status='garrison',hull=CASE WHEN status='repairing' THEN $2 ELSE hull END,build_complete_at=NULL,shipyard_work_remaining=NULL,shipyard_work_running=false,updated_at=now() WHERE id=$1`, p.UnitID, HullMax); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `UPDATE scheduled_events SET processed_at=now() WHERE event_type='ShipyardWorkComplete' AND payload->>'unit_id'=$1 AND processed_at IS NULL`, p.UnitID.String()); err != nil {
		return err
	}
	kind := "TrainComplete"
	body := map[string]any{"unit_id": p.UnitID, "unit_type": typ, "settlement_id": city, "count": 1, "size_after": 0}
	var settlementName string
	_ = tx.QueryRow(ctx, `SELECT name FROM settlements WHERE id=$1`, city).Scan(&settlementName)
	body["settlement_name"] = settlementName
	body["unit_name"] = unit.DisplayName(typ)
	stream := city
	if status == "repairing" {
		kind = EventShipRepaired
		body["hull"] = HullMax
		body["world_id"] = e.WorldID
		stream = p.UnitID
	}
	if h.store != nil {
		streamType := events.StreamProvince
		if status == "repairing" {
			streamType = events.StreamType(unit.StreamUnit)
		}
		if _, err = h.store.AppendTx(ctx, tx, stream, streamType, kind, body, e.WorldID, nil); err != nil {
			return err
		}
	}
	if err = tx.Commit(ctx); err != nil {
		return err
	}
	if err = economy.RecomputeProduction(ctx, h.pool, city); err != nil {
		slog.Warn("shipyard completion production", "err", err)
	}
	if h.hub != nil {
		level := 4
		if status == "repairing" {
			level = 3
			body["name"] = unit.LoadDisplayName(ctx, h.pool, p.UnitID)
		}
		if err = h.hub.NotifyPlayer(ctx, e.WorldID, owner, kind, level, body); err != nil {
			slog.Warn("shipyard completion notify", "err", err)
		}
	}
	return nil
}
