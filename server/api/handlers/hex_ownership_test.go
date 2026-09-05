package handlers

// HTTP-level tests for §2/§2b of megaron_plan_hexagarskap_och_stadsavstand.md.
// §2b (Timothy 2026-09-05) supersedes §2's original per-good global cap: a
// hex has exactly ONE owning settlement — the first to place any gubbe on it,
// for any good — and every other settlement (even one under the same wanax)
// has no access to it at all, regardless of how much room that good's own
// cap has left. These fixtures deliberately place two settlements close
// enough that their catchments share a hex, bypassing
// province.SettlementCatchmentOverlap (the founding-time gate that forbids
// this in practice today, §1/§3 — untouched by this slice) by inserting the
// settlement rows directly, the same way the rest of this package's
// fixtures skip the founding HTTP flow entirely.

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
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

// TestPlaceGubbe_HexOwnershipBlocksNeighborEvenWithRoomLeft is §2b's core
// claim made concrete, and the exact case that distinguishes it from §2's
// original per-good global cap (ad2e042): settlement A places a SINGLE
// grain worker on the shared plains hex, leaving three of the hex's four
// grain slots (hexCapacityRule{"grain",4,6,"farm"} with no farm — see
// internal/economy/recompute.go plainsCapacityRules) completely empty. Under
// §2 (globalOccupancy[hex][good] >= cap) B's grain placement would have been
// allowed — 1 < 4. Under §2b it must be rejected on B's very FIRST attempt,
// because A already owns the hex outright, room or no room.
func TestPlaceGubbe_HexOwnershipBlocksNeighborEvenWithRoomLeft(t *testing.T) {
	f := setupTwoSettlementHexFixture(t, "plains", [2]int{2, 0})

	code, resp := f.doAs(t, f.tokenA, http.MethodPost, f.placementsPath(f.provinceA),
		map[string]any{"target_kind": "hex", "hex_q": 2, "hex_r": 0, "good_key": "grain"})
	if code != http.StatusCreated {
		t.Fatalf("A's placement = %d: %v", code, resp)
	}

	code2, resp2 := f.doAs(t, f.tokenB, http.MethodPost, f.placementsPath(f.provinceB),
		map[string]any{"target_kind": "hex", "hex_q": 2, "hex_r": 0, "good_key": "grain"})
	if code2 != http.StatusConflict {
		t.Fatalf("B's placement on a hex A already holds (only 1 of 4 grain slots taken) = %d: %v, want 409", code2, resp2)
	}
	msg, _ := resp2["error"].(string)
	if want := f.nameA + " holds this hex"; msg != want {
		t.Errorf("rejection text = %q, want exactly %q", msg, want)
	}
}

// TestPlaceGubbe_HexOwnershipBlocksNeighborAcrossDifferentGoods proves
// ownership is PER HEX, not per good: A takes the shared plains hex with a
// grain worker; B then tries LIVESTOCK — a completely independent good with
// its own untouched cap (plainsCapacityRules: {"livestock",1,3,""}) — on the
// very same hex. A per-good check (§2) would have happily allowed this,
// since B's own livestock count there is zero. §2b forbids it: the hex
// belongs to A, full stop, regardless of which good B wants.
func TestPlaceGubbe_HexOwnershipBlocksNeighborAcrossDifferentGoods(t *testing.T) {
	f := setupTwoSettlementHexFixture(t, "plains", [2]int{2, 0})

	code, resp := f.doAs(t, f.tokenA, http.MethodPost, f.placementsPath(f.provinceA),
		map[string]any{"target_kind": "hex", "hex_q": 2, "hex_r": 0, "good_key": "grain"})
	if code != http.StatusCreated {
		t.Fatalf("A's grain placement = %d: %v", code, resp)
	}

	code2, resp2 := f.doAs(t, f.tokenB, http.MethodPost, f.placementsPath(f.provinceB),
		map[string]any{"target_kind": "hex", "hex_q": 2, "hex_r": 0, "good_key": "livestock"})
	if code2 != http.StatusConflict {
		t.Fatalf("B's livestock placement on A's grain hex = %d: %v, want 409", code2, resp2)
	}
	msg, _ := resp2["error"].(string)
	if want := f.nameA + " holds this hex"; msg != want {
		t.Errorf("rejection text = %q, want exactly %q", msg, want)
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

// TestPlaceGubbe_ConcurrentRaceForHexOwnershipAcrossDifferentGoods is
// §2b's race proof: A and B fire their FIRST placement on a still-EMPTY
// shared hex concurrently, for two DIFFERENT goods (grain, livestock) that
// each have plenty of their own room (plainsCapacityRules: grain cap 4,
// livestock cap 1 — neither anywhere near contested by a single gubbe). A
// per-good cap check would let both succeed; ownership must not — exactly
// one may claim the hex, because the map_tiles row lock in PlaceGubbe's hex
// branch serializes the two transactions ahead of the ownership check, not
// just ahead of the per-good capacity check.
func TestPlaceGubbe_ConcurrentRaceForHexOwnershipAcrossDifferentGoods(t *testing.T) {
	f := setupTwoSettlementHexFixture(t, "plains", [2]int{2, 0})

	var wg sync.WaitGroup
	codes := make([]int, 2)
	resps := make([]map[string]any, 2)
	wg.Add(2)
	go func() {
		defer wg.Done()
		codes[0], resps[0] = f.doAs(t, f.tokenA, http.MethodPost, f.placementsPath(f.provinceA),
			map[string]any{"target_kind": "hex", "hex_q": 2, "hex_r": 0, "good_key": "grain"})
	}()
	go func() {
		defer wg.Done()
		codes[1], resps[1] = f.doAs(t, f.tokenB, http.MethodPost, f.placementsPath(f.provinceB),
			map[string]any{"target_kind": "hex", "hex_q": 2, "hex_r": 0, "good_key": "livestock"})
	}()
	wg.Wait()

	successes := 0
	for _, c := range codes {
		if c == http.StatusCreated {
			successes++
		}
	}
	if successes != 1 {
		t.Fatalf("exactly one of two concurrent placements for DIFFERENT goods on an unowned hex must succeed — ownership is per-hex, not per-good; got codes=%v resps=%v", codes, resps)
	}

	// Confirm the DB agrees: the hex must hold exactly one gubbe TOTAL across
	// both goods after the race, never one of each.
	sharedHex := hexgrid.Coord{Q: 2, R: 0}
	occ, err := economy.GlobalHexOccupancy(context.Background(), p10TestPool(t), f.worldID, []hexgrid.Coord{sharedHex})
	if err != nil {
		t.Fatalf("GlobalHexOccupancy: %v", err)
	}
	if total := occ[sharedHex]["grain"] + occ[sharedHex]["livestock"]; total != 1 {
		t.Fatalf("hex should hold exactly one gubbe total (grain+livestock) after the race, got %d", total)
	}
}
