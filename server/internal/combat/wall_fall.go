package combat

// Murarna faller (bönesystemet v3 steg 5c): Potnia's offensive prayer. The cast schedules a
// ScheduledWallsFall WallsFallTicks later; when it runs, the named city's wall loses one level
// (floor 0). The wall lives in two places, settlements.wall_level and the 'wall' row of buildings
// (migration 054's invariant: the row exists only while wall_level >= 1 and its level equals it),
// and both are lowered in one transaction. A battle already under way keeps its own snapshot
// (battle.go: battles.wall_level) - only battles begun afterwards see the new level.
// Idempotent on the cast: wall_falls has one row per cast event id.

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"formatet/megaron/server/internal/events"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// EventWallsFell is the outcome event: "the wall fell from X to Y".
const (
	EventWallsFell  = "WallsFell"
	NoticeWallsFell = "WallsFell"
)

// WallsFallPayload is the ScheduledWallsFall payload.
type WallsFallPayload struct {
	WorldID      uuid.UUID `json:"world_id"`
	SettlementID uuid.UUID `json:"settlement_id"`
	CastEventID  int64     `json:"cast_event_id"`
}

// WallsFell is the event body (no praying Wanax, ever).
type WallsFell struct {
	SettlementID uuid.UUID `json:"settlement_id"`
	WallBefore   int       `json:"wall_before"`
	WallAfter    int       `json:"wall_after"`
	CastEventID  int64     `json:"cast_event_id"`
}

// WallsFallHandler processes ScheduledWallsFall.
type WallsFallHandler struct {
	pool       *pgxpool.Pool
	eventStore *events.Store
	hub        Broadcaster
}

func NewWallsFallHandler(pool *pgxpool.Pool, store *events.Store, hub Broadcaster) *WallsFallHandler {
	return &WallsFallHandler{pool: pool, eventStore: store, hub: hub}
}

func (h *WallsFallHandler) Handle(ctx context.Context, e events.ScheduledEvent) error {
	var p WallsFallPayload
	if err := json.Unmarshal(e.Payload, &p); err != nil {
		return fmt.Errorf("unmarshal walls fall payload: %w", err)
	}
	tx, err := h.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	var owner *uuid.UUID
	var name, state string
	var wall, tickNow int
	if err := tx.QueryRow(ctx,
		`SELECT owner_id, name, state, wall_level, (SELECT current_tick FROM worlds WHERE id = world_id)
		   FROM settlements WHERE id = $1 FOR UPDATE`, p.SettlementID,
	).Scan(&owner, &name, &state, &wall, &tickNow); err != nil {
		if err == pgx.ErrNoRows {
			return tx.Commit(ctx) // the city is gone
		}
		return fmt.Errorf("walls fall: load settlement: %w", err)
	}
	// The cast is claimed once, before anything is changed; a re-run finds the claim and stops.
	tag, err := tx.Exec(ctx,
		`INSERT INTO wall_falls (cast_event_id, settlement_id, fell_tick, wall_before, wall_after)
		 VALUES ($1, $2, $3, $4, GREATEST($4 - 1, 0)) ON CONFLICT (cast_event_id) DO NOTHING`,
		p.CastEventID, p.SettlementID, tickNow, wall)
	if err != nil {
		return fmt.Errorf("walls fall: claim: %w", err)
	}
	if tag.RowsAffected() == 0 || wall <= 0 || state == "sunk" {
		return tx.Commit(ctx) // already carried out, or nothing left to fall (the gift is taken all the same)
	}
	after := wall - 1
	if _, err := tx.Exec(ctx, `UPDATE settlements SET wall_level = $2, updated_at = now() WHERE id = $1`, p.SettlementID, after); err != nil {
		return fmt.Errorf("walls fall: lower wall_level: %w", err)
	}
	if after == 0 {
		_, err = tx.Exec(ctx, `DELETE FROM buildings WHERE settlement_id = $1 AND building_type = 'wall'`, p.SettlementID)
	} else {
		_, err = tx.Exec(ctx, `UPDATE buildings SET level = $2 WHERE settlement_id = $1 AND building_type = 'wall'`, p.SettlementID, after)
	}
	if err != nil {
		return fmt.Errorf("walls fall: lower wall building: %w", err)
	}
	body := WallsFell{SettlementID: p.SettlementID, WallBefore: wall, WallAfter: after, CastEventID: p.CastEventID}
	ev, err := h.eventStore.AppendTx(ctx, tx, p.SettlementID, events.StreamProvince, EventWallsFell, body, p.WorldID, nil)
	if err != nil {
		return fmt.Errorf("walls fall: append event: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	h.eventStore.RecordCommitted(ctx, ev)
	if h.hub != nil && owner != nil {
		// The owner always hears it about their own city; never who prayed.
		notice := map[string]any{"settlement_id": p.SettlementID, "name": name, "wall_after": after}
		if err := h.hub.NotifyPlayer(ctx, p.WorldID, *owner, NoticeWallsFell, 3, notice); err != nil {
			slog.Warn("walls fall: notify failed", "settlement", p.SettlementID, "err", err)
		}
	}
	return nil
}
