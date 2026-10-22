package handlers

import (
	"context"
	"math"
	"net/http"
	"testing"
)

func TestDeepFishingHTTPZeroCapAndHarbour(t *testing.T) {
	f := setupPlacementFixture(t, map[[2]int]string{{1, 0}: "deep_sea"})
	pool := p10TestPool(t)
	ctx := context.Background()
	check := func(want int, rate float64) {
		t.Helper()
		code, resp := f.do(t, http.MethodGet, f.placementOptionsPath(), nil)
		if code != 200 {
			t.Fatalf("options%d %v", code, resp)
		}
		found := false
		for _, raw := range resp["hexes"].([]any) {
			h := raw.(map[string]any)
			if h["hex_q"] != float64(1) || h["hex_r"] != float64(0) {
				continue
			}
			found = true
			if _, leak := h["neighbours"]; leak {
				t.Fatal("neighbor terrain leaked")
			}
			for _, graw := range h["goods"].([]any) {
				g := graw.(map[string]any)
				if g["good_key"] != "fish" {
					continue
				}
				if g["cap"] != float64(want) {
					t.Fatalf("fish cap=%v want%d", g["cap"], want)
				}
				if math.Abs(g["marginal_yield"].(float64)-rate) > 1e-9 {
					t.Fatalf("marginal=%v want%v", g["marginal_yield"], rate)
				}
			}
		}
		if !found {
			t.Fatal("visible deep hex missing")
		}
	}
	check(0, 1)
	code, _ := f.do(t, http.MethodPost, f.placementsPath(), map[string]any{"target_kind": "hex", "hex_q": 1, "hex_r": 0, "good_key": "fish"})
	if code != http.StatusConflict {
		t.Fatalf("deep without harbour placement=%d want409", code)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO buildings(settlement_id,building_type,level) VALUES($1,'harbour',1)`, f.settlementID); err != nil {
		t.Fatal(err)
	}
	check(1, 1)
	code, resp := f.do(t, http.MethodPost, f.placementsPath(), map[string]any{"target_kind": "hex", "hex_q": 1, "hex_r": 0, "good_key": "fish"})
	if code != http.StatusCreated {
		t.Fatalf("deep with harbour placement=%d %v", code, resp)
	}
}

func TestBayFishingHTTPUsesLocalCapacity(t *testing.T) {
	f := setupPlacementFixture(t, map[[2]int]string{{1, 0}: "coastal_sea", {2, 0}: "river", {1, 1}: "river_ford", {1, -1}: "plains"})
	pool := p10TestPool(t)
	for _, level := range []int{0, 1, 2, 3} {
		if level > 0 {
			if _, err := pool.Exec(context.Background(), `INSERT INTO buildings(settlement_id,building_type,level) VALUES($1,'harbour',$2) ON CONFLICT(settlement_id,building_type) WHERE hex_q IS NULL AND building_type <> 'temple' DO UPDATE SET level=EXCLUDED.level`, f.settlementID, level); err != nil {
				t.Fatal(err)
			}
		}
		code, resp := f.do(t, http.MethodGet, f.placementOptionsPath(), nil)
		if code != 200 {
			t.Fatalf("options=%d %v", code, resp)
		}
		found := false
		for _, raw := range resp["hexes"].([]any) {
			h := raw.(map[string]any)
			if h["hex_q"] != float64(1) || h["hex_r"] != float64(0) {
				continue
			}
			found = true
			for _, key := range []string{"neighbors", "neighbours", "land_neighbors", "adjacent_terrain"} {
				if _, ok := h[key]; ok {
					t.Fatalf("leaked %s", key)
				}
			}
			for _, rawGood := range h["goods"].([]any) {
				g := rawGood.(map[string]any)
				if g["good_key"] != "fish" {
					continue
				}
				want := 2
				if level > 0 {
					want = 2
				}
				if g["cap"] != float64(want) {
					t.Fatalf("L%d bay=%v want%d", level, g["cap"], want)
				}
				if g["marginal_yield"] != float64(2) {
					t.Fatalf("bay L%d rate=%v want2", level, g["marginal_yield"])
				}
			}
		}
		if !found {
			t.Fatal("bay not visible")
		}
	}
}
