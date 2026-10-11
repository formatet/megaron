package handlers

import (
	"context"
	"encoding/json"
	"flag"
	"formatet/megaron/server/internal/auth"
	"formatet/megaron/server/internal/clock"
	"formatet/megaron/server/internal/world"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

var riverProofDir = flag.String("river-proof-dir", "", "optional real map response artifact")

func TestMapRiverNamesFOW(t *testing.T) {
	pool := citiesTestPool(t)
	ctx := context.Background()
	var wid uuid.UUID
	if e := pool.QueryRow(ctx, `INSERT INTO worlds(name,status) VALUES('River Names','archived') RETURNING id`).Scan(&wid); e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { pool.Exec(ctx, `DELETE FROM worlds WHERE id=$1`, wid) })
	svc := auth.NewService(pool, "river-test")
	viewer, token := registerViewer(t, ctx, svc, "river-names-viewer")
	var ts []world.MapTile
	for q := 0; q <= 18; q++ {
		terrain := world.TerrainRiver
		if q == 3 {
			terrain = world.TerrainRiverFord
		}
		if q >= 16 {
			terrain = world.TerrainRiverDelta
		}
		ts = append(ts, world.MapTile{Q: q, Terrain: terrain})
		if _, e := pool.Exec(ctx, `INSERT INTO map_tiles(world_id,q,r,terrain) VALUES($1,$2,0,$3)`, wid, q, terrain); e != nil {
			t.Fatal(e)
		}
	}
	if _, e := pool.Exec(ctx, `INSERT INTO units(world_id,owner_id,type,category,size,crew,status,q,r) VALUES($1,$2,'spearman','land',100,0,'positioned',0,0)`, wid, viewer); e != nil {
		t.Fatal(e)
	}
	for _, q := range []int{3, 8, 16, 18} {
		if _, e := pool.Exec(ctx, `INSERT INTO player_scouted_tiles(world_id,player_id,q,r) VALUES($1,$2,$3,0)`, wid, viewer, q); e != nil {
			t.Fatal(e)
		}
	}
	mux := chi.NewRouter()
	mux.Use(auth.Middleware(svc))
	h := NewWorldHandler(pool, svc, clock.NewTestClock(time.Date(2026, 10, 11, 12, 0, 0, 0, time.UTC)))
	mux.Get("/worlds/{worldID}/map", h.Map)
	read := func() []map[string]any {
		req := httptest.NewRequest(http.MethodGet, "/worlds/"+wid.String()+"/map", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != 200 {
			t.Fatalf("map %d %s", rec.Code, rec.Body.String())
		}
		var got []map[string]any
		if e := json.Unmarshal(rec.Body.Bytes(), &got); e != nil {
			t.Fatal(e)
		}
		return got
	}
	got := read()
	want := world.RiverNames(wid.String(), ts)[[2]int{0, 0}]
	counts := map[string]int{}
	for _, tile := range got {
		tier := tile["tier"].(string)
		counts[tier]++
		if tier == "fog" {
			if _, ok := tile["river_name"]; ok {
				t.Fatal("unknown river hex leaked river_name")
			}
			continue
		}
		if tile["river_name"] != want {
			t.Fatalf("known %s at %v missing same chain name", tier, tile["q"])
		}
	}
	for _, tier := range []string{"live", "remembered", "fog"} {
		if counts[tier] == 0 {
			t.Fatal("fixture missing", tier)
		}
	}
	if len(read()) != len(got) {
		t.Fatal("second map read changed tiles")
	}
	if *riverProofDir != "" {
		os.MkdirAll(*riverProofDir, 0755)
		b, _ := json.MarshalIndent(map[string]any{"world_id": wid.String(), "tiles": got}, "", "  ")
		if e := os.WriteFile(filepath.Join(*riverProofDir, "map-fow.json"), append(b, '\n'), 0644); e != nil {
			t.Fatal(e)
		}
	}
}
