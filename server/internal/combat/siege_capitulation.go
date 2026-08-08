package combat

// SiegeCapitulationHandler processes ScheduledSiegeCapitulation (belägring
// S3, megaron_plan_belagring.md §S3): a settlement whose starvation clock
// (settlements.siege_starvation_ticks, owned and incremented by
// internal/kharis/tick.go's daily decay) reached economy.SiegeCapitulationTicks
// falls to its strongest besieger — muren rörs aldrig, magen ger upp.
//
// Reuses the SAME occupied-state transition a battle win uses
// (occupation.go's occupySettlement): state='occupied', occupant_id set,
// owner_id UNCHANGED, occupied_since_tick reset to now, and the normal
// ExecuteOccupyAction (sack/burn/annex) flow + ScheduledOccupationCheck
// annex-maturity watch take over unchanged from there — the plan calls this
// "initiateOrJoin-erövringsflödet" explicitly. The one behavioural
// difference from a battle-won occupation: no attacking army marched to the
// hex, so no unit changes owner or moves — the besieging sentry unit(s)
// stay exactly where they were holding the chokepoint. Only the stale
// defending garrison is evicted (evictStaleGarrison, shared with
// occupySettlement) since a starved-out city cannot be left showing a live
// defender garrison either.
//
// Idempotent by construction (events.Worker requires this): re-reads
// besieged/state live and no-ops if the siege was relieved between
// scheduling and delivery (blockade lifted, city already fell some other
// way, or already occupied) rather than trusting the event was scheduled
// correctly.

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"formatet/megaron/server/internal/economy"
	"formatet/megaron/server/internal/events"
	"formatet/megaron/server/internal/gossip"
	"formatet/megaron/server/internal/hexgrid"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// SiegeCapitulationPayload is the ScheduledSiegeCapitulation payload.
type SiegeCapitulationPayload struct {
	SettlementID uuid.UUID `json:"settlement_id"`
	WorldID      uuid.UUID `json:"world_id"`
}

// SiegeCapitulationHandler processes ScheduledSiegeCapitulation events.
type SiegeCapitulationHandler struct {
	pool       *pgxpool.Pool
	eventStore *events.Store
	scheduler  *events.Scheduler
	hub        Broadcaster
}

// NewSiegeCapitulationHandler creates a SiegeCapitulationHandler. hub may be
// nil in tests (every NotifyPlayer call below is nil-guarded, matching the
// other combat handlers).
func NewSiegeCapitulationHandler(pool *pgxpool.Pool, store *events.Store, scheduler *events.Scheduler, hub Broadcaster) *SiegeCapitulationHandler {
	return &SiegeCapitulationHandler{pool: pool, eventStore: store, scheduler: scheduler, hub: hub}
}

// Handle processes one ScheduledSiegeCapitulation event.
func (h *SiegeCapitulationHandler) Handle(ctx context.Context, e events.ScheduledEvent) error {
	var p SiegeCapitulationPayload
	if err := json.Unmarshal(e.Payload, &p); err != nil {
		return fmt.Errorf("unmarshal siege capitulation payload: %w", err)
	}

	tx, err := h.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	var ownerID uuid.UUID
	var besieged bool
	var state, name string
	var provinceID uuid.UUID
	if err := tx.QueryRow(ctx,
		`SELECT owner_id, besieged, state, name, province_id
		 FROM settlements WHERE id = $1 FOR UPDATE`, p.SettlementID,
	).Scan(&ownerID, &besieged, &state, &name, &provinceID); err != nil {
		if err == pgx.ErrNoRows {
			return tx.Commit(ctx) // settlement gone — nothing left to capitulate
		}
		return fmt.Errorf("siege capitulation: load settlement: %w", err)
	}

	// The siege was relieved, or the city already fell some other way,
	// between scheduling and delivery — idempotent no-op, same shape as
	// OccupationCheckHandler's own live re-check.
	if state != "active" || !besieged {
		return tx.Commit(ctx)
	}

	var q, r int
	if err := tx.QueryRow(ctx, `SELECT map_q, map_r FROM provinces WHERE id = $1`, provinceID).Scan(&q, &r); err != nil {
		return fmt.Errorf("siege capitulation: load province: %w", err)
	}

	besiegers, err := economy.LoadBesiegers(ctx, tx, p.WorldID, ownerID, hexgrid.Coord{Q: q, R: r})
	if err != nil {
		return fmt.Errorf("siege capitulation: load besiegers: %w", err)
	}
	if len(besiegers) == 0 {
		// Defensive: the "billig förkoll" that gates ReachableCatchmentHexes
		// found nobody within range either — the blockade lifted at the
		// exact moment of delivery. Reset the clock and stop; tomorrow's
		// daily tick will re-derive `besieged` from scratch.
		if _, err := tx.Exec(ctx,
			`UPDATE settlements SET siege_starvation_ticks = 0 WHERE id = $1`, p.SettlementID,
		); err != nil {
			return fmt.Errorf("siege capitulation: reset clock: %w", err)
		}
		return tx.Commit(ctx)
	}
	occupantOwnerID := besiegers[0].OwnerID // strongest holder (LoadBesiegers orders by size DESC)

	if err := evictStaleGarrison(ctx, tx, p.SettlementID, occupantOwnerID); err != nil {
		return err
	}

	var currentTick int
	_ = tx.QueryRow(ctx, `SELECT current_world_tick()`).Scan(&currentTick)

	if _, err := tx.Exec(ctx,
		`UPDATE settlements SET
		   state = 'occupied', occupant_id = $2, occupied_since_tick = $3,
		   besieged = false, siege_starvation_ticks = 0, annex_ready_notified = false, updated_at = now()
		 WHERE id = $1`,
		p.SettlementID, occupantOwnerID, currentTick,
	); err != nil {
		return fmt.Errorf("siege capitulation: mark settlement occupied: %w", err)
	}

	_, _ = h.eventStore.Append(ctx, p.SettlementID, events.StreamProvince, EventSettlementOccupied,
		SettlementOccupiedPayload{
			SettlementID: p.SettlementID, WorldID: p.WorldID, OccupantID: occupantOwnerID,
			FormerOwnerID: &ownerID, Q: q, R: r, OccupiedTick: currentTick,
		}, p.WorldID, nil)

	if err := gossip.Broadcast(ctx, tx, p.WorldID, p.SettlementID, "military",
		name+" has starved into submission after a long siege — occupied by its besieger.",
		6, gossip.ImportanceMajor, p.SettlementID, ""); err != nil {
		slog.Warn("siege capitulation: broadcast gossip", "settlement", p.SettlementID, "err", err)
	}

	if h.hub != nil {
		_ = h.hub.NotifyPlayer(ctx, p.WorldID, ownerID, "CityOccupied", 1, map[string]any{
			"settlement_id": p.SettlementID, "name": name, "role": "defender", "cause": "starvation",
			"occupation_ticks_to_annex": occupationTicksToAnnex,
		})
		_ = h.hub.NotifyPlayer(ctx, p.WorldID, occupantOwnerID, "CityOccupied", 1, map[string]any{
			"settlement_id": p.SettlementID, "name": name, "role": "attacker", "cause": "starvation",
			"occupation_ticks_to_annex": occupationTicksToAnnex,
			"choices":                   []string{"occupy", "sack", "burn"},
			"default":                   "occupy",
		})
	}

	if err := scheduleOccupationCheck(ctx, tx, h.scheduler, p.WorldID, p.SettlementID, currentTick); err != nil {
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit siege capitulation: %w", err)
	}
	slog.Info("siege capitulation", "settlement", p.SettlementID, "occupant", occupantOwnerID)
	return nil
}
