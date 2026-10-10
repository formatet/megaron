package handlers

// Rise and Leave (megaron_sista_staden.md, Timothy 2026-10-10): a Wanax whose
// last city fell rises again as a small host on ANOTHER landmass, or leaves
// the world. The fixture makes the fallen landmass the least loaded one, so
// join's own landmass balance would put the refugee straight back on it — only
// the exclusion moves them. DB integration test (real Postgres, DATABASE_URL).

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"formatet/megaron/server/internal/auth"
	"formatet/megaron/server/internal/clock"
	"formatet/megaron/server/internal/combat"
	"formatet/megaron/server/internal/economy"
	"formatet/megaron/server/internal/events"
	"formatet/megaron/server/internal/unit"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

func TestRise_RefugeeHostOnAnotherLandmass_AndLeave(t *testing.T) {
	pool := citiesTestPool(t)
	ctx := context.Background()

	if _, err := pool.Exec(ctx, `UPDATE worlds SET status = 'archived' WHERE status = 'active'`); err != nil {
		t.Fatalf("archive leftover active worlds: %v", err)
	}
	var worldID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO worlds (name, map_width, map_height) VALUES ($1, 12, 100) RETURNING id`,
		"test-rise-"+uuid.New().String(),
	).Scan(&worldID); err != nil {
		t.Fatalf("create world: %v", err)
	}
	t.Cleanup(func() { _, _ = pool.Exec(ctx, `DELETE FROM worlds WHERE id = $1`, worldID) })

	// Landmass 1 holds the fallen city (load 1); landmass 2 holds two other
	// provinces (load 2) and two free tiles far from them.
	for _, tl := range []struct {
		q, r     int
		terrain  string
		landmass int
	}{
		{0, 0, "forest_olive_grove", 1}, {0, 20, "plains", 1}, {0, 21, "forest_olive_grove", 1},
		{0, 40, "forest_olive_grove", 2}, {0, 41, "plains", 2}, {0, 50, "plains", 2}, {0, 52, "plains", 2},
	} {
		if _, err := pool.Exec(ctx,
			`INSERT INTO map_tiles (world_id, q, r, terrain, landmass_id) VALUES ($1, $2, $3, $4, $5)`,
			worldID, tl.q, tl.r, tl.terrain, tl.landmass,
		); err != nil {
			t.Fatalf("seed tile (%d,%d): %v", tl.q, tl.r, err)
		}
	}
	mkProvince := func(q, r int) uuid.UUID {
		var id uuid.UUID
		if err := pool.QueryRow(ctx,
			`INSERT INTO provinces (world_id, map_q, map_r) VALUES ($1, $2, $3) RETURNING id`,
			worldID, q, r,
		).Scan(&id); err != nil {
			t.Fatalf("create province (%d,%d): %v", q, r, err)
		}
		return id
	}
	fallenProv := mkProvince(0, 0)
	mkProvince(0, 50)
	mkProvince(0, 52)
	var fallen uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO settlements (world_id, province_id, name, culture_id, state, population)
		 VALUES ($1, $2, 'Tiryns', 'minoan', 'collapsed', 0) RETURNING id`,
		worldID, fallenProv,
	).Scan(&fallen); err != nil {
		t.Fatalf("create fallen settlement: %v", err)
	}

	authSvc := auth.NewService(pool, "test-secret")
	jh := NewJoinHandler(pool, events.NewStore(pool), economy.LoadSitosConfig(), clock.NewTestClock(time.Now()), nil)
	r := chi.NewRouter()
	r.Use(auth.Middleware(authSvc))
	r.Post("/worlds/{worldID}/rise", jh.Rise)
	r.Post("/worlds/{worldID}/leave", jh.Leave)
	post := func(token, verb string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/worlds/"+worldID.String()+"/"+verb, strings.NewReader("{}"))
		req.Header.Set("Authorization", "Bearer "+token)
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		return rec
	}
	dispossessed := func(prefix string) (uuid.UUID, string) {
		playerID, token := registerViewer(t, ctx, authSvc, prefix)
		if _, err := pool.Exec(ctx,
			`INSERT INTO player_world_records (player_id, world_id, status, last_settlement_id)
			 VALUES ($1, $2, 'dispossessed', $3)`,
			playerID, worldID, fallen,
		); err != nil {
			t.Fatalf("record dispossessed %s: %v", prefix, err)
		}
		return playerID, token
	}

	saved := riseIntn
	riseIntn = func(n int) int { return n - 1 } // the largest roll: 8 gubbar
	t.Cleanup(func() { riseIntn = saved })

	// ── Rise ──
	risen, token := dispossessed("rise")
	if rec := post(token, "rise"); rec.Code != http.StatusCreated {
		t.Fatalf("POST /rise = %d %q, want 201", rec.Code, rec.Body.String())
	}
	var landmass, population int
	var silver float64
	if err := pool.QueryRow(ctx,
		`SELECT mt.landmass_id, fp.population, fp.silver_amount
		 FROM founder_phase fp
		 JOIN units hu ON hu.id = fp.host_unit_id
		 JOIN map_tiles mt ON mt.world_id = hu.world_id AND mt.q = hu.q AND mt.r = hu.r
		 WHERE fp.world_id = $1 AND fp.owner_id = $2 AND fp.active`,
		worldID, risen,
	).Scan(&landmass, &population, &silver); err != nil {
		t.Fatalf("read risen host: %v", err)
	}
	if landmass != 2 {
		t.Errorf("risen host stands on landmass %d, want 2 (never the landmass that fell)", landmass)
	}
	if population != 800 {
		t.Errorf("risen host population = %d, want 800 (8 gubbar at the largest roll)", population)
	}
	perTick := combat.UnitUpkeep(string(unit.TypeSpearman), string(unit.CategoryLand), nomadicHostSpearmenSize, "positioned")
	if want := perTick.Silver * risenHostRationTicks; silver != want {
		t.Errorf("risen host silver = %v, want %v (one cohort's sold for %d ticks)", silver, want, risenHostRationTicks)
	}
	var spearmen int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM units WHERE world_id = $1 AND owner_id = $2 AND type = $3`,
		worldID, risen, string(unit.TypeSpearman),
	).Scan(&spearmen); err != nil || spearmen != 1 {
		t.Errorf("risen host escort = %d spearmen (err %v), want 1", spearmen, err)
	}
	var status string
	_ = pool.QueryRow(ctx, `SELECT status FROM player_world_records WHERE player_id = $1 AND world_id = $2`,
		risen, worldID).Scan(&status)
	if status != "active" {
		t.Errorf("status after rise = %q, want active", status)
	}
	if rec := post(token, "rise"); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"existing":true`) {
		t.Errorf("second POST /rise = %d %q, want 200 existing", rec.Code, rec.Body.String())
	}

	// ── Only the dispossessed may choose ──
	_, bystander := registerViewer(t, ctx, authSvc, "never-joined")
	if rec := post(bystander, "rise"); rec.Code != http.StatusConflict {
		t.Errorf("POST /rise for a player who never fell = %d, want 409", rec.Code)
	}
	if rec := post(bystander, "leave"); rec.Code != http.StatusConflict {
		t.Errorf("POST /leave for a player who never fell = %d, want 409", rec.Code)
	}

	// ── Leave, then no rising in this world ──
	leaver, leaverToken := dispossessed("leave")
	if rec := post(leaverToken, "leave"); rec.Code != http.StatusOK {
		t.Fatalf("POST /leave = %d %q, want 200", rec.Code, rec.Body.String())
	}
	_ = pool.QueryRow(ctx, `SELECT status FROM player_world_records WHERE player_id = $1 AND world_id = $2`,
		leaver, worldID).Scan(&status)
	if status != "departed" {
		t.Errorf("status after leave = %q, want departed", status)
	}
	if rec := post(leaverToken, "rise"); rec.Code != http.StatusConflict {
		t.Errorf("POST /rise after leaving = %d, want 409", rec.Code)
	}
}
