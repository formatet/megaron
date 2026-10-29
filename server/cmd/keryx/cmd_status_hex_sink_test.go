package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"formatet/megaron/server/internal/province"
)

// Exercise the public JSON shape rather than constructing hexBuildSites directly.
func TestHexBuildSinkFromPlacementOptions(t *testing.T) {
	for _, typ := range []string{"farm", "mine", "lumbermill", "stonequarry"} {
		t.Run(typ, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet || r.URL.Path != "/api/v1/worlds/world/provinces/prov/placement-options" || r.Header.Get("Authorization") != "Bearer token" {
					t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
				}
				fmt.Fprintf(w, `{"hexes":[{"hex_q":1,"hex_r":0,"building":{"type":%q,"level":1}},{"hex_q":2,"hex_r":0,"building":{"type":%q,"level":3}}],"valid_hexes_for_building":{%q:[{"q":3,"r":0},{"q":4,"r":0}]}}`, typ, typ, typ)
			}))
			defer srv.Close()
			resp, err := fetchPlacementOptions(newClient(&Config{Server: srv.URL, Token: "token"}), "world", "prov")
			if err != nil {
				t.Fatal(err)
			}
			got := remainingBuildingCosts(allBuildingsMaxed(), 3, hexBuildSitesFrom(resp))
			// One level-1 building needs levels 2+3, one maxed building needs nothing,
			// and each of the two free hexes needs levels 1+2+3.
			want := map[string]float64{}
			for level, count := range map[int]float64{1: 2, 2: 3, 3: 3} {
				spec, ok := province.LevelledSpec(province.BuildingType(typ), level)
				if !ok {
					t.Fatalf("missing spec %s/%d", typ, level)
				}
				for good, amount := range spec.Costs {
					want[good] += count * amount
				}
				want["silver"] += count * spec.CostSilver
			}
			for good, amount := range want {
				if got[good] != amount {
					t.Errorf("%s: got %v want %v", good, got[good], amount)
				}
			}
			t.Logf("%s: material sink %v", typ, got)
		})
	}
}

// A stock between the one-farm and three-farm sinks must not be reported as
// excess; one above the complete sink must. Run the actual status command.
func TestStatusBuildSinkUsesAllPlacementHexes(t *testing.T) {
	oldCfg, oldJSON := cfg, jsonMode
	defer func() { cfg, jsonMode = oldCfg, oldJSON }()
	jsonMode = false
	fullFarm := 0.0
	for level := 1; level <= province.MaxBuildingLevel; level++ {
		spec, _ := province.LevelledSpec(province.BuildingFarm, level)
		fullFarm += spec.Costs["timber"]
	}
	for _, stock := range []float64{2 * fullFarm, 3*fullFarm + 1} {
		t.Run(fmt.Sprintf("stock_%g", stock), func(t *testing.T) {
			placementCalls := 0
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet || r.Header.Get("Authorization") != "Bearer token" {
					t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
				}
				w.Header().Set("Content-Type", "application/json")
				switch r.URL.Path {
				case "/api/v1/worlds/world/provinces/prov":
					buildings := []map[string]any{}
					for typ, level := range allBuildingsMaxed() {
						buildings = append(buildings, map[string]any{"type": typ, "level": level})
					}
					_ = json.NewEncoder(w).Encode(map[string]any{"settlement": map[string]any{"name": "Hex proof", "walls": 3, "buildings": buildings, "resources": map[string]any{"timber": map[string]any{"amount": stock, "rate": 1}}}})
				case "/api/v1/worlds/world/provinces/prov/placement-options":
					placementCalls++
					_, _ = w.Write([]byte(`{"hexes":[],"valid_hexes_for_building":{"farm":[{"q":1,"r":0},{"q":2,"r":0},{"q":3,"r":0}]}}`))
				case "/api/v1/worlds/world/provinces/prov/placements":
					_, _ = w.Write([]byte(`{"placements":[{"good_key":"timber","gubbe_ordinal":1}]}`))
				default:
					// Optional read views (greeting, contacts, recipes) are absent in this fixture.
					http.NotFound(w, r)
				}
			}))
			defer srv.Close()
			cfg = &Config{Server: srv.URL, Token: "token", WorldID: "world", ProvinceID: "prov"}
			out, err := captureStdout(t, statusCmd().Execute)
			if err != nil {
				t.Fatal(err)
			}
			if placementCalls != 1 {
				t.Fatalf("placement-options calls=%d want 1", placementCalls)
			}
			warns := strings.Contains(out, "produces with no receiver (`keryx place`)")
			want := stock > 3*fullFarm
			if warns != want {
				t.Fatalf("stock=%g full sink=%g warning=%t want %t; output:\n%s", stock, 3*fullFarm, warns, want, out)
			}
			t.Logf("stock %g, sink %g, warning %t", stock, 3*fullFarm, warns)
		})
	}
}
