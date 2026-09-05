package combat

// Proof for megaron_plan_tva_slices_20260905.md §2's standing-order half of
// the contract: "de stående leveranserna ska också välja naval, inte bara
// keryx transfer." api/handlers/province_trade_naval_test.go already proves
// the Trade handler (keryx transfer) picks naval; these tests prove the
// standing-order sweep (internal/combat/standing_orders.go) does the same for
// BOTH legs of a route, and prices the naval leg as the plan's §4 point 3
// says — the ship's own flat hull ration (UpkeepSpecs["merchantman"].Grain),
// never a gubbe.
//
// Map convention matches province_trade_naval_test.go: settlements at (0,0)
// and (5,0), a chain of coastal_sea tiles at q=1..4 connects their shores.

import (
	"context"
	"math"
	"testing"

	"formatet/megaron/server/internal/province"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// newCoastalStandingOrderFixture builds a world with two settlements, both
// coastal (provinces.coastal = true) and connected by a navigable chain of
// coastal_sea map_tiles — the same "coastal-or-harboured AND a real sea lane"
// gate province.ResolveTradeRoute checks. Returned as a supportFixture so the
// existing newStandingOrder/addOutbound/runStandingOrderTick helpers
// (standing_orders_test.go) work unchanged — those only ever read
// f.worldID/f.tick, plus the two settlement ids this function fills in.
func newCoastalStandingOrderFixture(t *testing.T, pool *pgxpool.Pool, tag string) supportFixture {
	t.Helper()
	ctx := context.Background()
	const tick = 3000

	if _, err := pool.Exec(ctx,
		`UPDATE worlds SET status = 'archived' WHERE status = 'active'`,
	); err != nil {
		t.Fatalf("archive leftover active test worlds: %v", err)
	}
	f := supportFixture{tick: tick}
	if err := pool.QueryRow(ctx,
		`INSERT INTO worlds (name, status, current_tick) VALUES ($1, 'active', $2) RETURNING id`,
		"test-"+tag+"-"+uuid.New().String(), tick,
	).Scan(&f.worldID); err != nil {
		t.Fatalf("create world: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `UPDATE worlds SET status = 'archived' WHERE id = $1`, f.worldID)
	})

	if err := pool.QueryRow(ctx,
		`INSERT INTO players (username, password_hash) VALUES ($1, 'x') RETURNING id`,
		tag+"-"+uuid.New().String(),
	).Scan(&f.owner); err != nil {
		t.Fatalf("create player: %v", err)
	}

	mapTile := func(q, r int, terrain string) {
		if _, err := pool.Exec(ctx,
			`INSERT INTO map_tiles (world_id, q, r, terrain) VALUES ($1, $2, $3, $4)`,
			f.worldID, q, r, terrain,
		); err != nil {
			t.Fatalf("insert map tile (%d,%d)=%s: %v", q, r, terrain, err)
		}
	}

	mkCoastal := func(q int, name string, capital bool) uuid.UUID {
		mapTile(q, 0, "plains")
		var prov, sid uuid.UUID
		if err := pool.QueryRow(ctx,
			`INSERT INTO provinces (world_id, map_q, map_r, terrain_type, coastal) VALUES ($1, $2, 0, 'plains', true) RETURNING id`,
			f.worldID, q,
		).Scan(&prov); err != nil {
			t.Fatalf("create province: %v", err)
		}
		ctype := "colony"
		if capital {
			ctype = "capital"
		}
		if err := pool.QueryRow(ctx,
			`INSERT INTO settlements (world_id, province_id, name, culture_id, owner_id, control_type, is_capital, state, population)
			 VALUES ($1, $2, $3, 'achaean', $4, $5, $6, 'active', 1000) RETURNING id`,
			f.worldID, prov, name, f.owner, ctype, capital,
		).Scan(&sid); err != nil {
			t.Fatalf("create settlement %s: %v", name, err)
		}
		return sid
	}
	f.capitalID = mkCoastal(0, "Byblos", true)
	f.townID = mkCoastal(5, "Ugarit", false)
	for q := 1; q <= 4; q++ {
		mapTile(q, 0, "coastal_sea")
	}
	return f
}

// transportCategoryForOrder reads the category of the most recent transport
// dispatched for a standing order — the field the whole slice is about.
func transportCategoryForOrder(t *testing.T, pool *pgxpool.Pool, orderID uuid.UUID) string {
	t.Helper()
	var category string
	if err := pool.QueryRow(context.Background(),
		`SELECT category FROM transports WHERE standing_order_id = $1 ORDER BY created_at DESC LIMIT 1`,
		orderID,
	).Scan(&category); err != nil {
		t.Fatalf("read transport category: %v", err)
	}
	return category
}

// 1. Both ends coastal + a navigable sea lane between them: the outbound leg
// must dispatch "naval", not the land default, and it must NOT require an
// idle gubbe at all — the plan is explicit a naval leg needs no owned ship,
// so placing every gubbe at the crewing settlement must not pause the order.
func TestStandingOrder_OutboundGoesNavalWhenBothEndsCoastal(t *testing.T) {
	pool := testPool(t)
	f := newCoastalStandingOrderFixture(t, pool, "so-naval-out")
	seedGoods(t, pool, f.capitalID, f.tick, 1000, 0)
	// Every gubbe placed — a land route would pause here (see
	// TestStandingOrder_PausesWhenCrewHasNoIdleGubbe); a naval route must not.
	placeGubbar(t, pool, f.capitalID, 10)

	orderID := newStandingOrder(t, pool, f.worldID, f.owner, f.capitalID, f.townID, f.capitalID)
	addOutbound(t, pool, orderID, "grain", 200)

	runStandingOrderTick(t, pool, f)

	status, reason := orderStatus(t, pool, orderID)
	if status != "active" {
		t.Fatalf("order status = %q (reason=%v), want active — a naval leg needs no idle gubbe", status, reason)
	}
	if got := transportCategoryForOrder(t, pool, orderID); got != "naval" {
		t.Fatalf("category = %q, want naval (both ends coastal, sea lane exists)", got)
	}

	// Naval pays the merchantman's flat hull ration (UpkeepSpecs), not a
	// gubbe's. Derive the expected provisions from the same production
	// functions the handler itself calls, rather than a hardcoded float, so
	// this test can't silently drift from the real formula.
	_, dist, err := province.ResolveTradeRoute(context.Background(), pool, f.worldID, true, true,
		province.MapPosition{Q: 0, R: 0}, province.MapPosition{Q: 5, R: 0})
	if err != nil {
		t.Fatalf("resolve trade route for expected-value math: %v", err)
	}
	travelMins := 30.0 + float64(dist)*2.0
	travelTicks := int(math.Round(travelMins / 60))
	if travelTicks < 1 {
		travelTicks = 1
	}
	provisions := VoyageProvisions(standingOrderNavalRation(), travelTicks, 0)
	want := 1000.0 - 200.0 - provisions
	if got := settledAmount(t, pool, f.capitalID, "grain"); got != want {
		t.Errorf("capital grain after naval dispatch = %v, want %v (200 shipped + merchantman ration %v, not a gubbe's)", got, want, provisions)
	}
}

// 2. The return leg of a route that sailed out must also sail home — it is
// the same lane, and the plan (§2) covers both legs, not just the outbound.
func TestStandingOrder_ReturnLegAlsoGoesNaval(t *testing.T) {
	pool := testPool(t)
	f := newCoastalStandingOrderFixture(t, pool, "so-naval-return")
	seedGoods(t, pool, f.capitalID, f.tick, 1000, 0)

	orderID := newStandingOrder(t, pool, f.worldID, f.owner, f.capitalID, f.townID, f.capitalID)
	addOutbound(t, pool, orderID, "grain", 200)
	addReturnGood(t, pool, orderID, "stone", 20)

	runStandingOrderTick(t, pool, f) // dispatch outbound leg

	outboundID, kind, status := latestTransportForOrder(t, pool, orderID)
	if kind != "standing_order_out" || status != "in_transit" {
		t.Fatalf("latest transport = (%s, %s), want (standing_order_out, in_transit)", kind, status)
	}
	if got := transportCategoryForOrder(t, pool, orderID); got != "naval" {
		t.Fatalf("outbound category = %q, want naval", got)
	}

	// Stand in for the generic ArrivalHandler (untouched by this slice).
	if _, err := pool.Exec(context.Background(),
		`UPDATE transports SET status = 'delivered' WHERE id = $1`, outboundID,
	); err != nil {
		t.Fatalf("mark outbound delivered: %v", err)
	}
	if _, err := pool.Exec(context.Background(),
		`INSERT INTO settlement_goods (settlement_id, good_key, amount, rate, cap, calc_tick)
		 VALUES ($1, 'stone', 50, 0, 1000000, $2)`,
		f.townID, f.tick,
	); err != nil {
		t.Fatalf("seed destination stone: %v", err)
	}

	runStandingOrderTick(t, pool, f) // dispatch the return leg

	returnID, kind, status := latestTransportForOrder(t, pool, orderID)
	if kind != "standing_order_return" || status != "in_transit" {
		t.Fatalf("latest transport = (%s, %s), want (standing_order_return, in_transit)", kind, status)
	}
	if got := transportCategoryForOrder(t, pool, orderID); got != "naval" {
		t.Errorf("return leg category = %q, want naval — the caravan sailed out, it must sail home", got)
	}
	_ = returnID
}
