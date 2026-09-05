package handlers

// HTTP-level tests for §2 of megaron_plan_hexagarskap_och_stadsavstand.md:
// a hex bears a FIXED number of gubbar TOTAL, across every settlement whose
// catchment includes it — not per settlement. These fixtures deliberately
// place two settlements close enough that their catchments share a hex,
// bypassing province.SettlementCatchmentOverlap (the founding-time gate that
// forbids this in practice today, §1/§3 — untouched by this slice) by
// inserting the settlement rows directly, the same way the rest of this
// package's fixtures skip the founding HTTP flow entirely.

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"formatet/megaron/server/internal/auth"
	"formatet/megaron/server/internal/clock"
	"formatet/megaron/server/internal/economy"
	"formatet/megaron/server/internal/events"
	"formatet/megaron/server/internal/hexgrid"
	"formatet/megaron/server/internal/notify"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

// twoSettlementHexFixture is setupPlacementFixture's sibling for two
// settlements in the SAME world: A centred at (0,0), B centred at (3,0) —
// hex distance 3, well inside the 2×CatchmentRadius=4 span where the two
// radius-2 catchments (disks, hexgrid.Ring excludes only the centre) share
// ground. sharedHex is seeded once with sharedTerrain; each settlement's own
// centre tile is seeded separately so PlaceGubbe's "unknown hex"/FOW checks
// never fire for reasons unrelated to what's being tested.
type twoSettlementHexFixture struct {
	worldID      uuid.UUID
	provinceA    uuid.UUID
	provinceB    uuid.UUID
	settlementA  uuid.UUID
	settlementB  uuid.UUID
	nameA, nameB string
	tokenA       string
	tokenB       string
	router       *chi.Mux
}

func setupTwoSettlementHexFixture(t *testing.T, sharedTerrain string, sharedHex [2]int) *twoSettlementHexFixture {
	t.Helper()
	pool := p10TestPool(t)
	ctx := context.Background()

	if _, err := pool.Exec(ctx,
		`UPDATE worlds SET status = 'archived' WHERE status = 'active' AND name LIKE 'test-hexown-%'`,
	); err != nil {
		t.Fatalf("archive leftover test worlds: %v", err)
	}
	var worldID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO worlds (name, status, current_tick) VALUES ($1, 'active', 100) RETURNING id`,
		"test-hexown-"+uuid.New().String(),
	).Scan(&worldID); err != nil {
		t.Fatalf("create world: %v", err)
	}
	t.Cleanup(func() { _, _ = pool.Exec(ctx, `UPDATE worlds SET status = 'archived' WHERE id = $1`, worldID) })

	authSvc := auth.NewService(pool, "test-secret")

	registerPlayer := func(label string) (playerID uuid.UUID, token string) {
		username := "hexown-" + label + "-" + uuid.New().String()
		token, _, err := authSvc.Register(ctx, username, "x")
		if err != nil {
			t.Fatalf("register test player %s: %v", label, err)
		}
		claims, err := authSvc.ValidateAccessToken(token)
		if err != nil {
			t.Fatalf("validate token %s: %v", label, err)
		}
		return claims.PlayerID, token
	}
	playerA, tokenA := registerPlayer("a")
	playerB, tokenB := registerPlayer("b")

	seedTile := func(q, r int, terrain string) {
		coastal := terrain == "coastal_sea"
		if _, err := pool.Exec(ctx,
			`INSERT INTO map_tiles (world_id, q, r, terrain, coastal) VALUES ($1, $2, $3, $4, $5)
			 ON CONFLICT (world_id, q, r) DO NOTHING`,
			worldID, q, r, terrain, coastal,
		); err != nil {
			t.Fatalf("seed tile (%d,%d): %v", q, r, err)
		}
	}
	seedTile(0, 0, "plains") // A's own hex
	seedTile(3, 0, "plains") // B's own hex
	seedTile(sharedHex[0], sharedHex[1], sharedTerrain)

	makeSettlement := func(name string, playerID uuid.UUID, q, r int) (provinceID, settlementID uuid.UUID) {
		if err := pool.QueryRow(ctx,
			`INSERT INTO provinces (world_id, map_q, map_r, terrain_type) VALUES ($1, $2, $3, 'plains') RETURNING id`,
			worldID, q, r,
		).Scan(&provinceID); err != nil {
			t.Fatalf("create province %s: %v", name, err)
		}
		if err := pool.QueryRow(ctx,
			`INSERT INTO settlements (world_id, province_id, name, culture_id, owner_id, control_type, is_capital, population)
			 VALUES ($1, $2, $3, 'achaean', $4, 'capital', true, 1000) RETURNING id`,
			worldID, provinceID, name, playerID,
		).Scan(&settlementID); err != nil {
			t.Fatalf("create settlement %s: %v", name, err)
		}
		if err := economy.RecomputeProduction(ctx, pool, settlementID); err != nil {
			t.Fatalf("initial RecomputeProduction %s: %v", name, err)
		}
		return provinceID, settlementID
	}
	nameA, nameB := "Alpha", "Beta"
	provinceA, settlementA := makeSettlement(nameA, playerA, 0, 0)
	provinceB, settlementB := makeSettlement(nameB, playerB, 3, 0)

	clk := clock.NewTestClock(time.Now())
	scheduler := events.NewScheduler(pool, clk)
	eventStore := events.NewStore(pool)
	hub := notify.New()
	hub.SetPool(pool)
	ph := NewProvinceHandler(pool, scheduler, clk, economy.SitosConfig{}, eventStore, hub)

	r := chi.NewRouter()
	r.Use(auth.Middleware(authSvc))
	r.Get("/worlds/{worldID}/provinces/{provinceID}/placement-options", ph.PlacementOptions)
	r.Get("/worlds/{worldID}/provinces/{provinceID}/placements", ph.Placements)
	r.Post("/worlds/{worldID}/provinces/{provinceID}/placements", ph.PlaceGubbe)
	r.Delete("/worlds/{worldID}/provinces/{provinceID}/placements/{ordinal}", ph.UnplaceGubbe)

	return &twoSettlementHexFixture{
		worldID: worldID, provinceA: provinceA, provinceB: provinceB,
		settlementA: settlementA, settlementB: settlementB,
		nameA: nameA, nameB: nameB, tokenA: tokenA, tokenB: tokenB, router: r,
	}
}

func (f *twoSettlementHexFixture) placementsPath(provinceID uuid.UUID) string {
	return "/worlds/" + f.worldID.String() + "/provinces/" + provinceID.String() + "/placements"
}

// doAs mirrors placementFixture.do (settlement_placement_test.go) but takes
// an explicit bearer token — this fixture has two players sharing one
// router, so a fixed f.accessToken field (that struct's shape) doesn't fit.
func (f *twoSettlementHexFixture) doAs(t *testing.T, token, method, path string, body map[string]any) (int, map[string]any) {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			t.Fatalf("encode body: %v", err)
		}
	}
	req := httptest.NewRequest(method, path, &buf)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	f.router.ServeHTTP(rec, req)
	var resp map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	return rec.Code, resp
}

// TestPlaceGubbe_SharedHexIsCappedGloballyNotPerSettlement is the plan's
// core §1 bug made concrete: settlement A places 3 of the shared bare-plains
// hex's grain cap (4, hexCapacityRule{"grain",4,6,"farm"} with no farm — see
// internal/economy/recompute.go plainsCapacityRules), settlement B places 1
// (global count 3+1=4, still legal — B is not yet at ITS OWN cap of 4 rows
// either, so a settlement-scoped check would have wrongly allowed this too).
// B's SECOND attempt is where the two checks diverge: B's OWN count on this
// hex is only 1 (a settlement-scoped check would happily allow 3 more), but
// the GLOBAL count is already 4 = cap. A pre-§2 settlement-scoped check would
// let the settlement's own count grow to 4 independently of A's 3, giving 7
// gubbar total on one 4-gubbe hex — exactly "marken skördas två gånger."
func TestPlaceGubbe_SharedHexIsCappedGloballyNotPerSettlement(t *testing.T) {
	f := setupTwoSettlementHexFixture(t, "plains", [2]int{2, 0})

	for i := 0; i < 3; i++ {
		code, resp := f.doAs(t, f.tokenA, http.MethodPost, f.placementsPath(f.provinceA),
			map[string]any{"target_kind": "hex", "hex_q": 2, "hex_r": 0, "good_key": "grain"})
		if code != http.StatusCreated {
			t.Fatalf("A's placement #%d = %d: %v", i+1, code, resp)
		}
	}

	code, resp := f.doAs(t, f.tokenB, http.MethodPost, f.placementsPath(f.provinceB),
		map[string]any{"target_kind": "hex", "hex_q": 2, "hex_r": 0, "good_key": "grain"})
	if code != http.StatusCreated {
		t.Fatalf("B's first placement (global count 3 of cap 4) = %d: %v", code, resp)
	}

	code2, resp2 := f.doAs(t, f.tokenB, http.MethodPost, f.placementsPath(f.provinceB),
		map[string]any{"target_kind": "hex", "hex_q": 2, "hex_r": 0, "good_key": "grain"})
	if code2 != http.StatusConflict {
		t.Fatalf("B's second placement on a globally-full hex (A holds 3, B holds 1, cap 4) = %d: %v, want 409", code2, resp2)
	}
	msg, _ := resp2["error"].(string)
	if !strings.Contains(msg, f.nameA) {
		t.Errorf("conflict message should name the neighbouring settlement (%s) holding the hex, got %q", f.nameA, msg)
	}
}

// TestPlaceGubbe_SharedHexRejectionNamesHolderOnlyWhenForeign is the mirror
// case: a settlement fully staffing a hex ENTIRELY WITH ITS OWN gubbar keeps
// the plain, unnamed message — naming only fires when an occupant is truly
// someone else's (megaron_plan_hexagarskap_och_stadsavstand.md §5, and the
// ordinary single-city case that every existing placement test already
// covers must not change wording).
func TestPlaceGubbe_SharedHexRejectionNamesHolderOnlyWhenForeign(t *testing.T) {
	f := setupTwoSettlementHexFixture(t, "plains", [2]int{2, 0})

	var lastCode int
	var lastResp map[string]any
	for i := 0; i < 5; i++ {
		lastCode, lastResp = f.doAs(t, f.tokenA, http.MethodPost, f.placementsPath(f.provinceA),
			map[string]any{"target_kind": "hex", "hex_q": 2, "hex_r": 0, "good_key": "grain"})
		if lastCode != http.StatusCreated {
			break
		}
	}
	if lastCode != http.StatusConflict {
		t.Fatalf("A's 5th grain placement on its own fully-self-staffed hex = %d: %v, want 409", lastCode, lastResp)
	}
	msg, _ := lastResp["error"].(string)
	if msg != "this hex is fully staffed for that good" {
		t.Errorf("a hex filled entirely by the CALLER's own gubbar must keep the plain message, got %q", msg)
	}
}

// TestPlaceGubbe_ConcurrentRaceForSharedHexLastSlot is the plan's §5 race
// proof: A and B fire their placement for the shared hex's LAST slot (cap=1,
// coastal_sea with no harbour — hexCapacityRule{"fish",1,2,"harbour"})
// concurrently. Exactly one may succeed — the map_tiles row lock in
// PlaceGubbe's hex branch serializes the two transactions so the second one
// re-reads a post-commit occupancy count instead of racing on a stale read.
func TestPlaceGubbe_ConcurrentRaceForSharedHexLastSlot(t *testing.T) {
	f := setupTwoSettlementHexFixture(t, "coastal_sea", [2]int{2, 0})

	var wg sync.WaitGroup
	codes := make([]int, 2)
	resps := make([]map[string]any, 2)
	wg.Add(2)
	go func() {
		defer wg.Done()
		codes[0], resps[0] = f.doAs(t, f.tokenA, http.MethodPost, f.placementsPath(f.provinceA),
			map[string]any{"target_kind": "hex", "hex_q": 2, "hex_r": 0, "good_key": "fish"})
	}()
	go func() {
		defer wg.Done()
		codes[1], resps[1] = f.doAs(t, f.tokenB, http.MethodPost, f.placementsPath(f.provinceB),
			map[string]any{"target_kind": "hex", "hex_q": 2, "hex_r": 0, "good_key": "fish"})
	}()
	wg.Wait()

	successes := 0
	for _, c := range codes {
		if c == http.StatusCreated {
			successes++
		}
	}
	if successes != 1 {
		t.Fatalf("exactly one of two concurrent placements on a shared cap=1 hex must succeed, got codes=%v resps=%v", codes, resps)
	}

	// Confirm the DB agrees with the HTTP result: the hex's global occupancy
	// for fish must be exactly 1, never 2 — the invariant itself, not just
	// the two status codes, in case a bug let both rows insert but only one
	// HTTP response reported success.
	sharedHex := hexgrid.Coord{Q: 2, R: 0}
	occ, err := economy.GlobalHexOccupancy(context.Background(), p10TestPool(t), f.worldID, []hexgrid.Coord{sharedHex})
	if err != nil {
		t.Fatalf("GlobalHexOccupancy: %v", err)
	}
	if n := occ[sharedHex]["fish"]; n != 1 {
		t.Fatalf("global occupancy for fish on the shared hex after the race = %d, want exactly 1", n)
	}
}
