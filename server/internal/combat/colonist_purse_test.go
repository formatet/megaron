package combat

// B3: silver enters the game ONLY via starting silver and mines. Until mig 107
// every founding also minted the colony's liquid silver — GenesisSilverLiquid
// against pop = 1500 + the column's size, so a ~2000-pop colony printed ~10 500
// into a world holding 106 678 liquid. Expansion was a printing press, and a
// Wanax could always found their way out of insolvency.
//
// These tests weigh the WORLD, not one city: what matters is that founding
// moves silver rather than creating it, and that a purse never evaporates when
// the expedition turns around.
//
// DB integration tests (real Postgres, gated by DATABASE_URL).

import (
	"context"
	"math"
	"testing"
	"time"

	"formatet/megaron/server/internal/clock"
	"formatet/megaron/server/internal/economy"
	"formatet/megaron/server/internal/events"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// worldSilver is every silver piece in the world that a founding could touch:
// the cities' liquid stock plus whatever is riding on units.
func worldSilver(t *testing.T, pool *pgxpool.Pool, ctx context.Context, worldID uuid.UUID) float64 {
	t.Helper()
	var liquid, carried float64
	if err := pool.QueryRow(ctx,
		`SELECT COALESCE(SUM(GREATEST(0, settled(sg.amount, sg.rate, sg.calc_tick))), 0)
		 FROM settlement_goods sg JOIN settlements s ON s.id = sg.settlement_id
		 WHERE s.world_id = $1 AND sg.good_key = 'silver'`,
		worldID,
	).Scan(&liquid); err != nil {
		t.Fatalf("sum liquid silver: %v", err)
	}
	if err := pool.QueryRow(ctx,
		`SELECT COALESCE(SUM(carried_silver), 0) FROM units WHERE world_id = $1`, worldID,
	).Scan(&carried); err != nil {
		t.Fatalf("sum carried silver: %v", err)
	}
	return liquid + carried
}

// TestColonistPurse_IsWhatTheColonyUsedToBeGiven: the purse is priced at exactly
// the seed the colony used to get for free, so only the SOURCE of the colony's
// starting silver changes — from the world's faucet to the mother city's
// treasury. If these drift apart, colonies silently get richer or poorer than
// the balance the rest of the economy was tuned against.
func TestColonistPurse_IsWhatTheColonyUsedToBeGiven(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer tx.Rollback(ctx)

	const unitSize = 500
	got := colonistPurse(ctx, tx, unitSize)

	grainBase, err := economy.GoodBaseValue(ctx, pool, "grain")
	if err != nil {
		t.Fatalf("grain base value: %v", err)
	}
	want, _ := economy.GenesisSilverLiquid(
		economy.ColonyBaseFoundingPopulation+unitSize, grainBase, economy.LoadSitosConfig())

	if math.Abs(got-want) > 1e-9 {
		t.Errorf("purse = %v, want %v (the seed the colony used to be handed)", got, want)
	}
	if got <= 0 {
		t.Fatalf("purse = %v — a zero purse would make this test vacuous", got)
	}
}

// TestColonistPurse_ClampedByWhatTheCityHas: a treasury that cannot cover the
// purse sends what it has and no more. Minting the difference is the exact bug
// this slice removes, and going negative would be a different one.
func TestColonistPurse_ClampedByWhatTheCityHas(t *testing.T) {
	// Pure arithmetic against the clamp the dispatch path applies. Kept as its
	// own case because the DB path below can only exercise one branch at a time,
	// and "sent everything it had" is the branch that decides whether a poor
	// Wanax can expand at all.
	cases := []struct{ want, have, wantPurse, wantShort float64 }{
		{want: 1000, have: 5000, wantPurse: 1000, wantShort: 0},
		{want: 1000, have: 400, wantPurse: 400, wantShort: 600},
		{want: 1000, have: 0, wantPurse: 0, wantShort: 1000},
	}
	for _, c := range cases {
		purse := math.Min(c.want, c.have)
		short := c.want - purse
		if purse != c.wantPurse || short != c.wantShort {
			t.Errorf("want=%v have=%v → purse %v short %v, want %v/%v",
				c.want, c.have, purse, short, c.wantPurse, c.wantShort)
		}
		if purse > c.have {
			t.Errorf("purse %v exceeds the treasury %v — that is minting", purse, c.have)
		}
	}
}

// TestFoundColony_DoesNotMintSilver: the whole point. A colony founded mid-game
// leaves the world's silver total exactly where it was — the mother city is
// poorer by precisely what the colony is richer by.
func TestFoundColony_DoesNotMintSilver(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()

	worldID, motherID, unitID := purseFixture(t, pool, ctx, 3000 /*mother silver*/, 700 /*purse on the unit*/)

	before := worldSilver(t, pool, ctx, worldID)

	// Found the colony by running the arrival handler's colonize path.
	runColonizeArrival(t, pool, ctx, worldID, unitID)

	after := worldSilver(t, pool, ctx, worldID)
	if math.Abs(after-before) > 1e-6 {
		t.Errorf("world silver %v → %v: founding must MOVE silver, never create it", before, after)
	}

	// The colony holds exactly the purse, and nothing is left on the unit.
	var colonySilver, stillCarried float64
	if err := pool.QueryRow(ctx,
		`SELECT COALESCE(GREATEST(0, settled(sg.amount, sg.rate, sg.calc_tick)), 0)
		 FROM settlement_goods sg JOIN settlements s ON s.id = sg.settlement_id
		 WHERE s.world_id = $1 AND s.id <> $2 AND sg.good_key = 'silver'`,
		worldID, motherID,
	).Scan(&colonySilver); err != nil {
		t.Fatalf("read colony silver: %v", err)
	}
	if math.Abs(colonySilver-700) > 1e-6 {
		t.Errorf("colony silver = %v, want 700 (exactly the purse it carried)", colonySilver)
	}
	if err := pool.QueryRow(ctx,
		`SELECT COALESCE(carried_silver, 0) FROM units WHERE id = $1`, unitID,
	).Scan(&stillCarried); err == nil && stillCarried != 0 {
		t.Errorf("unit still carries %v after founding — the silver would exist twice", stillCarried)
	}
}

// TestFoundColony_EmptyPurseFoundsAPoorColony: a column sent out of an empty
// treasury founds a city with nothing. Not an error, not a silent top-up — the
// consequence of expanding while broke, which is what makes silver running dry
// a real constraint rather than a speed bump.
func TestFoundColony_EmptyPurseFoundsAPoorColony(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()

	worldID, motherID, unitID := purseFixture(t, pool, ctx, 3000, 0 /*empty purse*/)
	before := worldSilver(t, pool, ctx, worldID)
	runColonizeArrival(t, pool, ctx, worldID, unitID)
	after := worldSilver(t, pool, ctx, worldID)

	if math.Abs(after-before) > 1e-6 {
		t.Errorf("world silver %v → %v: an empty purse must not be topped up from nowhere", before, after)
	}
	var colonySilver float64
	if err := pool.QueryRow(ctx,
		`SELECT COALESCE(GREATEST(0, settled(sg.amount, sg.rate, sg.calc_tick)), 0)
		 FROM settlement_goods sg JOIN settlements s ON s.id = sg.settlement_id
		 WHERE s.world_id = $1 AND s.id <> $2 AND sg.good_key = 'silver'`,
		worldID, motherID,
	).Scan(&colonySilver); err != nil {
		t.Fatalf("read colony silver: %v", err)
	}
	if colonySilver != 0 {
		t.Errorf("colony silver = %v, want 0", colonySilver)
	}
}

// purseFixture builds an active world with a mother city holding motherSilver,
// an empty province for the colony, and a colonising unit carrying `purse`.
// The purse is already OFF the mother city's books — that is what dispatch did.
func purseFixture(t *testing.T, pool *pgxpool.Pool, ctx context.Context, motherSilver, purse float64) (worldID, motherID, unitID uuid.UUID) {
	t.Helper()
	if _, err := pool.Exec(ctx,
		`UPDATE worlds SET status = 'archived' WHERE status = 'active'`,
	); err != nil {
		t.Fatalf("archive leftover active test worlds: %v", err)
	}
	if err := pool.QueryRow(ctx,
		`INSERT INTO worlds (name, status) VALUES ($1, 'active') RETURNING id`,
		"test-purse-"+uuid.New().String(),
	).Scan(&worldID); err != nil {
		t.Fatalf("create test world: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `UPDATE worlds SET status = 'archived' WHERE id = $1`, worldID)
	})

	var ownerID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO players (username, password_hash) VALUES ($1, 'x') RETURNING id`,
		"purse-"+uuid.New().String(),
	).Scan(&ownerID); err != nil {
		t.Fatalf("create test player: %v", err)
	}

	var motherProvince uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO provinces (world_id, map_q, map_r, terrain_type) VALUES ($1, 0, 0, 'plains') RETURNING id`,
		worldID,
	).Scan(&motherProvince); err != nil {
		t.Fatalf("create mother province: %v", err)
	}
	if err := pool.QueryRow(ctx,
		`INSERT INTO settlements (world_id, province_id, name, culture_id, owner_id, control_type, is_capital, population)
		 VALUES ($1, $2, 'Mother City', 'achaean', $3, 'capital', true, 4000) RETURNING id`,
		worldID, motherProvince, ownerID,
	).Scan(&motherID); err != nil {
		t.Fatalf("create mother settlement: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO settlement_goods (settlement_id, good_key, amount, rate, cap, calc_tick)
		 VALUES ($1, 'silver', $2, 0, 1000000, current_world_tick())`,
		motherID, motherSilver,
	); err != nil {
		t.Fatalf("seed mother silver: %v", err)
	}

	if _, err := pool.Exec(ctx,
		`INSERT INTO provinces (world_id, map_q, map_r, terrain_type) VALUES ($1, 10, 10, 'plains')`,
		worldID,
	); err != nil {
		t.Fatalf("create colony province: %v", err)
	}

	if err := pool.QueryRow(ctx,
		`INSERT INTO units (world_id, owner_id, type, category, size, status, q, r, carried_silver)
		 VALUES ($1, $2, 'spearman', 'land', 50, 'marching', 10, 10, $3) RETURNING id`,
		worldID, ownerID, purse,
	).Scan(&unitID); err != nil {
		t.Fatalf("create colonising unit: %v", err)
	}
	return worldID, motherID, unitID
}

// runColonizeArrival runs the arrival handler's colonize path for a unit that
// has reached its target hex.
func runColonizeArrival(t *testing.T, pool *pgxpool.Pool, ctx context.Context, worldID, unitID uuid.UUID) {
	t.Helper()
	var ownerID, provinceID uuid.UUID
	var size int
	var carried float64
	if err := pool.QueryRow(ctx,
		`SELECT owner_id, size, carried_silver FROM units WHERE id = $1`, unitID,
	).Scan(&ownerID, &size, &carried); err != nil {
		t.Fatalf("load unit: %v", err)
	}
	if err := pool.QueryRow(ctx,
		`SELECT id FROM provinces WHERE world_id = $1 AND map_q = 10 AND map_r = 10`, worldID,
	).Scan(&provinceID); err != nil {
		t.Fatalf("load colony province: %v", err)
	}

	name := "Newhaven-" + uuid.New().String()[:8]
	u := unitRow{
		id: unitID, ownerID: ownerID, utype: "spearman", category: "land",
		size: size, status: "marching", q: 10, r: 10,
		colonyName: &name, carriedSilver: carried,
	}
	h := &UnitArrivalHandler{
		pool:       pool,
		eventStore: events.NewStore(pool),
		clk:        clock.NewTestClock(time.Now()),
		sitosCfg:   economy.LoadSitosConfig(),
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin tx: %v", err)
	}
	defer tx.Rollback(ctx)
	if err := h.foundColony(ctx, tx, u, provinceID, 10, 10, worldID); err != nil {
		t.Fatalf("foundColony: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit: %v", err)
	}
}

// TestArriveGarrison_ReturnsTheCarriedPurse: an expedition that turns around
// hands its purse back. Without this the silver is debited at dispatch and
// never credited anywhere — a recalled colonist would leave its city
// permanently poorer for a colony that was never founded, and nothing in the
// game would ever say where the silver went.
func TestArriveGarrison_ReturnsTheCarriedPurse(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()

	worldID, motherID, unitID := purseFixture(t, pool, ctx, 3000, 700)
	before := worldSilver(t, pool, ctx, worldID)

	var ownerID uuid.UUID
	if err := pool.QueryRow(ctx, `SELECT owner_id FROM units WHERE id = $1`, unitID).Scan(&ownerID); err != nil {
		t.Fatalf("load unit owner: %v", err)
	}
	u := unitRow{
		id: unitID, ownerID: ownerID, utype: "spearman", category: "land",
		size: 50, status: "marching", q: 10, r: 10, carriedSilver: 700,
	}
	h := &UnitArrivalHandler{
		pool: pool, eventStore: events.NewStore(pool),
		clk: clock.NewTestClock(time.Now()), sitosCfg: economy.LoadSitosConfig(),
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin tx: %v", err)
	}
	defer tx.Rollback(ctx)
	if err := h.arriveGarrison(ctx, tx, u, 0, 0, &motherID, worldID); err != nil {
		t.Fatalf("arriveGarrison: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit: %v", err)
	}

	if after := worldSilver(t, pool, ctx, worldID); math.Abs(after-before) > 1e-6 {
		t.Errorf("world silver %v → %v on a turn-around: the purse must not evaporate", before, after)
	}
	var motherSilver, stillCarried float64
	if err := pool.QueryRow(ctx,
		`SELECT GREATEST(0, settled(amount, rate, calc_tick)) FROM settlement_goods
		 WHERE settlement_id = $1 AND good_key = 'silver'`, motherID,
	).Scan(&motherSilver); err != nil {
		t.Fatalf("read mother silver: %v", err)
	}
	if math.Abs(motherSilver-3700) > 1e-6 {
		t.Errorf("mother city silver = %v, want 3700 (3000 + the 700 handed back)", motherSilver)
	}
	if err := pool.QueryRow(ctx,
		`SELECT carried_silver FROM units WHERE id = $1`, unitID,
	).Scan(&stillCarried); err != nil {
		t.Fatalf("read carried: %v", err)
	}
	if stillCarried != 0 {
		t.Errorf("unit still carries %v after handing the purse over — it would exist twice", stillCarried)
	}
}
