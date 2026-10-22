package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"formatet/megaron/server/internal/auth"
	"formatet/megaron/server/internal/clock"
	"formatet/megaron/server/internal/events"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

// A saved weighted route puts the Host at (11,0), while stored q/r still
// identify departure (0,0). Single candidates make the random tiebreak inert.
func TestJoinSpawnUsesPhysicalHostClearance(t *testing.T) {
	for _, tc := range []struct {
		name     string
		q, want  int
		inactive bool
	}{
		{"departed_ground_is_free", 2, http.StatusCreated, false},
		{"physical_ground_is_occupied", 11, http.StatusConflict, false},
		{"four_hex_boundary_is_occupied", 15, http.StatusConflict, false},
		{"five_hex_clearance_is_free", 16, http.StatusCreated, false},
		{"inactive_phase_does_not_occupy", 11, http.StatusCreated, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			pool := citiesTestPool(t)
			ctx := context.Background()
			wid := seedJoinableWorld(t, pool)
			now := time.Date(2026, 10, 22, 12, 0, 0, 0, time.UTC)
			clk := clock.NewTestClock(now)
			svc := auth.NewService(pool, "test-secret")
			router := joinCultureRouter(pool, svc, clk)
			owner, token := registerViewer(t, ctx, svc, "moving-spawn-owner")
			post := func(token string) *httptest.ResponseRecorder {
				req := httptest.NewRequest("POST", "/worlds/"+wid.String()+"/join", strings.NewReader("{}"))
				req.Header.Set("Authorization", "Bearer "+token)
				rec := httptest.NewRecorder()
				router.ServeHTTP(rec, req)
				return rec
			}
			first := post(token)
			if first.Code != 201 {
				t.Fatalf("first join: %d %s", first.Code, first.Body)
			}
			var host uuid.UUID
			if err := pool.QueryRow(ctx, `SELECT host_unit_id FROM founder_phase WHERE world_id=$1 AND owner_id=$2`, wid, owner).Scan(&host); err != nil {
				t.Fatal(err)
			}
			route := spawnWeightedHostRoute(t)
			if _, err := pool.Exec(ctx, `UPDATE worlds SET current_tick=3,last_tick_at=$2 WHERE id=$1`, wid, now); err != nil {
				t.Fatal(err)
			}
			if _, err := pool.Exec(ctx, `UPDATE units SET status='marching',target_q=12,target_r=0,depart_tick=0,arrive_tick=6,march_route=$2,departs_at=$3,arrives_at=$4 WHERE id=$1`, host, route, now.Add(-3*time.Hour), now.Add(3*time.Hour)); err != nil {
				t.Fatal(err)
			}
			physical, err := loadPhysicalHost(ctx, pool, wid, owner, now, false)
			if err != nil || physical.Position.Q != 11 || physical.Position.R != 0 {
				t.Fatalf("physical Host %+v: %v", physical, err)
			}
			if _, err := pool.Exec(ctx, `UPDATE map_tiles SET terrain='mountain_red' WHERE world_id=$1`, wid); err != nil {
				t.Fatal(err)
			}
			if _, err := pool.Exec(ctx, `INSERT INTO map_tiles(world_id,q,r,terrain) VALUES($1,$2,0,'plains')`, wid, tc.q); err != nil {
				t.Fatal(err)
			}
			if tc.inactive {
				if _, err := pool.Exec(ctx, `UPDATE founder_phase SET active=false WHERE host_unit_id=$1`, host); err != nil {
					t.Fatal(err)
				}
			}
			_, newtoken := registerViewer(t, ctx, svc, "moving-spawn-joiner")
			rec := post(newtoken)
			if rec.Code != tc.want {
				t.Fatalf("join while Host physically at (11,0), candidate (%d,0): got %d %s; want %d", tc.q, rec.Code, rec.Body, tc.want)
			}
			var unchanged bool
			if err := pool.QueryRow(ctx, `SELECT q=0 AND r=0 AND march_route=$2::jsonb AND depart_tick=0 AND arrive_tick=6 FROM units WHERE id=$1`, host, route).Scan(&unchanged); err != nil || !unchanged {
				t.Fatalf("spawn changed saved movement: %v %v", unchanged, err)
			}
		})
	}
}

func spawnWeightedHostRoute(t *testing.T) string {
	t.Helper()
	hexes := [][2]int{}
	costs := []int64{}
	for q := 0; q <= 12; q++ {
		hexes = append(hexes, [2]int{q, 0})
		if q > 0 {
			cost := int64(100)
			if q >= 10 {
				cost = 3000
			}
			costs = append(costs, cost)
		}
	}
	raw, err := json.Marshal(map[string]any{"start_tick": 0, "end_tick": 6, "hexes": hexes, "costs": costs})
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func TestJoinSpawnBalancesPhysicalHostLandmass(t *testing.T) {
	pool := citiesTestPool(t)
	ctx := context.Background()
	wid := seedJoinableWorld(t, pool)
	now := time.Date(2026, 10, 22, 12, 0, 0, 0, time.UTC)
	clk := clock.NewTestClock(now)
	svc := auth.NewService(pool, "test-secret")
	owner, _ := registerViewer(t, ctx, svc, "landmass-moving-host")
	var host uuid.UUID
	if err := pool.QueryRow(ctx, `INSERT INTO units(world_id,owner_id,type,category,size,status,q,r,target_q,target_r,depart_tick,arrive_tick,march_route) VALUES($1,$2,'nomadic_host','land',1,'marching',0,0,12,0,0,6,$3) RETURNING id`, wid, owner, spawnWeightedHostRoute(t)).Scan(&host); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO founder_phase(world_id,owner_id,host_unit_id,population,grain_amount,grain_rate,silver_amount,silver_rate) VALUES($1,$2,$3,1000,100,0,100,0)`, wid, owner, host); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE worlds SET current_tick=3,last_tick_at=$2 WHERE id=$1`, wid, now); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE map_tiles SET terrain='mountain_red',landmass_id=1 WHERE world_id=$1`, wid); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO map_tiles(world_id,q,r,terrain,landmass_id) VALUES($1,11,0,'mountain_red',2),($1,0,20,'forest_olive_grove',1),($1,20,20,'forest_olive_grove',2)`, wid); err != nil {
		t.Fatal(err)
	}
	_, token := registerViewer(t, ctx, svc, "landmass-new-host")
	r := joinCultureRouter(pool, svc, clk)
	req := httptest.NewRequest("POST", "/worlds/"+wid.String()+"/join", strings.NewReader("{}"))
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != 201 {
		t.Fatalf("join %d %s", rec.Code, rec.Body)
	}
	var body struct {
		Tile struct{ Q, R int } `json:"tile"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Tile.Q != 0 || body.Tile.R != 20 {
		t.Fatalf("spawn (%d,%d), want unloaded departure landmass1 at (0,20); Host actually occupies landmass2", body.Tile.Q, body.Tile.R)
	}
}

// Rise and Join share the same picker; exercise the authenticated refugee
// entrypoint too, including fallback when the only free ground is on the fallen landmass.
func TestRiseSpawnClearsPhysicalHost(t *testing.T) {
	pool := citiesTestPool(t)
	ctx := context.Background()
	wid := seedJoinableWorld(t, pool)
	now := time.Date(2026, 10, 22, 12, 0, 0, 0, time.UTC)
	svc := auth.NewService(pool, "test-secret")
	owner, _ := registerViewer(t, ctx, svc, "rise-moving-host")
	var host uuid.UUID
	if err := pool.QueryRow(ctx, `INSERT INTO units(world_id,owner_id,type,category,size,status,q,r,target_q,target_r,depart_tick,arrive_tick,march_route) VALUES($1,$2,'nomadic_host','land',1,'marching',0,0,12,0,0,6,$3) RETURNING id`, wid, owner, spawnWeightedHostRoute(t)).Scan(&host); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO founder_phase(world_id,owner_id,host_unit_id,population,grain_amount,grain_rate,silver_amount,silver_rate) VALUES($1,$2,$3,1000,100,0,100,0)`, wid, owner, host); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE worlds SET current_tick=3,last_tick_at=$2 WHERE id=$1`, wid, now); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE map_tiles SET terrain='mountain_red' WHERE world_id=$1`, wid); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO map_tiles(world_id,q,r,terrain) VALUES($1,2,0,'forest_olive_grove')`, wid); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE map_tiles SET landmass_id=1 WHERE world_id=$1 AND q=2`, wid); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO map_tiles(world_id,q,r,terrain,landmass_id) VALUES($1,100,100,'mountain_red',1)`, wid); err != nil {
		t.Fatal(err)
	}
	var fallenProv, fallen uuid.UUID
	if err := pool.QueryRow(ctx, `INSERT INTO provinces(world_id,map_q,map_r) VALUES($1,100,100) RETURNING id`, wid).Scan(&fallenProv); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO settlements(world_id,province_id,name,culture_id,state,population) VALUES($1,$2,'Fallen','minoan','collapsed',0) RETURNING id`, wid, fallenProv).Scan(&fallen); err != nil {
		t.Fatal(err)
	}
	player, token := registerViewer(t, ctx, svc, "rising-new-host")
	if _, err := pool.Exec(ctx, `INSERT INTO player_world_records(player_id,world_id,status,last_settlement_id) VALUES($1,$2,'dispossessed',$3)`, player, wid, fallen); err != nil {
		t.Fatal(err)
	}
	h := NewJoinHandler(pool, events.NewStore(pool), clock.NewTestClock(now), nil)
	r := chi.NewRouter()
	r.Use(auth.Middleware(svc))
	r.Post("/worlds/{worldID}/rise", h.Rise)
	req := httptest.NewRequest("POST", "/worlds/"+wid.String()+"/rise", strings.NewReader("{}"))
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != 201 {
		t.Fatalf("rise on departed ground %d %s, want201", rec.Code, rec.Body)
	}
}
