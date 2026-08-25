package handlers

// Proof that GET /provinces/:id/goods reports worker counts from the P4
// placement model (settlement_placement) rather than the pre-P4 settlement_labor
// weights, which are inert dead writes for every non-cult good and made every
// "Workers" count a lie (stone agent finding, 2026-08-24).

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"formatet/megaron/server/internal/economy"
	"formatet/megaron/server/internal/hexgrid"
)

// Pure logic — runs without a database. One placed gubbe = 100 citizens; counts
// sum across both hex and building placements; an unplaced good is zero.
func TestPlacedCitizensByGood_SumsHexAndBuilding(t *testing.T) {
	pc := economy.PlacementCounts{
		Hex: map[hexgrid.Coord]map[string]int{
			{Q: 1, R: 0}: {"grain": 2},
			{Q: 0, R: 1}: {"grain": 1, "fish": 1},
		},
		Building: map[string]map[string]int{
			"foundry": {"bronze": 3},
		},
		Total: 7,
	}
	got := placedCitizensByGood(pc)
	want := map[string]int{"grain": 300, "fish": 100, "bronze": 300}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("placedCitizensByGood[%q] = %d, want %d", k, got[k], v)
		}
	}
	if got["timber"] != 0 {
		t.Errorf("an unplaced good should be 0 citizens, got %d", got["timber"])
	}
}

// End-to-end through the HTTP handler against a real DB (DATABASE_URL-gated, same
// harness as the placement endpoint tests). Place known gubbar, then read /goods
// and assert the worker counts match the placements — not phantom weight-derived
// numbers.
func TestGoods_EmployedReflectsPlacements(t *testing.T) {
	f := setupPlacementFixture(t, map[[2]int]string{{1, 0}: "plains", {0, 1}: "coastal_sea"})

	// 2 grain gubbar on the plains hex (cap 4), 1 fish gubbe on the coastal hex (cap 1).
	for i := 0; i < 2; i++ {
		code, resp := f.do(t, http.MethodPost, f.placementsPath(),
			map[string]any{"target_kind": "hex", "hex_q": 1, "hex_r": 0, "good_key": "grain"})
		if code != http.StatusCreated {
			t.Fatalf("grain placement %d = %d: %v", i, code, resp)
		}
	}
	if code, resp := f.do(t, http.MethodPost, f.placementsPath(),
		map[string]any{"target_kind": "hex", "hex_q": 0, "hex_r": 1, "good_key": "fish"}); code != http.StatusCreated {
		t.Fatalf("fish placement = %d: %v", code, resp)
	}

	req := httptest.NewRequest(http.MethodGet, f.goodsPath(), nil)
	req.Header.Set("Authorization", "Bearer "+f.accessToken)
	rec := httptest.NewRecorder()
	f.router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET goods = %d: %s", rec.Code, rec.Body.String())
	}
	var goods []map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &goods); err != nil {
		t.Fatalf("decode goods: %v (body: %s)", err, rec.Body.String())
	}
	byKey := make(map[string]map[string]any, len(goods))
	totalEmployed := 0
	for _, g := range goods {
		byKey[g["key"].(string)] = g
		if e, ok := g["employed_citizens"].(float64); ok {
			totalEmployed += int(e)
		}
	}

	emp := func(good string) int {
		g, ok := byKey[good]
		if !ok {
			t.Fatalf("good %q missing from /goods response", good)
		}
		return int(g["employed_citizens"].(float64))
	}

	if got := emp("grain"); got != 200 {
		t.Errorf("grain employed_citizens = %d, want 200 (2 placed gubbar × 100)", got)
	}
	if got := emp("fish"); got != 100 {
		t.Errorf("fish employed_citizens = %d, want 100 (1 placed gubbe × 100)", got)
	}
	// Placement enforces caps at write time, so no over-allocation state exists.
	if got := int(byKey["grain"]["unserved_citizens"].(float64)); got != 0 {
		t.Errorf("grain unserved_citizens = %d, want 0 (placement cannot over-allocate)", got)
	}
	// No phantom counts anywhere: total employed = only the 3 placed gubbar.
	if totalEmployed != 300 {
		t.Errorf("sum of employed_citizens across all goods = %d, want 300 (3 placed gubbar × 100)", totalEmployed)
	}
	// Idle = 500 pop − 300 placed = 200; it is the same value on every row.
	if got := int(byKey["grain"]["idle_citizens"].(float64)); got != 200 {
		t.Errorf("idle_citizens = %d, want 200 (500 pop − 300 placed)", got)
	}
}
