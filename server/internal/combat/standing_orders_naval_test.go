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
	"strings"
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

// shipAt inserts a garrison galley/merchantman at settlementID — sjöhandel
// kräver skepp (megaron_plan_sjohandel_kraver_skepp.md R4): since this slice,
// a standing sea route needs a real free hull in its from-settlement, not
// just a navigable sea lane.
func shipAt(t *testing.T, pool *pgxpool.Pool, worldID, owner, settlementID uuid.UUID, shipType string) uuid.UUID {
	t.Helper()
	var id uuid.UUID
	if err := pool.QueryRow(context.Background(),
		`INSERT INTO units (world_id, owner_id, type, category, size, crew, status, settlement_id)
		 VALUES ($1, $2, $3, 'naval', 1, 10, 'garrison', $4) RETURNING id`,
		worldID, owner, shipType, settlementID,
	).Scan(&id); err != nil {
		t.Fatalf("create ship %s at %s: %v", shipType, settlementID, err)
	}
	return id
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
	shipID := shipAt(t, pool, f.worldID, f.owner, f.capitalID, "merchantman")

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

	// R2/R4: the ship is bound (freighting) and saved onto the order.
	var shipStatus string
	if err := pool.QueryRow(context.Background(), `SELECT status FROM units WHERE id = $1`, shipID).Scan(&shipStatus); err != nil {
		t.Fatalf("read ship status: %v", err)
	}
	if shipStatus != "freighting" {
		t.Errorf("ship status = %q, want freighting", shipStatus)
	}
	var savedShipID *uuid.UUID
	if err := pool.QueryRow(context.Background(), `SELECT ship_unit_id FROM standing_orders WHERE id = $1`, orderID).Scan(&savedShipID); err != nil {
		t.Fatalf("read order ship_unit_id: %v", err)
	}
	if savedShipID == nil || *savedShipID != shipID {
		t.Errorf("order ship_unit_id = %v, want %s", savedShipID, shipID)
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

	// R1/R4: a merchantman's capacity (200 weight units) caps the manifest —
	// grain weighs 2.0/unit (goods.weight), so at most 100 of the 200 needed
	// can actually go this trip, not the full shortfall.
	var grainWeight float64
	if err := pool.QueryRow(context.Background(), `SELECT weight FROM goods WHERE key = 'grain'`).Scan(&grainWeight); err != nil {
		t.Fatalf("look up grain weight: %v", err)
	}
	shippedQty := 200.0
	if capped := 200.0 /* merchantman capacity */ / grainWeight; shippedQty > capped {
		shippedQty = capped
	}

	want := 1000.0 - shippedQty - provisions
	if got := settledAmount(t, pool, f.capitalID, "grain"); got != want {
		t.Errorf("capital grain after naval dispatch = %v, want %v (%v shipped [capped by ship capacity] + merchantman ration %v, not a gubbe's)",
			got, want, shippedQty, provisions)
	}
}

// 2. The return leg of a route that sailed out must also sail home — it is
// the same lane, and the plan (§2) covers both legs, not just the outbound.
func TestStandingOrder_ReturnLegAlsoGoesNaval(t *testing.T) {
	pool := testPool(t)
	f := newCoastalStandingOrderFixture(t, pool, "so-naval-return")
	seedGoods(t, pool, f.capitalID, f.tick, 1000, 0)
	shipID := shipAt(t, pool, f.worldID, f.owner, f.capitalID, "merchantman")

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

	// R4: the SAME ship carries the return leg, and stays freighting — the
	// route isn't over, it's just between legs.
	var shipStatus string
	if err := pool.QueryRow(context.Background(), `SELECT status FROM units WHERE id = $1`, shipID).Scan(&shipStatus); err != nil {
		t.Fatalf("read ship status: %v", err)
	}
	if shipStatus != "freighting" {
		t.Errorf("ship status after return dispatch = %q, want still freighting", shipStatus)
	}
}

// shipUnitStatus reads a unit's status + settlement_id.
func shipUnitStatus(t *testing.T, pool *pgxpool.Pool, id uuid.UUID) (status string, settlementID *uuid.UUID) {
	t.Helper()
	if err := pool.QueryRow(context.Background(),
		`SELECT status, settlement_id FROM units WHERE id = $1`, id,
	).Scan(&status, &settlementID); err != nil {
		t.Fatalf("read ship status: %v", err)
	}
	return status, settlementID
}

// 3a. R4 pause case 1: the ship is already idle, bound, sitting in the
// from-harbour (between legs) when the Wanax pauses the route — it must be
// released immediately (garrison, at the from-settlement), not left bound to
// a route nobody is running.
func TestStandingOrder_PauseReleasesIdleShipInHomePort(t *testing.T) {
	pool := testPool(t)
	f := newCoastalStandingOrderFixture(t, pool, "so-naval-pause-home")
	seedGoods(t, pool, f.capitalID, f.tick, 1000, 0)
	shipID := shipAt(t, pool, f.worldID, f.owner, f.capitalID, "merchantman")
	if _, err := pool.Exec(context.Background(), `UPDATE units SET status = 'freighting' WHERE id = $1`, shipID); err != nil {
		t.Fatalf("pre-bind ship: %v", err)
	}

	orderID := newStandingOrder(t, pool, f.worldID, f.owner, f.capitalID, f.townID, f.capitalID)
	addOutbound(t, pool, orderID, "grain", 200)
	if _, err := pool.Exec(context.Background(),
		`UPDATE standing_orders SET ship_unit_id = $2, status = 'paused', pause_reason = 'paused by Wanax' WHERE id = $1`,
		orderID, shipID,
	); err != nil {
		t.Fatalf("bind ship to order + pause: %v", err)
	}

	runStandingOrderTick(t, pool, f)

	status, settlementID := shipUnitStatus(t, pool, shipID)
	if status != "garrison" {
		t.Errorf("ship status after pause-idle sweep = %q, want garrison", status)
	}
	if settlementID == nil || *settlementID != f.capitalID {
		t.Errorf("ship settlement after release = %v, want home port %s", settlementID, f.capitalID)
	}
}

// 3b/3c. R4 pause cases 2 ("ute") and 3 ("i mål-hamnen"), chained: the Wanax
// pauses while the outbound leg has already landed at the destination
// (mirrors "ute" finishing its leg and reaching "i mål-hamnen") — the very
// next sweep must send the ship home EMPTY (not the normal floor-based return
// goods, since the route is shutting down), and once THAT leg lands the sweep
// must release the ship at its home port.
func TestStandingOrder_PauseAtDestinationSailsHomeEmptyThenReleases(t *testing.T) {
	pool := testPool(t)
	f := newCoastalStandingOrderFixture(t, pool, "so-naval-pause-dest")
	seedGoods(t, pool, f.capitalID, f.tick, 1000, 0)
	shipID := shipAt(t, pool, f.worldID, f.owner, f.capitalID, "merchantman")

	orderID := newStandingOrder(t, pool, f.worldID, f.owner, f.capitalID, f.townID, f.capitalID)
	addOutbound(t, pool, orderID, "grain", 50)
	addReturnGood(t, pool, orderID, "stone", 20)

	runStandingOrderTick(t, pool, f) // dispatch outbound leg (active)

	outboundID, kind, status := latestTransportForOrder(t, pool, orderID)
	if kind != "standing_order_out" || status != "in_transit" {
		t.Fatalf("latest transport = (%s, %s), want (standing_order_out, in_transit)", kind, status)
	}
	// Simulate the outbound leg's arrival at the destination — same
	// stand-in the other naval test uses (the generic ArrivalHandler is
	// untouched by this slice).
	if _, err := pool.Exec(context.Background(),
		`UPDATE transports SET status = 'delivered' WHERE id = $1`, outboundID,
	); err != nil {
		t.Fatalf("mark outbound delivered: %v", err)
	}
	// Plenty of surplus stone above the floor — if the route were still
	// active, the return leg would carry it. Paused, it must carry nothing.
	if _, err := pool.Exec(context.Background(),
		`INSERT INTO settlement_goods (settlement_id, good_key, amount, rate, cap, calc_tick)
		 VALUES ($1, 'stone', 500, 0, 1000000, $2)`,
		f.townID, f.tick,
	); err != nil {
		t.Fatalf("seed destination stone: %v", err)
	}

	// The Wanax pauses the route while the ship sits in the destination port.
	if _, err := pool.Exec(context.Background(),
		`UPDATE standing_orders SET status = 'paused', pause_reason = 'paused by Wanax' WHERE id = $1`, orderID,
	); err != nil {
		t.Fatalf("pause order: %v", err)
	}

	runStandingOrderTick(t, pool, f) // dispatch the (empty) return leg despite being paused

	returnID, kind, status := latestTransportForOrder(t, pool, orderID)
	if kind != "standing_order_return" || status != "in_transit" {
		t.Fatalf("latest transport = (%s, %s), want (standing_order_return, in_transit)", kind, status)
	}
	var stoneQty int
	if err := pool.QueryRow(context.Background(),
		`SELECT count(*) FROM transport_goods WHERE transport_id = $1`, returnID,
	).Scan(&stoneQty); err != nil {
		t.Fatalf("count return manifest: %v", err)
	}
	if stoneQty != 0 {
		t.Errorf("return manifest rows = %d, want 0 (paused route sails home empty)", stoneQty)
	}
	shipStatus, _ := shipUnitStatus(t, pool, shipID)
	if shipStatus != "freighting" {
		t.Errorf("ship status right after empty-return dispatch = %q, want still freighting (not home yet)", shipStatus)
	}

	// Simulate the return leg's arrival at home.
	if _, err := pool.Exec(context.Background(),
		`UPDATE transports SET status = 'delivered' WHERE id = $1`, returnID,
	); err != nil {
		t.Fatalf("mark return delivered: %v", err)
	}

	runStandingOrderTick(t, pool, f) // idle, paused — releases the ship

	finalStatus, settlementID := shipUnitStatus(t, pool, shipID)
	if finalStatus != "garrison" {
		t.Errorf("final ship status = %q, want garrison", finalStatus)
	}
	if settlementID == nil || *settlementID != f.capitalID {
		t.Errorf("final ship settlement = %v, want home port %s", settlementID, f.capitalID)
	}
}

// 4. R1/R4: with no free ship in the from-harbour and no land bridge between
// the two settlements (pure sea lane, like province_trade_naval_test.go's
// island case), the route pauses with an actionable reason instead of
// silently doing nothing or crashing.
func TestStandingOrder_PausesWithNoShipAndNoLandRoute(t *testing.T) {
	pool := testPool(t)
	f := newCoastalStandingOrderFixture(t, pool, "so-naval-no-ship")
	seedGoods(t, pool, f.capitalID, f.tick, 1000, 0)
	// No ship created — the harbour is empty.

	orderID := newStandingOrder(t, pool, f.worldID, f.owner, f.capitalID, f.townID, f.capitalID)
	addOutbound(t, pool, orderID, "grain", 200)

	runStandingOrderTick(t, pool, f)

	status, reason := orderStatus(t, pool, orderID)
	if status != "paused" {
		t.Fatalf("order status = %q, want paused (no ship, no land route)", status)
	}
	if reason == nil || !strings.Contains(*reason, "no free galley or merchantman") || !strings.Contains(*reason, "Byblos") {
		t.Errorf("pause reason = %v, want it to name Byblos and explain no free ship", reason)
	}
}
