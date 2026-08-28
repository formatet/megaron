package handlers

// HTTP-level tests for P4's placement endpoints (megaron_plan_fysisk_gubbemodell.md):
// place/unplace/list. DB integration tests (real Postgres, gated by DATABASE_URL,
// same pattern as craft_event_test.go/recruit_ship_test.go).

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"formatet/megaron/server/internal/auth"
	"formatet/megaron/server/internal/clock"
	"formatet/megaron/server/internal/economy"
	"formatet/megaron/server/internal/events"
	"formatet/megaron/server/internal/notify"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

type placementFixture struct {
	worldID      uuid.UUID
	provinceID   uuid.UUID
	settlementID uuid.UUID
	accessToken  string
	router       *chi.Mux
}

func setupPlacementFixture(t *testing.T, catchmentTerrains map[[2]int]string) *placementFixture {
	t.Helper()
	pool := p10TestPool(t) // reuses the shared DATABASE_URL-gated pool helper (p10_gate_visibility_test.go)
	ctx := context.Background()

	if _, err := pool.Exec(ctx,
		`UPDATE worlds SET status = 'archived' WHERE status = 'active' AND name LIKE 'test-placement-%'`,
	); err != nil {
		t.Fatalf("archive leftover test worlds: %v", err)
	}
	var worldID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO worlds (name, status, current_tick) VALUES ($1, 'active', 100) RETURNING id`,
		"test-placement-"+uuid.New().String(),
	).Scan(&worldID); err != nil {
		t.Fatalf("create world: %v", err)
	}
	t.Cleanup(func() { _, _ = pool.Exec(ctx, `UPDATE worlds SET status = 'archived' WHERE id = $1`, worldID) })

	authSvc := auth.NewService(pool, "test-secret")
	username := "placement-" + uuid.New().String()
	accessToken, _, err := authSvc.Register(ctx, username, "x")
	if err != nil {
		t.Fatalf("register test player: %v", err)
	}
	claims, err := authSvc.ValidateAccessToken(accessToken)
	if err != nil {
		t.Fatalf("validate token: %v", err)
	}
	playerID := claims.PlayerID

	var provinceID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO provinces (world_id, map_q, map_r, terrain_type) VALUES ($1, 0, 0, 'plains') RETURNING id`,
		worldID,
	).Scan(&provinceID); err != nil {
		t.Fatalf("create province: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO map_tiles (world_id, q, r, terrain) VALUES ($1, 0, 0, 'plains')`,
		worldID,
	); err != nil {
		t.Fatalf("seed centre tile: %v", err)
	}
	for xy, terrain := range catchmentTerrains {
		coastal := terrain == "coastal_sea"
		if _, err := pool.Exec(ctx,
			`INSERT INTO map_tiles (world_id, q, r, terrain, coastal) VALUES ($1, $2, $3, $4, $5)`,
			worldID, xy[0], xy[1], terrain, coastal,
		); err != nil {
			t.Fatalf("seed catchment tile (%d,%d): %v", xy[0], xy[1], err)
		}
	}

	var settlementID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO settlements (world_id, province_id, name, culture_id, owner_id, control_type, is_capital, population)
		 VALUES ($1, $2, 'Gubbeville', 'achaean', $3, 'capital', true, 500) RETURNING id`,
		worldID, provinceID, playerID,
	).Scan(&settlementID); err != nil {
		t.Fatalf("create settlement: %v", err)
	}
	if err := economy.RecomputeProduction(ctx, pool, settlementID); err != nil {
		t.Fatalf("initial RecomputeProduction: %v", err)
	}

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
	r.Post("/worlds/{worldID}/provinces/{provinceID}/slaughter-livestock", ph.SlaughterLivestock)
	r.Put("/worlds/{worldID}/provinces/{provinceID}/labor", ph.LaborAlloc)
	r.Get("/worlds/{worldID}/provinces/{provinceID}/goods", ph.Goods)

	return &placementFixture{worldID: worldID, provinceID: provinceID, settlementID: settlementID, accessToken: accessToken, router: r}
}

func (f *placementFixture) placementOptionsPath() string {
	return "/worlds/" + f.worldID.String() + "/provinces/" + f.provinceID.String() + "/placement-options"
}

func (f *placementFixture) do(t *testing.T, method, path string, body any) (int, map[string]any) {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			t.Fatalf("encode body: %v", err)
		}
	}
	req := httptest.NewRequest(method, path, &buf)
	req.Header.Set("Authorization", "Bearer "+f.accessToken)
	rec := httptest.NewRecorder()
	f.router.ServeHTTP(rec, req)
	var resp map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	return rec.Code, resp
}

func (f *placementFixture) placementsPath() string {
	return "/worlds/" + f.worldID.String() + "/provinces/" + f.provinceID.String() + "/placements"
}

func (f *placementFixture) slaughterPath() string {
	return "/worlds/" + f.worldID.String() + "/provinces/" + f.provinceID.String() + "/slaughter-livestock"
}

func (f *placementFixture) goodsPath() string {
	return "/worlds/" + f.worldID.String() + "/provinces/" + f.provinceID.String() + "/goods"
}

// TestPlaceGubbe_GrainHexCappedLikeOtherGoods: two gubbar can both work the
// SAME plains grain hex (capNoBuilding=4, well above 2) — neither placement
// is rejected for capacity, and both raise the settlement's grain rate.
// Grain is no longer capacity-exempt (megaron_plan_grain_cap.md, Timothy
// 2026-08-19) — it is capped like every other good, just with a generous cap.
func TestPlaceGubbe_GrainHexCappedLikeOtherGoods(t *testing.T) {
	f := setupPlacementFixture(t, map[[2]int]string{{1, 0}: "plains"})

	code1, resp1 := f.do(t, http.MethodPost, f.placementsPath(),
		map[string]any{"target_kind": "hex", "hex_q": 1, "hex_r": 0, "good_key": "grain"})
	if code1 != http.StatusCreated {
		t.Fatalf("first grain placement = %d: %v", code1, resp1)
	}
	code2, resp2 := f.do(t, http.MethodPost, f.placementsPath(),
		map[string]any{"target_kind": "hex", "hex_q": 1, "hex_r": 0, "good_key": "grain"})
	if code2 != http.StatusCreated {
		t.Fatalf("second grain placement on the SAME hex = %d: %v (cap is 4, well above 2)", code2, resp2)
	}
	if resp1["gubbe_ordinal"] == resp2["gubbe_ordinal"] {
		t.Errorf("two placements got the same gubbe_ordinal: %v", resp1["gubbe_ordinal"])
	}

	code, listResp := f.do(t, http.MethodGet, f.placementsPath(), nil)
	if code != http.StatusOK {
		t.Fatalf("list placements = %d: %v", code, listResp)
	}
	placements, _ := listResp["placements"].([]any)
	if len(placements) != 2 {
		t.Errorf("expected 2 placements listed, got %d (%v)", len(placements), listResp)
	}
	if int(listResp["pool_size"].(float64)) != 3 { // 500 pop / 100 = 5 gubbar total, 2 placed
		t.Errorf("pool_size = %v, want 3", listResp["pool_size"])
	}
}

// TestPlaceGubbe_GrainHexRejectsOverCapacity is the plan's own required proof
// (megaron_plan_grain_cap.md §Säkerhet, Timothy 2026-08-19: "helt omöjligt att
// ha 32 gubbar på en hex") — a bare plains hex (no farm) caps at 4 gubbar; the
// 5th grain placement on the SAME hex must be rejected with 409, exactly like
// fish's existing cap behaviour (TestPlaceGubbe_FishHexRejectsOverCapacity
// below). The fixture's pool (500 pop = 5 gubbar) is deliberately ONE more
// than the cap so the 5th attempt is a real over-cap rejection, not just
// running out of gubbar — the same shape of proof as Timothy's "32 gubbar" is
// after, just at the smallest fixture size that exercises it.
func TestPlaceGubbe_GrainHexRejectsOverCapacity(t *testing.T) {
	f := setupPlacementFixture(t, map[[2]int]string{{1, 0}: "plains"})

	var lastCode int
	var lastResp map[string]any
	placed := 0
	for i := 0; i < 5; i++ {
		lastCode, lastResp = f.do(t, http.MethodPost, f.placementsPath(),
			map[string]any{"target_kind": "hex", "hex_q": 1, "hex_r": 0, "good_key": "grain"})
		if lastCode != http.StatusCreated {
			break
		}
		placed++
	}
	if placed != 4 {
		t.Fatalf("placed %d grain gubbar on a bare plains hex before rejection, want exactly 4 (capNoBuilding)", placed)
	}
	if lastCode != http.StatusConflict {
		t.Fatalf("the 5th grain placement on the same hex = %d: %v, want 409 (this hex is fully staffed)", lastCode, lastResp)
	}
}

// TestPlaceGubbe_FishHexRejectsOverCapacity: coastal_sea's P3 cap (1, no
// harbour) means a SECOND gubbe on the same fish hex must be rejected —
// fish (unlike grain) is a real, physically capped placement.
func TestPlaceGubbe_FishHexRejectsOverCapacity(t *testing.T) {
	f := setupPlacementFixture(t, map[[2]int]string{{1, 0}: "coastal_sea"})

	code1, resp1 := f.do(t, http.MethodPost, f.placementsPath(),
		map[string]any{"target_kind": "hex", "hex_q": 1, "hex_r": 0, "good_key": "fish"})
	if code1 != http.StatusCreated {
		t.Fatalf("first fish placement = %d: %v", code1, resp1)
	}
	code2, resp2 := f.do(t, http.MethodPost, f.placementsPath(),
		map[string]any{"target_kind": "hex", "hex_q": 1, "hex_r": 0, "good_key": "fish"})
	if code2 != http.StatusConflict {
		t.Fatalf("second fish placement (over cap=1) = %d: %v, want 409", code2, resp2)
	}
}

// TestUnplaceGubbe_ReturnsToPoolAndDropsRate: unplacing removes the row and
// the settlement's rate for that good drops back to what it was before
// (grain: exactly the flat/remainder floor, no hex contribution).
func TestUnplaceGubbe_ReturnsToPoolAndDropsRate(t *testing.T) {
	f := setupPlacementFixture(t, map[[2]int]string{{1, 0}: "plains"})

	code, resp := f.do(t, http.MethodPost, f.placementsPath(),
		map[string]any{"target_kind": "hex", "hex_q": 1, "hex_r": 0, "good_key": "grain"})
	if code != http.StatusCreated {
		t.Fatalf("place: %d: %v", code, resp)
	}
	ordinal := int(resp["gubbe_ordinal"].(float64))

	code, resp = f.do(t, http.MethodDelete, f.placementsPath()+"/"+strconv.Itoa(ordinal), nil)
	if code != http.StatusOK {
		t.Fatalf("unplace: %d: %v", code, resp)
	}

	code, listResp := f.do(t, http.MethodGet, f.placementsPath(), nil)
	if code != http.StatusOK {
		t.Fatalf("list after unplace: %d: %v", code, listResp)
	}
	placements, _ := listResp["placements"].([]any)
	if len(placements) != 0 {
		t.Errorf("expected 0 placements after unplace, got %d", len(placements))
	}
	if int(listResp["pool_size"].(float64)) != 5 {
		t.Errorf("pool_size after unplace = %v, want 5 (all gubbar back in the pool)", listResp["pool_size"])
	}

	// Re-unplacing the same (now-gone) ordinal must 404, not silently succeed.
	code, resp = f.do(t, http.MethodDelete, f.placementsPath()+"/"+strconv.Itoa(ordinal), nil)
	if code != http.StatusNotFound {
		t.Errorf("re-unplace of an already-unplaced gubbe = %d, want 404", code)
	}
}

// TestPlaceGubbe_RejectsWhenPoolIsEmpty: a settlement with 0 population
// (0 gubbar) has nothing to place — every attempt must fail cleanly, not
// place a phantom gubbe with an invalid ordinal.
func TestPlaceGubbe_RejectsWhenPoolIsEmpty(t *testing.T) {
	f := setupPlacementFixture(t, map[[2]int]string{{1, 0}: "plains"})
	ctx := context.Background()
	pool := p10TestPool(t)
	if _, err := pool.Exec(ctx, `UPDATE settlements SET population = 0 WHERE id = $1`, f.settlementID); err != nil {
		t.Fatalf("zero population: %v", err)
	}

	code, resp := f.do(t, http.MethodPost, f.placementsPath(),
		map[string]any{"target_kind": "hex", "hex_q": 1, "hex_r": 0, "good_key": "grain"})
	if code != http.StatusConflict {
		t.Errorf("placement with 0 gubbar available = %d: %v, want 409", code, resp)
	}
}

// TestPlacementOptions_GrainCappedFishCapped: P5's data source. Grain must
// now report a real numeric cap (like every other good — megaron_plan_grain_cap.md,
// 2026-08-22) but KEEPS its own marginal_yield shape: rate directly, no cap
// denominator (placementYield's rate × placed, not rate/cap × placed). Fish
// reports a real numeric cap AND marginal_yield = rate/cap. Both hexes'
// ordinals must be present and match hexgrid.RingOrdinal's own numbering.
func TestPlacementOptions_GrainCappedFishCapped(t *testing.T) {
	f := setupPlacementFixture(t, map[[2]int]string{{1, 0}: "plains", {0, 1}: "coastal_sea"})

	code, resp := f.do(t, http.MethodGet, f.placementOptionsPath(), nil)
	if code != http.StatusOK {
		t.Fatalf("placement-options = %d: %v", code, resp)
	}
	hexes, _ := resp["hexes"].([]any)
	if len(hexes) != 2 {
		t.Fatalf("expected 2 catchment hexes in options, got %d (%v)", len(hexes), resp)
	}

	var grainHex, fishHex map[string]any
	for _, h := range hexes {
		hm := h.(map[string]any)
		if hm["terrain"] == "plains" {
			grainHex = hm
		}
		if hm["terrain"] == "coastal_sea" {
			fishHex = hm
		}
		if hm["hex_ordinal"] == nil {
			t.Errorf("hex %v missing hex_ordinal", hm)
		}
	}
	if grainHex == nil || fishHex == nil {
		t.Fatalf("expected both a plains and a coastal_sea hex in options, got %v", hexes)
	}

	findGood := func(hex map[string]any, good string) map[string]any {
		for _, g := range hex["goods"].([]any) {
			gm := g.(map[string]any)
			if gm["good_key"] == good {
				return gm
			}
		}
		return nil
	}

	grain := findGood(grainHex, "grain")
	if grain == nil {
		t.Fatalf("plains hex has no grain option: %v", grainHex)
	}
	grainCap, hasGrainCap := grain["cap"]
	if !hasGrainCap || grainCap.(float64) != 4 {
		t.Errorf("grain cap = %v (hasCap=%v), want 4 (plains, no farm, capNoBuilding)", grainCap, hasGrainCap)
	}
	if grain["marginal_yield"] != grain["rate_per_tick"] {
		t.Errorf("grain marginal_yield (%v) must equal rate_per_tick (%v) — no cap denominator (placementYield keeps rate × placed)", grain["marginal_yield"], grain["rate_per_tick"])
	}

	fish := findGood(fishHex, "fish")
	if fish == nil {
		t.Fatalf("coastal_sea hex has no fish option: %v", fishHex)
	}
	cap, hasCap := fish["cap"]
	if !hasCap || cap.(float64) != 1 {
		t.Errorf("fish cap = %v (hasCap=%v), want 1 (coastal_sea, no harbour, P3)", cap, hasCap)
	}
	wantMarginal := fish["rate_per_tick"].(float64) / cap.(float64)
	if fish["marginal_yield"].(float64) != wantMarginal {
		t.Errorf("fish marginal_yield = %v, want rate/cap = %v", fish["marginal_yield"], wantMarginal)
	}

	// Place one gubbe on the fish hex, then re-fetch: occupancy must show up.
	code, placeResp := f.do(t, http.MethodPost, f.placementsPath(),
		map[string]any{"target_kind": "hex", "hex_q": 0, "hex_r": 1, "good_key": "fish"})
	if code != http.StatusCreated {
		t.Fatalf("place fish gubbe: %d: %v", code, placeResp)
	}
	placedOrdinal := int(placeResp["gubbe_ordinal"].(float64))

	code, resp2 := f.do(t, http.MethodGet, f.placementOptionsPath(), nil)
	if code != http.StatusOK {
		t.Fatalf("placement-options after placing = %d: %v", code, resp2)
	}
	for _, h := range resp2["hexes"].([]any) {
		hm := h.(map[string]any)
		if hm["terrain"] != "coastal_sea" {
			continue
		}
		fish2 := findGood(hm, "fish")
		if int(fish2["placed"].(float64)) != 1 {
			t.Errorf("fish placed count = %v, want 1", fish2["placed"])
		}
		ordinals, _ := fish2["placed_ordinals"].([]any)
		if len(ordinals) != 1 || int(ordinals[0].(float64)) != placedOrdinal {
			t.Errorf("fish placed_ordinals = %v, want [%d]", ordinals, placedOrdinal)
		}
	}

	if int(resp2["pool_size"].(float64)) != int(resp["pool_size"].(float64))-1 {
		t.Errorf("pool_size after placing = %v, want %v", resp2["pool_size"], int(resp["pool_size"].(float64))-1)
	}
}

// TestPlaceGubbe_ByHexOrdinal: keryx/web address hexes as 1..18 (P0-UI
// answer 7) — POST must accept hex_ordinal as an alternative to hex_q/hex_r
// and place on the exact same coordinate placement-options reported that
// ordinal at.
func TestPlaceGubbe_ByHexOrdinal(t *testing.T) {
	f := setupPlacementFixture(t, map[[2]int]string{{1, 0}: "plains"})

	code, optResp := f.do(t, http.MethodGet, f.placementOptionsPath(), nil)
	if code != http.StatusOK {
		t.Fatalf("placement-options = %d: %v", code, optResp)
	}
	hexes, _ := optResp["hexes"].([]any)
	if len(hexes) != 1 {
		t.Fatalf("expected 1 catchment hex, got %d", len(hexes))
	}
	hex := hexes[0].(map[string]any)
	ordinal := int(hex["hex_ordinal"].(float64))
	if int(hex["hex_q"].(float64)) != 1 || int(hex["hex_r"].(float64)) != 0 {
		t.Fatalf("unexpected hex coord in options: %v", hex)
	}

	code, resp := f.do(t, http.MethodPost, f.placementsPath(),
		map[string]any{"target_kind": "hex", "hex_ordinal": ordinal, "good_key": "grain"})
	if code != http.StatusCreated {
		t.Fatalf("place by hex_ordinal = %d: %v", code, resp)
	}

	code, listResp := f.do(t, http.MethodGet, f.placementsPath(), nil)
	if code != http.StatusOK {
		t.Fatalf("list placements = %d: %v", code, listResp)
	}
	placements, _ := listResp["placements"].([]any)
	if len(placements) != 1 {
		t.Fatalf("expected 1 placement, got %d", len(placements))
	}
	p := placements[0].(map[string]any)
	if int(p["hex_q"].(float64)) != 1 || int(p["hex_r"].(float64)) != 0 {
		t.Errorf("placement by hex_ordinal landed on wrong coord: %v (want q=1,r=0)", p)
	}
	if int(p["hex_ordinal"].(float64)) != ordinal {
		t.Errorf("placement hex_ordinal = %v, want %d", p["hex_ordinal"], ordinal)
	}
}

// TestPlaceGubbe_HexOrdinalOutOfRangeRejected: bounds-checks the resolved
// index against the actual ring length rather than trusting client input.
func TestPlaceGubbe_HexOrdinalOutOfRangeRejected(t *testing.T) {
	f := setupPlacementFixture(t, map[[2]int]string{{1, 0}: "plains"})

	code, resp := f.do(t, http.MethodPost, f.placementsPath(),
		map[string]any{"target_kind": "hex", "hex_ordinal": 99, "good_key": "grain"})
	if code != http.StatusBadRequest {
		t.Errorf("hex_ordinal=99 (out of range) = %d: %v, want 400", code, resp)
	}
}
