package economy

import (
	"context"
	"math"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"formatet/megaron/server/internal/events"
)

// testPool connects to a real Postgres — the Sitos tick is SQL orchestration
// across settlements/settlement_goods that a mock can't stand in for. Skips
// (not fails) when DATABASE_URL isn't set, so `go test ./...` stays green
// without a DB. Mirrors combat/unit_arrival_colonize_test.go.
func testPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL not set — skipping DB integration test")
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatalf("connect to test database: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

// sitosFixture builds an active world + one settlement with silver + grain rows
// at calc_tick = current_tick (so settled()==amount) and a seeded fund.
func sitosFixture(t *testing.T, pool *pgxpool.Pool, ctx context.Context, currentTick int, pop int, fund, silver, grainAmount, grainRate float64) (worldID, settlementID uuid.UUID) {
	t.Helper()
	// Free the one_active_world partial unique index from any leftover of ours.
	if _, err := pool.Exec(ctx,
		`UPDATE worlds SET status = 'archived' WHERE status = 'active' AND name LIKE 'test-sitos-%'`,
	); err != nil {
		t.Fatalf("archive leftover test worlds: %v", err)
	}
	if err := pool.QueryRow(ctx,
		`INSERT INTO worlds (name, status, current_tick) VALUES ($1, 'active', $2) RETURNING id`,
		"test-sitos-"+uuid.New().String(), currentTick,
	).Scan(&worldID); err != nil {
		t.Fatalf("create world: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `UPDATE worlds SET status = 'archived' WHERE id = $1`, worldID)
	})

	var ownerID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO players (username, email, password_hash) VALUES ($1, $2, 'x') RETURNING id`,
		"sitos-"+uuid.New().String(), "sitos-"+uuid.New().String()+"@test.invalid",
	).Scan(&ownerID); err != nil {
		t.Fatalf("create player: %v", err)
	}

	var provinceID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO provinces (world_id, map_q, map_r, terrain_type) VALUES ($1, 0, 0, 'plains') RETURNING id`,
		worldID,
	).Scan(&provinceID); err != nil {
		t.Fatalf("create province: %v", err)
	}
	if err := pool.QueryRow(ctx,
		`INSERT INTO settlements (world_id, province_id, name, culture_id, owner_id, control_type, is_capital, population, sitos_fund_silver)
		 VALUES ($1, $2, 'Sitosville', 'achaean', $3, 'capital', true, $4, $5) RETURNING id`,
		worldID, provinceID, ownerID, pop, fund,
	).Scan(&settlementID); err != nil {
		t.Fatalf("create settlement: %v", err)
	}

	for _, g := range []struct {
		key    string
		amount float64
		rate   float64
		cap    float64
	}{
		{"silver", silver, 0, 100000},
		{"grain", grainAmount, grainRate, 1000},
	} {
		if _, err := pool.Exec(ctx,
			`INSERT INTO settlement_goods (settlement_id, good_key, amount, rate, cap, calc_tick)
			 VALUES ($1, $2, $3, $4, $5, $6)`,
			settlementID, g.key, g.amount, g.rate, g.cap, currentTick,
		); err != nil {
			t.Fatalf("seed good %s: %v", g.key, err)
		}
	}
	return worldID, settlementID
}

func totalSilver(t *testing.T, pool *pgxpool.Pool, ctx context.Context, settlementID uuid.UUID) float64 {
	t.Helper()
	var fund, silver float64
	if err := pool.QueryRow(ctx,
		`SELECT GREATEST(0, sitos_fund_silver) FROM settlements WHERE id = $1`, settlementID,
	).Scan(&fund); err != nil {
		t.Fatalf("read fund: %v", err)
	}
	if err := pool.QueryRow(ctx,
		`SELECT GREATEST(0, settled(amount, rate, calc_tick)) FROM settlement_goods WHERE settlement_id = $1 AND good_key = 'silver'`,
		settlementID,
	).Scan(&silver); err != nil {
		t.Fatalf("read silver: %v", err)
	}
	return fund + silver
}

// TestSitosTick_SilverConserved: a shortage-sell + tax leg must leave
// silver(settlement) + silver(fund) exactly constant — silver only moves
// fund↔settlement, never created or destroyed.
func TestSitosTick_SilverConserved(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	cfg := testSitosCfg()

	const tick = 100
	// Grain in deep shortage (amount 5, cap 1000 → below reference 300) → fund sells.
	worldID, settlementID := sitosFixture(t, pool, ctx, tick, 1000, /*fund*/ 5000, /*silver*/ 2000, /*grain*/ 5, /*rate*/ 0)

	before := totalSilver(t, pool, ctx, settlementID)

	h := NewSitosTickHandler(pool, events.NewScheduler(pool, nil), events.NewStore(pool), nil, cfg)
	grainBase, err := GoodBaseValue(ctx, pool, "grain")
	if err != nil {
		t.Fatalf("grain base value: %v", err)
	}
	if err := h.tickSettlement(ctx, settlementID, worldID, grainBase); err != nil {
		t.Fatalf("tickSettlement: %v", err)
	}

	after := totalSilver(t, pool, ctx, settlementID)
	if math.Abs(after-before) > 1e-6 {
		t.Errorf("silver not conserved: before=%.6f after=%.6f (Δ=%.6f)", before, after, after-before)
	}
}

// releaseEvent returns the (silver_moved, fund_after) of the latest
// SitosFundRelease event for a settlement, or moved=-1 if none was emitted.
func releaseEvent(t *testing.T, pool *pgxpool.Pool, ctx context.Context, settlementID uuid.UUID) (moved, fundAfter float64) {
	t.Helper()
	err := pool.QueryRow(ctx,
		`SELECT (payload->>'silver_moved')::float, (payload->>'fund_after')::float
		 FROM events WHERE stream_id = $1 AND event_type = 'SitosFundRelease'
		 ORDER BY id DESC LIMIT 1`,
		settlementID,
	).Scan(&moved, &fundAfter)
	if err != nil {
		return -1, 0
	}
	return moved, fundAfter
}

// TestSitosTick_ReleaseConservesToCapWhenHeadroom: a fund seeded above cap, with
// ample liquid headroom, releases the whole overhang into liquid silver — total
// silver conserved, fund lands exactly on cap. Subsistence goods disabled so only
// the release leg moves silver (tax noops at cap).
func TestSitosTick_ReleaseConservesToCapWhenHeadroom(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	cfg := testSitosCfg()
	cfg.FundCapMult = 1        // cap = dailyGrainNeedInSilver = 1500 for pop 1000
	cfg.SubsistenceGoods = nil // isolate the release leg

	const tick = 100
	// fund 5000 (overhang 3500 over cap 1500), liquid 2000 with a roomy cap.
	worldID, settlementID := sitosFixture(t, pool, ctx, tick, 1000, /*fund*/ 5000, /*silver*/ 2000, /*grain*/ 300, /*rate*/ 0)

	before := totalSilver(t, pool, ctx, settlementID)

	h := NewSitosTickHandler(pool, events.NewScheduler(pool, nil), events.NewStore(pool), nil, cfg)
	grainBase, err := GoodBaseValue(ctx, pool, "grain")
	if err != nil {
		t.Fatalf("grain base value: %v", err)
	}
	if err := h.tickSettlement(ctx, settlementID, worldID, grainBase); err != nil {
		t.Fatalf("tickSettlement: %v", err)
	}

	after := totalSilver(t, pool, ctx, settlementID)
	if math.Abs(after-before) > 1e-6 {
		t.Errorf("silver not conserved: before=%.6f after=%.6f (Δ=%.6f)", before, after, after-before)
	}
	var fund float64
	if err := pool.QueryRow(ctx, `SELECT sitos_fund_silver FROM settlements WHERE id = $1`, settlementID).Scan(&fund); err != nil {
		t.Fatalf("read fund: %v", err)
	}
	if math.Abs(fund-1500) > 1e-6 {
		t.Errorf("fund after release = %.4f, want 1500 (= cap)", fund)
	}
	moved, fundAfter := releaseEvent(t, pool, ctx, settlementID)
	if math.Abs(moved-3500) > 1e-6 || math.Abs(fundAfter-1500) > 1e-6 {
		t.Errorf("SitosFundRelease = {moved %.4f, fund_after %.4f}, want {3500, 1500}", moved, fundAfter)
	}
}

// TestSitosTick_ReleaseRespectsLiquidCap: when liquid headroom can't absorb the
// whole overhang, only the headroom is released; the fund stays above cap (never
// below) and silver is still conserved.
func TestSitosTick_ReleaseRespectsLiquidCap(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	cfg := testSitosCfg()
	cfg.FundCapMult = 1
	cfg.SubsistenceGoods = nil

	const tick = 100
	worldID, settlementID := sitosFixture(t, pool, ctx, tick, 1000, /*fund*/ 5000, /*silver*/ 2000, /*grain*/ 300, /*rate*/ 0)
	// Tighten the liquid silver cap to 3000 → headroom 1000 < overhang 3500.
	if _, err := pool.Exec(ctx, `UPDATE settlement_goods SET cap = 3000 WHERE settlement_id = $1 AND good_key = 'silver'`, settlementID); err != nil {
		t.Fatalf("tighten silver cap: %v", err)
	}

	before := totalSilver(t, pool, ctx, settlementID)

	h := NewSitosTickHandler(pool, events.NewScheduler(pool, nil), events.NewStore(pool), nil, cfg)
	grainBase, err := GoodBaseValue(ctx, pool, "grain")
	if err != nil {
		t.Fatalf("grain base value: %v", err)
	}
	if err := h.tickSettlement(ctx, settlementID, worldID, grainBase); err != nil {
		t.Fatalf("tickSettlement: %v", err)
	}

	after := totalSilver(t, pool, ctx, settlementID)
	if math.Abs(after-before) > 1e-6 {
		t.Errorf("silver not conserved: before=%.6f after=%.6f (Δ=%.6f)", before, after, after-before)
	}
	var fund, liquid float64
	if err := pool.QueryRow(ctx, `SELECT sitos_fund_silver FROM settlements WHERE id = $1`, settlementID).Scan(&fund); err != nil {
		t.Fatalf("read fund: %v", err)
	}
	if err := pool.QueryRow(ctx, `SELECT settled(amount, rate, calc_tick) FROM settlement_goods WHERE settlement_id = $1 AND good_key = 'silver'`, settlementID).Scan(&liquid); err != nil {
		t.Fatalf("read liquid: %v", err)
	}
	if math.Abs(liquid-3000) > 1e-6 {
		t.Errorf("liquid after release = %.4f, want 3000 (at cap)", liquid)
	}
	if math.Abs(fund-4000) > 1e-6 || fund < 1500 {
		t.Errorf("fund after partial release = %.4f, want 4000 (still > cap)", fund)
	}
	if moved, _ := releaseEvent(t, pool, ctx, settlementID); math.Abs(moved-1000) > 1e-6 {
		t.Errorf("release moved = %.4f, want 1000 (= liquid headroom)", moved)
	}
}

// TestSitosTick_NoReleaseWithinCap: a fund at or below cap emits no release event
// and leaves the fund untouched by the release leg.
func TestSitosTick_NoReleaseWithinCap(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	cfg := testSitosCfg()
	cfg.FundCapMult = 1
	cfg.SubsistenceGoods = nil

	const tick = 100
	// fund 1000 < cap 1500 → no overhang.
	worldID, settlementID := sitosFixture(t, pool, ctx, tick, 1000, /*fund*/ 1000, /*silver*/ 2000, /*grain*/ 300, /*rate*/ 0)

	h := NewSitosTickHandler(pool, events.NewScheduler(pool, nil), events.NewStore(pool), nil, cfg)
	grainBase, err := GoodBaseValue(ctx, pool, "grain")
	if err != nil {
		t.Fatalf("grain base value: %v", err)
	}
	if err := h.tickSettlement(ctx, settlementID, worldID, grainBase); err != nil {
		t.Fatalf("tickSettlement: %v", err)
	}
	if moved, _ := releaseEvent(t, pool, ctx, settlementID); moved != -1 {
		t.Errorf("expected no SitosFundRelease event, got moved=%.4f", moved)
	}
}

// TestSitosTick_FundNeverNegative: repeated buy pressure (surplus grain, tiny
// fund) must never drive sitos_fund_silver below 0 and must not crash.
func TestSitosTick_FundNeverNegative(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	cfg := testSitosCfg()

	const tick = 100
	// Grain surplus (amount 990 near cap 1000 → above reference) → fund buys and drains.
	worldID, settlementID := sitosFixture(t, pool, ctx, tick, 1000, /*fund*/ 50, /*silver*/ 1000, /*grain*/ 990, /*rate*/ 0)

	h := NewSitosTickHandler(pool, events.NewScheduler(pool, nil), events.NewStore(pool), nil, cfg)
	grainBase, err := GoodBaseValue(ctx, pool, "grain")
	if err != nil {
		t.Fatalf("grain base value: %v", err)
	}

	for i := 0; i < 5; i++ {
		if err := h.tickSettlement(ctx, settlementID, worldID, grainBase); err != nil {
			t.Fatalf("tickSettlement iter %d: %v", i, err)
		}
		var fund float64
		if err := pool.QueryRow(ctx,
			`SELECT sitos_fund_silver FROM settlements WHERE id = $1`, settlementID,
		).Scan(&fund); err != nil {
			t.Fatalf("read fund iter %d: %v", i, err)
		}
		if fund < 0 {
			t.Fatalf("fund went negative on iter %d: %.6f", i, fund)
		}
	}
}
