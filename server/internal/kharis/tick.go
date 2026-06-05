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
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/poleia/server/internal/ai"
	"github.com/poleia/server/internal/economy"
	"github.com/poleia/server/internal/events"
	"github.com/poleia/server/internal/timescale"
)

const (
	decayOnMissed     = 0.10  // 10% kharis lost when maintenance missed
	punishThreshold   = 100.0 // kharis below this risks divine punishment
	punishProbability = 0.30  // 30% chance of divine punishment per missed day below threshold
	blessThreshold    = 200.0 // kharis above this may attract divine favour
	blessProbability  = 0.15  // 15% chance of divine blessing per maintained day above threshold
)

// cultSpec defines the daily cost and kharis gain for each cult level.
// grain, wine, oil, and livestock are deducted from settlement_goods.
type cultSpec struct {
	gold       float64
	grain      float64 // from settlement_goods
	wine       float64 // from settlement_goods
	oil        float64 // from settlement_goods
	livestock  float64 // from settlement_goods — prestigious sacrifice
	kharisGain float64
}

var cultLevelSpecs = map[string]cultSpec{
	"forsummad": {kharisGain: 0},                                              // no payment, kharis decays
	"enkel":     {gold: 3, grain: 3, kharisGain: 2},
	"vardig":    {gold: 6, grain: 5, kharisGain: 5},
	"praktfull": {gold: 12, grain: 10, wine: 2, oil: 2, kharisGain: 10},
	"overdadig": {gold: 20, grain: 15, wine: 5, oil: 5, livestock: 2, kharisGain: 18},
}

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
// Kharis lives on player_world_records; resources come from the capital settlement.
type wanaxSnap struct {
	playerID    uuid.UUID
	settlementID uuid.UUID // capital settlement (for resource deduction and event emission)
	kharis      float64
	gold        float64
	grain       float64
	wine        float64
	oil         float64
	livestock   float64
	cultLevel   string
}

// Handle processes a KharisTick scheduled event.
func (h *TickHandler) Handle(ctx context.Context, e events.ScheduledEvent) error {
	// ── 1. Kharis maintenance: one tick per player_world_record ────────────
	rows, err := h.pool.Query(ctx,
		`SELECT pwr.player_id, s.id,
		    GREATEST(0, pwr.kharis_amount + (EXTRACT(EPOCH FROM (now() - pwr.kharis_calc_at))/60 * pwr.kharis_rate)),
		    GREATEST(0, s.silver_amount + (EXTRACT(EPOCH FROM (now() - s.silver_calc_at))/60 * s.silver_rate)),
		    GREATEST(0, COALESCE(grain.amount + (EXTRACT(EPOCH FROM (now() - grain.calc_at))/60 * grain.rate), 0)),
		    pwr.cult_level,
		    GREATEST(0, COALESCE(wine.amount  + (EXTRACT(EPOCH FROM (now() - wine.calc_at))/60  * wine.rate),  0)),
		    GREATEST(0, COALESCE(oil.amount   + (EXTRACT(EPOCH FROM (now() - oil.calc_at))/60   * oil.rate),   0)),
		    GREATEST(0, COALESCE(livestock.amount + (EXTRACT(EPOCH FROM (now() - livestock.calc_at))/60 * livestock.rate), 0))
		 FROM player_world_records pwr
		 JOIN settlements s ON s.owner_id = pwr.player_id AND s.world_id = pwr.world_id AND s.is_capital = true
		 LEFT JOIN settlement_goods grain     ON grain.settlement_id     = s.id AND grain.good_key     = 'grain'
		 LEFT JOIN settlement_goods wine      ON wine.settlement_id      = s.id AND wine.good_key      = 'wine'
		 LEFT JOIN settlement_goods oil       ON oil.settlement_id       = s.id AND oil.good_key       = 'oil'
		 LEFT JOIN settlement_goods livestock ON livestock.settlement_id = s.id AND livestock.good_key = 'livestock'
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
			&w.kharis, &w.gold, &w.grain, &w.cultLevel,
			&w.wine, &w.oil, &w.livestock); err == nil {
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

	return h.scheduler.EnqueueAfter(ctx, e.WorldID, events.ScheduledKharisTick,
		struct{}{}, timescale.Apply(24*time.Hour))
}

func (h *TickHandler) processMaintenance(ctx context.Context, w wanaxSnap, worldID uuid.UUID) error {
	spec, ok := cultLevelSpecs[w.cultLevel]
	if !ok {
		spec = cultLevelSpecs["enkel"]
	}

	// forsummad: player skips temple — kharis decays.
	if w.cultLevel == "forsummad" {
		_, err := h.pool.Exec(ctx,
			`UPDATE player_world_records SET
			   kharis_amount  = GREATEST(0,
			       (kharis_amount + (EXTRACT(EPOCH FROM (now() - kharis_calc_at))/60 * kharis_rate)) * $1),
			   kharis_calc_at = now()
			 WHERE player_id = $2 AND world_id = $3`,
			1.0-decayOnMissed, w.playerID, worldID,
		)
		if err != nil {
			return fmt.Errorf("kharis decay (forsummad): %w", err)
		}
		_, _ = h.store.Append(ctx, w.settlementID, events.StreamProvince, "KharisMissedMaintenance",
			map[string]any{"cult_level": w.cultLevel, "decay_fraction": decayOnMissed}, worldID, nil)
		newKharis := w.kharis * (1.0 - decayOnMissed)
		if newKharis < punishThreshold && rand.Float64() < punishProbability {
			h.applyDivinePunishment(ctx, w.settlementID, worldID)
		}
		return nil
	}

	canAfford := w.gold >= spec.gold && w.grain >= spec.grain &&
		w.wine >= spec.wine && w.oil >= spec.oil && w.livestock >= spec.livestock
	if canAfford {
		// Deduct silver from capital settlement, gain kharis in player_world_records.
		_, err := h.pool.Exec(ctx,
			`UPDATE settlements SET
			   silver_amount  = silver_amount + (EXTRACT(EPOCH FROM (now() - silver_calc_at))/60 * silver_rate) - $1,
			   silver_calc_at = now()
			 WHERE id = $2`,
			spec.gold, w.settlementID,
		)
		if err != nil {
			return fmt.Errorf("pay silver maintenance: %w", err)
		}
		_, err = h.pool.Exec(ctx,
			`UPDATE player_world_records SET
			   kharis_amount  = LEAST(kharis_cap,
			       kharis_amount + (EXTRACT(EPOCH FROM (now() - kharis_calc_at))/60 * kharis_rate) + $1),
			   kharis_calc_at = now()
			 WHERE player_id = $2 AND world_id = $3`,
			spec.kharisGain, w.playerID, worldID,
		)
		if err != nil {
			return fmt.Errorf("update kharis after maintenance: %w", err)
		}
		// Deduct goods from capital settlement.
		for goodKey, qty := range map[string]float64{
			"grain": spec.grain, "wine": spec.wine,
			"oil": spec.oil, "livestock": spec.livestock,
		} {
			if qty <= 0 {
				continue
			}
			_, _ = h.pool.Exec(ctx,
				`UPDATE settlement_goods SET
				   amount  = GREATEST(0, amount + EXTRACT(EPOCH FROM (now() - calc_at))/60 * rate - $1),
				   calc_at = now()
				 WHERE settlement_id = $2 AND good_key = $3`,
				qty, w.settlementID, goodKey,
			)
		}
		_, _ = h.store.Append(ctx, w.settlementID, events.StreamProvince, "KharisMaintained",
			map[string]any{"cult_level": w.cultLevel, "silver": spec.gold, "grain": spec.grain,
				"wine": spec.wine, "oil": spec.oil, "livestock": spec.livestock, "kharis_gain": spec.kharisGain},
			worldID, nil)
		newKharis := w.kharis + spec.kharisGain
		if newKharis > blessThreshold && rand.Float64() < blessProbability {
			h.applyDivineBlessing(ctx, w.settlementID, worldID)
		}
		if rand.Float64() < 0.20 {
			h.generateOmen(ctx, w.settlementID, worldID)
		}
		return nil
	}

	// Cannot afford — kharis decays.
	_, err := h.pool.Exec(ctx,
		`UPDATE player_world_records SET
		   kharis_amount  = GREATEST(0,
		       (kharis_amount + (EXTRACT(EPOCH FROM (now() - kharis_calc_at))/60 * kharis_rate)) * $1),
		   kharis_calc_at = now()
		 WHERE player_id = $2 AND world_id = $3`,
		1.0-decayOnMissed, w.playerID, worldID,
	)
	if err != nil {
		return fmt.Errorf("kharis decay: %w", err)
	}
	_, _ = h.store.Append(ctx, w.settlementID, events.StreamProvince, "KharisMissedMaintenance",
		map[string]any{"cult_level": w.cultLevel, "decay_fraction": decayOnMissed, "reason": "insufficient_resources"},
		worldID, nil)
	newKharis := w.kharis * (1.0 - decayOnMissed)
	if newKharis < punishThreshold && rand.Float64() < punishProbability {
		h.applyDivinePunishment(ctx, w.settlementID, worldID)
	}
	return nil
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
		               (sg.amount + EXTRACT(EPOCH FROM (now()-sg.calc_at))/60 * sg.rate) * 0.99
		               - s.population * 0.5
		           ELSE
		               (sg.amount + EXTRACT(EPOCH FROM (now()-sg.calc_at))/60 * sg.rate) * 0.99
		       END),
		   calc_at = now()
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
	//   pop < 100  → absolute: +2 (grain only) to +3 (full diet). Prevents recovery-lock at floor.
	//   pop ≥ 100  → proportional: 0.5% base × food-variety multiplier × soft-cap factor.
	//                food_variety = 1.0 (grain) + 0.1 per extra food type (fish/oil/wine/livestock) → max 1.4
	//                soft_cap = max(0, 1 − pop/5000)  → growth → 0 near 5000
	//   starvation → −0.5% (pop ≥ 100) or −2 (pop < 100), floor 50.
	if _, err := h.pool.Exec(ctx,
		`UPDATE settlements s SET
		   invasions_today = 0,
		   population = GREATEST(50, LEAST(5000,
		     CASE WHEN COALESCE(
		              (SELECT sg.amount + EXTRACT(EPOCH FROM (now()-sg.calc_at))/60 * sg.rate
		               FROM settlement_goods sg
		               WHERE sg.settlement_id = s.id AND sg.good_key = 'grain'), 0) > 0
		          THEN
		            CASE WHEN s.population < 100
		                 -- absolute mode: +2 base, +1 if any luxury food present
		                 THEN s.population + 2 + LEAST(1,
		                     (CASE WHEN COALESCE((SELECT sg.amount FROM settlement_goods sg WHERE sg.settlement_id = s.id AND sg.good_key = 'fish'),0)      > 0 THEN 1 ELSE 0 END) +
		                     (CASE WHEN COALESCE((SELECT sg.amount FROM settlement_goods sg WHERE sg.settlement_id = s.id AND sg.good_key = 'oil'),0)       > 0 THEN 1 ELSE 0 END) +
		                     (CASE WHEN COALESCE((SELECT sg.amount FROM settlement_goods sg WHERE sg.settlement_id = s.id AND sg.good_key = 'wine'),0)      > 0 THEN 1 ELSE 0 END) +
		                     (CASE WHEN COALESCE((SELECT sg.amount FROM settlement_goods sg WHERE sg.settlement_id = s.id AND sg.good_key = 'livestock'),0) > 0 THEN 1 ELSE 0 END))
		                 -- proportional mode: 0.5% × variety(1.0–1.4) × soft-cap
		                 ELSE s.population + GREATEST(1, ROUND(
		                     s.population
		                     * 0.005
		                     * (1.0
		                         + 0.1 * (
		                             (CASE WHEN COALESCE((SELECT sg.amount FROM settlement_goods sg WHERE sg.settlement_id = s.id AND sg.good_key = 'fish'),0)      > 0 THEN 1 ELSE 0 END) +
		                             (CASE WHEN COALESCE((SELECT sg.amount FROM settlement_goods sg WHERE sg.settlement_id = s.id AND sg.good_key = 'oil'),0)       > 0 THEN 1 ELSE 0 END) +
		                             (CASE WHEN COALESCE((SELECT sg.amount FROM settlement_goods sg WHERE sg.settlement_id = s.id AND sg.good_key = 'wine'),0)      > 0 THEN 1 ELSE 0 END) +
		                             (CASE WHEN COALESCE((SELECT sg.amount FROM settlement_goods sg WHERE sg.settlement_id = s.id AND sg.good_key = 'livestock'),0) > 0 THEN 1 ELSE 0 END)))
		                     * GREATEST(0, 1.0 - s.population::float / 5000.0)
		                 ))
		            END
		          -- starvation
		          ELSE
		            CASE WHEN s.population < 100
		                 THEN GREATEST(50, s.population - 2)
		                 ELSE GREATEST(50, ROUND(s.population * 0.995))
		            END
		     END))
		 WHERE s.world_id = $1 AND s.owner_id IS NOT NULL AND s.state != 'sunk'`,
		worldID,
	); err != nil {
		slog.Error("daily decay failed", "world", worldID, "err", err)
	}

	// Recompute production for all active settlements: population changed, so
	// labor_pool (and therefore rates) must be updated.
	sidRows, err := h.pool.Query(ctx,
		`SELECT id FROM settlements WHERE world_id = $1 AND owner_id IS NOT NULL AND state != 'sunk'`,
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
// cavalry each lose 5% (minimum 1 unit) per day.
func (h *TickHandler) applyStarvation(ctx context.Context, worldID uuid.UUID) {
	rows, err := h.pool.Query(ctx,
		`UPDATE settlements s SET
		   infantry = GREATEST(0, infantry - GREATEST(1, (infantry * 0.05)::int)),
		   cavalry  = GREATEST(0, cavalry  - GREATEST(1, (cavalry  * 0.05)::int))
		 WHERE s.world_id = $1 AND s.owner_id IS NOT NULL AND s.state != 'sunk'
		   AND (s.infantry > 0 OR s.cavalry > 0)
		   AND COALESCE(
		           (SELECT sg.amount + EXTRACT(EPOCH FROM (now()-sg.calc_at))/60 * sg.rate
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
			"cavalry_loss",
			"The gods have scattered your horses in the night. Cavalry has perished.",
			`UPDATE settlements SET cavalry = GREATEST(0, cavalry - GREATEST(1, cavalry/5)) WHERE id = $1`,
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
			   amount  = GREATEST(0, (amount + EXTRACT(EPOCH FROM (now() - calc_at))/60 * rate) * 0.5),
			   calc_at = now()
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
			   amount  = LEAST(cap, (amount + EXTRACT(EPOCH FROM (now() - calc_at))/60 * rate) * 1.25),
			   calc_at = now()
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
