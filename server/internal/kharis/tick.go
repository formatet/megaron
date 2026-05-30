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
	"github.com/poleia/server/internal/events"
)

const (
	decayOnMissed     = 0.10  // 10% kharis lost when maintenance missed
	punishThreshold   = 100.0 // kharis below this risks divine punishment
	punishProbability = 0.30  // 30% chance of divine punishment per missed day below threshold
	blessThreshold    = 200.0 // kharis above this may attract divine favour
	blessProbability  = 0.15  // 15% chance of divine blessing per maintained day above threshold
)

// cultSpec defines the daily cost and kharis gain for each cult level.
type cultSpec struct {
	gold      float64
	food      float64
	kharisGain float64
}

var cultLevelSpecs = map[string]cultSpec{
	"forsummad": {gold: 0, food: 0, kharisGain: 0},    // no payment, kharis decays
	"enkel":     {gold: 3, food: 3, kharisGain: 2},
	"vardig":    {gold: 6, food: 5, kharisGain: 5},
	"praktfull": {gold: 12, food: 10, kharisGain: 10},
	"overdadig": {gold: 20, food: 15, kharisGain: 18},
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

type settlementSnap struct {
	id        uuid.UUID
	kharis    float64
	gold      float64
	food      float64
	cultLevel string
}

// Handle processes a KharisTick scheduled event.
func (h *TickHandler) Handle(ctx context.Context, e events.ScheduledEvent) error {
	rows, err := h.pool.Query(ctx,
		`SELECT id,
		    GREATEST(0, kharis_amount + (EXTRACT(EPOCH FROM (now() - kharis_calc_at))/60 * kharis_rate)),
		    GREATEST(0, gold_amount   + (EXTRACT(EPOCH FROM (now() - gold_calc_at))/60   * gold_rate)),
		    GREATEST(0, food_amount   + (EXTRACT(EPOCH FROM (now() - food_calc_at))/60   * food_rate)),
		    cult_level
		 FROM settlements
		 WHERE world_id = $1 AND owner_id IS NOT NULL AND state != 'sunk'`,
		e.WorldID,
	)
	if err != nil {
		return fmt.Errorf("query settlements for kharis tick: %w", err)
	}
	defer rows.Close()

	var snaps []settlementSnap
	for rows.Next() {
		var s settlementSnap
		if err := rows.Scan(&s.id, &s.kharis, &s.gold, &s.food, &s.cultLevel); err == nil {
			snaps = append(snaps, s)
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}

	for _, s := range snaps {
		if err := h.processMaintenance(ctx, s, e.WorldID); err != nil {
			slog.Error("kharis maintenance failed", "settlement", s.id, "err", err)
		}
	}

	h.applyDecay(ctx, e.WorldID)
	h.applyStarvation(ctx, e.WorldID)
	h.accumulatePrestige(ctx, e.WorldID)

	return h.scheduler.EnqueueAfter(ctx, e.WorldID, events.ScheduledKharisTick,
		struct{}{}, 24*time.Hour)
}

func (h *TickHandler) processMaintenance(ctx context.Context, s settlementSnap, worldID uuid.UUID) error {
	spec, ok := cultLevelSpecs[s.cultLevel]
	if !ok {
		spec = cultLevelSpecs["enkel"] // safe fallback
	}

	// forsummad: player deliberately skips temple — kharis decays.
	if s.cultLevel == "forsummad" {
		_, err := h.pool.Exec(ctx,
			`UPDATE settlements SET
			   kharis_amount  = GREATEST(0,
			       (kharis_amount + (EXTRACT(EPOCH FROM (now() - kharis_calc_at))/60 * kharis_rate)) * $1),
			   kharis_calc_at = now()
			 WHERE id = $2`,
			1.0-decayOnMissed, s.id,
		)
		if err != nil {
			return fmt.Errorf("kharis decay (forsummad): %w", err)
		}
		_, _ = h.store.Append(ctx, s.id, events.StreamProvince, "KharisMissedMaintenance",
			map[string]any{"cult_level": s.cultLevel, "decay_fraction": decayOnMissed}, worldID, nil)
		newKharis := s.kharis * (1.0 - decayOnMissed)
		if newKharis < punishThreshold && rand.Float64() < punishProbability {
			h.applyDivinePunishment(ctx, s.id, worldID)
		}
		return nil
	}

	// Can the settlement afford the tiered cost?
	if s.gold >= spec.gold && s.food >= spec.food {
		_, err := h.pool.Exec(ctx,
			`UPDATE settlements SET
			   gold_amount    = gold_amount   + (EXTRACT(EPOCH FROM (now() - gold_calc_at))/60   * gold_rate)   - $1,
			   gold_calc_at   = now(),
			   food_amount    = food_amount   + (EXTRACT(EPOCH FROM (now() - food_calc_at))/60   * food_rate)   - $2,
			   food_calc_at   = now(),
			   kharis_amount  = LEAST(kharis_cap,
			       kharis_amount + (EXTRACT(EPOCH FROM (now() - kharis_calc_at))/60 * kharis_rate) + $3),
			   kharis_calc_at = now()
			 WHERE id = $4`,
			spec.gold, spec.food, spec.kharisGain, s.id,
		)
		if err != nil {
			return fmt.Errorf("pay maintenance: %w", err)
		}
		_, _ = h.store.Append(ctx, s.id, events.StreamProvince, "KharisMaintained",
			map[string]any{"cult_level": s.cultLevel, "gold": spec.gold, "food": spec.food, "kharis_gain": spec.kharisGain},
			worldID, nil)
		newKharis := s.kharis + spec.kharisGain
		if newKharis > blessThreshold && rand.Float64() < blessProbability {
			h.applyDivineBlessing(ctx, s.id, worldID)
		}
		return nil
	}

	// Cannot afford chosen tier — treat as missed maintenance.
	_, err := h.pool.Exec(ctx,
		`UPDATE settlements SET
		   kharis_amount  = GREATEST(0,
		       (kharis_amount + (EXTRACT(EPOCH FROM (now() - kharis_calc_at))/60 * kharis_rate)) * $1),
		   kharis_calc_at = now()
		 WHERE id = $2`,
		1.0-decayOnMissed, s.id,
	)
	if err != nil {
		return fmt.Errorf("kharis decay: %w", err)
	}
	_, _ = h.store.Append(ctx, s.id, events.StreamProvince, "KharisMissedMaintenance",
		map[string]any{"cult_level": s.cultLevel, "decay_fraction": decayOnMissed, "reason": "insufficient_resources"},
		worldID, nil)
	newKharis := s.kharis * (1.0 - decayOnMissed)
	if newKharis < punishThreshold && rand.Float64() < punishProbability {
		h.applyDivinePunishment(ctx, s.id, worldID)
	}
	return nil
}

// applyDecay reduces food and lumber by 1% and resets invasions_today across all
// active settlements. Called once per daily tick.
func (h *TickHandler) applyDecay(ctx context.Context, worldID uuid.UUID) {
	if _, err := h.pool.Exec(ctx,
		`UPDATE settlements SET
		   food_amount    = GREATEST(0,
		       (food_amount   + EXTRACT(EPOCH FROM (now()-food_calc_at))/60   * food_rate)   * 0.99),
		   food_calc_at   = now(),
		   lumber_amount  = GREATEST(0,
		       (lumber_amount + EXTRACT(EPOCH FROM (now()-lumber_calc_at))/60 * lumber_rate) * 0.99),
		   lumber_calc_at = now(),
		   invasions_today = 0
		 WHERE world_id = $1 AND owner_id IS NOT NULL AND state != 'sunk'`,
		worldID,
	); err != nil {
		slog.Error("daily decay failed", "world", worldID, "err", err)
	}
}

// applyStarvation punishes settlements where food has hit zero: infantry and
// cavalry each lose 5% (minimum 1 unit) per day.
func (h *TickHandler) applyStarvation(ctx context.Context, worldID uuid.UUID) {
	rows, err := h.pool.Query(ctx,
		`UPDATE settlements SET
		   infantry = GREATEST(0, infantry - GREATEST(1, (infantry * 0.05)::int)),
		   cavalry  = GREATEST(0, cavalry  - GREATEST(1, (cavalry  * 0.05)::int))
		 WHERE world_id = $1 AND owner_id IS NOT NULL AND state != 'sunk'
		   AND (infantry > 0 OR cavalry > 0)
		   AND food_amount + EXTRACT(EPOCH FROM (now()-food_calc_at))/60 * food_rate <= 0
		 RETURNING id`,
		worldID,
	)
	if err != nil {
		slog.Error("starvation tick failed", "world", worldID, "err", err)
		return
	}
	defer rows.Close()

	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			continue
		}
		_, _ = h.store.Append(ctx, id, events.StreamProvince, "StarvationDamage",
			map[string]any{"reason": "no_food"}, worldID, nil)
		slog.Info("starvation damage applied", "settlement", id)
	}
}

// applyDivinePunishment randomly selects and applies one divine punishment.
func (h *TickHandler) applyDivinePunishment(ctx context.Context, settlementID, worldID uuid.UUID) {
	type punishment struct {
		name string
		sql  string
	}

	punishments := []punishment{
		{
			"cavalry_loss",
			`UPDATE settlements SET cavalry = GREATEST(0, cavalry - GREATEST(1, cavalry/5)) WHERE id = $1`,
		},
		{
			"ship_loss",
			`UPDATE settlements SET ship = GREATEST(0, ship - 1) WHERE id = $1`,
		},
		{
			"harvest_failure",
			`UPDATE settlements SET
			   food_amount = GREATEST(0,
			       (food_amount + (EXTRACT(EPOCH FROM (now() - food_calc_at))/60 * food_rate)) * 0.5),
			   food_calc_at = now()
			 WHERE id = $1`,
		},
		{
			"garrison_plague",
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
	slog.Info("divine punishment applied", "settlement", settlementID, "type", p.name)
}

// applyDivineBlessing randomly selects and applies one divine blessing for settlements
// that maintain high kharis. Mirror of applyDivinePunishment.
func (h *TickHandler) applyDivineBlessing(ctx context.Context, settlementID, worldID uuid.UUID) {
	type blessing struct {
		name string
		sql  string
	}

	blessings := []blessing{
		{
			"harvest_blessing",
			`UPDATE settlements SET
			   food_amount = LEAST(food_cap,
			       (food_amount + (EXTRACT(EPOCH FROM (now() - food_calc_at))/60 * food_rate)) * 1.25),
			   food_calc_at = now()
			 WHERE id = $1`,
		},
		{
			"divine_recruits",
			`UPDATE settlements SET
			   infantry = infantry + GREATEST(2, infantry / 5)
			 WHERE id = $1`,
		},
		{
			"sea_blessing",
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
	slog.Info("divine blessing applied", "settlement", settlementID, "type", b.name)
}

// accumulatePrestige adds daily prestige to the world based on active cult devotion.
// One point per active settlement, plus a tier bonus (vardig+1, praktfull+2, overdadig+3).
// Prestige feeds into the collapse risk algorithm.
func (h *TickHandler) accumulatePrestige(ctx context.Context, worldID uuid.UUID) {
	_, err := h.pool.Exec(ctx,
		`UPDATE worlds SET prestige = prestige + (
		    SELECT COALESCE(SUM(
		        1 + CASE cult_level
		            WHEN 'vardig'    THEN 1
		            WHEN 'praktfull' THEN 2
		            WHEN 'overdadig' THEN 3
		            ELSE 0
		        END
		    ), 0)
		    FROM settlements
		    WHERE world_id = $1 AND owner_id IS NOT NULL AND state != 'sunk'
		      AND cult_level != 'forsummad'
		)
		WHERE id = $1`,
		worldID,
	)
	if err != nil {
		slog.Error("prestige accumulation failed", "world", worldID, "err", err)
	}
}
