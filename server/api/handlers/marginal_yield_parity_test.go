package handlers

// TestMarginalYield_GoodsMatchesPlacementOptions is the plan's §4.5
// acceptance test (megaron_plan_p4_arvet_i_province.md): /goods' marginal_yield
// per good must be IDENTICAL to /placement-options' marginal_yield for the
// SAME good in the SAME city — both now call the shared
// economy.MarginalYieldForSlot / economy.MarginalYieldPerGood, never a
// second copy of the formula. This is the repo's most repeated bug class
// (the /goods-lögnen, the grundningsprognosen's "en formel, två anrop") —
// this test fails the moment a future edit reintroduces a second formula.

import (
	"encoding/json"
	"math"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestMarginalYield_GoodsMatchesPlacementOptions(t *testing.T) {
	f := setupPlacementFixture(t, map[[2]int]string{
		{1, 0}: "plains",      // grain
		{0, 1}: "coastal_sea", // fish
	})

	code, optResp := f.do(t, http.MethodGet, f.placementOptionsPath(), nil)
	if code != http.StatusOK {
		t.Fatalf("GET placement-options = %d: %v", code, optResp)
	}

	// The best (highest) marginal_yield per good across every hex/building
	// entry placement-options offers — the same "which slot would my next
	// gubbe actually go to" selection economy.MarginalYieldPerGood makes
	// internally for /goods' single number.
	bestByGood := map[string]float64{}
	collect := func(entries []any) {
		for _, raw := range entries {
			container, _ := raw.(map[string]any)
			goodsRaw, _ := container["goods"].([]any)
			for _, gr := range goodsRaw {
				g, _ := gr.(map[string]any)
				key, _ := g["good_key"].(string)
				my, ok := g["marginal_yield"].(float64)
				if !ok || key == "" {
					continue
				}
				if cur, seen := bestByGood[key]; !seen || my > cur {
					bestByGood[key] = my
				}
			}
		}
	}
	hexes, _ := optResp["hexes"].([]any)
	buildings, _ := optResp["buildings"].([]any)
	collect(hexes)
	collect(buildings)
	if len(bestByGood) == 0 {
		t.Fatal("placement-options returned no goods with a marginal_yield — fixture broken")
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
	goodsMY := make(map[string]float64, len(goods))
	for _, g := range goods {
		key, _ := g["key"].(string)
		if my, ok := g["marginal_yield"].(float64); ok {
			goodsMY[key] = my
		}
	}

	for good, want := range bestByGood {
		got, ok := goodsMY[good]
		if !ok {
			t.Errorf("good %q: placement-options offers marginal_yield=%.6f but /goods has no marginal_yield for it", good, want)
			continue
		}
		if math.Abs(got-want) > 1e-9 {
			t.Errorf("good %q: /goods marginal_yield=%.6f, /placement-options best marginal_yield=%.6f — the two have drifted apart", good, got, want)
		}
	}
}
