package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"formatet/megaron/server/internal/auth"
	"formatet/megaron/server/internal/clock"
	"formatet/megaron/server/internal/kharis"
	"github.com/google/uuid"
)

func TestJoinHostKharisFloorVisibleInMe(t *testing.T) {
	pool := citiesTestPool(t)
	ctx := context.Background()
	worldID := seedJoinableWorld(t, pool)
	if _, err := pool.Exec(ctx, `UPDATE worlds SET current_tick=500 WHERE id=$1`, worldID); err != nil {
		t.Fatal(err)
	}
	svc := auth.NewService(pool, "host-floor-test")
	player, token := registerViewer(t, ctx, svc, "host-floor-"+uuid.NewString())
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DELETE FROM worlds WHERE id=$1`, worldID)
		_, _ = pool.Exec(ctx, `DELETE FROM players WHERE id=$1`, player)
	})
	router := joinCultureRouter(pool, svc, clock.NewTestClock(time.Now()))
	router.Get("/worlds/{worldID}/me", NewWorldHandler(pool, svc, nil).Me)
	request := func(method, path string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, "/worlds/"+worldID.String()+path, strings.NewReader("{}"))
		req.Header.Set("Authorization", "Bearer "+token)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		return rec
	}
	if rec := request("POST", "/join"); rec.Code != 201 {
		t.Fatalf("join %d %s", rec.Code, rec.Body.String())
	}
	var amount, rate float64
	var tick int
	if err := pool.QueryRow(ctx, `SELECT kharis_amount,kharis_rate,kharis_calc_tick FROM player_world_records WHERE player_id=$1 AND world_id=$2`, player, worldID).Scan(&amount, &rate, &tick); err != nil {
		t.Fatal(err)
	}
	if amount != kharis.Floor || rate != 0 || tick != 500 {
		t.Fatalf("new Host missing shared kharis floor: amount=%v rate=%v tick=%v", amount, rate, tick)
	}
	rec := request(http.MethodGet, "/me")
	var profile WorldProfile
	if rec.Code != 200 {
		t.Fatalf("me %d %s", rec.Code, rec.Body.String())
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &profile); err != nil {
		t.Fatal(err)
	}
	if profile.Kharis != kharis.Floor || profile.FoundedTick != nil {
		t.Fatalf("Host profile must show floor before founding: %+v", profile)
	}
	// A repeat join returns the same Host; it must not seed a second life.
	if _, err := pool.Exec(ctx, `UPDATE player_world_records SET kharis_amount=$1 WHERE player_id=$2 AND world_id=$3`, kharis.Floor+4, player, worldID); err != nil {
		t.Fatal(err)
	}
	if rec := request("POST", "/join"); rec.Code != 200 {
		t.Fatalf("repeat join %d", rec.Code)
	}
	if err := pool.QueryRow(ctx, `SELECT kharis_amount FROM player_world_records WHERE player_id=$1 AND world_id=$2`, player, worldID).Scan(&amount); err != nil {
		t.Fatal(err)
	}
	if amount != kharis.Floor+4 {
		t.Fatal("repeat join reset existing Host kharis")
	}
}

func TestHostKharisFloorMigrationOnlyRepairsLivingHosts(t *testing.T) {
	pool := citiesTestPool(t)
	ctx := context.Background()
	worldID := seedJoinableWorld(t, pool)
	migration, err := os.ReadFile("../../db/migrations/174_host_kharis_floor.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := tx.Exec(ctx, sql, args...); err != nil {
			t.Fatal(err)
		}
	}
	exec(`UPDATE worlds SET current_tick=700 WHERE id=$1`, worldID)
	for _, tc := range []struct {
		name, status string
		phase, city  bool
		amount       float64
		repair       bool
	}{
		{"low-host", "active", true, false, 0, true},
		{"high-host", "active", true, false, 4, false},
		{"city", "active", true, true, 0, false},
		{"finished-phase", "active", false, false, 0, false},
		{"dispossessed", "dispossessed", true, false, 0, false},
		{"no-phase", "active", false, false, 0, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var player uuid.UUID
			if err := tx.QueryRow(ctx, `INSERT INTO players(username,password_hash)VALUES($1,'x')RETURNING id`, "host-mig-"+uuid.NewString()).Scan(&player); err != nil {
				t.Fatal(err)
			}
			exec(`INSERT INTO player_world_records(player_id,world_id,status,kharis_amount,kharis_rate,kharis_calc_tick) VALUES($1,$2,$3,$4,0.5,12)`, player, worldID, tc.status, tc.amount)
			if tc.name != "no-phase" {
				exec(`INSERT INTO founder_phase(world_id,owner_id,population,grain_amount,grain_rate,silver_amount,silver_rate,active) VALUES($1,$2,400,0,0,0,0,$3)`, worldID, player, tc.phase)
			}
			if tc.city {
				var province uuid.UUID
				if err := tx.QueryRow(ctx, `INSERT INTO provinces(world_id,map_q,map_r)VALUES($1,2,2)RETURNING id`, worldID).Scan(&province); err != nil {
					t.Fatal(err)
				}
				exec(`INSERT INTO settlements(world_id,province_id,owner_id,name,culture_id,state)VALUES($1,$2,$3,'Mycenae','minoan','active')`, worldID, province, player)
			}
			for i := 0; i < 2; i++ {
				exec(string(migration))
				var amount, rate float64
				var tick int
				if err := tx.QueryRow(ctx, `SELECT kharis_amount,kharis_rate,kharis_calc_tick FROM player_world_records WHERE player_id=$1 AND world_id=$2`, player, worldID).Scan(&amount, &rate, &tick); err != nil {
					t.Fatal(err)
				}
				wantAmount, wantRate, wantTick := tc.amount, 0.5, 12
				if tc.repair {
					wantAmount, wantRate, wantTick = kharis.Floor, 0, 700
				}
				if amount != wantAmount || rate != wantRate || tick != wantTick {
					t.Fatalf("Host migration wrong scope/tuple: amount%v rate%v tick%v want%v/%v/%v", amount, rate, tick, wantAmount, wantRate, wantTick)
				}
			}
		})
	}
}
