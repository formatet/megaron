package combat

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/poleia/server/internal/events"
	"github.com/poleia/server/internal/timescale"
)

// UpkeepSpec — grain + silver per upkeep-period för en full-size enhet.
type UpkeepSpec struct {
	Grain  float64
	Silver float64
}

// UpkeepSpecs: landenheter skalas med size/100; navala är flat (size=1).
// Präst = ingen upkeep (kult kostar inget löpande).
var UpkeepSpecs = map[string]UpkeepSpec{
	"infantry":       {Grain: 5, Silver: 2},
	"elite_infantry": {Grain: 6, Silver: 4},
	"chariot":        {Grain: 8, Silver: 6},
	"ship":           {Grain: 4, Silver: 3},
	"war_galley":     {Grain: 6, Silver: 5},
	"merchantman":    {Grain: 3, Silver: 2},
	"priest":         {Grain: 0, Silver: 0},
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
}

// UpkeepHandler applies grain + silver upkeep to all active units each day.
// Grain-brist → attrition; silver-brist → desertering after upkeepDesertionPeriods.
type UpkeepHandler struct {
	pool      *pgxpool.Pool
	scheduler *events.Scheduler
	store     *events.Store
}

// NewUpkeepHandler creates an UpkeepHandler.
func NewUpkeepHandler(pool *pgxpool.Pool, sched *events.Scheduler, store *events.Store) *UpkeepHandler {
	return &UpkeepHandler{pool: pool, scheduler: sched, store: store}
}

// Handle processes a ScheduledUpkeepTick event.
func (h *UpkeepHandler) Handle(ctx context.Context, e events.ScheduledEvent) error {
	// 1. Load all active units in the world.
	rows, err := h.pool.Query(ctx,
		`SELECT id, owner_id, type, category, size, settlement_id, unpaid_periods
		 FROM units
		 WHERE world_id = $1
		   AND status IN ('garrison', 'marching', 'positioned')`,
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
			&u.size, &u.settlementID, &u.unpaidPeriods); err != nil {
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

	// 3. Process each unit.
	for _, u := range units {
		spec, ok := UpkeepSpecs[u.unitType]
		if !ok || (spec.Grain == 0 && spec.Silver == 0) {
			continue // priest or unknown type — no upkeep
		}

		sizeFactor := 1.0
		if u.category == "land" {
			sizeFactor = float64(u.size) / 100.0
		}

		grainNeed := spec.Grain * sizeFactor
		silverNeed := spec.Silver * sizeFactor

		sid, hasSid := payingSettlement(u)

		// Track whether grain already disbanded the unit this tick.
		disbanded := false

		// ── Grain upkeep ─────────────────────────────────────────────────────
		if grainNeed > 0 {
			if !hasSid {
				// No paying settlement — treat as grain shortage.
				disbanded = h.applyAttrition(ctx, u, grainNeed, e.WorldID)
			} else {
				tag, err := h.pool.Exec(ctx,
					`UPDATE settlement_goods
					 SET amount  = settled(amount, rate, calc_at) - $1,
					     calc_at = now()
					 WHERE settlement_id = $2
					   AND good_key = 'grain'
					   AND settled(amount, rate, calc_at) >= $1`,
					grainNeed, sid,
				)
				if err != nil {
					slog.Error("upkeep: grain deduction failed", "unit", u.id, "err", err)
				} else if tag.RowsAffected() == 0 {
					// Grain shortage — attrition.
					disbanded = h.applyAttrition(ctx, u, grainNeed, e.WorldID)
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
			// No paying settlement — treat as unpaid.
			h.recordUnpaid(ctx, u, e.WorldID)
			continue
		}

		tag, err := h.pool.Exec(ctx,
			`UPDATE settlement_goods
			 SET amount   = LEAST(settled(amount, rate, calc_at), cap) - $1,
			     calc_at  = now()
			 WHERE settlement_id = $2 AND good_key = 'silver'
			   AND LEAST(settled(amount, rate, calc_at), cap) >= $1`,
			silverNeed, sid,
		)
		if err != nil {
			slog.Error("upkeep: silver deduction failed", "unit", u.id, "err", err)
			continue
		}

		if tag.RowsAffected() > 0 {
			// Paid — reset unpaid_periods if needed.
			if u.unpaidPeriods > 0 {
				if _, err := h.pool.Exec(ctx,
					`UPDATE units SET unpaid_periods = 0 WHERE id = $1`,
					u.id,
				); err != nil {
					slog.Error("upkeep: reset unpaid_periods failed", "unit", u.id, "err", err)
				}
			}
		} else {
			// Unpaid.
			h.recordUnpaid(ctx, u, e.WorldID)
		}
	}

	// 4. Re-enqueue for the next daily cycle.
	return h.scheduler.EnqueueAfter(ctx, e.WorldID, events.ScheduledUpkeepTick,
		upkeepDailyTickPayload{}, timescale.Apply(24*time.Hour))
}

// applyAttrition removes upkeepAttritionStep men from the unit due to grain shortage.
// Returns true if the unit was disbanded.
func (h *UpkeepHandler) applyAttrition(ctx context.Context, u upkeepUnitRow, _ float64, worldID uuid.UUID) bool {
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
	return disbanded
}

// recordUnpaid increments unpaid_periods and applies desertion if the threshold is reached.
func (h *UpkeepHandler) recordUnpaid(ctx context.Context, u upkeepUnitRow, worldID uuid.UUID) {
	np := u.unpaidPeriods + 1

	if np >= upkeepDesertionPeriods {
		// Desertion.
		lost := upkeepDesertionStep
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
