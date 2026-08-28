package handlers

// AK4 (fisk-föder-befolkningen, 2026-07-31): the army upkeep path
// (internal/combat/upkeep.go) must never touch fish — a settlement with an
// empty grain stock and a large fish stock must starve its garrison exactly
// like on master, and the fish stock must be completely untouched by the
// upkeep tick. This is guaranteed by construction (upkeep.go's grain
// deduction is a hardcoded `good_key = 'grain'` UPDATE, never touched by this
// slice — internal/combat is explicit non-scope), but is proved here as a
// real DB integration test rather than left as a code-reading claim.
//
// DB integration test (real Postgres, gated by DATABASE_URL via the shared
// armyDisplayTestPool helper).

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

func TestUpkeep_AK4_EmptyGrainLargeFish_StillAttrits_FishUntouched(t *testing.T) {
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
		`INSERT INTO players (username, email, password_hash) VALUES ($1, $2, 'x') RETURNING id`,
		"upkeepfish-"+uuid.New().String(), "upkeepfish-"+uuid.New().String()+"@test.invalid",
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
	if sizeAfter >= sizeBefore {
		t.Errorf("AK4: garrison with empty grain must attrit exactly like on master, size stayed %d", sizeAfter)
	}

	fishAfter := settledGoodAmount(t, pool, settlementID, "fish")
	if fishAfter != fishBefore {
		t.Errorf("AK4: upkeep must NEVER touch fish, fish went %v -> %v", fishBefore, fishAfter)
	}

	// Silver upkeep should still be paid normally (plenty in the till) —
	// confirms the grain-only attrition didn't also break the silver leg.
	silverAfter := settledGoodAmount(t, pool, settlementID, "silver")
	if silverAfter >= silverBefore {
		t.Errorf("AK4 sanity: silver upkeep should still be debited normally, went %v -> %v", silverBefore, silverAfter)
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
