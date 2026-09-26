package combat

// NavalSeizureOutcomeHandler processes ScheduledNavalSeizureOutcome
// (megaron_plan_sjohandel_kraver_skepp.md R5) — the "captured" branch of a
// naval interception. transport.InterceptScanHandler.seize rolls the outcome
// and credits the cargo (transport package, G1: may not import combat); this
// event carries the ONE consequence that needs combat's march machinery —
// changing the ship's owner and turning it toward the captor's nearest own
// port — across that boundary. "limped"/"sunk" never reach here: transport
// resolves those directly with SQL + transport.Dispatch, no march involved.
//
// Idempotent by construction (events.Worker requires this, same shape as
// SiegeCapitulationHandler): re-reads the ship's live status under FOR UPDATE
// and no-ops if it isn't 'freighting' any more — a re-run after the first one
// already changed owner and started the march finds 'marching', not
// 'freighting', and stops there.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"formatet/megaron/server/internal/clock"
	"formatet/megaron/server/internal/events"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// NavalSeizureOutcomePayload is the ScheduledNavalSeizureOutcome payload.
type NavalSeizureOutcomePayload struct {
	TransportID uuid.UUID `json:"transport_id"`
	ShipUnitID  uuid.UUID `json:"ship_unit_id"`
	Outcome     string    `json:"outcome"`
	CaptorID    uuid.UUID `json:"captor_id"`
	Q           int       `json:"q"`
	R           int       `json:"r"`
}

// NavalSeizureOutcomeHandler processes ScheduledNavalSeizureOutcome events.
type NavalSeizureOutcomeHandler struct {
	pool      *pgxpool.Pool
	scheduler *events.Scheduler
	clk       clock.Clock
	hub       Broadcaster // nil-guarded (tests)
}

// NewNavalSeizureOutcomeHandler creates a NavalSeizureOutcomeHandler. hub may be nil.
func NewNavalSeizureOutcomeHandler(pool *pgxpool.Pool, sched *events.Scheduler, clk clock.Clock, hub Broadcaster) *NavalSeizureOutcomeHandler {
	return &NavalSeizureOutcomeHandler{pool: pool, scheduler: sched, clk: clk, hub: hub}
}

// Handle processes one ScheduledNavalSeizureOutcome event.
func (h *NavalSeizureOutcomeHandler) Handle(ctx context.Context, e events.ScheduledEvent) error {
	var p NavalSeizureOutcomePayload
	if err := json.Unmarshal(e.Payload, &p); err != nil {
		return fmt.Errorf("unmarshal naval seizure outcome: %w", err)
	}
	if p.Outcome != "captured" {
		return nil // defensive; only "captured" ever needs this handler
	}

	tx, err := h.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	var status, utype string
	var prevOwner uuid.UUID
	if err := tx.QueryRow(ctx,
		`SELECT status, type, owner_id FROM units WHERE id = $1 FOR UPDATE`, p.ShipUnitID,
	).Scan(&status, &utype, &prevOwner); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return tx.Commit(ctx) // ship gone entirely — nothing left to capture
		}
		return fmt.Errorf("load ship: %w", err)
	}
	if status != "freighting" {
		return tx.Commit(ctx) // already processed (retry), or no longer eligible
	}

	// Change hands and stamp its position at the capture hex — a freighting
	// ship carries no live q/r (R2: it's parked, invisible, the TRANSPORT
	// draws the map position, not the ship), so marchShipToNearestOwnPort's
	// own position lookup needs one set here before it can route a march.
	if _, err := tx.Exec(ctx,
		`UPDATE units SET owner_id = $2, q = $3, r = $4, updated_at = now() WHERE id = $1`,
		p.ShipUnitID, p.CaptorID, p.Q, p.R,
	); err != nil {
		return fmt.Errorf("change ship owner: %w", err)
	}

	if err := marchShipToNearestOwnPort(ctx, tx, h.clk, h.scheduler, e.WorldID,
		p.ShipUnitID, p.CaptorID, utype, e.DueTick, "captured_return"); err != nil {
		return fmt.Errorf("march captured ship home: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit: %w", err)
	}

	if h.hub != nil {
		_ = h.hub.NotifyPlayer(ctx, e.WorldID, p.CaptorID, "ShipCaptured", 3, map[string]any{
			"unit_id": p.ShipUnitID, "former_owner_id": prevOwner, "q": p.Q, "r": p.R,
		})
		_ = h.hub.NotifyPlayer(ctx, e.WorldID, prevOwner, "ShipLost", 3, map[string]any{
			"unit_id": p.ShipUnitID, "captor_id": p.CaptorID, "q": p.Q, "r": p.R,
		})
	}
	return nil
}
