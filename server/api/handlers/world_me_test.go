package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"formatet/megaron/server/internal/auth"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

func TestWorldMe_OwnWorldIsolation(t *testing.T) {
	pool := settlementsOverviewTestPool(t)
	ctx := context.Background()
	worldID := settlementsOverviewTestWorld(t, pool, ctx)
	svc := auth.NewService(pool, "profile-test-secret")
	a, token := settlementsOverviewRegisterPlayer(t, svc, ctx, "profile-a-"+uuid.New().String())
	b, _ := settlementsOverviewRegisterPlayer(t, svc, ctx, "profile-b-"+uuid.New().String())
	var other uuid.UUID
	// The final username-fallback fixture must not become an Agora provisioning
	// candidate for later packages. Remove only this test's worlds and accounts.
	t.Cleanup(func() {
		if _, err := pool.Exec(ctx, `DELETE FROM worlds WHERE id IN ($1,$2)`, worldID, other); err != nil {
			t.Errorf("clean profile worlds: %v", err)
			return
		}
		if _, err := pool.Exec(ctx, `DELETE FROM players WHERE id IN ($1,$2)`, a, b); err != nil {
			t.Errorf("clean profile players: %v", err)
		}
	})
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, sql, args...); err != nil {
			t.Fatal(err)
		}
	}
	exec(`UPDATE players SET wanax_name='Nestor' WHERE id=$1`, a)
	exec(`INSERT INTO player_world_records (player_id,world_id,founded_tick,kharis_amount,kharis_rate,kharis_calc_tick) VALUES ($1,$3,42,12.5,0.25,990),($2,$3,900,70,0,1000)`, a, b, worldID)
	exec(`INSERT INTO founder_phase (world_id,owner_id,population,grain_amount,grain_rate,silver_amount,silver_rate,active,founded_tick) VALUES ($1,$2,4000,0,0,0,0,false,10)`, worldID, a)
	settlementsOverviewSeedSettlement(t, pool, ctx, worldID, a, "Petras", true, 0, 0)
	settlementsOverviewSeedSettlement(t, pool, ctx, worldID, a, "Zakros", false, 1, 0)
	rival := settlementsOverviewSeedSettlement(t, pool, ctx, worldID, b, "Rival", true, 2, 0)
	for i, state := range []string{"sunk", "collapsed", "razed"} {
		id := settlementsOverviewSeedSettlement(t, pool, ctx, worldID, a, state, false, 3+i, 0)
		exec(`UPDATE settlements SET state=$1,population=9000 WHERE id=$2`, state, id)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO worlds (name,status) VALUES ($1,'archived') RETURNING id`, "profile-other-"+uuid.New().String()).Scan(&other); err != nil {
		t.Fatal(err)
	}
	settlementsOverviewSeedSettlement(t, pool, ctx, other, a, "OtherWorld", false, 0, 0)
	router := chi.NewRouter()
	router.With(auth.Middleware(svc)).Get("/worlds/{worldID}/me", NewWorldHandler(pool, svc, nil).Me)
	request := func(world string, bearer string) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(http.MethodGet, "/worlds/"+world+"/me?player_id="+b.String(), nil)
		if bearer != "" {
			req.Header.Set("Authorization", "Bearer "+bearer)
		}
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		return rec
	}
	read := func() WorldProfile {
		t.Helper()
		rec := request(worldID.String(), token)
		if rec.Code != 200 {
			t.Fatalf("me = %d: %s", rec.Code, rec.Body.String())
		}
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(rec.Body.Bytes(), &fields); err != nil {
			t.Fatal(err)
		}
		if len(fields) != 6 {
			t.Fatalf("profile must have six fields including standing: %s", rec.Body.String())
		}
		var got WorldProfile
		if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
			t.Fatal(err)
		}
		return got
	}
	first := read()
	if first.KharisStanding.Summary == "" || first.KharisStanding.Band != "Suspicious" {
		t.Fatalf("missing profile meaning: %+v", first)
	}
	if first.WanaxName != "Nestor" || first.FoundedTick == nil || *first.FoundedTick != 42 || first.Cities != 2 || first.Population != 1600 || first.Kharis != 15 {
		t.Fatalf("own profile includes foreign/stale data or raw kharis: %+v", first)
	}
	exec(`UPDATE settlements SET population=98765 WHERE id=$1`, rival)
	settlementsOverviewSeedSettlement(t, pool, ctx, worldID, b, "AnotherRival", false, 10, 0)
	exec(`UPDATE player_world_records SET kharis_amount=900,founded_tick=999 WHERE player_id=$1 AND world_id=$2`, b, worldID)
	exec(`UPDATE players SET wanax_name='RivalChanged' WHERE id=$1`, b)
	if got := read(); !reflect.DeepEqual(first, got) {
		t.Fatalf("another Wanax changed own profile: before=%+v after=%+v", first, got)
	}
	for _, tc := range []struct {
		name, world, token string
		status             int
	}{
		{"missing bearer", worldID.String(), "", 401},
		{"invalid bearer", worldID.String(), "invalid", 401},
		{"invalid world", "bad", token, 400},
		{"unjoined world", other.String(), token, 404},
		{"unknown world", uuid.New().String(), token, 404},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if rec := request(tc.world, tc.token); rec.Code != tc.status {
				t.Fatalf("status=%d want=%d body=%s", rec.Code, tc.status, rec.Body.String())
			}
		})
	}
	// Fallen realms still retain the current life's founding tick, but no cities.
	exec(`UPDATE player_world_records SET status='fallen' WHERE player_id=$1 AND world_id=$2`, a, worldID)
	exec(`UPDATE settlements SET owner_id=NULL WHERE owner_id=$1 AND world_id=$2`, a, worldID)
	if got := read(); got.FoundedTick == nil || *got.FoundedTick != 42 || got.Cities != 0 || got.Population != 0 {
		t.Fatalf("fallen profile: %+v", got)
	}
	// A risen Host keeps the stale realm tick until founding, but exposes null.
	exec(`UPDATE player_world_records SET status='active' WHERE player_id=$1 AND world_id=$2`, a, worldID)
	exec(`UPDATE founder_phase SET active=true,founded_tick=NULL WHERE owner_id=$1 AND world_id=$2`, a, worldID)
	if got := read(); got.FoundedTick != nil || got.Cities != 0 || got.Population != 0 {
		t.Fatalf("risen Host profile: %+v", got)
	}
	// A new Host is also ungrounded when the realm's default tick is zero.
	exec(`UPDATE player_world_records SET founded_tick=0 WHERE player_id=$1 AND world_id=$2`, a, worldID)
	if got := read(); got.FoundedTick != nil {
		t.Fatalf("new Host tick should be null: %+v", got)
	}
	// Founding at tick zero is valid; it must not be mistaken for absence.
	exec(`UPDATE founder_phase SET active=false,founded_tick=0 WHERE owner_id=$1 AND world_id=$2`, a, worldID)
	if got := read(); got.FoundedTick == nil || *got.FoundedTick != 0 {
		t.Fatalf("founded tick zero lost: %+v", got)
	}
	// Legacy realms without founder_phase retain the recorded zero and own username fallback.
	exec(`DELETE FROM founder_phase WHERE owner_id=$1 AND world_id=$2`, a, worldID)
	exec(`UPDATE players SET wanax_name=NULL WHERE id=$1`, a)
	var username string
	if err := pool.QueryRow(ctx, `SELECT username FROM players WHERE id=$1`, a).Scan(&username); err != nil {
		t.Fatal(err)
	}
	if got := read(); got.FoundedTick == nil || *got.FoundedTick != 0 || got.WanaxName != username {
		t.Fatalf("legacy profile: %+v", got)
	}
}
