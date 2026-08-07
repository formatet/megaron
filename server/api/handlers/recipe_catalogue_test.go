package handlers

// Test for RecipeCatalogue (GET /api/v1/recipes) — added alongside the new
// endpoint so the web client can stop hardcoding recipe strings (bronze's
// copper:tin ratio changed silently under it once already, mig 099,
// 2026-07-25). Asserts the handler's response against the DB's own
// recipes/recipe_ingredients rows rather than hardcoding expected numbers —
// a numeric assertion here would go stale exactly the way the client's
// hardcoded string did the moment a future migration changes a recipe again.
// Uses p10TestPool (p10_gate_visibility_test.go) — same DATABASE_URL-gated
// skip as BuildingCatalogue's own test, no world/settlement needed since
// recipes are static reference data.

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sort"
	"testing"
	"time"

	"formatet/megaron/server/internal/clock"
	"formatet/megaron/server/internal/economy"
)

func TestRecipeCatalogue_MatchesDB(t *testing.T) {
	pool := p10TestPool(t)
	ctx := context.Background()

	ph := NewProvinceHandler(pool, nil, clock.NewTestClock(time.Now()), economy.SitosConfig{}, nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/recipes", nil)
	rec := httptest.NewRecorder()
	ph.RecipeCatalogue(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("RecipeCatalogue = %d: %s", rec.Code, rec.Body.String())
	}

	type ingredient struct {
		GoodKey  string  `json:"good_key"`
		Quantity float64 `json:"quantity"`
	}
	type recipe struct {
		ID           int          `json:"id"`
		OutputKey    string       `json:"output_key"`
		OutputQty    float64      `json:"output_qty"`
		BuildingType string       `json:"building_type"`
		Ingredients  []ingredient `json:"ingredients"`
	}
	var got []recipe
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("parse RecipeCatalogue response: %v", err)
	}
	if len(got) == 0 {
		t.Fatal("RecipeCatalogue returned zero recipes — expected at least the seeded bronze recipe")
	}

	// Cross-check against the DB directly: same rows, no client-side drift.
	rows, err := pool.Query(ctx, `SELECT id, output_key, output_qty, building_type FROM recipes ORDER BY id`)
	if err != nil {
		t.Fatalf("query recipes: %v", err)
	}
	defer rows.Close()
	want := map[int]recipe{}
	var wantOrder []int
	for rows.Next() {
		var rcp recipe
		if err := rows.Scan(&rcp.ID, &rcp.OutputKey, &rcp.OutputQty, &rcp.BuildingType); err != nil {
			t.Fatalf("scan recipe row: %v", err)
		}
		want[rcp.ID] = rcp
		wantOrder = append(wantOrder, rcp.ID)
	}
	if len(got) != len(want) {
		t.Fatalf("RecipeCatalogue returned %d recipes, DB has %d", len(got), len(want))
	}

	for i, g := range got {
		if i >= len(wantOrder) || g.ID != wantOrder[i] {
			t.Fatalf("recipe order mismatch at index %d: got id=%d", i, g.ID)
		}
		w := want[g.ID]
		if g.OutputKey != w.OutputKey || g.OutputQty != w.OutputQty || g.BuildingType != w.BuildingType {
			t.Errorf("recipe %d = %+v, want output/building fields matching DB row %+v", g.ID, g, w)
		}

		ingRows, err := pool.Query(ctx,
			`SELECT good_key, quantity FROM recipe_ingredients WHERE recipe_id = $1 ORDER BY good_key`, g.ID)
		if err != nil {
			t.Fatalf("query ingredients for recipe %d: %v", g.ID, err)
		}
		var wantIng []ingredient
		for ingRows.Next() {
			var ing ingredient
			if err := ingRows.Scan(&ing.GoodKey, &ing.Quantity); err != nil {
				ingRows.Close()
				t.Fatalf("scan ingredient row: %v", err)
			}
			wantIng = append(wantIng, ing)
		}
		ingRows.Close()

		gotIng := append([]ingredient{}, g.Ingredients...)
		sort.Slice(gotIng, func(a, b int) bool { return gotIng[a].GoodKey < gotIng[b].GoodKey })
		if len(gotIng) != len(wantIng) {
			t.Fatalf("recipe %d has %d ingredients in response, %d in DB", g.ID, len(gotIng), len(wantIng))
		}
		for j := range wantIng {
			if gotIng[j] != wantIng[j] {
				t.Errorf("recipe %d ingredient[%d] = %+v, want %+v", g.ID, j, gotIng[j], wantIng[j])
			}
		}
	}

	// The bronze recipe must exist, live in a foundry, and require both
	// copper and tin — the specific exchange rate is deliberately NOT
	// asserted here (it's a tunable; mig 099 already changed it once); only
	// the SHAPE (which goods gate it, which building) is a stable contract.
	var bronze *recipe
	for i := range got {
		if got[i].OutputKey == "bronze" {
			bronze = &got[i]
			break
		}
	}
	if bronze == nil {
		t.Fatal("no bronze recipe in catalogue")
	}
	if bronze.BuildingType != "foundry" {
		t.Errorf("bronze recipe building_type = %q, want foundry", bronze.BuildingType)
	}
	goods := map[string]bool{}
	for _, ing := range bronze.Ingredients {
		goods[ing.GoodKey] = true
	}
	if !goods["copper"] || !goods["tin"] {
		t.Errorf("bronze recipe ingredients = %v, want copper and tin present", bronze.Ingredients)
	}
}

// TestRecipeCatalogue_LuxuryRemoved: mig 118 (megaron_plan_varukatalogen.md
// S4) removed luxury entirely, not parked it — the catalogue must return
// ONLY the bronze recipe, and no goods row should exist for luxury either.
func TestRecipeCatalogue_LuxuryRemoved(t *testing.T) {
	pool := p10TestPool(t)
	ctx := context.Background()

	ph := NewProvinceHandler(pool, nil, clock.NewTestClock(time.Now()), economy.SitosConfig{}, nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/recipes", nil)
	rec := httptest.NewRecorder()
	ph.RecipeCatalogue(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("RecipeCatalogue = %d: %s", rec.Code, rec.Body.String())
	}

	type recipe struct {
		OutputKey string `json:"output_key"`
	}
	var got []recipe
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("parse RecipeCatalogue response: %v", err)
	}
	if len(got) != 1 || got[0].OutputKey != "bronze" {
		t.Errorf("RecipeCatalogue = %v, want exactly one recipe (bronze)", got)
	}

	var luxuryGoodExists bool
	if err := pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM goods WHERE key = 'luxury')`).Scan(&luxuryGoodExists); err != nil {
		t.Fatalf("query goods for luxury: %v", err)
	}
	if luxuryGoodExists {
		t.Error("goods table still has a row for luxury after S4")
	}
}
