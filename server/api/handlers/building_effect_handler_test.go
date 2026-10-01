package handlers

// Handler-level acceptance tests for byggnadsregeln step 6: the effect text of a
// hex-bound building lives in GET .../placement-options (valid_hexes_for_building
// [type][].effect and hexes[].building.upgrade_effect), and the old per-building
// "effects" arrays are gone from GET /api/v1/buildings and .../buildings.
//
// Real Postgres, gated by DATABASE_URL — reuses buildHexFixture
// (province_build_hex_test.go), whose ring hex (1,0) is seeded plains.

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"formatet/megaron/server/internal/clock"
	"formatet/megaron/server/internal/economy"
)

func TestBuildingCatalogue_HasNoEffectsField(t *testing.T) {
	pool := p10TestPool(t)
	ph := NewProvinceHandler(pool, nil, clock.NewTestClock(time.Now()), economy.SitosConfig{}, nil, nil)

	rec := httptest.NewRecorder()
	ph.BuildingCatalogue(rec, httptest.NewRequest(http.MethodGet, "/api/v1/buildings", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("BuildingCatalogue = %d: %s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), `"effects"`) {
		t.Errorf("catalogue still carries an effects field: %.200s", rec.Body.String())
	}
}

type placementOptionsEffects struct {
	Hexes []struct {
		HexQ     int `json:"hex_q"`
		HexR     int `json:"hex_r"`
		Building *struct {
			Type          string `json:"type"`
			Level         int    `json:"level"`
			UpgradeEffect string `json:"upgrade_effect"`
		} `json:"building"`
	} `json:"hexes"`
	Valid map[string][]struct {
		Q      int    `json:"q"`
		R      int    `json:"r"`
		Effect string `json:"effect"`
	} `json:"valid_hexes_for_building"`
}

func getPlacementOptionsEffects(t *testing.T, f *mineGateFixture) placementOptionsEffects {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/worlds/"+f.worldID.String()+"/provinces/"+f.provinceID.String()+"/placement-options", nil)
	req.Header.Set("Authorization", "Bearer "+f.accessToken)
	rec := httptest.NewRecorder()
	f.router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("PlacementOptions = %d: %s", rec.Code, rec.Body.String())
	}
	var out placementOptionsEffects
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("parse PlacementOptions: %v", err)
	}
	return out
}

// A buildable hex carries the effect text for just that hex and the building's
// own good; a built farm carries the upgrade text, and none at max level; the
// buildings list has no effects.
func TestPlacementOptions_EffectAndUpgradeEffect(t *testing.T) {
	f := buildHexFixture(t)
	ph := NewProvinceHandler(f.pool, nil, clock.NewTestClock(time.Now()), economy.SitosConfig{}, nil, nil)
	f.router.Get("/worlds/{worldID}/provinces/{provinceID}/placement-options", ph.PlacementOptions)
	f.router.Get("/worlds/{worldID}/provinces/{provinceID}/buildings", ph.Buildings)

	out := getPlacementOptionsEffects(t, f)
	found := false
	for _, v := range out.Valid["farm"] {
		if v.Q == 1 && v.R == 0 {
			found = true
			if want := "grain production ×1.7 · space for 4 more workers"; v.Effect != want {
				t.Errorf("farm effect on plains (1,0) = %q, want %q", v.Effect, want)
			}
		}
		if v.Q == 0 && v.R == 1 {
			t.Errorf("deep_sea (0,1) must not be a valid farm hex")
		}
	}
	if !found {
		t.Fatalf("hex (1,0) missing from valid farm hexes: %+v", out.Valid["farm"])
	}

	// One building per hex: a mine standing on (1,0) takes it off the farm list.
	if _, err := f.pool.Exec(context.Background(),
		`INSERT INTO buildings (settlement_id, building_type, level, hex_q, hex_r) VALUES ($1, 'mine', 1, 1, 0)`,
		f.settlementID); err != nil {
		t.Fatalf("seed mine: %v", err)
	}
	for _, v := range getPlacementOptionsEffects(t, f).Valid["farm"] {
		if v.Q == 1 && v.R == 0 {
			t.Errorf("hex (1,0) with a mine is still a valid farm hex")
		}
	}
	if _, err := f.pool.Exec(context.Background(),
		`DELETE FROM buildings WHERE settlement_id = $1 AND building_type = 'mine'`, f.settlementID); err != nil {
		t.Fatalf("clear mine: %v", err)
	}

	upgradeAt := func(level int) string {
		if _, err := f.pool.Exec(context.Background(),
			`DELETE FROM buildings WHERE settlement_id = $1 AND building_type = 'farm'`, f.settlementID); err != nil {
			t.Fatalf("clear farm: %v", err)
		}
		if _, err := f.pool.Exec(context.Background(),
			`INSERT INTO buildings (settlement_id, building_type, level, hex_q, hex_r) VALUES ($1, 'farm', $2, 1, 0)`,
			f.settlementID, level); err != nil {
			t.Fatalf("insert farm: %v", err)
		}
		got := getPlacementOptionsEffects(t, f)
		for _, h := range got.Hexes {
			if h.HexQ == 1 && h.HexR == 0 {
				if h.Building == nil {
					t.Fatalf("hex (1,0) lists no building")
				}
				return h.Building.UpgradeEffect
			}
		}
		t.Fatalf("hex (1,0) not in placement options")
		return ""
	}
	if got, want := upgradeAt(1), "grain production ×1.7 → ×2.4"; got != want {
		t.Errorf("L1 upgrade_effect = %q, want %q", got, want)
	}
	if got, want := upgradeAt(2), "grain production ×2.4 → ×3.1"; got != want {
		t.Errorf("L2 upgrade_effect = %q, want %q", got, want)
	}
	if got := upgradeAt(3); got != "" {
		t.Errorf("max-level upgrade_effect = %q, want none", got)
	}

	req := httptest.NewRequest(http.MethodGet, "/worlds/"+f.worldID.String()+"/provinces/"+f.provinceID.String()+"/buildings", nil)
	req.Header.Set("Authorization", "Bearer "+f.accessToken)
	rec := httptest.NewRecorder()
	f.router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("Buildings = %d: %s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), `"effects"`) {
		t.Errorf("buildings list still carries an effects field: %s", rec.Body.String())
	}
}
