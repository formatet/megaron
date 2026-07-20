package combat

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/poleia/server/internal/events"
)

// UpkeepSpec — grain + silver per upkeep-period för en full-size enhet.
type UpkeepSpec struct {
	Grain  float64
	Silver float64
}

// UpkeepSpecs: landenheter skalas med size/100; navala är flat (size=1).
// Präst = ingen upkeep (kult kostar inget löpande).
var UpkeepSpecs = map[string]UpkeepSpec{
	"spearman":       {Grain: 5, Silver: 2},
	"elite_infantry": {Grain: 6, Silver: 4},
	"war_chariot":    {Grain: 8, Silver: 6},
	"galley":         {Grain: 4, Silver: 3},
	"war_galley":     {Grain: 6, Silver: 5},
	"merchantman":    {Grain: 3, Silver: 2},
	"priest":         {Grain: 0, Silver: 0},
}

// UnitUpkeep returns the grain + silver one unit costs per upkeep-period (the
// daily upkeep tick). Land units scale with size/100; naval and everything else
// are flat (per vessel); priest and unknown types cost nothing. This is the single
// source of truth for the scaling — both the charging loop (Handle) and the army
// read surface (api/handlers) call it, so shown upkeep can never drift from what
// is actually debited.
func UnitUpkeep(unitType, category string, size int) UpkeepSpec {
	spec, ok := UpkeepSpecs[unitType]
	if !ok {
		return UpkeepSpec{}
	}
	if category == "land" {
		f := float64(size) / 100.0
		return UpkeepSpec{Grain: spec.Grain * f, Silver: spec.Silver * f}
	}
	return spec // naval/other: flat
}

const (
	upkeepAttritionStep    = 10 // män förlorade per tick vid grain-brist
	upkeepDesertionStep    = 10 // män förlorade per tick vid silver-brist (efter tröskel)
	upkeepDesertionPeriods = 3  // obetalda silver-perioder före desertering börjar
)

// upkeepDailyTickPayload is the payload for the recurring upkeep tick.
type upkeepDailyTickPayload struct{}

// upkeepUnitRow holds the columns we need per unit during the upkeep loop.
type upkeepUnitRow struct {
	id            uuid.UUID
	ownerID       uuid.UUID
	unitType      string
	category      string
	size          int
	settlementID  *uuid.UUID
	unpaidPeriods int
	cargoUnitID   *uuid.UUID
}

// UpkeepHandler applies grain + silver upkeep to all active units each day.
// Grain-brist → attrition; silver-brist → desertering after upkeepDesertionPeriods.
type UpkeepHandler struct {
	pool      *pgxpool.Pool
	scheduler *events.Scheduler
	store     *events.Store
	hub       Broadcaster
}

// NewUpkeepHandler creates an UpkeepHandler. hub may be nil (tests) — every
// NotifyPlayer call is nil-guarded, matching the other combat handlers.
func NewUpkeepHandler(pool *pgxpool.Pool, sched *events.Scheduler, store *events.Store, hub Broadcaster) *UpkeepHandler {
	return &UpkeepHandler{pool: pool, scheduler: sched, store: store, hub: hub}
}

// Handle processes a ScheduledUpkeepTick event.
func (h *UpkeepHandler) Handle(ctx context.Context, e events.ScheduledEvent) error {
	// 1. Load all active units in the world.
	//
	// Units belonging to a player still in the founder phase are skipped: their
	// keep is already folded into founder_phase's grain/silver drain rate, and
	// there is no capital for payingSettlement to fall back on yet. Charging them
	// here would bill the same cohort twice (temenos_nomadic_host_bygg.md B3).
	// The exclusion lifts by itself at founding, when active flips to false.
	rows, err := h.pool.Query(ctx,
		`SELECT id, owner_id, type, category, size, settlement_id, unpaid_periods, cargo_unit_id
		 FROM units u
		 WHERE world_id = $1
		   AND status IN ('garrison', 'marching', 'positioned')
		   AND NOT EXISTS (
		       SELECT 1 FROM founder_phase fp
		       WHERE fp.world_id = u.world_id
		         AND fp.owner_id = u.owner_id
		         AND fp.active
		   )`,
		e.WorldID,
	)
	if err != nil {
		return fmt.Errorf("upkeep: query units: %w", err)
	}
	defer rows.Close()

	var units []upkeepUnitRow
	for rows.Next() {
		var u upkeepUnitRow
		if err := rows.Scan(&u.id, &u.ownerID, &u.unitType, &u.category,
			&u.size, &u.settlementID, &u.unpaidPeriods, &u.cargoUnitID); err != nil {
			return fmt.Errorf("upkeep: scan unit: %w", err)
		}
		units = append(units, u)
	}
	if err := rows.Err(); err != nil {
		return err
	}

	// 2. Cache capital settlement id per owner to avoid repeated queries.
	capitalCache := make(map[uuid.UUID]uuid.UUID)

	payingSettlement := func(u upkeepUnitRow) (uuid.UUID, bool) {
		if u.settlementID != nil {
			return *u.settlementID, true
		}
		if sid, ok := capitalCache[u.ownerID]; ok {
			return sid, true
		}
		var sid uuid.UUID
		err := h.pool.QueryRow(ctx,
			`SELECT id FROM settlements
			 WHERE owner_id = $1 AND world_id = $2 AND is_capital = true`,
			u.ownerID, e.WorldID,
		).Scan(&sid)
		if err != nil {
			return uuid.UUID{}, false
		}
		capitalCache[u.ownerID] = sid
		return sid, true
	}

	// Per-settlement silver/grain accounting for the UpkeepSettled audit event
	// (silver flow bookkeeping, Del A). Keyed by paying settlement; units with no
	// paying settlement (no capital yet) are not attributed.
	aggs := make(map[uuid.UUID]*upkeepAgg)
	agg := func(sid uuid.UUID) *upkeepAgg {
		a := aggs[sid]
		if a == nil {
			a = &upkeepAgg{}
			aggs[sid] = a
		}
		return a
	}

	// 3. Process each unit.
	for _, u := range units {
		up := UnitUpkeep(u.unitType, u.category, u.size)
		if up.Grain == 0 && up.Silver == 0 {
			continue // priest or unknown type — no upkeep
		}

		grainNeed := up.Grain
		silverNeed := up.Silver

		sid, hasSid := payingSettlement(u)

		// Track whether grain already disbanded the unit this tick.
		disbanded := false

		// ── Grain upkeep ─────────────────────────────────────────────────────
		if grainNeed > 0 {
			if !hasSid {
				// No paying settlement — treat as grain shortage.
				disbanded = h.applyAttrition(ctx, u, grainNeed, e.WorldID, sid)
			} else {
				tag, err := h.pool.Exec(ctx,
					`UPDATE settlement_goods
					 SET amount  = settled(amount, rate, calc_tick) - $1,
					     calc_tick = current_world_tick()
					 WHERE settlement_id = $2
					   AND good_key = 'grain'
					   AND settled(amount, rate, calc_tick) >= $1`,
					grainNeed, sid,
				)
				if err != nil {
					slog.Error("upkeep: grain deduction failed", "unit", u.id, "err", err)
				} else if tag.RowsAffected() == 0 {
					// Grain shortage — attrition.
					disbanded = h.applyAttrition(ctx, u, grainNeed, e.WorldID, sid)
				} else {
					agg(sid).grainTotal += grainNeed
				}
			}
		}

		if disbanded {
			continue
		}

		// ── Silver upkeep ────────────────────────────────────────────────────
		if silverNeed <= 0 {
			continue
		}

		if !hasSid {
			// No paying settlement — treat as unpaid (unknown loyalty → baseline).
			h.recordUnpaid(ctx, u, e.WorldID, defaultLoyalty, sid)
			continue
		}

		// L2: the supplying settlement's loyalty scales desertion severity.
		loyalty := settlementLoyalty(ctx, h.pool, sid)

		tag, err := h.pool.Exec(ctx,
			`UPDATE settlement_goods
			 SET amount   = LEAST(settled(amount, rate, calc_tick), cap) - $1,
			     calc_tick = current_world_tick()
			 WHERE settlement_id = $2 AND good_key = 'silver'
			   AND LEAST(settled(amount, rate, calc_tick), cap) >= $1`,
			silverNeed, sid,
		)
		if err != nil {
			slog.Error("upkeep: silver deduction failed", "unit", u.id, "err", err)
			continue
		}

		if tag.RowsAffected() > 0 {
			// Paid. Pre-Del-C the whole debit leaves the world (destroyed); Del C
			// splits gross into circulated (garrison sold spent at home) + destroyed.
			a := agg(sid)
			a.unitsPaid++
			a.silverGross += silverNeed
			a.silverDestroyed += silverNeed
			// Reset unpaid_periods if needed.
			if u.unpaidPeriods > 0 {
				if _, err := h.pool.Exec(ctx,
					`UPDATE units SET unpaid_periods = 0 WHERE id = $1`,
					u.id,
				); err != nil {
					slog.Error("upkeep: reset unpaid_periods failed", "unit", u.id, "err", err)
				}
			}
		} else {
			// Unpaid — silver the town couldn't afford. silver_unpaid keeps this
			// out of silver_destroyed: it never left the world, the gate stopped it.
			a := agg(sid)
			a.unitsUnpaid++
			a.silverUnpaid += silverNeed
			h.recordUnpaid(ctx, u, e.WorldID, loyalty, sid)
		}
	}

	// Silver flow bookkeeping (Del A): one UpkeepSettled per paying settlement,
	// one SilverAudit for the world — both best-effort, after the loop.
	h.emitUpkeepSettled(ctx, e.WorldID, aggs)
	h.emitSilverAudit(ctx, e.WorldID)

	// 4. Re-enqueue for the next daily cycle.
	return h.scheduler.EnqueueTickRecurring(ctx, e.WorldID, events.ScheduledUpkeepTick,
		upkeepDailyTickPayload{}, e.DueTick, events.TicksPerDay)
}

// notifyUnitLoss pushes a player-facing notification for grain attrition or
// silver desertion. Before this, these losses were audit-only (an event append
// with no NotifyPlayer), so units starved/deserted to death entirely silently —
// a whole army could evaporate without a single chip (Sparta 5000→300, live
// 2026-07-12). level 2 = the unit was destroyed; 3 = it merely bled men.
func (h *UpkeepHandler) notifyUnitLoss(ctx context.Context, u upkeepUnitRow, worldID, sid uuid.UUID, kind, reason string, lost int, disbanded bool) {
	if h.hub == nil {
		return
	}
	level := 3
	if disbanded {
		level = 2
	}
	// Dedupe (DEL D, megaron_ekonomi_legibilitet_plan.md): a unit bleeding men
	// day after day would otherwise notify every upkeep tick — spam on a sped-up
	// world. Skip if an UNREAD notification of the same kind for this unit already
	// exists. A destruction (disbanded) is never suppressed: it's the outcome the
	// Wanax most needs to see, even if an earlier bleed is still unread.
	if !disbanded {
		var exists bool
		if err := h.pool.QueryRow(ctx,
			`SELECT EXISTS (
			    SELECT 1 FROM notifications
			    WHERE world_id = $1 AND player_id = $2 AND kind = $3 AND read_at IS NULL
			      AND body_json->>'unit_id' = $4
			 )`,
			worldID, u.ownerID, kind, u.id.String(),
		).Scan(&exists); err == nil && exists {
			return
		}
	}
	payload := map[string]any{
		"unit_id":   u.id,
		"unit_type": u.unitType,
		"lost":      lost,
		"disbanded": disbanded,
		"reason":    reason,
	}
	if sid != (uuid.UUID{}) {
		payload["settlement_id"] = sid
	}
	_ = h.hub.NotifyPlayer(ctx, worldID, u.ownerID, kind, level, payload)
}

// cascadeCargoDisband disbands a ship's embarked cargo unit when the ship itself
// is disbanded (grain attrition or silver desertion). Mirrors collapse.go's
// cargo cascade — without this, a deserted/starved ship's cargo_unit_id points
// at a unit stuck in 'embarked' with no ship, unreachable by march/unload/disband.
func (h *UpkeepHandler) cascadeCargoDisband(ctx context.Context, shipID uuid.UUID, cargoUnitID *uuid.UUID) {
	if cargoUnitID == nil {
		return
	}
	if _, err := h.pool.Exec(ctx,
		`UPDATE units SET status = 'disbanded', updated_at = now() WHERE id = $1 AND status = 'embarked'`,
		*cargoUnitID,
	); err != nil {
		slog.Error("upkeep: disband cargo unit after ship loss", "ship", shipID, "cargo", *cargoUnitID, "err", err)
	}
}

// applyAttrition removes upkeepAttritionStep men from the unit due to grain shortage.
// Returns true if the unit was disbanded. sid = the settlement that failed to feed
// it (uuid.Nil if none), passed through to the notification for deep-linking.
func (h *UpkeepHandler) applyAttrition(ctx context.Context, u upkeepUnitRow, _ float64, worldID, sid uuid.UUID) bool {
	lost := upkeepAttritionStep
	if lost > u.size {
		lost = u.size
	}
	newSize := u.size - lost

	var disbanded bool
	var updateErr error
	if newSize <= 0 {
		_, updateErr = h.pool.Exec(ctx,
			`UPDATE units SET status = 'disbanded', size = 0, updated_at = now() WHERE id = $1`,
			u.id,
		)
		disbanded = true
	} else {
		_, updateErr = h.pool.Exec(ctx,
			`UPDATE units SET size = $1, updated_at = now() WHERE id = $2`,
			newSize, u.id,
		)
	}
	if updateErr != nil {
		slog.Error("upkeep: attrition update failed", "unit", u.id, "err", updateErr)
	}
	if disbanded {
		h.cascadeCargoDisband(ctx, u.id, u.cargoUnitID)
	}

	_, _ = h.store.Append(ctx, u.id, events.StreamCombat, "UnitAttrition",
		map[string]any{
			"unit_id":   u.id,
			"lost":      lost,
			"disbanded": disbanded,
			"reason":    "grain_shortage",
		},
		worldID, nil,
	)
	slog.Info("upkeep: grain attrition", "unit", u.id, "lost", lost, "disbanded", disbanded)
	h.notifyUnitLoss(ctx, u, worldID, sid, "UnitAttrition", "grain_shortage", lost, disbanded)
	return disbanded
}

// recordUnpaid increments unpaid_periods and applies desertion if the threshold is reached.
// loyalty is the supplying settlement's loyalty (L2): lower loyalty ⇒ more men desert.
// sid = the settlement that failed to pay (uuid.Nil if none), for the notification deep-link.
func (h *UpkeepHandler) recordUnpaid(ctx context.Context, u upkeepUnitRow, worldID uuid.UUID, loyalty int, sid uuid.UUID) {
	np := u.unpaidPeriods + 1

	if np >= upkeepDesertionPeriods {
		// Desertion — severity scales with the supplying settlement's loyalty.
		lost := desertionStepForLoyalty(loyalty)
		if lost > u.size {
			lost = u.size
		}
		newSize := u.size - lost

		var disbanded bool
		var updateErr error
		if newSize <= 0 {
			_, updateErr = h.pool.Exec(ctx,
				`UPDATE units SET status = 'disbanded', size = 0, unpaid_periods = $1, updated_at = now() WHERE id = $2`,
				np, u.id,
			)
			disbanded = true
		} else {
			_, updateErr = h.pool.Exec(ctx,
				`UPDATE units SET size = $1, unpaid_periods = $2, updated_at = now() WHERE id = $3`,
				newSize, np, u.id,
			)
		}
		if updateErr != nil {
			slog.Error("upkeep: desertion update failed", "unit", u.id, "err", updateErr)
		}
		if disbanded {
			h.cascadeCargoDisband(ctx, u.id, u.cargoUnitID)
		}

		_, _ = h.store.Append(ctx, u.id, events.StreamCombat, "UnitDeserted",
			map[string]any{
				"unit_id":   u.id,
				"lost":      lost,
				"disbanded": disbanded,
				"reason":    "silver_shortage",
			},
			worldID, nil,
		)
		slog.Info("upkeep: silver desertion", "unit", u.id, "lost", lost, "disbanded", disbanded)
		h.notifyUnitLoss(ctx, u, worldID, sid, "UnitDeserted", "silver_shortage", lost, disbanded)
	} else {
		// Not yet at threshold — just increment counter.
		if _, err := h.pool.Exec(ctx,
			`UPDATE units SET unpaid_periods = $1 WHERE id = $2`,
			np, u.id,
		); err != nil {
			slog.Error("upkeep: increment unpaid_periods failed", "unit", u.id, "err", err)
		}
	}
}
