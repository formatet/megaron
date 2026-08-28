package economy

import (
	"context"
	"math"
	"os"
	"testing"

	"formatet/megaron/server/internal/events"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// The granary's one hard invariant: FOOD IS STRICTLY CONSERVED. It moves
// city <-> granary and is never created or destroyed. The fund it replaced broke
// exactly this — stabilizeGood DESTROYED grain on a buy and CONJURED it on a
// sell, which is the "Victoria 3 conjures goods to fill shortages" failure the
// plan names. Every test here weighs the two sides before and after.
//
// testPool connects to a real Postgres — the tick is SQL orchestration across
// settlements/settlement_goods/settlement_granary that a mock can't stand in
// for. Skips (not fails) when DATABASE_URL isn't set.
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

// granaryFixture builds an active world + one settlement with grain and fish
// rows at calc_tick = current_tick (so settled()==amount) and a seeded granary.
// grainCap/fishCap bound the CITY's rows — the release leg has to respect them.
type fixtureGood struct {
	key    string
	amount float64
	cap    float64
}

func granaryFixture(t *testing.T, pool *pgxpool.Pool, ctx context.Context, currentTick, pop int, goods []fixtureGood, granary map[string]float64) (worldID, settlementID uuid.UUID) {
	t.Helper()
	// Free the one_active_world partial unique index from any leftover of ours.
	if _, err := pool.Exec(ctx,
		`UPDATE worlds SET status = 'archived' WHERE status = 'active'`,
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
		`INSERT INTO players (username, password_hash) VALUES ($1, 'x') RETURNING id`,
		"sitos-"+uuid.New().String(),
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
		`INSERT INTO settlements (world_id, province_id, name, culture_id, owner_id, control_type, is_capital, population)
		 VALUES ($1, $2, 'Sitosville', 'achaean', $3, 'capital', true, $4) RETURNING id`,
		worldID, provinceID, ownerID, pop,
	).Scan(&settlementID); err != nil {
		t.Fatalf("create settlement: %v", err)
	}

	// Silver always exists in a real city; seeded here so a test that asserts
	// "no silver moved" has something that COULD have moved.
	if _, err := pool.Exec(ctx,
		`INSERT INTO settlement_goods (settlement_id, good_key, amount, rate, cap, calc_tick)
		 VALUES ($1, 'silver', 5000, 0, 100000, $2)`,
		settlementID, currentTick,
	); err != nil {
		t.Fatalf("seed silver: %v", err)
	}
	for _, g := range goods {
		if _, err := pool.Exec(ctx,
			`INSERT INTO settlement_goods (settlement_id, good_key, amount, rate, cap, calc_tick)
			 VALUES ($1, $2, $3, 0, $4, $5)`,
			settlementID, g.key, g.amount, g.cap, currentTick,
		); err != nil {
			t.Fatalf("seed good %s: %v", g.key, err)
		}
	}
	for good, amount := range granary {
		if _, err := pool.Exec(ctx,
			`INSERT INTO settlement_granary (settlement_id, good_key, amount) VALUES ($1, $2, $3)`,
			settlementID, good, amount,
		); err != nil {
			t.Fatalf("seed granary %s: %v", good, err)
		}
	}
	return worldID, settlementID
}

// totalFood weighs both sides: what the city holds plus what the granary holds.
func totalFood(t *testing.T, pool *pgxpool.Pool, ctx context.Context, settlementID uuid.UUID) (city, granary float64) {
	t.Helper()
	if err := pool.QueryRow(ctx,
		`SELECT COALESCE(SUM(GREATEST(0, settled(amount, rate, calc_tick))), 0)
		 FROM settlement_goods WHERE settlement_id = $1 AND good_key IN ('grain', 'fish')`,
		settlementID,
	).Scan(&city); err != nil {
		t.Fatalf("read city food: %v", err)
	}
	if err := pool.QueryRow(ctx,
		`SELECT COALESCE(SUM(amount), 0) FROM settlement_granary WHERE settlement_id = $1`,
		settlementID,
	).Scan(&granary); err != nil {
		t.Fatalf("read granary: %v", err)
	}
	return city, granary
}

func liquidSilver(t *testing.T, pool *pgxpool.Pool, ctx context.Context, settlementID uuid.UUID) float64 {
	t.Helper()
	var s float64
	if err := pool.QueryRow(ctx,
		`SELECT GREATEST(0, settled(amount, rate, calc_tick)) FROM settlement_goods
		 WHERE settlement_id = $1 AND good_key = 'silver'`,
		settlementID,
	).Scan(&s); err != nil {
		t.Fatalf("read silver: %v", err)
	}
	return s
}

func newTestHandler(pool *pgxpool.Pool, cfg SitosConfig) *SitosTickHandler {
	return NewSitosTickHandler(pool, events.NewScheduler(pool, nil), events.NewStore(pool), nil, cfg)
}

// TestGranary_StoreConservesFood: a city well above the high threshold puts a
// tithe aside, and the sum city+granary is unchanged to the last unit. Also the
// case B6 exists for: the tithe is taken from BOTH grain and fish, in
// proportion, so a fish-fed city contributes fish.
func TestGranary_StoreConservesFood(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	cfg := testSitosCfg()

	const tick = 100
	const pop = 1000 // need 5/day, high threshold 150, cap 300 (mig 136, ÷100: 500→5, 15000→150, 30000→300)
	// 160 grain + 80 fish (16000/8000 → ÷100) = 240 food = 48 days. Surplus 90,
	// tithe 9, split 2:1 by stock → 6 grain, 3 fish.
	worldID, settlementID := granaryFixture(t, pool, ctx, tick, pop,
		[]fixtureGood{{"grain", 160, 1000000}, {"fish", 80, 1000000}}, nil)

	cityBefore, granBefore := totalFood(t, pool, ctx, settlementID)
	silverBefore := liquidSilver(t, pool, ctx, settlementID)

	if err := newTestHandler(pool, cfg).tickSettlement(ctx, settlementID, worldID, 1); err != nil {
		t.Fatalf("tickSettlement: %v", err)
	}

	cityAfter, granAfter := totalFood(t, pool, ctx, settlementID)
	if math.Abs((cityAfter+granAfter)-(cityBefore+granBefore)) > 1e-6 {
		t.Errorf("food not conserved: before=%.6f after=%.6f", cityBefore+granBefore, cityAfter+granAfter)
	}
	if math.Abs(granAfter-9) > 1e-6 {
		t.Errorf("granary = %.6f, want 9 (10%% of the 90 above the threshold)", granAfter)
	}
	// B6: both goods contribute, in proportion to what the city holds.
	var storedGrain, storedFish float64
	_ = pool.QueryRow(ctx, `SELECT COALESCE(amount,0) FROM settlement_granary WHERE settlement_id=$1 AND good_key='grain'`, settlementID).Scan(&storedGrain)
	_ = pool.QueryRow(ctx, `SELECT COALESCE(amount,0) FROM settlement_granary WHERE settlement_id=$1 AND good_key='fish'`, settlementID).Scan(&storedFish)
	if math.Abs(storedGrain-6) > 1e-6 || math.Abs(storedFish-3) > 1e-6 {
		t.Errorf("stored grain=%.3f fish=%.3f, want 6/3 (2:1, the city's own mix)", storedGrain, storedFish)
	}
	// B3: the granary must not touch silver on ANY path.
	if s := liquidSilver(t, pool, ctx, settlementID); math.Abs(s-silverBefore) > 1e-9 {
		t.Errorf("silver moved: %.6f → %.6f — the granary must never touch silver", silverBefore, s)
	}
}

// TestGranary_ReleasesIntoFamine: the case E1 forbade and B2 struck. A city in a
// real, stable famine gets food out of its own reserve. Under the fund this was
// impossible twice over — it was ruled out by policy, and its momentum-detector
// trigger was silent at a stable equilibrium anyway.
func TestGranary_ReleasesIntoFamine(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	cfg := testSitosCfg()

	const tick = 100
	const pop = 1000 // need 5/day, low threshold 50 (mig 136, ÷100: 500→5, 5000→50)
	// 10 grain (1000→10) = 2 days. Granary holds 200 grain (20000→200) →
	// release the 40 shortfall (4000→40).
	worldID, settlementID := granaryFixture(t, pool, ctx, tick, pop,
		[]fixtureGood{{"grain", 10, 1000000}}, map[string]float64{"grain": 200})

	cityBefore, granBefore := totalFood(t, pool, ctx, settlementID)

	if err := newTestHandler(pool, cfg).tickSettlement(ctx, settlementID, worldID, 1); err != nil {
		t.Fatalf("tickSettlement: %v", err)
	}

	cityAfter, granAfter := totalFood(t, pool, ctx, settlementID)
	if math.Abs((cityAfter+granAfter)-(cityBefore+granBefore)) > 1e-6 {
		t.Errorf("food not conserved: before=%.6f after=%.6f", cityBefore+granBefore, cityAfter+granAfter)
	}
	if math.Abs(cityAfter-50) > 1e-6 {
		t.Errorf("city food = %.6f, want 50 (topped up to the low threshold)", cityAfter)
	}
	if math.Abs(granAfter-160) > 1e-6 {
		t.Errorf("granary = %.6f, want 160", granAfter)
	}
}

// TestGranary_EmptyGranaryCannotSave: the same famine, with nothing set aside.
// The city gets NOTHING — and no food is conjured to fill the gap. This is the
// only limit on famine relief (B2), and it is also the property the fund's sell
// leg violated: it created grain out of nothing.
func TestGranary_EmptyGranaryCannotSave(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	cfg := testSitosCfg()

	const tick = 100
	// 1000 → 10 (mig 136, ÷100).
	worldID, settlementID := granaryFixture(t, pool, ctx, tick, 1000,
		[]fixtureGood{{"grain", 10, 1000000}}, nil)

	if err := newTestHandler(pool, cfg).tickSettlement(ctx, settlementID, worldID, 1); err != nil {
		t.Fatalf("tickSettlement: %v", err)
	}

	cityAfter, granAfter := totalFood(t, pool, ctx, settlementID)
	if math.Abs(cityAfter-10) > 1e-6 {
		t.Errorf("city food = %.6f, want 10 unchanged — an empty granary must not conjure food", cityAfter)
	}
	if granAfter != 0 {
		t.Errorf("granary = %.6f, want 0", granAfter)
	}
}

// TestGranary_ReleaseRespectsCityCap: the city's own good cap bounds the
// release. Crediting past the cap would have LEAST() swallow the difference and
// the food would vanish from both sides — the triple-gate principle, carried
// over from the fund's silver legs to the granary's food legs. What the cap
// refuses stays in the granary for a later tick.
func TestGranary_ReleaseRespectsCityCap(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	cfg := testSitosCfg()

	const tick = 100
	const pop = 1000 // low threshold 50 (mig 136, ÷100: 5000→50); shortfall from 10 is 40
	// The city's grain cap is 25 (2500→25, mig 136 ÷100), so only 15 can physically arrive.
	worldID, settlementID := granaryFixture(t, pool, ctx, tick, pop,
		[]fixtureGood{{"grain", 10, 25}}, map[string]float64{"grain": 200})

	cityBefore, granBefore := totalFood(t, pool, ctx, settlementID)

	if err := newTestHandler(pool, cfg).tickSettlement(ctx, settlementID, worldID, 1); err != nil {
		t.Fatalf("tickSettlement: %v", err)
	}

	cityAfter, granAfter := totalFood(t, pool, ctx, settlementID)
	if math.Abs((cityAfter+granAfter)-(cityBefore+granBefore)) > 1e-6 {
		t.Errorf("food not conserved under a binding cap: before=%.6f after=%.6f (this is where clipping destroys it)",
			cityBefore+granBefore, cityAfter+granAfter)
	}
	if math.Abs(cityAfter-25) > 1e-6 {
		t.Errorf("city food = %.6f, want 25 (its cap)", cityAfter)
	}
	if math.Abs(granAfter-185) > 1e-6 {
		t.Errorf("granary = %.6f, want 185 — the rest waits for a later tick", granAfter)
	}
}

// TestGranary_QuietInsideTheBand: between the thresholds nothing moves at all.
func TestGranary_QuietInsideTheBand(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	cfg := testSitosCfg()

	const tick = 100
	// 10000 food at need 500/day = 20 days: inside [10, 30].
	worldID, settlementID := granaryFixture(t, pool, ctx, tick, 1000,
		[]fixtureGood{{"grain", 10000, 1000000}}, map[string]float64{"grain": 5000})

	cityBefore, granBefore := totalFood(t, pool, ctx, settlementID)
	if err := newTestHandler(pool, cfg).tickSettlement(ctx, settlementID, worldID, 1); err != nil {
		t.Fatalf("tickSettlement: %v", err)
	}
	cityAfter, granAfter := totalFood(t, pool, ctx, settlementID)
	if cityAfter != cityBefore || granAfter != granBefore {
		t.Errorf("something moved inside the band: city %.3f→%.3f, granary %.3f→%.3f",
			cityBefore, cityAfter, granBefore, granAfter)
	}
}

// TestGranary_DoubleFireIsIdempotent: replaying the SAME ScheduledSitosTick
// event must not store the tithe twice. The claim in processed_sitos_ticks
// commits in the same transaction as the writes, so a retry short-circuits.
func TestGranary_DoubleFireIsIdempotent(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	cfg := testSitosCfg()

	const tick = 100
	worldID, settlementID := granaryFixture(t, pool, ctx, tick, 1000,
		[]fixtureGood{{"grain", 20000, 1000000}}, nil)

	h := newTestHandler(pool, cfg)
	if err := h.tickSettlement(ctx, settlementID, worldID, 4242); err != nil {
		t.Fatalf("first tick: %v", err)
	}
	_, granOnce := totalFood(t, pool, ctx, settlementID)
	if err := h.tickSettlement(ctx, settlementID, worldID, 4242); err != nil {
		t.Fatalf("replay: %v", err)
	}
	_, granTwice := totalFood(t, pool, ctx, settlementID)

	if math.Abs(granOnce-granTwice) > 1e-9 {
		t.Errorf("replaying the same event stored again: %.6f → %.6f", granOnce, granTwice)
	}
}
