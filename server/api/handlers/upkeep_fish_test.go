package handlers

// Order28a replaces the historical grain-only army restriction: a fish-only
// city now feeds its garrison while preserving the separate silver ledger.

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"formatet/megaron/server/internal/clock"
	"formatet/megaron/server/internal/combat"
	"formatet/megaron/server/internal/events"
)

func TestUpkeep_EmptyGrainLargeFish_FeedsGarrison(t *testing.T) {
	pool := armyDisplayTestPool(t)
	ctx := context.Background()

	if _, err := pool.Exec(ctx,
		`UPDATE worlds SET status = 'archived' WHERE status = 'active' AND name LIKE 'test-upkeep-fish-%'`,
	); err != nil {
		t.Fatalf("archive leftover active test worlds: %v", err)
	}
	const tick = 4000
	var worldID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO worlds (name, status, current_tick) VALUES ($1, 'active', $2) RETURNING id`,
		"test-upkeep-fish-"+uuid.New().String(), tick,
	).Scan(&worldID); err != nil {
		t.Fatalf("create world: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `UPDATE worlds SET status = 'archived' WHERE id = $1`, worldID)
	})

	var ownerID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO players (username, password_hash) VALUES ($1, 'x') RETURNING id`,
		"upkeepfish-"+uuid.New().String(),
	).Scan(&ownerID); err != nil {
		t.Fatalf("create player: %v", err)
	}

	var provinceID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO provinces (world_id, map_q, map_r, terrain_type, coastal) VALUES ($1, 0, 0, 'coastal_sea', true) RETURNING id`,
		worldID,
	).Scan(&provinceID); err != nil {
		t.Fatalf("create province: %v", err)
	}
	var settlementID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO settlements (world_id, province_id, name, culture_id, owner_id, control_type, is_capital, state, population)
		 VALUES ($1, $2, 'Halieia', 'achaean', $3, 'capital', true, 'active', 1000) RETURNING id`,
		worldID, provinceID, ownerID,
	).Scan(&settlementID); err != nil {
		t.Fatalf("create settlement: %v", err)
	}

	// The AK1/AK4 scenario: grain stock EMPTY, fish stock LARGE, plenty of
	// silver (so any loss is grain attrition, not silver desertion).
	if _, err := pool.Exec(ctx,
		`INSERT INTO settlement_goods (settlement_id, good_key, amount, rate, cap, calc_tick) VALUES
		   ($1, 'grain',  0,      0, 1000000, $2),
		   ($1, 'fish',   50000,  0, 1000000, $2),
		   ($1, 'silver', 100000, 0, 1000000, $2)`,
		settlementID, tick,
	); err != nil {
		t.Fatalf("seed goods: %v", err)
	}

	var unitID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO units (world_id, owner_id, type, category, size, crew, status,
		                    settlement_id, support_settlement_id, unpaid_periods)
		 VALUES ($1, $2, 'spearman', 'land', 100, 0, 'garrison', $3, $3, 0)
		 RETURNING id`,
		worldID, ownerID, settlementID,
	).Scan(&unitID); err != nil {
		t.Fatalf("create unit: %v", err)
	}

	fishBefore := settledGoodAmount(t, pool, settlementID, "fish")
	silverBefore := settledGoodAmount(t, pool, settlementID, "silver")
	sizeBefore := 100

	h := combat.NewUpkeepHandler(pool, events.NewScheduler(pool, clock.NewTestClock(time.Now())),
		events.NewStore(pool), nil, nil)
	if err := h.Handle(ctx, events.ScheduledEvent{WorldID: worldID, DueTick: tick}); err != nil {
		t.Fatalf("upkeep Handle: %v", err)
	}

	var sizeAfter int
	if err := pool.QueryRow(ctx, `SELECT size FROM units WHERE id = $1`, unitID).Scan(&sizeAfter); err != nil {
		t.Fatalf("read unit size: %v", err)
	}
	if sizeAfter != sizeBefore {
		t.Errorf("fish-fed garrison size=%d want100", sizeAfter)
	}

	fishAfter := settledGoodAmount(t, pool, settlementID, "fish")
	if fishAfter != fishBefore-combat.UnitUpkeep("spearman", "land", 100, "garrison").Grain {
		t.Errorf("upkeep fish went %v -> %v, want exactly nominal food ration", fishBefore, fishAfter)
	}

	// Silver upkeep should still be paid normally (plenty in the till) —
	// confirms the grain-only attrition didn't also break the silver leg.
	silverAfter := settledGoodAmount(t, pool, settlementID, "silver")
	if silverAfter != silverBefore {
		t.Errorf("ration-only spear should preserve silver during grain shortage, went %v -> %v", silverBefore, silverAfter)
	}
}

func settledGoodAmount(t *testing.T, pool *pgxpool.Pool, settlementID uuid.UUID, good string) float64 {
	t.Helper()
	var v float64
	if err := pool.QueryRow(context.Background(),
		`SELECT settled(amount, rate, calc_tick) FROM settlement_goods WHERE settlement_id = $1 AND good_key = $2`,
		settlementID, good,
	).Scan(&v); err != nil {
		t.Fatalf("read settled %s: %v", good, err)
	}
	return v
}
