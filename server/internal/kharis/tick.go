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

const (
	decayOnMissed     = 0.10  // 10% kharis lost when maintenance missed
	punishThreshold   = 100.0 // kharis below this risks divine punishment
	punishProbability = 0.30  // 30% chance of divine punishment per missed day below threshold
	blessThreshold    = 200.0 // kharis above this may attract divine favour
	blessProbability  = 0.15  // 15% chance of divine blessing per maintained day above threshold
)

// kharisPerCult is the conversion factor from accumulated cult stock to kharis gain per tick.
// Tunbar — calibrated in W8.
const kharisPerCult = 0.01

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
	h.applyStarvation(ctx, e.WorldID)
	h.accumulatePrestige(ctx, e.WorldID)

	return h.scheduler.EnqueueTick(ctx, e.WorldID, events.ScheduledKharisTick,
		struct{}{}, e.DueTick+1)
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

// deriveMood maps kharis level to a mood label (replaces player-set cult_level).
func deriveMood(kharis float64) string {
	switch {
	case kharis >= 200:
		return "overdadig"
	case kharis >= 100:
		return "praktfull"
	case kharis >= 50:
		return "vardig"
	case kharis > 0:
		return "enkel"
	default:
		return "forsummad"
	}
}

// applyDecay applies 1% daily decay to grain and timber stocks, resets
// invasions_today, and adjusts population. (Rite success is driven by Kharis
// mood, not a priest-strength stat — there is no priest_strength to regenerate.)
func (h *TickHandler) applyDecay(ctx context.Context, worldID uuid.UUID) {
	// Decay grain and timber by 1% per day; grain also consumed by population (0.5/person/day).
	// Cedar is a luxury store-of-value (ädelträ) and does not rot.
	if _, err := h.pool.Exec(ctx,
		`UPDATE settlement_goods sg SET
		   amount = GREATEST(0,
		       CASE sg.good_key
		           WHEN 'grain' THEN
		               settled(sg.amount, sg.rate, sg.calc_tick) * 0.99
		               - s.population * 0.5
		           ELSE
		               settled(sg.amount, sg.rate, sg.calc_tick) * 0.99
		       END),
		   calc_tick = current_world_tick()
		 FROM settlements s
		 WHERE sg.settlement_id = s.id
		   AND s.world_id = $1 AND s.owner_id IS NOT NULL AND s.state != 'sunk'
		   AND sg.good_key IN ('grain', 'timber')`,
		worldID,
	); err != nil {
		slog.Error("goods decay failed", "world", worldID, "err", err)
	}

	// Reset invasions_today, update population.
	//
	// Growth model (daily tick):
	//   pop ≥ 100  → proportional: 0.5% base × food-variety multiplier × soft-cap factor.
	//                food_variety = 1.0 (grain) + 0.1 per extra food type (fish/oil/wine/livestock) → max 1.4
	//                soft_cap = max(0, 1 − pop/30000)  → growth → 0 near 30000
	//   starvation → −0.5% (pop ≥ 100), floor 101 (collapse fires for pop ≤ 100).
	//
	// C-collapse: the floor is 101, not 50. Any settlement that would drop below 101
	// from starvation is held at 101 here; a follow-up query then schedules
	// CollapseSettlement events for all settlements at pop ≤ 100.
	if _, err := h.pool.Exec(ctx,
		`UPDATE settlements s SET
		   invasions_today = 0,
		   population = GREATEST(101, LEAST(30000,
		     CASE WHEN COALESCE(
		              (SELECT settled(sg.amount, sg.rate, sg.calc_tick)
		               FROM settlement_goods sg
		               WHERE sg.settlement_id = s.id AND sg.good_key = 'grain'), 0) > 0
		          THEN
		            -- proportional mode: 0.5% × variety(1.0–1.4) × soft-cap
		            s.population + GREATEST(1, ROUND(
		                s.population
		                * 0.005
		                * (1.0
		                    + 0.1 * (
		                        (CASE WHEN COALESCE((SELECT sg.amount FROM settlement_goods sg WHERE sg.settlement_id = s.id AND sg.good_key = 'fish'),0)      > 0 THEN 1 ELSE 0 END) +
		                        (CASE WHEN COALESCE((SELECT sg.amount FROM settlement_goods sg WHERE sg.settlement_id = s.id AND sg.good_key = 'oil'),0)       > 0 THEN 1 ELSE 0 END) +
		                        (CASE WHEN COALESCE((SELECT sg.amount FROM settlement_goods sg WHERE sg.settlement_id = s.id AND sg.good_key = 'wine'),0)      > 0 THEN 1 ELSE 0 END) +
		                        (CASE WHEN COALESCE((SELECT sg.amount FROM settlement_goods sg WHERE sg.settlement_id = s.id AND sg.good_key = 'livestock'),0) > 0 THEN 1 ELSE 0 END)))
		                * GREATEST(0, 1.0 - s.population::float / 30000.0)
		            ))
		          -- starvation: -0.5%, floor 101 so collapse logic fires below
		          ELSE GREATEST(101, ROUND(s.population * 0.995))
		     END))
		 WHERE s.world_id = $1 AND s.owner_id IS NOT NULL AND s.state NOT IN ('sunk', 'collapsed')`,
		worldID,
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
// One point per active settlement, plus a tier bonus (vardig+1, praktfull+2, overdadig+3).
// Prestige feeds into the collapse risk algorithm.
func (h *TickHandler) accumulatePrestige(ctx context.Context, worldID uuid.UUID) {
	// Prestige is driven by cult level — now lives on player_world_records.
	_, err := h.pool.Exec(ctx,
		`UPDATE worlds SET prestige = prestige + (
		    SELECT COALESCE(SUM(
		        1 + CASE pwr.cult_level
		            WHEN 'vardig'    THEN 1
		            WHEN 'praktfull' THEN 2
		            WHEN 'overdadig' THEN 3
		            ELSE 0
		        END
		    ), 0)
		    FROM player_world_records pwr
		    WHERE pwr.world_id = $1 AND pwr.cult_level != 'forsummad'
		)
		WHERE id = $1`,
		worldID,
	)
	if err != nil {
		slog.Error("prestige accumulation failed", "world", worldID, "err", err)
	}
}
