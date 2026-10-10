package handlers

// GET /worlds/{worldID}/storms (megaron_plan_stormar.md): a storm is live only inside the
// Wanax's sight, is remembered where last seen afterwards, and its true position never
// leaks outside sight.

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"formatet/megaron/server/internal/auth"
	"formatet/megaron/server/internal/clock"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type stormsResp struct {
	Tick   int `json:"tick"`
	Storms []struct {
		ID       uuid.UUID `json:"id"`
		Tier     string    `json:"tier"`
		Heading  string    `json:"heading"`
		SeenTick int       `json:"seen_tick"`
		Hexes    []struct {
			Q int `json:"q"`
			R int `json:"r"`
		} `json:"hexes"`
	} `json:"storms"`
}

func stormsRig(t *testing.T) (*pgxpool.Pool, uuid.UUID, uuid.UUID, string, func() stormsResp) {
	t.Helper()
	pool := citiesTestPool(t)
	ctx := context.Background()
	var worldID uuid.UUID
	if err := pool.QueryRow(ctx, `INSERT INTO worlds (name, status, current_tick) VALUES ($1, 'archived', 7) RETURNING id`,
		"test-world-"+uuid.New().String()).Scan(&worldID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = pool.Exec(ctx, `DELETE FROM worlds WHERE id = $1`, worldID) })
	authSvc := auth.NewService(pool, "test-secret")
	viewerID, token := registerViewer(t, ctx, authSvc, "storm-viewer")
	var prov uuid.UUID
	if err := pool.QueryRow(ctx, `INSERT INTO provinces (world_id, map_q, map_r, terrain_type) VALUES ($1, 0, 0, 'plains') RETURNING id`, worldID).Scan(&prov); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO settlements (world_id, province_id, name, culture_id, owner_id, control_type, is_capital)
		 VALUES ($1, $2, 'Viewerton', 'achaean', $3, 'capital', true)`, worldID, prov, viewerID); err != nil {
		t.Fatal(err)
	}
	wh := NewWorldHandler(pool, authSvc, clock.NewTestClock(time.Now()))
	r := chi.NewRouter()
	r.Use(auth.Middleware(authSvc))
	r.Get("/worlds/{worldID}/storms", wh.Storms)
	get := func() stormsResp {
		req := httptest.NewRequest(http.MethodGet, "/worlds/"+worldID.String()+"/storms", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("GET /storms = %d %q", rec.Code, rec.Body.String())
		}
		var out stormsResp
		if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
			t.Fatal(err)
		}
		return out
	}
	return pool, worldID, viewerID, token, get
}

// seedStorm stores a storm whose current (tick 7) shape is hexes, on coastal_sea tiles.
func seedStorm(t *testing.T, pool *pgxpool.Pool, worldID uuid.UUID, heading int, hexes [3][2]int) uuid.UUID {
	t.Helper()
	ctx := context.Background()
	var id uuid.UUID
	if err := pool.QueryRow(ctx, `INSERT INTO sea_storms (world_id, heading, created_tick) VALUES ($1, $2, 0) RETURNING id`, worldID, heading).Scan(&id); err != nil {
		t.Fatal(err)
	}
	for slot, hx := range hexes {
		if _, err := pool.Exec(ctx, `INSERT INTO map_tiles (world_id, q, r, terrain) VALUES ($1, $2, $3, 'coastal_sea') ON CONFLICT DO NOTHING`, worldID, hx[0], hx[1]); err != nil {
			t.Fatal(err)
		}
		if _, err := pool.Exec(ctx, `INSERT INTO sea_storm_track (storm_id, tick, slot, q, r) VALUES ($1, 7, $2, $3, $4)`, id, slot, hx[0], hx[1]); err != nil {
			t.Fatal(err)
		}
	}
	return id
}

func TestStorms_LiveOnlyInSightAndFarStormNeverLeaks(t *testing.T) {
	pool, worldID, _, _, get := stormsRig(t)
	near := seedStorm(t, pool, worldID, 1, [3][2]int{{2, 0}, {3, 0}, {2, 1}})
	far := seedStorm(t, pool, worldID, 0, [3][2]int{{12, 0}, {13, 0}, {12, 1}})

	got := get()
	if len(got.Storms) != 1 || got.Storms[0].ID != near || got.Storms[0].Tier != "live" {
		t.Fatalf("storms = %+v, want only the near storm, live (the far storm %s is out of sight)", got.Storms, far)
	}
	if got.Storms[0].Heading != "NE" || got.Storms[0].SeenTick != 7 || len(got.Storms[0].Hexes) != 3 {
		t.Fatalf("live storm = %+v, want heading NE, seen_tick 7, three hexes", got.Storms[0])
	}
	var n int
	_ = pool.QueryRow(context.Background(), `SELECT count(*) FROM player_storm_sightings WHERE storm_id = $1`, near).Scan(&n)
	if n != 1 {
		t.Fatalf("a storm seen live must be remembered: sightings = %d", n)
	}
}

// Out of sight, a storm shows where it was last seen, marked remembered — never where it is.
func TestStorms_RememberedWhereLastSeenNotWhereItIs(t *testing.T) {
	pool, worldID, viewerID, _, get := stormsRig(t)
	id := seedStorm(t, pool, worldID, 0, [3][2]int{{20, 5}, {21, 5}, {20, 6}}) // where it is now
	if _, err := pool.Exec(context.Background(),
		`INSERT INTO player_storm_sightings (player_id, storm_id, seen_tick, hexes) VALUES ($1, $2, 3, '[{"q":12,"r":0},{"q":13,"r":0},{"q":12,"r":1}]')`,
		viewerID, id); err != nil {
		t.Fatal(err)
	}
	got := get()
	if len(got.Storms) != 1 || got.Storms[0].Tier != "remembered" || got.Storms[0].SeenTick != 3 {
		t.Fatalf("storms = %+v, want one remembered storm last seen at tick 3", got.Storms)
	}
	if h := got.Storms[0].Hexes[0]; h.Q != 12 || h.R != 0 {
		t.Fatalf("remembered storm shows %+v, want the last-seen hex (12,0), not its true position", h)
	}
	if got.Storms[0].Heading != "" {
		t.Fatalf("a remembered storm must not tell its heading, got %q", got.Storms[0].Heading)
	}
}

// Looking at the place where you last saw it and finding it gone: it has moved on, forget it.
func TestStorms_ForgottenWhenItsRememberedPlaceIsInSightAndEmpty(t *testing.T) {
	pool, worldID, viewerID, _, get := stormsRig(t)
	id := seedStorm(t, pool, worldID, 0, [3][2]int{{20, 5}, {21, 5}, {20, 6}})
	if _, err := pool.Exec(context.Background(),
		`INSERT INTO map_tiles (world_id, q, r, terrain) VALUES ($1, 1, 0, 'coastal_sea'), ($1, 2, 0, 'coastal_sea'), ($1, 1, 1, 'coastal_sea') ON CONFLICT DO NOTHING`, worldID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(context.Background(),
		`INSERT INTO player_storm_sightings (player_id, storm_id, seen_tick, hexes) VALUES ($1, $2, 3, '[{"q":1,"r":0},{"q":2,"r":0},{"q":1,"r":1}]')`,
		viewerID, id); err != nil {
		t.Fatal(err)
	}
	if got := get(); len(got.Storms) != 0 {
		t.Fatalf("storms = %+v, want none: the remembered place is in sight and empty", got.Storms)
	}
	var n int
	_ = pool.QueryRow(context.Background(), `SELECT count(*) FROM player_storm_sightings WHERE storm_id = $1`, id).Scan(&n)
	if n != 0 {
		t.Fatalf("the stale memory must be deleted, %d rows left", n)
	}
}
