package handlers

// DB integration tests for the coastal advantage (megaron_plan_tva_slices_
// 20260905.md §2): a transfer between two coastal-or-harboured settlements
// connected by a navigable sea lane must dispatch as "naval", not the
// hardcoded "land" every internal transfer used before. Real Postgres,
// gated by DATABASE_URL — same harness as province_trade_internal_test.go.
//
// Map convention (same as march_start_crew_speed_test.go): consecutive q at
// a fixed r are adjacent hexes.

import (
	"context"
	"testing"
	"time"

	"formatet/megaron/server/internal/auth"
	"formatet/megaron/server/internal/clock"
	"formatet/megaron/server/internal/economy"
	"formatet/megaron/server/internal/events"
	"formatet/megaron/server/internal/notify"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type tradeNavalFixture struct {
	pool      *pgxpool.Pool
	worldID   uuid.UUID
	playerID  uuid.UUID
	token     string
	router    *chi.Mux
	scheduler *events.Scheduler
	clk       clock.Clock
}

func setupTradeNavalFixture(t *testing.T) *tradeNavalFixture {
	t.Helper()
	pool := recruitShipTestPool(t)
	ctx := context.Background()

	if _, err := pool.Exec(ctx,
		`UPDATE worlds SET status = 'archived' WHERE status = 'active'`,
	); err != nil {
		t.Fatalf("archive leftover active test worlds: %v", err)
	}
	var worldID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO worlds (name, status) VALUES ($1, 'active') RETURNING id`,
		"test-world-"+uuid.New().String(),
	).Scan(&worldID); err != nil {
		t.Fatalf("create test world: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `UPDATE worlds SET status = 'archived' WHERE id = $1`, worldID)
	})

	authSvc := auth.NewService(pool, "test-secret")
	username := "sailor-" + uuid.New().String()
	token, _, err := authSvc.Register(ctx, username, "x")
	if err != nil {
		t.Fatalf("register test player: %v", err)
	}
	claims, err := authSvc.ValidateAccessToken(token)
	if err != nil {
		t.Fatalf("validate minted token: %v", err)
	}
	playerID := claims.PlayerID

	clk := clock.NewTestClock(time.Now())
	scheduler := events.NewScheduler(pool, clk)
	eventStore := events.NewStore(pool)
	hub := notify.New()
	hub.SetPool(pool)
	ph := NewProvinceHandler(pool, scheduler, clk, economy.SitosConfig{}, eventStore, hub)

	r := chi.NewRouter()
	r.Use(auth.Middleware(authSvc))
	r.Post("/worlds/{worldID}/provinces/{provinceID}/trade", ph.Trade)

	return &tradeNavalFixture{
		pool: pool, worldID: worldID, playerID: playerID, token: token,
		router: r, scheduler: scheduler, clk: clk,
	}
}

func (f *tradeNavalFixture) post(t *testing.T, path string, body any) (int, map[string]any) {
	t.Helper()
	rf := &recruitShipFixture{accessToken: f.token, router: f.router}
	rec, resp := rf.post(t, path, body)
	return rec.Code, resp
}

// mapTile inserts one map_tiles row.
func (f *tradeNavalFixture) mapTile(t *testing.T, q, r int, terrain string) {
	t.Helper()
	if _, err := f.pool.Exec(context.Background(),
		`INSERT INTO map_tiles (world_id, q, r, terrain) VALUES ($1, $2, $3, $4)`,
		f.worldID, q, r, terrain,
	); err != nil {
		t.Fatalf("insert map tile (%d,%d)=%s: %v", q, r, terrain, err)
	}
}

// settlement creates one settlement owned by f.playerID at (q,0), with its
// province.coastal flag set as given, and 1000 grain seeded so a transfer
// has something to send. Also inserts the settlement's own map_tiles hex as
// plains, matching mapTile's convention.
func (f *tradeNavalFixture) settlement(t *testing.T, name string, q int, coastal, capital bool) (settlementID, provinceID uuid.UUID) {
	t.Helper()
	ctx := context.Background()
	f.mapTile(t, q, 0, "plains")
	if err := f.pool.QueryRow(ctx,
		`INSERT INTO provinces (world_id, map_q, map_r, terrain_type, coastal) VALUES ($1, $2, 0, 'plains', $3) RETURNING id`,
		f.worldID, q, coastal,
	).Scan(&provinceID); err != nil {
		t.Fatalf("create province %s: %v", name, err)
	}
	if err := f.pool.QueryRow(ctx,
		`INSERT INTO settlements (world_id, province_id, name, culture_id, owner_id, control_type, is_capital, state, population)
		 VALUES ($1, $2, $3, 'achaean', $4, 'capital', $5, 'active', 5000) RETURNING id`,
		f.worldID, provinceID, name, f.playerID, capital,
	).Scan(&settlementID); err != nil {
		t.Fatalf("create settlement %s: %v", name, err)
	}
	if _, err := f.pool.Exec(ctx,
		`INSERT INTO settlement_goods (settlement_id, good_key, amount, rate, cap, calc_tick)
		 VALUES ($1, 'grain', 100000, 0, 1000000, 0)`,
		settlementID,
	); err != nil {
		t.Fatalf("seed %s grain: %v", name, err)
	}
	return settlementID, provinceID
}

func (f *tradeNavalFixture) transportCategory(t *testing.T, worldID, originID, destID uuid.UUID) string {
	t.Helper()
	var category string
	if err := f.pool.QueryRow(context.Background(),
		`SELECT category FROM transports WHERE world_id = $1 AND origin_id = $2 AND dest_id = $3
		 ORDER BY created_at DESC LIMIT 1`,
		worldID, originID, destID,
	).Scan(&category); err != nil {
		t.Fatalf("no transports row for %s -> %s: %v", originID, destID, err)
	}
	return category
}

// TestTrade_TwoCoastalSettlementsWithSeaRouteGoNaval is the slice's primary
// contract: both ends coastal, a navigable sea lane of coastal_sea hexes
// connects their shores (q=1..4 between the two settlement hexes q=0 and
// q=5) — the caravan must dispatch category "naval".
func TestTrade_TwoCoastalSettlementsWithSeaRouteGoNaval(t *testing.T) {
	f := setupTradeNavalFixture(t)

	originID, originProvince := f.settlement(t, "Byblos", 0, true, true)
	destID, _ := f.settlement(t, "Ugarit", 5, true, false)
	for q := 1; q <= 4; q++ {
		f.mapTile(t, q, 0, "coastal_sea")
	}

	code, resp := f.post(t, "/worlds/"+f.worldID.String()+"/provinces/"+originProvince.String()+"/trade",
		map[string]any{"destination_id": destID.String(), "good_key": "grain", "quantity": 10.0})
	if code != 201 {
		t.Fatalf("trade returned %d: %v", code, resp)
	}

	got := f.transportCategory(t, f.worldID, originID, destID)
	if got != "naval" {
		t.Fatalf("category = %q, want naval (two coastal settlements with a navigable sea lane)", got)
	}
}

// TestTrade_TwoInlandSettlementsGoLand is the negative control: neither end
// coastal — no sea route is even attempted, category stays "land" exactly
// as before this slice.
func TestTrade_TwoInlandSettlementsGoLand(t *testing.T) {
	f := setupTradeNavalFixture(t)

	originID, originProvince := f.settlement(t, "Mycenae", 0, false, true)
	destID, _ := f.settlement(t, "Tiryns", 3, false, false)
	// Plain land hexes in between — no sea anywhere on this map.
	f.mapTile(t, 1, 0, "plains")
	f.mapTile(t, 2, 0, "plains")

	code, resp := f.post(t, "/worlds/"+f.worldID.String()+"/provinces/"+originProvince.String()+"/trade",
		map[string]any{"destination_id": destID.String(), "good_key": "grain", "quantity": 10.0})
	if code != 201 {
		t.Fatalf("trade returned %d: %v", code, resp)
	}

	got := f.transportCategory(t, f.worldID, originID, destID)
	if got != "land" {
		t.Fatalf("category = %q, want land (neither settlement is coastal)", got)
	}

	// Proof that an unchanged land route costs EXACTLY what it did before
	// naval selection existed: hex distance 0->3 is 3, so base = 30 + 3*2 =
	// 36 minutes — the pre-existing weight penalty for whatever "grain"
	// weighs in the goods table (untouched by this slice) is derived from
	// live data here, not guessed, so this doesn't hardcode a value this
	// slice never changed in the first place.
	var grainWeight float64
	if err := f.pool.QueryRow(context.Background(),
		`SELECT weight FROM goods WHERE key = 'grain'`,
	).Scan(&grainWeight); err != nil {
		t.Fatalf("look up grain weight: %v", err)
	}
	wantWeightPenalty := 0.0
	if grainWeight > 1.0 {
		wantWeightPenalty = (grainWeight - 1.0) * 0.1
	}
	want := 36.0 * (1.0 + wantWeightPenalty)
	if travelMin, _ := resp["travel_min"].(float64); travelMin != want {
		t.Fatalf("travel_min = %v, want exactly %v (30 + 3*2, the pre-existing land formula, unchanged)", travelMin, want)
	}
}

// TestTrade_OneCoastalOneInlandGoesLand: only one end can reach the sea, so
// no sea route can ever connect them — must fall back to land even though a
// sea lane exists off the coastal settlement's own shore.
func TestTrade_OneCoastalOneInlandGoesLand(t *testing.T) {
	f := setupTradeNavalFixture(t)

	originID, originProvince := f.settlement(t, "Pylos", 0, true, true)
	destID, _ := f.settlement(t, "Sparta", 5, false, false)
	for q := 1; q <= 4; q++ {
		f.mapTile(t, q, 0, "coastal_sea")
	}

	code, resp := f.post(t, "/worlds/"+f.worldID.String()+"/provinces/"+originProvince.String()+"/trade",
		map[string]any{"destination_id": destID.String(), "good_key": "grain", "quantity": 10.0})
	if code != 201 {
		t.Fatalf("trade returned %d: %v", code, resp)
	}

	got := f.transportCategory(t, f.worldID, originID, destID)
	if got != "land" {
		t.Fatalf("category = %q, want land (destination is inland, no harbour)", got)
	}
}

// TestTrade_NavalRouteTravelTimeFollowsSeaPathNotStraightLine proves the
// second success criterion: a sea route that must sail AROUND a headland is
// LONGER than the straight-line hex count between the two settlements, and
// the dispatched ETA must reflect that real path, not the land formula's
// straight-line distance.
//
// Map: origin (0,0) and destination (0,3) are 3 hexes apart in a straight
// line, but the only sea lane between their shores bends out and back
// through (1,0)->(2,0)->(3,0)->(3,1)->(3,2)->(2,3)->(1,3), a 6-hex route —
// landDist=3 would under-time the voyage if naval blindly reused it.
func TestTrade_NavalRouteTravelTimeFollowsSeaPathNotStraightLine(t *testing.T) {
	f := setupTradeNavalFixture(t)
	ctx := context.Background()

	// Origin at (0,0), destination at (0,3) — straight-line hex distance 3.
	// The only sea lane bends through a detour of 5 hexes (1,0)->(2,0)->
	// (3,0)->(3,1)->(3,2)->(2,3), strictly longer than landDist.
	f.mapTile(t, 0, 0, "plains")
	var originProvince, originID uuid.UUID
	if err := f.pool.QueryRow(ctx,
		`INSERT INTO provinces (world_id, map_q, map_r, terrain_type, coastal) VALUES ($1,0,0,'plains',true) RETURNING id`,
		f.worldID,
	).Scan(&originProvince); err != nil {
		t.Fatalf("create origin province: %v", err)
	}
	if err := f.pool.QueryRow(ctx,
		`INSERT INTO settlements (world_id, province_id, name, culture_id, owner_id, control_type, is_capital, state, population)
		 VALUES ($1,$2,'Knossos','achaean',$3,'capital',true,'active',5000) RETURNING id`,
		f.worldID, originProvince, f.playerID,
	).Scan(&originID); err != nil {
		t.Fatalf("create origin settlement: %v", err)
	}
	if _, err := f.pool.Exec(ctx,
		`INSERT INTO settlement_goods (settlement_id, good_key, amount, rate, cap, calc_tick) VALUES ($1,'grain',100000,0,1000000,0)`,
		originID,
	); err != nil {
		t.Fatalf("seed origin grain: %v", err)
	}

	f.mapTile(t, 0, 3, "plains")
	var destProvince, destID uuid.UUID
	if err := f.pool.QueryRow(ctx,
		`INSERT INTO provinces (world_id, map_q, map_r, terrain_type, coastal) VALUES ($1,0,3,'plains',true) RETURNING id`,
		f.worldID,
	).Scan(&destProvince); err != nil {
		t.Fatalf("create dest province: %v", err)
	}
	if err := f.pool.QueryRow(ctx,
		`INSERT INTO settlements (world_id, province_id, name, culture_id, owner_id, control_type, is_capital, state, population)
		 VALUES ($1,$2,'Phaistos','achaean',$3,'capital',false,'active',5000) RETURNING id`,
		f.worldID, destProvince, f.playerID,
	).Scan(&destID); err != nil {
		t.Fatalf("create dest settlement: %v", err)
	}
	if _, err := f.pool.Exec(ctx,
		`INSERT INTO settlement_goods (settlement_id, good_key, amount, rate, cap, calc_tick) VALUES ($1,'grain',100000,0,1000000,0)`,
		destID,
	); err != nil {
		t.Fatalf("seed dest grain: %v", err)
	}

	// The only sea tiles on this map form ONE chain, bending out and back:
	// (1,0)->(2,0)->(3,0)->(3,1)->(3,2)->(2,3)->(1,3) — 6 hops (dist 6) to
	// connect the same two shores a straight line would cross in 3. Since no
	// other sea tile exists anywhere else on the map, A* has no shortcut —
	// the resolved naval distance can only come out longer than landDist.
	f.mapTile(t, 1, 0, "coastal_sea")
	f.mapTile(t, 2, 0, "coastal_sea")
	f.mapTile(t, 3, 0, "coastal_sea")
	f.mapTile(t, 3, 1, "coastal_sea")
	f.mapTile(t, 3, 2, "coastal_sea")
	f.mapTile(t, 2, 3, "coastal_sea")
	f.mapTile(t, 1, 3, "coastal_sea")

	code, resp := f.post(t, "/worlds/"+f.worldID.String()+"/provinces/"+originProvince.String()+"/trade",
		map[string]any{"destination_id": destID.String(), "good_key": "grain", "quantity": 10.0})
	if code != 201 {
		t.Fatalf("trade returned %d: %v", code, resp)
	}

	got := f.transportCategory(t, f.worldID, originID, destID)
	if got != "naval" {
		t.Fatalf("category = %q, want naval", got)
	}

	// landDist=3 -> land formula would give 30+3*2=36 min. The real sea path
	// is longer, so travel_min in the response must exceed that.
	travelMin, _ := resp["travel_min"].(float64)
	if travelMin <= 36.0 {
		t.Fatalf("travel_min = %v, want > 36 (must reflect the real bending sea path, not the 3-hex straight line)", travelMin)
	}
}
