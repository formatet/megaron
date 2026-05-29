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
	maintenanceGold  = 3.0  // gold per day
	maintenanceFood  = 3.0  // food per day
	decayOnMissed    = 0.10 // 10% kharis lost when maintenance missed
	punishThreshold  = 100.0
	punishProbability = 0.30 // 30% chance of divine punishment per missed day below threshold
)

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
	id     uuid.UUID
	kharis float64
	gold   float64
	food   float64
}

// Handle processes a KharisTick scheduled event.
func (h *TickHandler) Handle(ctx context.Context, e events.ScheduledEvent) error {
	rows, err := h.pool.Query(ctx,
		`SELECT id,
		    GREATEST(0, kharis_amount + (EXTRACT(EPOCH FROM (now() - kharis_calc_at))/60 * kharis_rate)),
		    GREATEST(0, gold_amount   + (EXTRACT(EPOCH FROM (now() - gold_calc_at))/60   * gold_rate)),
		    GREATEST(0, food_amount   + (EXTRACT(EPOCH FROM (now() - food_calc_at))/60   * food_rate))
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
		if err := rows.Scan(&s.id, &s.kharis, &s.gold, &s.food); err == nil {
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

	return h.scheduler.Enqueue(ctx, e.WorldID, events.ScheduledKharisTick,
		struct{}{}, time.Now().Add(24*time.Hour))
}

func (h *TickHandler) processMaintenance(ctx context.Context, s settlementSnap, worldID uuid.UUID) error {
	if s.gold >= maintenanceGold && s.food >= maintenanceFood {
		// Pay maintenance — small kharis reward (+2, capped).
		_, err := h.pool.Exec(ctx,
			`UPDATE settlements SET
			   gold_amount    = gold_amount   + (EXTRACT(EPOCH FROM (now() - gold_calc_at))/60   * gold_rate)   - $1,
			   gold_calc_at   = now(),
			   food_amount    = food_amount   + (EXTRACT(EPOCH FROM (now() - food_calc_at))/60   * food_rate)   - $2,
			   food_calc_at   = now(),
			   kharis_amount  = LEAST(kharis_cap,
			       kharis_amount + (EXTRACT(EPOCH FROM (now() - kharis_calc_at))/60 * kharis_rate) + 2),
			   kharis_calc_at = now()
			 WHERE id = $3`,
			maintenanceGold, maintenanceFood, s.id,
		)
		if err != nil {
			return fmt.Errorf("pay maintenance: %w", err)
		}
		_, _ = h.store.Append(ctx, s.id, events.StreamProvince, "KharisMaintained",
			map[string]any{"gold": maintenanceGold, "food": maintenanceFood, "kharis_gain": 2},
			worldID, nil)
		return nil
	}

	// Maintenance missed: kharis decays by 10%.
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
		map[string]any{"decay_fraction": decayOnMissed}, worldID, nil)

	// Divine punishment rolls when kharis is critically low.
	newKharis := s.kharis * (1.0 - decayOnMissed)
	if newKharis < punishThreshold && rand.Float64() < punishProbability {
		h.applyDivinePunishment(ctx, s.id, worldID)
	}

	return nil
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
