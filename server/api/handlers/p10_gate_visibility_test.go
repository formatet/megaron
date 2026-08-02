package handlers

// Regression tests for P10 (soak 2026-07-18, fixlista §P10 "Dolda gates syns
// bara vid misslyckat försök"): several build/recruit gates were only ever
// revealed by a 422 AFTER a failed attempt. Two gates were genuinely missing
// from the pre-flight catalogue (the rest — harbour/coastal, mine/deposit,
// rite/kharis, war_chariot+elite_infantry/bronze — were already fully
// visible on inspection, see the soak-fix report):
//
//   - winery's ENTIRE production is gated to a hills/plains/scrub_maquis tile
//     in catchment (mig 103 — every production_rules row for winery names a
//     terrain, no NULL-terrain fallback) — built off those terrains it
//     silently produces nothing. Now exposed as BuildingCatalogue's
//     requires_terrain field.
//   - the build queue's concurrent-slot cap (province.go's maxParallelBuilds)
//     only ever appeared in the 422 refusing a 3rd build. Now exposed as
//     province Get's build_queue_max field, alongside the existing
//     build_queue array.

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"formatet/megaron/server/internal/clock"
	"formatet/megaron/server/internal/economy"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func p10TestPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL not set — skipping DB integration test")
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatalf("connect to test database: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

// TestBuildingCatalogue_ExposesTerrainGate reads production_rules straight
// from the DB (no world/settlement needed — BuildingCatalogue is static
// reference data), so it only needs a live DB, not a seeded world.
func TestBuildingCatalogue_ExposesTerrainGate(t *testing.T) {
	pool := p10TestPool(t)

	ph := NewProvinceHandler(pool, nil, clock.NewTestClock(time.Now()), economy.SitosConfig{}, nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/buildings", nil)
	rec := httptest.NewRecorder()
	ph.BuildingCatalogue(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("BuildingCatalogue = %d: %s", rec.Code, rec.Body.String())
	}

	var catalogue []struct {
		Type            string   `json:"type"`
		RequiresTerrain []string `json:"requires_terrain"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &catalogue); err != nil {
		t.Fatalf("parse BuildingCatalogue response: %v", err)
	}

	byType := map[string][]string{}
	for _, b := range catalogue {
		byType[b.Type] = b.RequiresTerrain
	}

	// mig 103 (druvor-utanfor-hills) + mig 105 (vin i floddalen, Timothy
	// 2026-08-02): winery's rows now name hills, plains, river_valley and
	// scrub_maquis — still genuinely terrain-gated (no NULL-terrain fallback),
	// just no longer hills-only. Alphabetical, per BuildingCatalogue's sort.
	// requires_terrain is DERIVED from production_rules (province.go:1311), so
	// adding a wine rule for a terrain also makes the winery buildable there —
	// this list must follow every wine-terrain migration.
	wantWinery := []string{"hills", "plains", "river_valley", "scrub_maquis"}
	winery, ok := byType["winery"]
	if !ok {
		t.Fatal("no winery entry in building catalogue")
	}
	if len(winery) != len(wantWinery) {
		t.Errorf("winery requires_terrain = %v, want %v", winery, wantWinery)
	} else {
		for i := range wantWinery {
			if winery[i] != wantWinery[i] {
				t.Errorf("winery requires_terrain = %v, want %v", winery, wantWinery)
				break
			}
		}
	}

	// farm ÄR terräng-gatead, tvärtemot vad det här testet ursprungligen
	// påstod. Kontrollerat mot migration 008 (där reglerna föddes) och mot en
	// färdigmigrerad DB 2026-07-26: farm har ALDRIG haft en NULL-terrängrad.
	// Alla fem reglerna namnger en terräng, så en farm på kalksten eller i en
	// olivlund producerar exakt ingenting. Att katalogen flaggar det är precis
	// den dolda gaten P10 finns till för att stänga — inte ett fel.
	wantFarm := []string{"hills", "plains", "river_delta", "river_valley"}
	farm, ok := byType["farm"]
	if !ok {
		t.Fatal("no farm entry in building catalogue")
	}
	if len(farm) != len(wantFarm) {
		t.Errorf("farm requires_terrain = %v, want %v", farm, wantFarm)
	} else {
		for i := range wantFarm {
			if farm[i] != wantFarm[i] {
				t.Errorf("farm requires_terrain = %v, want %v", farm, wantFarm)
				break
			}
		}
	}

	// Kontrollen åt andra hållet: lumbermill HAR terrängfria basrader vid sidan
	// av sin skogsbonus, producerar alltså något överallt, och får därför inte
	// flaggas. Utan den här raden skulle testet passera även om katalogen
	// flaggade varenda byggnad.
	if lm, ok := byType["lumbermill"]; ok && len(lm) > 0 {
		t.Errorf("lumbermill requires_terrain = %v, want none — den har terrängfria basrader "+
			"och producerar något på varje terräng", lm)
	}
}

// TestProvinceGet_ExposesBuildQueueMax seeds a minimal settlement and checks
// that Get's response carries build_queue_max alongside build_queue, so a
// player can see "how many slots do I have" without first hitting the 422.
func TestProvinceGet_ExposesBuildQueueMax(t *testing.T) {
	pool := p10TestPool(t)
	ctx := context.Background()

	if _, err := pool.Exec(ctx,
		`UPDATE worlds SET status = 'archived' WHERE status = 'active' AND name LIKE 'test-world-%'`,
	); err != nil {
		t.Fatalf("archive leftover active test worlds: %v", err)
	}
	var worldID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO worlds (name, status, current_tick) VALUES ($1, 'active', 100) RETURNING id`,
		"test-world-"+uuid.New().String(),
	).Scan(&worldID); err != nil {
		t.Fatalf("create test world: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `UPDATE worlds SET status = 'archived' WHERE id = $1`, worldID)
	})

	var ownerID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO players (username, email, password_hash) VALUES ($1, $2, 'x') RETURNING id`,
		"p10-"+uuid.New().String(), "p10-"+uuid.New().String()+"@test.invalid",
	).Scan(&ownerID); err != nil {
		t.Fatalf("create test player: %v", err)
	}

	var provinceID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO provinces (world_id, map_q, map_r, terrain_type) VALUES ($1, 0, 0, 'plains') RETURNING id`,
		worldID,
	).Scan(&provinceID); err != nil {
		t.Fatalf("create province: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO settlements (world_id, province_id, name, culture_id, owner_id, control_type, is_capital)
		 VALUES ($1, $2, 'P10ton', 'achaean', $3, 'capital', true)`,
		worldID, provinceID, ownerID,
	); err != nil {
		t.Fatalf("create settlement: %v", err)
	}

	ph := NewProvinceHandler(pool, nil, clock.NewTestClock(time.Now()), economy.SitosConfig{}, nil, nil)
	r := chi.NewRouter()
	r.Get("/worlds/{worldID}/provinces/{provinceID}", ph.Get)

	req := httptest.NewRequest(http.MethodGet, "/worlds/"+worldID.String()+"/provinces/"+provinceID.String(), nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("Get = %d: %s", rec.Code, rec.Body.String())
	}

	var resp struct {
		Settlement struct {
			BuildQueue    []any `json:"build_queue"`
			BuildQueueMax *int  `json:"build_queue_max"`
		} `json:"settlement"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("parse Get response: %v", err)
	}
	if resp.Settlement.BuildQueueMax == nil {
		t.Fatal("settlement.build_queue_max missing from Get response")
	}
	if *resp.Settlement.BuildQueueMax != maxParallelBuilds {
		t.Errorf("settlement.build_queue_max = %d, want %d (maxParallelBuilds)", *resp.Settlement.BuildQueueMax, maxParallelBuilds)
	}
}
