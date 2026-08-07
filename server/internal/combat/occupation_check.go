package combat

// OccupationCheckHandler processes ScheduledOccupationCheck (S2,
// megaron_plan_erovring.md) — see occupation.go's file header and
// scheduleOccupationCheck's doc comment for the self-correcting shape: this
// always re-reads the LIVE occupied_since_tick rather than trusting the
// deadline it was scheduled for, so a counter reset (occupation.go's
// resetOccupationDefense) after this check was enqueued is picked up
// automatically instead of firing a stale notification.

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

// OccupationCheckPayload is the ScheduledOccupationCheck payload.
type OccupationCheckPayload struct {
	SettlementID uuid.UUID `json:"settlement_id"`
}

// OccupationCheckHandler processes ScheduledOccupationCheck events.
type OccupationCheckHandler struct {
	pool      *pgxpool.Pool
	scheduler *events.Scheduler
	hub       Broadcaster
}

// NewOccupationCheckHandler creates an OccupationCheckHandler. hub may be nil
// in tests (every NotifyPlayer call below is nil-guarded).
func NewOccupationCheckHandler(pool *pgxpool.Pool, scheduler *events.Scheduler, hub Broadcaster) *OccupationCheckHandler {
	return &OccupationCheckHandler{pool: pool, scheduler: scheduler, hub: hub}
}

// Handle processes one ScheduledOccupationCheck event.
func (h *OccupationCheckHandler) Handle(ctx context.Context, e events.ScheduledEvent) error {
	var p OccupationCheckPayload
	if err := json.Unmarshal(e.Payload, &p); err != nil {
		return fmt.Errorf("unmarshal occupation check payload: %w", err)
	}

	tx, err := h.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	var state, name string
	var occupantID *uuid.UUID
	var sinceTick *int
	var alreadyNotified bool
	if err := tx.QueryRow(ctx,
		`SELECT state, occupant_id, occupied_since_tick, name, annex_ready_notified
		 FROM settlements WHERE id = $1 FOR UPDATE`, p.SettlementID,
	).Scan(&state, &occupantID, &sinceTick, &name, &alreadyNotified); err != nil {
		if err == pgx.ErrNoRows {
			return tx.Commit(ctx) // settlement gone — nothing left to watch
		}
		return fmt.Errorf("occupation check: load settlement: %w", err)
	}

	// Occupation ended one way or another (annexed/sacked/burned/recaptured
	// into an active or razed state) since this check was scheduled — stop
	// watching, idempotent no-op.
	if state != "occupied" || occupantID == nil || sinceTick == nil {
		return tx.Commit(ctx)
	}

	var currentTick int
	_ = tx.QueryRow(ctx, `SELECT current_world_tick()`).Scan(&currentTick)
	elapsed := currentTick - *sinceTick

	if elapsed >= occupationTicksToAnnex {
		if !alreadyNotified {
			if _, err := tx.Exec(ctx, `UPDATE settlements SET annex_ready_notified = true WHERE id = $1`, p.SettlementID); err != nil {
				return fmt.Errorf("occupation check: mark notified: %w", err)
			}
			if h.hub != nil {
				_ = h.hub.NotifyPlayer(ctx, e.WorldID, *occupantID, "CityAnnexReady", 2, map[string]any{
					"settlement_id": p.SettlementID, "name": name,
				})
			}
			slog.Info("occupation matured — annex offered", "settlement", p.SettlementID, "occupant", *occupantID)
		}
		// Matured — no further automated check needed. A later attack that
		// resets the counter re-schedules its own check (occupation.go's
		// resetOccupationDefense), so annex readiness is never silently
		// stuck stale.
		return tx.Commit(ctx)
	}

	// Not matured yet — the counter was reset after this check was scheduled.
	// Re-enqueue against the corrected, live deadline.
	if err := scheduleOccupationCheck(ctx, tx, h.scheduler, e.WorldID, p.SettlementID, *sinceTick); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
