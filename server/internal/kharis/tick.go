// Package kharis implements the daily temple maintenance tick for Poleia.
// Kharis is a reciprocal relationship between a settlement and its gods.
// Settlements that maintain their temples accumulate divine favour;
// those that neglect maintenance lose it — and eventually suffer.
package kharis

import (
	"context"
	"fmt"
	"log/slog"
	"math/rand"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/poleia/server/internal/ai"
	"github.com/poleia/server/internal/economy"
	"github.com/poleia/server/internal/events"
)

// Kharis omdesign (Timothy 2026-07-09, temenos_kharis.md §"KANONISK OMDESIGN"):
// the pool moved from a 0–2000 scale to a single hidden 0–100 number. All
// thresholds below are STRAWMAN — balance is calibrated later
// (temenos_balans_spakar.md §9). See megaron_kharis_plan.md FAS 0.
const (
	// decayOnMissed: 10% kharis lost when maintenance missed. Superseded by dailyDecay
	// (FAS 2) as the sole depreciation source once that lands — kept only until then.
	decayOnMissed     = 0.10
	punishThreshold   = 30.0 // kharis below this risks divine punishment (was 100/2000)
	punishProbability = 0.30 // 30% chance of divine punishment per missed day below threshold
	blessThreshold    = 60.0 // kharis above this may attract divine favour (was 200/2000)
	blessProbability  = 0.15 // 15% chance of divine blessing per maintained day above threshold
)

// kharisPerCult is the conversion factor from accumulated cult stock to kharis gain per tick.
// Rescaled 0.01 → 0.0005 for the 0–100 scale: the old rate produced ~16/2000 ≈ 0.8%/day
// of cap from typical cult production, so the new rate targets the same ~0.8/day of a
// 0–100 pool. Tunbar — calibrated in W8.
const kharisPerCult = 0.0005

// grainPerCitizen is the grain cost of one new citizen at the daily growth
// tick — makes growth a real economic draw on grain instead of a binary gate.
//
// Calibration story (see TestApplyDecay_GrainFundedGrowth_* for the measured
// numbers): a naive read of "consume 50–70% of surplus" against the good's
// storage CAP (1000) doesn't hold up once measured — the decay step above
// writes an uncapped settled() value, so any self-sufficient catchment's raw
// daily accrual (rate × TicksPerDay) is many multiples of the 1000 cap
// (≈5450 for the minimal one-plains-tile guaranteed floor, ≈13000+ for a
// two-plains-tile catchment at start pop 5000). A modest draw against that
// (25 × desired_new, ≈500/day) vanishes into the overshoot and
// RecomputeProduction's own end-of-tick LEAST(cap,…) clamp re-pins the stock
// at 1000 regardless — satisfying neither "cap un-pinned" nor "richer
// catchment grows faster" (both catchments simply re-saturate identically).
// grainPerCitizen=300 instead prices growth against that *raw* daily accrual:
// the minimal one-plains catchment can only ever afford ~17–18 of the 21
// desired new citizens/day (its cost, 21×300=6300, exceeds its ≈5450
// accrual), spending nearly all of it and leaving a small-but-always-positive
// remainder (1–300 grain, varies day to day, never zero — proven over 40 days
// in TestApplyDecay_GrainFundedGrowth_MinimalCitySelfSufficient) — this is
// what makes success criterion #2 (cap un-pinned) hold. A richer catchment
// (≥2 grain tiles) has proportionally more accrual against the SAME cost
// (desired_new depends on population/soft-cap only, not catchment), so it
// affords desired growth in full every day and grows measurably faster —
// criterion #3 — while its own stock re-saturates at cap (expected: its
// surplus genuinely exceeds what a day's growth can spend). The floor-division
// throttle (§ actual_new = floor(grain_now/grainPerCitizen) when unaffordable)
// necessarily leaves a remainder in [0, grainPerCitizen) — occasionally small
// in absolute terms on a given day — but it is mathematically always ≥ 0 and
// never sign-flips negative (GREATEST(0,…) floors throughout), and a second
// same-tick firing is a safe no-op (draw=0 when nothing is affordable). If
// this ever measures as breaking the never-starve invariant for some other
// catchment shape, lower it.
const grainPerCitizen = 300.0

// TickHandler applies daily temple maintenance to all active settlements in a world.
type TickHandler struct {
	pool      *pgxpool.Pool
	scheduler *events.Scheduler
	store     *events.Store
}

// NewTickHandler creates a TickHandler.
func NewTickHandler(pool *pgxpool.Pool, sched *events.Scheduler, store *events.Store) *TickHandler {
	return &TickHandler{pool: pool, scheduler: sched, store: store}
}

// wanaxSnap holds the per-Wanax state needed for daily temple maintenance.
// Kharis lives on player_world_records; cultSum aggregates cult good across all
// of the player's settlements in this world.
type wanaxSnap struct {
	playerID     uuid.UUID
	settlementID uuid.UUID // capital settlement (for event emission and divine effects)
	kharis       float64
	kharisCap    float64
	cultSum      float64
}

// Handle processes a KharisTick scheduled event.
func (h *TickHandler) Handle(ctx context.Context, e events.ScheduledEvent) error {
	// ── 1. Kharis maintenance: one tick per player_world_record ────────────
	rows, err := h.pool.Query(ctx,
		`SELECT pwr.player_id, s.id AS capital_id,
		    GREATEST(0, settled(pwr.kharis_amount, pwr.kharis_rate, pwr.kharis_calc_tick)) AS kharis,
		    pwr.kharis_cap,
		    COALESCE((
		        SELECT SUM(GREATEST(0, settled(sg.amount, sg.rate, sg.calc_tick)))
		        FROM settlement_goods sg
		        JOIN settlements s2 ON s2.id = sg.settlement_id
		        WHERE s2.owner_id = pwr.player_id AND s2.world_id = pwr.world_id AND sg.good_key = 'cult'
		    ), 0) AS cult_sum
		 FROM player_world_records pwr
		 JOIN settlements s ON s.owner_id = pwr.player_id AND s.world_id = pwr.world_id AND s.is_capital = true
		 WHERE pwr.world_id = $1`,
		e.WorldID,
	)
	if err != nil {
		return fmt.Errorf("query player_world_records for kharis tick: %w", err)
	}
	defer rows.Close()

	var snaps []wanaxSnap
	for rows.Next() {
		var w wanaxSnap
		if err := rows.Scan(&w.playerID, &w.settlementID,
			&w.kharis, &w.kharisCap, &w.cultSum); err == nil {
			snaps = append(snaps, w)
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}

	for _, w := range snaps {
		if err := h.processMaintenance(ctx, w, e.WorldID); err != nil {
			slog.Error("kharis maintenance failed", "player", w.playerID, "err", err)
		}
	}

	// ── 2. AI governor ticks (per settlement, unchanged) ──────────────────
	aiRows, err := h.pool.Query(ctx,
		`SELECT id FROM settlements WHERE world_id = $1 AND governor_is_ai = true AND state != 'sunk'`,
		e.WorldID,
	)
	if err == nil {
		defer aiRows.Close()
		for aiRows.Next() {
			var sid uuid.UUID
			if aiRows.Scan(&sid) == nil {
				if err := ai.PassiveGovernorTick(ctx, h.pool, sid, e.WorldID); err != nil {
					slog.Warn("passive governor tick failed", "settlement", sid, "err", err)
				}
			}
		}
	}

	h.applyDecay(ctx, e.WorldID)
	h.applyStarvationWarning(ctx, e.WorldID)
	h.applyStarvation(ctx, e.WorldID)
	h.accumulatePrestige(ctx, e.WorldID)

	return h.scheduler.EnqueueTick(ctx, e.WorldID, events.ScheduledKharisTick,
		struct{}{}, e.DueTick+events.TicksPerDay)
}

func (h *TickHandler) processMaintenance(ctx context.Context, w wanaxSnap, worldID uuid.UUID) error {
	var newKharis float64

	if w.cultSum > 0 {
		gain := w.cultSum * kharisPerCult

		// 1. Add kharis (capped).
		if _, err := h.pool.Exec(ctx,
			`UPDATE player_world_records SET
			   kharis_amount  = LEAST(kharis_cap,
			       settled(kharis_amount, kharis_rate, kharis_calc_tick) + $1),
			   kharis_calc_tick = current_world_tick()
			 WHERE player_id = $2 AND world_id = $3`,
			gain, w.playerID, worldID,
		); err != nil {
			return fmt.Errorf("update kharis after cult maintenance: %w", err)
		}

		// 2. Consume cult across all player's settlements (zero it out).
		if _, err := h.pool.Exec(ctx,
			`UPDATE settlement_goods sg SET
			   amount   = 0,
			   calc_tick = current_world_tick()
			 FROM settlements s2
			 WHERE sg.settlement_id = s2.id
			   AND s2.owner_id = $1 AND s2.world_id = $2
			   AND sg.good_key = 'cult'`,
			w.playerID, worldID,
		); err != nil {
			return fmt.Errorf("consume cult goods: %w", err)
		}

		// 3. Event + divine effects.
		_, _ = h.store.Append(ctx, w.settlementID, events.StreamProvince, "KharisMaintained",
			map[string]any{"cult_consumed": w.cultSum, "kharis_gain": gain},
			worldID, nil)
		newKharis = w.kharis + gain
		if newKharis > blessThreshold && rand.Float64() < blessProbability {
			h.applyDivineBlessing(ctx, w.settlementID, worldID)
		}
		if rand.Float64() < 0.20 {
			h.generateOmen(ctx, w.settlementID, worldID)
		}
	} else {
		// No temple production — kharis decays.
		if _, err := h.pool.Exec(ctx,
			`UPDATE player_world_records SET
			   kharis_amount  = GREATEST(0,
			       settled(kharis_amount, kharis_rate, kharis_calc_tick) * $1),
			   kharis_calc_tick = current_world_tick()
			 WHERE player_id = $2 AND world_id = $3`,
			1.0-decayOnMissed, w.playerID, worldID,
		); err != nil {
			return fmt.Errorf("kharis decay (no cult production): %w", err)
		}
		_, _ = h.store.Append(ctx, w.settlementID, events.StreamProvince, "KharisMissedMaintenance",
			map[string]any{"reason": "no_cult_production", "decay_fraction": decayOnMissed},
			worldID, nil)
		newKharis = w.kharis * (1.0 - decayOnMissed)
		if newKharis < punishThreshold && rand.Float64() < punishProbability {
			h.applyDivinePunishment(ctx, w.settlementID, worldID)
		}
	}

	// 4. Derive mood and write back cult_level (drives prestige + display).
	derived := deriveMood(newKharis)
	_, _ = h.pool.Exec(ctx,
		`UPDATE player_world_records SET cult_level = $1
		 WHERE player_id = $2 AND world_id = $3`,
		derived, w.playerID, worldID,
	)
	return nil
}

// deriveMood maps the 0–100 kharis level to a mood label (replaces player-set
// cult_level). This is the SINGLE canonical threshold table — mood, rite success
// (settlement.go), and api/handlers.kharisToMood (web.go) all read the same four
// tiers (60/30/10, strawman — temenos_balans_spakar.md §9) so there is no longer a
// dual scale. Swedish labels for the two lower tiers ("tveksam"/"vredgad") are new
// strawman coinages — the design doc only names the English mood words for those.
func deriveMood(kharis float64) string {
	switch {
	case kharis >= 60:
		return "overdadig" // Favorable
	case kharis >= 30:
		return "vardig" // Indifferent
	case kharis >= 10:
		return "tveksam" // Suspicious
	default:
		return "vredgad" // Wrathful
	}
}

// applyDecay applies 1% daily decay to grain and timber stocks, resets
// invasions_today, and adjusts population. (Rite success is driven by Kharis
// mood, not a priest-strength stat — there is no priest_strength to regenerate.)
func (h *TickHandler) applyDecay(ctx context.Context, worldID uuid.UUID) {
	// Decay grain and timber by 1% per day. Population grain-consumption is NOT
	// applied here anymore: it is folded into grain's net rate in
	// economy.RecomputeProduction (continuous per-tick draw), so it never exceeds
	// the grain cap and a self-sufficient city holds a stable positive stock.
	// Cedar is a luxury store-of-value (ädelträ) and does not rot.
	if _, err := h.pool.Exec(ctx,
		`UPDATE settlement_goods sg SET
		   amount = GREATEST(0, settled(sg.amount, sg.rate, sg.calc_tick) * 0.99),
		   calc_tick = current_world_tick()
		 FROM settlements s
		 WHERE sg.settlement_id = s.id
		   AND s.world_id = $1 AND s.owner_id IS NOT NULL AND s.state != 'sunk'
		   AND sg.good_key IN ('grain', 'timber')`,
		worldID,
	); err != nil {
		slog.Error("goods decay failed", "world", worldID, "err", err)
	}

	// Reset invasions_today, update population, and — grain-funded growth —
	// draw the grain cost of whatever growth is actually affordable.
	//
	// Growth model (daily tick):
	//   pop ≥ 100  → proportional: 0.5% base × food-variety multiplier × soft-cap factor
	//                gives a DESIRED new-citizen count; food_variety = 1.0 (grain) +
	//                0.1 per extra food type (fish/oil/wine/livestock) → max 1.4,
	//                soft_cap = max(0, 1 − pop/30000) → growth → 0 near 30000.
	//                That desired growth then costs desired_new × grainPerCitizen
	//                grain: if the settled grain stock affords it in full, all of
	//                it is applied and the cost is deducted; if not, growth is
	//                throttled to floor(grain_now / grainPerCitizen) citizens and
	//                grain is drawn down to (near) zero. Growth never grows the
	//                city for grain it doesn't have.
	//   starvation → −0.5% (pop ≥ 100), floor 101 (collapse fires for pop ≤ 100).
	//                Unchanged — no grain is drawn on the starvation path.
	//
	// C-collapse: the floor is 101, not 50. Any settlement that would drop below 101
	// from starvation is held at 101 here; a follow-up query then schedules
	// CollapseSettlement events for all settlements at pop ≤ 100.
	//
	// Single CTE-chained statement (not a bare TX) so the population increment and
	// the grain deduction are computed ONCE from the same snapshot and applied
	// atomically — pop-added always equals grain-drawn/grainPerCitizen, never more.
	//
	// grain_now reads the raw settled() value (uncapped) — the same value the
	// rest of the codebase treats as "available now" before a write clamps it.
	// This matters for catchment differentiation (success criterion #3): the
	// good's storage cap (1000) is a fixed constant unrelated to a catchment's
	// richness, so clamping grain_now to it before pricing growth would make
	// every self-sufficient catchment (however rich) read identically and grow
	// at the identical rate — erasing the very signal geography is supposed to
	// provide. Leaving it uncapped means a poor catchment's genuinely smaller
	// daily accrual can fall short of desired growth's cost (throttling it)
	// while a rich catchment's larger accrual doesn't — see
	// TestApplyDecay_GrainFundedGrowth_GeographyDifferentiates.
	if _, err := h.pool.Exec(ctx,
		`WITH growth_calc AS (
		     SELECT
		         s.id,
		         s.population AS pop,
		         COALESCE(
		             (SELECT settled(sg.amount, sg.rate, sg.calc_tick)
		              FROM settlement_goods sg
		              WHERE sg.settlement_id = s.id AND sg.good_key = 'grain'), 0
		         ) AS grain_now,
		         (1.0 + 0.1 * (
		             (CASE WHEN COALESCE((SELECT sg.amount FROM settlement_goods sg WHERE sg.settlement_id = s.id AND sg.good_key = 'fish'),0)      > 0 THEN 1 ELSE 0 END) +
		             (CASE WHEN COALESCE((SELECT sg.amount FROM settlement_goods sg WHERE sg.settlement_id = s.id AND sg.good_key = 'oil'),0)       > 0 THEN 1 ELSE 0 END) +
		             (CASE WHEN COALESCE((SELECT sg.amount FROM settlement_goods sg WHERE sg.settlement_id = s.id AND sg.good_key = 'wine'),0)      > 0 THEN 1 ELSE 0 END) +
		             (CASE WHEN COALESCE((SELECT sg.amount FROM settlement_goods sg WHERE sg.settlement_id = s.id AND sg.good_key = 'livestock'),0) > 0 THEN 1 ELSE 0 END)
		         )) AS variety,
		         GREATEST(0, 1.0 - s.population::float / 30000.0) AS softcap
		     FROM settlements s
		     WHERE s.world_id = $1 AND s.owner_id IS NOT NULL AND s.state NOT IN ('sunk', 'collapsed')
		 ),
		 resolved AS (
		     SELECT
		         id, pop, grain_now,
		         (grain_now > 0) AS growing,
		         GREATEST(1, ROUND(pop * 0.005 * variety * softcap)) AS desired_new
		     FROM growth_calc
		 ),
		 priced AS (
		     SELECT
		         id, pop, grain_now, growing,
		         CASE
		             WHEN NOT growing THEN 0
		             WHEN grain_now >= desired_new * $2::float THEN desired_new
		             ELSE FLOOR(grain_now / $2::float)
		         END AS actual_new
		     FROM resolved
		 ),
		 final AS (
		     SELECT
		         id, grain_now,
		         GREATEST(101, LEAST(30000,
		             CASE WHEN growing THEN pop + actual_new ELSE ROUND(pop * 0.995) END
		         )) AS new_pop,
		         CASE WHEN growing THEN actual_new * $2::float ELSE 0 END AS grain_draw
		     FROM priced
		 ),
		 pop_upd AS (
		     UPDATE settlements s SET
		         invasions_today = 0,
		         population = f.new_pop
		     FROM final f
		     WHERE f.id = s.id
		     RETURNING s.id
		 ),
		 grain_upd AS (
		     UPDATE settlement_goods sg SET
		         amount    = GREATEST(0, f.grain_now - f.grain_draw),
		         calc_tick = current_world_tick()
		     FROM final f
		     WHERE f.grain_draw > 0 AND sg.settlement_id = f.id AND sg.good_key = 'grain'
		     RETURNING sg.settlement_id
		 )
		 SELECT count(*) FROM pop_upd`,
		worldID, grainPerCitizen,
	); err != nil {
		slog.Error("daily decay failed", "world", worldID, "err", err)
	}

	// C-collapse: schedule CollapseSettlement for any settlement that has already
	// reached pop ≤ 100 (e.g. from overmobilisation via Recruit). The bulk UPDATE
	// above floors at 101 so starvation alone won't create new ≤100 cases in one
	// tick, but once pop is already at 101 and starvation fires, the GREATEST(101,…)
	// clips it — meaning starvation settlement-death takes a second tick to manifest.
	// This is acceptable: starvation collapse is a gradual process.
	collapseRows, err := h.pool.Query(ctx,
		`SELECT id FROM settlements
		 WHERE world_id = $1 AND owner_id IS NOT NULL
		   AND state NOT IN ('sunk', 'collapsed')
		   AND population <= 100`,
		worldID,
	)
	if err == nil {
		var collapseIDs []uuid.UUID
		for collapseRows.Next() {
			var sid uuid.UUID
			if collapseRows.Scan(&sid) == nil {
				collapseIDs = append(collapseIDs, sid)
			}
		}
		collapseRows.Close()
		var currentTick int
		_ = h.pool.QueryRow(ctx, `SELECT current_world_tick()`).Scan(&currentTick)
		for _, sid := range collapseIDs {
			if err := h.scheduler.EnqueueTick(ctx, worldID, events.ScheduledCollapseSettlement,
				struct {
					SettlementID uuid.UUID `json:"settlement_id"`
					WorldID      uuid.UUID `json:"world_id"`
					Cause        string    `json:"cause"`
				}{SettlementID: sid, WorldID: worldID, Cause: "starvation"},
				currentTick,
			); err != nil {
				slog.Warn("collapse: could not schedule collapse event",
					"settlement", sid, "err", err)
			}
		}
	}

	// Recompute production for all active settlements: population changed, so
	// labor_pool (and therefore rates) must be updated.
	sidRows, err := h.pool.Query(ctx,
		`SELECT id FROM settlements
		 WHERE world_id = $1 AND owner_id IS NOT NULL AND state NOT IN ('sunk', 'collapsed')`,
		worldID,
	)
	if err == nil {
		var ids []uuid.UUID
		for sidRows.Next() {
			var sid uuid.UUID
			if sidRows.Scan(&sid) == nil {
				ids = append(ids, sid)
			}
		}
		sidRows.Close()
		for _, sid := range ids {
			if err := economy.RecomputeProduction(ctx, h.pool, sid); err != nil {
				slog.Warn("recompute after pop tick failed", "settlement", sid, "err", err)
			}
		}
	}
}

// applyStarvationWarning is the proactive counterpart to applyStarvation
// (Fas 2d): the reactive "⚠ X is starving" gossip line only ever fired after
// grain had ALREADY hit zero and damage was already being applied — a Wanax
// got no notice before the harm. This warns once per day while grain is
// still positive but trending to empty within the next game-day (net rate
// negative, amount/|rate| <= TicksPerDay) — same gossip channel, same
// no-dedup-while-condition-holds precedent as SitosFundLow (economy package):
// it re-fires every day the trend still holds, which is a reminder, not spam,
// at daily cadence.
func (h *TickHandler) applyStarvationWarning(ctx context.Context, worldID uuid.UUID) {
	rows, err := h.pool.Query(ctx,
		`SELECT s.id, s.owner_id, s.name,
		        settled(sg.amount, sg.rate, sg.calc_tick) / -sg.rate AS ticks_to_empty
		 FROM settlements s
		 JOIN settlement_goods sg ON sg.settlement_id = s.id AND sg.good_key = 'grain'
		 WHERE s.world_id = $1 AND s.owner_id IS NOT NULL AND s.state != 'sunk'
		   AND sg.rate < 0
		   AND settled(sg.amount, sg.rate, sg.calc_tick) > 0
		   AND settled(sg.amount, sg.rate, sg.calc_tick) / -sg.rate <= $2`,
		worldID, events.TicksPerDay,
	)
	if err != nil {
		slog.Error("starvation warning tick failed", "world", worldID, "err", err)
		return
	}
	defer rows.Close()

	for rows.Next() {
		var id, ownerID uuid.UUID
		var name string
		var ticksToEmpty float64
		if err := rows.Scan(&id, &ownerID, &name, &ticksToEmpty); err != nil {
			continue
		}
		_, _ = h.pool.Exec(ctx,
			`INSERT INTO gossip_events (world_id, recipient_id, source_region, category, text)
			 VALUES ($1, $2, $3, 'economy', $4)`,
			worldID, ownerID, name,
			fmt.Sprintf("⚠ %s's grain is running out — empty in ~%.0f hours at current rate. Buy or reallocate labor before stores hit zero.",
				name, ticksToEmpty),
		)
	}
}

// applyStarvation punishes settlements where grain has hit zero: infantry and
// chariots each lose 5% (minimum 1 unit) per day.
func (h *TickHandler) applyStarvation(ctx context.Context, worldID uuid.UUID) {
	rows, err := h.pool.Query(ctx,
		`UPDATE settlements s SET
		   infantry = GREATEST(0, infantry - GREATEST(1, (infantry * 0.05)::int)),
		   chariot  = GREATEST(0, chariot  - GREATEST(1, (chariot  * 0.05)::int))
		 WHERE s.world_id = $1 AND s.owner_id IS NOT NULL AND s.state != 'sunk'
		   AND (s.infantry > 0 OR s.chariot > 0)
		   AND COALESCE(
		           (SELECT settled(sg.amount, sg.rate, sg.calc_tick)
		            FROM settlement_goods sg
		            WHERE sg.settlement_id = s.id AND sg.good_key = 'grain'), 0) <= 0
		 RETURNING s.id, s.owner_id, s.name`,
		worldID,
	)
	if err != nil {
		slog.Error("starvation tick failed", "world", worldID, "err", err)
		return
	}
	defer rows.Close()

	for rows.Next() {
		var id, ownerID uuid.UUID
		var name string
		if err := rows.Scan(&id, &ownerID, &name); err != nil {
			continue
		}
		_, _ = h.store.Append(ctx, id, events.StreamProvince, "StarvationDamage",
			map[string]any{"reason": "no_food"}, worldID, nil)
		// Gossip: notify the owner so it appears in their messages/notifications.
		_, _ = h.pool.Exec(ctx,
			`INSERT INTO gossip_events (world_id, recipient_id, source_region, category, text)
			 VALUES ($1, $2, $3, 'economy', $4)`,
			worldID, ownerID, name,
			"⚠ "+name+" is starving — grain stores empty. Send messengers to buy grain urgently.",
		)
		slog.Info("starvation damage applied", "settlement", id)
	}
}

// applyDivinePunishment randomly selects and applies one divine punishment.
func (h *TickHandler) applyDivinePunishment(ctx context.Context, settlementID, worldID uuid.UUID) {
	type punishment struct {
		name string
		text string
		sql  string
	}

	punishments := []punishment{
		{
			"chariot_loss",
			"The gods have scattered your war chariots in the night. Chariots have perished.",
			`UPDATE settlements SET chariot = GREATEST(0, chariot - GREATEST(1, chariot/5)) WHERE id = $1`,
		},
		{
			"ship_loss",
			"A divine storm has claimed a vessel from your harbour.",
			`UPDATE settlements SET ship = GREATEST(0, ship - 1) WHERE id = $1`,
		},
		{
			"harvest_failure",
			"The fields lie fallow by divine will. Half your grain stores have rotted.",
			`UPDATE settlement_goods SET
			   amount  = GREATEST(0, settled(amount, rate, calc_tick) * 0.5),
			   calc_tick = current_world_tick()
			 WHERE settlement_id = $1 AND good_key = 'grain'`,
		},
		{
			"garrison_plague",
			"A dark pestilence has moved through the barracks. Many hoplites have fallen.",
			`UPDATE settlements SET infantry = GREATEST(0, infantry - GREATEST(1, infantry/5)) WHERE id = $1`,
		},
	}

	p := punishments[rand.Intn(len(punishments))]
	if _, err := h.pool.Exec(ctx, p.sql, settlementID); err != nil {
		slog.Error("divine punishment failed", "settlement", settlementID, "type", p.name, "err", err)
		return
	}

	_, _ = h.store.Append(ctx, settlementID, events.StreamProvince, "DivinePunishment",
		map[string]any{"type": p.name}, worldID, nil)
	h.addDivineGossip(ctx, settlementID, worldID, "divine_wrath", p.text)
	slog.Info("divine punishment applied", "settlement", settlementID, "type", p.name)
}

// applyDivineBlessing randomly selects and applies one divine blessing for settlements
// that maintain high kharis. Mirror of applyDivinePunishment.
func (h *TickHandler) applyDivineBlessing(ctx context.Context, settlementID, worldID uuid.UUID) {
	type blessing struct {
		name string
		text string
		sql  string
	}

	blessings := []blessing{
		{
			"harvest_blessing",
			"The gods smile upon your fields. An abundant harvest fills your granaries.",
			`UPDATE settlement_goods SET
			   amount  = LEAST(cap, settled(amount, rate, calc_tick) * 1.25),
			   calc_tick = current_world_tick()
			 WHERE settlement_id = $1 AND good_key = 'grain'`,
		},
		{
			"divine_recruits",
			"Warriors answer a divine call and join your ranks. New hoplites have arrived.",
			`UPDATE settlements SET
			   infantry = infantry + GREATEST(2, infantry / 5)
			 WHERE id = $1`,
		},
		{
			"sea_blessing",
			"Poseidon guides a vessel to your harbour. A trireme joins your fleet.",
			`UPDATE settlements SET ship = ship + 1 WHERE id = $1`,
		},
	}

	b := blessings[rand.Intn(len(blessings))]
	if _, err := h.pool.Exec(ctx, b.sql, settlementID); err != nil {
		slog.Error("divine blessing failed", "settlement", settlementID, "type", b.name, "err", err)
		return
	}

	_, _ = h.store.Append(ctx, settlementID, events.StreamProvince, "DivineBlessing",
		map[string]any{"type": b.name}, worldID, nil)
	h.addDivineGossip(ctx, settlementID, worldID, "divine_favour", b.text)
	slog.Info("divine blessing applied", "settlement", settlementID, "type", b.name)
}

// generateOmen produces an atmospheric temple omen (20% chance per maintained day).
// Omens are written to the gossip stream and appear in the player's Rumours panel.
func (h *TickHandler) generateOmen(ctx context.Context, settlementID, worldID uuid.UUID) {
	omens := []string{
		"The heart of the offering lay clean and red. The gods are pleased.",
		"Smoke rose straight toward heaven — a season of calm and steady winds.",
		"The sacred birds ate freely from the priest's hand. The harvest will be generous.",
		"The flame consumed the offering without hesitation. Order holds for now.",
		"A serpent crossed the temple threshold and departed unharmed. Old powers watch this place.",
		"Birds flew westward in tight formation. Something stirs beyond your sight.",
		"The offering was pale but the liver whole. The gods withhold judgement.",
		"Clouds gathered during the rite, then passed without rain. The future is contested.",
		"The sacred flame guttered three times before catching. Patience is called for.",
		"A dark mark appeared near the gate of the liver — a shadow at the threshold.",
		"The birds fell silent for a long time before resuming their cries. The gods listen.",
		"Wind shifted against the smoke during the final prayer. Something turns.",
		"Two ravens circled the altar three times. The gods debate.",
		"The entrails were tangled — an augur's nightmare. Ambiguity rules this season.",
		"A child laughed outside the temple during the rite. The gods find something amusing.",
	}
	text := omens[rand.Intn(len(omens))]
	h.addDivineGossip(ctx, settlementID, worldID, "omen", text)
}

// addDivineGossip inserts a gossip event for the owner of the given settlement.
func (h *TickHandler) addDivineGossip(ctx context.Context, settlementID, worldID uuid.UUID, category, text string) {
	var ownerID *uuid.UUID
	var name string
	_ = h.pool.QueryRow(ctx,
		`SELECT owner_id, name FROM settlements WHERE id = $1`,
		settlementID,
	).Scan(&ownerID, &name)
	if ownerID == nil {
		return
	}
	_, _ = h.pool.Exec(ctx,
		`INSERT INTO gossip_events (world_id, recipient_id, source_region, category, text)
		 VALUES ($1, $2, $3, $4, $5)`,
		worldID, *ownerID, name, category, text,
	)
}

// accumulatePrestige adds daily prestige to the world based on active cult devotion.
// One point per active (non-Wrathful) settlement, plus a tier bonus (vardig+1,
// overdadig+2 — strawman, rescaled for the 4-tier mood table, FAS 0).
// Prestige feeds into the collapse risk algorithm.
func (h *TickHandler) accumulatePrestige(ctx context.Context, worldID uuid.UUID) {
	// Prestige is driven by cult level — now lives on player_world_records.
	_, err := h.pool.Exec(ctx,
		`UPDATE worlds SET prestige = prestige + (
		    SELECT COALESCE(SUM(
		        1 + CASE pwr.cult_level
		            WHEN 'vardig'    THEN 1
		            WHEN 'overdadig' THEN 2
		            ELSE 0
		        END
		    ), 0)
		    FROM player_world_records pwr
		    WHERE pwr.world_id = $1 AND pwr.cult_level != 'vredgad'
		)
		WHERE id = $1`,
		worldID,
	)
	if err != nil {
		slog.Error("prestige accumulation failed", "world", worldID, "err", err)
	}
}
