package handlers

// Handler-level acceptance tests for megaron_plan_byggnad_pa_hex.md §B
// (B1): GET /api/v1/buildings (BuildingCatalogue) and GET
// .../provinces/{id}/buildings (Buildings) both carry an "effects" field
// computed by economy.BuildingEffectsForCatalogue/BuildingEffectsForHex.
//
// Real Postgres, gated by DATABASE_URL — reuses buildHexFixture
// (province_build_hex_test.go), whose ring hex (1,0) is seeded plains.

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"formatet/megaron/server/internal/clock"
	"formatet/megaron/server/internal/economy"
)

type effectRowJSON struct {
	Good    string                   `json:"good"`
	Kind    string                   `json:"kind"`
	Terrain string                   `json:"terrain"`
	Without struct {
		PerGubbe float64 `json:"per_gubbe"`
		Gubbar   int     `json:"gubbar"`
	} `json:"without"`
	Levels []struct {
		Level    int     `json:"level"`
		PerGubbe float64 `json:"per_gubbe"`
		Gubbar   int     `json:"gubbar"`
	} `json:"levels"`
	Text string `json:"text"`
}

// TestBuildingCatalogue_ExposesFarmEffects is acceptance criterion 1: the
// static catalogue's farm entry carries grain's without/level numbers, and
// no level ever moves grain's per-gubbe rate.
func TestBuildingCatalogue_ExposesFarmEffects(t *testing.T) {
	pool := p10TestPool(t)
	ph := NewProvinceHandler(pool, nil, clock.NewTestClock(time.Now()), economy.SitosConfig{}, nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/buildings", nil)
	rec := httptest.NewRecorder()
	ph.BuildingCatalogue(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("BuildingCatalogue = %d: %s", rec.Code, rec.Body.String())
	}

	var catalogue []struct {
		Type    string          `json:"type"`
		Effects []effectRowJSON `json:"effects"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &catalogue); err != nil {
		t.Fatalf("parse BuildingCatalogue response: %v", err)
	}

	var farm []effectRowJSON
	for _, b := range catalogue {
		if b.Type == "farm" {
			farm = b.Effects
		}
	}
	if farm == nil {
		t.Fatal("no farm effects in catalogue")
	}

	var grainOnPlains *effectRowJSON
	for i := range farm {
		if farm[i].Good == "grain" && farm[i].Terrain == "plains" {
			grainOnPlains = &farm[i]
		}
	}
	if grainOnPlains == nil {
		t.Fatalf("no grain/plains row in farm's catalogue effects: %+v", farm)
	}
	if grainOnPlains.Without.Gubbar != 4 || grainOnPlains.Without.PerGubbe != 1.00 {
		t.Errorf("grain without farm = %d × %.2f, want 4 × 1.00", grainOnPlains.Without.Gubbar, grainOnPlains.Without.PerGubbe)
	}
	wantGubbar := []int{8, 10, 12}
	for i, lvl := range grainOnPlains.Levels {
		if lvl.PerGubbe != 2.70 {
			t.Errorf("grain L%d per_gubbe = %.2f, want 2.70", lvl.Level, lvl.PerGubbe)
		}
		if lvl.Gubbar != wantGubbar[i] {
			t.Errorf("grain L%d gubbar = %d, want %d", lvl.Level, lvl.Gubbar, wantGubbar[i])
		}
	}
	if grainOnPlains.Text == "" {
		t.Error("grain/plains row has no rendered text")
	}
}

// TestBuildings_HexBoundEffectsUseTheBuildingsOwnRealHex is acceptance
// criterion 4: a farm built on ring hex (1,0) (seeded plains in
// buildHexFixture) gets EFFECT rows for its OWN hex's real terrain, with
// current_level reflecting what's actually built.
func TestBuildings_HexBoundEffectsUseTheBuildingsOwnRealHex(t *testing.T) {
	f := buildHexFixture(t)
	ph := NewProvinceHandler(f.pool, nil, clock.NewTestClock(time.Now()), economy.SitosConfig{}, nil, nil)
	f.router.Get("/worlds/{worldID}/provinces/{provinceID}/buildings", ph.Buildings)

	// Insert the built row directly (province_mine_catchment_test.go's
	// pattern) rather than going through POST .../build — that flow only
	// queues the build, completed later by the async event worker; this
	// test is about how Buildings READS a built row, not the build verb.
	if _, err := f.pool.Exec(context.Background(),
		`INSERT INTO buildings (settlement_id, building_type, level, hex_q, hex_r) VALUES ($1, 'farm', 1, 1, 0)`,
		f.settlementID,
	); err != nil {
		t.Fatalf("insert built farm: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/worlds/"+f.worldID.String()+"/provinces/"+f.provinceID.String()+"/buildings", nil)
	req.Header.Set("Authorization", "Bearer "+f.accessToken)
	rec := httptest.NewRecorder()
	f.router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("Buildings = %d: %s", rec.Code, rec.Body.String())
	}

	var built []struct {
		Type         string          `json:"type"`
		CurrentLevel int             `json:"current_level"`
		HexQ         *int            `json:"hex_q"`
		HexR         *int            `json:"hex_r"`
		Effects      []effectRowJSON `json:"effects"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &built); err != nil {
		t.Fatalf("parse Buildings response: %v", err)
	}

	var farm *struct {
		Type         string          `json:"type"`
		CurrentLevel int             `json:"current_level"`
		HexQ         *int            `json:"hex_q"`
		HexR         *int            `json:"hex_r"`
		Effects      []effectRowJSON `json:"effects"`
	}
	for i := range built {
		if built[i].Type == "farm" {
			farm = &built[i]
		}
	}
	if farm == nil {
		t.Fatalf("no farm in built buildings list: %+v", built)
	}
	if farm.HexQ == nil || *farm.HexQ != 1 || farm.HexR == nil || *farm.HexR != 0 {
		t.Fatalf("farm hex = (%v,%v), want (1,0)", farm.HexQ, farm.HexR)
	}
	if farm.CurrentLevel != 1 {
		t.Fatalf("farm current_level = %d, want 1", farm.CurrentLevel)
	}
	var grain *effectRowJSON
	for i := range farm.Effects {
		if farm.Effects[i].Good == "grain" {
			grain = &farm.Effects[i]
		}
	}
	if grain == nil {
		t.Fatalf("no grain effect row on the built farm: %+v", farm.Effects)
	}
	if grain.Terrain != "plains" {
		t.Errorf("built farm's grain effect terrain = %q, want plains (its real hex)", grain.Terrain)
	}
}
