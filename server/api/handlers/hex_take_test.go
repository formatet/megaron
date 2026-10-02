package handlers

// Delad catchment (megaron_plan_delad_catchment.md §7, Timothy 2026-10-02):
// a hex held by another settlement can be TAKEN by placing a gubbe there when
// the placer's Wanax has a unit in fortify/sentry on it and the holder's Wanax
// does not. Fixture: hex_ownership_test.go's twoSettlementHexFixture (A at
// (0,0), B at (3,0), shared hex (2,0)).

import (
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
	"formatet/megaron/server/internal/province"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

const (
	takeHexQ = 2
	takeHexR = 0
)

func (f *twoSettlementHexFixture) owner(t *testing.T, settlementID uuid.UUID) uuid.UUID {
	t.Helper()
	var id uuid.UUID
	if err := p10TestPool(t).QueryRow(context.Background(),
		`SELECT owner_id FROM settlements WHERE id = $1`, settlementID).Scan(&id); err != nil {
		t.Fatalf("load owner: %v", err)
	}
	return id
}

// unit seeds a unit for owner on the shared hex with an explicit status/stance.
func (f *twoSettlementHexFixture) unit(t *testing.T, owner uuid.UUID, status string, stance *string) {
	t.Helper()
	if _, err := p10TestPool(t).Exec(context.Background(),
		`INSERT INTO units (world_id, owner_id, type, category, size, status, q, r, stance)
		 VALUES ($1, $2, 'spearman', 'land', 40, $3, $4, $5, $6)`,
		f.worldID, owner, status, takeHexQ, takeHexR, stance,
	); err != nil {
		t.Fatalf("seed unit: %v", err)
	}
}

func str(s string) *string { return &s }

func (f *twoSettlementHexFixture) place(t *testing.T, token string, province uuid.UUID, good string) (int, map[string]any) {
	t.Helper()
	return f.doAs(t, token, http.MethodPost, f.placementsPath(province),
		map[string]any{"target_kind": "hex", "hex_q": takeHexQ, "hex_r": takeHexR, "good_key": good})
}

// aHoldsHex makes A hold the shared hex with n grain gubbar.
func (f *twoSettlementHexFixture) aHoldsHex(t *testing.T, n int) {
	t.Helper()
	for i := 0; i < n; i++ {
		if code, resp := f.place(t, f.tokenA, f.provinceA, "grain"); code != http.StatusCreated {
			t.Fatalf("A's setup placement %d = %d: %v", i, code, resp)
		}
	}
}

func (f *twoSettlementHexFixture) placementCount(t *testing.T, settlementID uuid.UUID) int {
	t.Helper()
	var n int
	if err := p10TestPool(t).QueryRow(context.Background(),
		`SELECT COUNT(*) FROM settlement_placement WHERE settlement_id = $1 AND target_kind = 'hex' AND hex_q = $2 AND hex_r = $3`,
		settlementID, takeHexQ, takeHexR).Scan(&n); err != nil {
		t.Fatalf("count placements: %v", err)
	}
	return n
}

func (f *twoSettlementHexFixture) grainRate(t *testing.T, settlementID uuid.UUID) float64 {
	t.Helper()
	var rate float64
	if err := p10TestPool(t).QueryRow(context.Background(),
		`SELECT COALESCE(rate, 0) FROM settlement_goods WHERE settlement_id = $1 AND good_key = 'grain'`,
		settlementID).Scan(&rate); err != nil {
		t.Fatalf("grain rate: %v", err)
	}
	return rate
}

// Test 1: B's sentry unit on X; B places on A's hex -> 201, A's placements on
// X are gone, the farm changed owner and kept its level, A got a HexTaken
// event (+ dispatch) with q,r, and both settlements' rates were recomputed.
func TestPlaceGubbe_TakesHeldHexWithOwnUnit(t *testing.T) {
	f := setupTwoSettlementHexFixture(t, "plains", [2]int{takeHexQ, takeHexR})
	pool := p10TestPool(t)
	ctx := context.Background()

	f.aHoldsHex(t, 3)
	if _, err := pool.Exec(ctx,
		`INSERT INTO buildings (settlement_id, building_type, level, hex_q, hex_r) VALUES ($1, 'farm', 2, $2, $3)`,
		f.settlementA, takeHexQ, takeHexR); err != nil {
		t.Fatalf("seed farm: %v", err)
	}
	if err := economy.RecomputeProduction(ctx, pool, f.settlementA); err != nil {
		t.Fatalf("recompute A: %v", err)
	}
	aRateBefore, bRateBefore := f.grainRate(t, f.settlementA), f.grainRate(t, f.settlementB)
	if aRateBefore <= 0 {
		t.Fatalf("setup: A should produce grain from its 3 gubbar, rate=%v", aRateBefore)
	}
	f.unit(t, f.owner(t, f.settlementB), "positioned", str("sentry"))

	code, resp := f.place(t, f.tokenB, f.provinceB, "grain")
	if code != http.StatusCreated {
		t.Fatalf("B's take = %d: %v, want 201", code, resp)
	}
	if n := f.placementCount(t, f.settlementA); n != 0 {
		t.Errorf("A still has %d placements on the taken hex, want 0", n)
	}
	if n := f.placementCount(t, f.settlementB); n != 1 {
		t.Errorf("B has %d placements on the taken hex, want 1", n)
	}
	var owner uuid.UUID
	var level int
	if err := pool.QueryRow(ctx,
		`SELECT settlement_id, level FROM buildings WHERE building_type = 'farm' AND hex_q = $1 AND hex_r = $2`,
		takeHexQ, takeHexR).Scan(&owner, &level); err != nil {
		t.Fatalf("load farm: %v", err)
	}
	if owner != f.settlementB || level != 2 {
		t.Errorf("farm owner/level = %v/%d, want B/%d", owner, level, 2)
	}

	var payload struct {
		SettlementID uuid.UUID `json:"settlement_id"`
		Name         string    `json:"name"`
		Taker        string    `json:"taker"`
		Q            int       `json:"q"`
		R            int       `json:"r"`
		Workers      int       `json:"workers"`
		Building     string    `json:"building"`
	}
	if err := pool.QueryRow(ctx,
		`SELECT payload FROM events WHERE stream_id = $1 AND event_type = 'HexTaken' AND world_id = $2`,
		f.settlementA, f.worldID).Scan(&payload); err != nil {
		t.Fatalf("HexTaken event for A: %v", err)
	}
	if payload.SettlementID != f.settlementA || payload.Name != f.nameA || payload.Taker != f.nameB ||
		payload.Q != takeHexQ || payload.R != takeHexR || payload.Workers != 3 || payload.Building != "farm" {
		t.Errorf("HexTaken payload = %+v", payload)
	}
	var notifs int
	if err := pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM notifications WHERE player_id = $1 AND kind = 'HexTaken'`,
		f.owner(t, f.settlementA)).Scan(&notifs); err != nil || notifs != 1 {
		t.Errorf("HexTaken dispatch rows for A's Wanax = %d (err %v), want 1", notifs, err)
	}

	if got := f.grainRate(t, f.settlementA); got >= aRateBefore {
		t.Errorf("A's grain rate %v not recomputed down from %v", got, aRateBefore)
	}
	if got := f.grainRate(t, f.settlementB); got <= bRateBefore {
		t.Errorf("B's grain rate %v not recomputed up from %v", got, bRateBefore)
	}
}

// Test 2: no unit of B's on the hex -> 409 saying how it is taken.
func TestPlaceGubbe_HeldHexWithoutOwnUnitIsRefusedWithHowToTake(t *testing.T) {
	f := setupTwoSettlementHexFixture(t, "plains", [2]int{takeHexQ, takeHexR})
	f.aHoldsHex(t, 1)

	code, resp := f.place(t, f.tokenB, f.provinceB, "grain")
	if code != http.StatusConflict {
		t.Fatalf("B without a unit = %d: %v, want 409", code, resp)
	}
	want := f.nameA + " holds this hex — stand a unit there in fortify or sentry to take it"
	if msg, _ := resp["error"].(string); msg != want {
		t.Errorf("message = %q, want %q", msg, want)
	}
	if n := f.placementCount(t, f.settlementA); n != 1 {
		t.Errorf("A's placement must be untouched, has %d", n)
	}
}

// Test 3: both Wanaxes have a unit in hållning -> strid först.
func TestPlaceGubbe_HolderWithOwnUnitOnHexBlocksTake(t *testing.T) {
	f := setupTwoSettlementHexFixture(t, "plains", [2]int{takeHexQ, takeHexR})
	f.aHoldsHex(t, 1)
	f.unit(t, f.owner(t, f.settlementB), "positioned", str("sentry"))
	f.unit(t, f.owner(t, f.settlementA), "positioned", str("fortify"))

	code, resp := f.place(t, f.tokenB, f.provinceB, "grain")
	if code != http.StatusConflict {
		t.Fatalf("B's take against a defended hex = %d: %v, want 409", code, resp)
	}
	want := f.nameA + " holds this hex and has a unit standing guard there — that unit must be defeated before the hex can be taken"
	if msg, _ := resp["error"].(string); msg != want {
		t.Errorf("message = %q, want %q", msg, want)
	}
	if n := f.placementCount(t, f.settlementA); n != 1 {
		t.Errorf("A's placement must be untouched, has %d", n)
	}
}

// Test 4: presence is not enough — a marching unit, or a positioned unit
// without fortify/sentry (storm, no stance), never takes.
func TestPlaceGubbe_PresenceWithoutPositionedStanceDoesNotTake(t *testing.T) {
	cases := []struct {
		name   string
		status string
		stance *string
	}{
		{"marching with sentry stance", "marching", str("sentry")},
		{"positioned without stance", "positioned", nil},
		{"positioned in storm", "positioned", str("storm")},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := setupTwoSettlementHexFixture(t, "plains", [2]int{takeHexQ, takeHexR})
			f.aHoldsHex(t, 1)
			f.unit(t, f.owner(t, f.settlementB), tc.status, tc.stance)

			code, resp := f.place(t, f.tokenB, f.provinceB, "grain")
			if code != http.StatusConflict {
				t.Fatalf("B with a %s unit = %d: %v, want 409", tc.name, code, resp)
			}
			if f.placementCount(t, f.settlementA) != 1 {
				t.Errorf("A's placement must be untouched")
			}
		})
	}
}

// A holder's marching / stance-less unit does NOT defend the hex.
func TestPlaceGubbe_HolderUnitWithoutHoldingStanceDoesNotDefend(t *testing.T) {
	f := setupTwoSettlementHexFixture(t, "plains", [2]int{takeHexQ, takeHexR})
	f.aHoldsHex(t, 1)
	f.unit(t, f.owner(t, f.settlementB), "positioned", str("sentry"))
	f.unit(t, f.owner(t, f.settlementA), "marching", str("fortify"))

	if code, resp := f.place(t, f.tokenB, f.provinceB, "grain"); code != http.StatusCreated {
		t.Fatalf("take against a marching holder unit = %d: %v, want 201", code, resp)
	}
}

// Same Wanax owning both cities: unchanged 409, even with a unit standing there.
func TestPlaceGubbe_SameWanaxNeverTakesFromItself(t *testing.T) {
	f := setupTwoSettlementHexFixture(t, "plains", [2]int{takeHexQ, takeHexR})
	pool := p10TestPool(t)
	ownerA := f.owner(t, f.settlementA)
	if _, err := pool.Exec(context.Background(), `UPDATE settlements SET owner_id = $1 WHERE id = $2`, ownerA, f.settlementB); err != nil {
		t.Fatalf("give B to A's Wanax: %v", err)
	}
	f.aHoldsHex(t, 1)
	f.unit(t, ownerA, "positioned", str("sentry"))

	code, resp := f.place(t, f.tokenA, f.provinceB, "grain")
	if code != http.StatusConflict {
		t.Fatalf("own-city take = %d: %v, want 409", code, resp)
	}
	if msg, _ := resp["error"].(string); msg != f.nameA+" holds this hex" {
		t.Errorf("message = %q, want the unchanged %q", msg, f.nameA+" holds this hex")
	}
}

// Test 5: A has a farm in the build queue on X -> cancelled and refunded.
func TestPlaceGubbe_TakeCancelsAndRefundsHoldersQueuedBuild(t *testing.T) {
	f := setupTwoSettlementHexFixture(t, "plains", [2]int{takeHexQ, takeHexR})
	pool := p10TestPool(t)
	ctx := context.Background()
	f.aHoldsHex(t, 1)

	spec, ok := province.LevelledSpec(province.BuildingFarm, 1)
	if !ok {
		t.Fatalf("no farm level-1 spec")
	}
	for good := range spec.Costs {
		if _, err := pool.Exec(ctx,
			`INSERT INTO settlement_goods (settlement_id, good_key, amount, rate, cap, calc_tick)
			 VALUES ($1, $2, 0, 0, 1000, current_world_tick())
			 ON CONFLICT (settlement_id, good_key) DO UPDATE SET amount = 0, rate = 0, cap = 1000, calc_tick = current_world_tick()`,
			f.settlementA, good); err != nil {
			t.Fatalf("seed A's %s: %v", good, err)
		}
	}
	queueID := uuid.New()
	if _, err := pool.Exec(ctx,
		`INSERT INTO build_queue (id, settlement_id, world_id, building_type, complete_at, hex_q, hex_r)
		 VALUES ($1, $2, $3, 'farm', now() + interval '1 hour', $4, $5)`,
		queueID, f.settlementA, f.worldID, takeHexQ, takeHexR); err != nil {
		t.Fatalf("seed queue: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO scheduled_events (world_id, event_type, payload, due_tick, process_after)
		 VALUES ($1, 'BuildComplete', jsonb_build_object('build_queue_id', $2::text), 999999, now() + interval '1 hour')`,
		f.worldID, queueID.String()); err != nil {
		t.Fatalf("seed scheduled event: %v", err)
	}
	f.unit(t, f.owner(t, f.settlementB), "positioned", str("fortify"))

	if code, resp := f.place(t, f.tokenB, f.provinceB, "grain"); code != http.StatusCreated {
		t.Fatalf("take = %d: %v", code, resp)
	}
	var n int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM build_queue WHERE id = $1`, queueID).Scan(&n); err != nil || n != 0 {
		t.Errorf("queued build still present (n=%d, err=%v)", n, err)
	}
	if err := pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM scheduled_events WHERE event_type = 'BuildComplete' AND (payload->>'build_queue_id') = $1 AND processed_at IS NULL`,
		queueID.String()).Scan(&n); err != nil || n != 0 {
		t.Errorf("BuildComplete event still scheduled (n=%d, err=%v)", n, err)
	}
	for good, qty := range spec.Costs {
		var amount float64
		if err := pool.QueryRow(ctx,
			`SELECT settled(amount, rate, calc_tick) FROM settlement_goods WHERE settlement_id = $1 AND good_key = $2`,
			f.settlementA, good).Scan(&amount); err != nil {
			t.Fatalf("read A's %s: %v", good, err)
		}
		if amount < qty-1e-6 || amount > qty+1e-6 {
			t.Errorf("A's %s after refund = %v, want %v", good, amount, qty)
		}
	}
}

// Test 6: B takes while A places on the same hex. Exactly one owner results,
// B (it holds the unit) always ends up the owner, and the DB matches.
func TestPlaceGubbe_ConcurrentTakeAndHolderPlacement(t *testing.T) {
	f := setupTwoSettlementHexFixture(t, "plains", [2]int{takeHexQ, takeHexR})
	pool := p10TestPool(t)
	f.aHoldsHex(t, 1)
	f.unit(t, f.owner(t, f.settlementB), "positioned", str("sentry"))

	for round := 0; round < 6; round++ {
		var wg sync.WaitGroup
		var codeA, codeB int
		var respA, respB map[string]any
		wg.Add(2)
		go func() { defer wg.Done(); codeB, respB = f.place(t, f.tokenB, f.provinceB, "grain") }()
		go func() { defer wg.Done(); codeA, respA = f.place(t, f.tokenA, f.provinceA, "grain") }()
		wg.Wait()

		if codeB != http.StatusCreated {
			t.Fatalf("round %d: B's take = %d: %v", round, codeB, respB)
		}
		if codeA != http.StatusCreated && codeA != http.StatusConflict {
			t.Fatalf("round %d: A's placement = %d: %v", round, codeA, respA)
		}
		hexes := []hexgrid.Coord{{Q: takeHexQ, R: takeHexR}}
		if nA, nB := f.placementCount(t, f.settlementA), f.placementCount(t, f.settlementB); nA != 0 || nB != 1 {
			t.Fatalf("round %d: placements on the hex A=%d B=%d (A's code %d), want A=0 B=1 — one owner", round, nA, nB, codeA)
		}
		if occ, err := economy.GlobalHexOccupancy(context.Background(), pool, f.worldID, hexes); err != nil || occ[hexes[0]]["grain"] != 1 {
			t.Fatalf("round %d: global occupancy %v err %v, want grain=1", round, occ, err)
		}

		// Reset: B hands the hex back (unplace) and A re-holds it.
		if _, err := pool.Exec(context.Background(),
			`DELETE FROM settlement_placement WHERE target_kind = 'hex' AND hex_q = $1 AND hex_r = $2`, takeHexQ, takeHexR); err != nil {
			t.Fatalf("reset: %v", err)
		}
		f.aHoldsHex(t, 1)
	}
}

// Read surface: PlacementOptions marks the hex takeable exactly when the
// write gate lets the placement through — yes and no cases, same fixture.
func TestPlacementOptions_TakeableMatchesWriteGate(t *testing.T) {
	hexOf := func(t *testing.T, f *twoSettlementHexFixture) map[string]any {
		t.Helper()
		code, resp := f.doAs(t, f.tokenB, http.MethodGet,
			"/worlds/"+f.worldID.String()+"/provinces/"+f.provinceB.String()+"/placement-options", nil)
		if code != http.StatusOK {
			t.Fatalf("placement-options = %d: %v", code, resp)
		}
		hexes, _ := resp["hexes"].([]any)
		for _, h := range hexes {
			m := h.(map[string]any)
			if int(m["hex_q"].(float64)) == takeHexQ && int(m["hex_r"].(float64)) == takeHexR {
				return m
			}
		}
		t.Fatalf("shared hex missing from placement-options: %v", resp)
		return nil
	}
	grain := func(t *testing.T, hex map[string]any) map[string]any {
		t.Helper()
		for _, g := range hex["goods"].([]any) {
			if gm := g.(map[string]any); gm["good_key"] == "grain" {
				return gm
			}
		}
		t.Fatalf("no grain on the hex: %v", hex)
		return nil
	}

	cases := []struct {
		name     string
		setup    func(t *testing.T, f *twoSettlementHexFixture)
		takeable bool
	}{
		{"own sentry unit", func(t *testing.T, f *twoSettlementHexFixture) {
			f.unit(t, f.owner(t, f.settlementB), "positioned", str("sentry"))
		}, true},
		{"no unit", func(t *testing.T, f *twoSettlementHexFixture) {}, false},
		{"holder defends", func(t *testing.T, f *twoSettlementHexFixture) {
			f.unit(t, f.owner(t, f.settlementB), "positioned", str("fortify"))
			f.unit(t, f.owner(t, f.settlementA), "positioned", str("sentry"))
		}, false},
		{"marching unit", func(t *testing.T, f *twoSettlementHexFixture) {
			f.unit(t, f.owner(t, f.settlementB), "marching", str("sentry"))
		}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := setupTwoSettlementHexFixture(t, "plains", [2]int{takeHexQ, takeHexR})
			f.aHoldsHex(t, 2)
			tc.setup(t, f)

			hex := hexOf(t, f)
			if hex["held_by"] != f.nameA {
				t.Errorf("held_by = %v, want %q", hex["held_by"], f.nameA)
			}
			if got, _ := hex["takeable"].(bool); got != tc.takeable {
				t.Errorf("takeable = %v, want %v", hex["takeable"], tc.takeable)
			}
			g := grain(t, hex)
			placed, _ := g["placed"].(float64)
			capv, _ := g["cap"].(float64)
			if tc.takeable && placed != 0 {
				t.Errorf("takeable hex: holder's gubbar must be subtracted, placed = %v", placed)
			}
			if !tc.takeable && (capv == 0 || placed != capv) {
				t.Errorf("held non-takeable hex must read full: placed %v cap %v", placed, capv)
			}

			// The write gate agrees.
			code, resp := f.place(t, f.tokenB, f.provinceB, "grain")
			if (code == http.StatusCreated) != tc.takeable {
				t.Errorf("write gate gave %d (%v) but takeable = %v", code, resp, tc.takeable)
			}
		})
	}
}

// CancelBuild's behaviour must be unchanged by the cancelQueuedBuild
// extraction (the take shares it): 200 {"cancelled": type}, row gone, costs
// refunded; a second cancel finds nothing (404); another Wanax gets 404.
func TestCancelBuild_RefundsAndDeletesQueueRow(t *testing.T) {
	f := setupTwoSettlementHexFixture(t, "plains", [2]int{takeHexQ, takeHexR})
	pool := p10TestPool(t)
	ctx := context.Background()

	clk := clock.NewTestClock(time.Now())
	ph := NewProvinceHandler(pool, events.NewScheduler(pool, clk), clk, economy.SitosConfig{}, events.NewStore(pool), nil)
	r := chi.NewRouter()
	r.Use(auth.Middleware(auth.NewService(pool, "test-secret")))
	r.Delete("/worlds/{worldID}/provinces/{provinceID}/build-queue/{queueID}", ph.CancelBuild)

	spec, _ := province.LevelledSpec(province.BuildingFarm, 1)
	for good := range spec.Costs {
		if _, err := pool.Exec(ctx,
			`INSERT INTO settlement_goods (settlement_id, good_key, amount, rate, cap, calc_tick)
			 VALUES ($1, $2, 0, 0, 1000, current_world_tick())
			 ON CONFLICT (settlement_id, good_key) DO UPDATE SET amount = 0, rate = 0, cap = 1000, calc_tick = current_world_tick()`,
			f.settlementA, good); err != nil {
			t.Fatalf("seed %s: %v", good, err)
		}
	}
	queueID := uuid.New()
	if _, err := pool.Exec(ctx,
		`INSERT INTO build_queue (id, settlement_id, world_id, building_type, complete_at, hex_q, hex_r)
		 VALUES ($1, $2, $3, 'farm', now() + interval '1 hour', $4, $5)`,
		queueID, f.settlementA, f.worldID, takeHexQ, takeHexR); err != nil {
		t.Fatalf("seed queue: %v", err)
	}
	path := "/worlds/" + f.worldID.String() + "/provinces/" + f.provinceA.String() + "/build-queue/" + queueID.String()

	do := func(token string) (int, map[string]any) {
		req := httptest.NewRequest(http.MethodDelete, path, nil)
		req.Header.Set("Authorization", "Bearer "+token)
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		var resp map[string]any
		_ = json.Unmarshal(rec.Body.Bytes(), &resp)
		return rec.Code, resp
	}
	if code, _ := do(f.tokenB); code != http.StatusNotFound {
		t.Fatalf("another Wanax cancelling = %d, want 404", code)
	}
	code, resp := do(f.tokenA)
	if code != http.StatusOK || resp["cancelled"] != "farm" {
		t.Fatalf("cancel = %d %v, want 200 {cancelled: farm}", code, resp)
	}
	for good, qty := range spec.Costs {
		var amount float64
		if err := pool.QueryRow(ctx,
			`SELECT settled(amount, rate, calc_tick) FROM settlement_goods WHERE settlement_id = $1 AND good_key = $2`,
			f.settlementA, good).Scan(&amount); err != nil || amount != qty {
			t.Errorf("refund of %s = %v (err %v), want %v", good, amount, err, qty)
		}
	}
	if code, _ := do(f.tokenA); code != http.StatusNotFound {
		t.Fatalf("second cancel = %d, want 404", code)
	}
}
