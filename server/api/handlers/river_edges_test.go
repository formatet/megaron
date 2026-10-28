package handlers

import (
	"context"
	"encoding/json"
	"formatet/megaron/server/internal/auth"
	"formatet/megaron/server/internal/clock"
	"formatet/megaron/server/internal/world"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"
)

func TestMapRiverEdgesContinueIntoFogWithoutNeighbourData(t *testing.T) {
	pool := citiesTestPool(t)
	ctx := context.Background()
	var wid uuid.UUID
	if err := pool.QueryRow(ctx, `INSERT INTO worlds(name,status) VALUES('River edges','archived') RETURNING id`).Scan(&wid); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { pool.Exec(ctx, `DELETE FROM worlds WHERE id=$1`, wid) })
	svc := auth.NewService(pool, "river-edge-test")
	viewer, token := registerViewer(t, ctx, svc, "river-viewer")
	_, other := registerViewer(t, ctx, svc, "river-uninformed")
	specs := []struct {
		q, r    int
		terrain string
		known   bool
	}{
		{0, 0, "river", true}, {1, 0, "river", false}, {0, 1, "coastal_sea", false}, {-1, 1, "river_ford", false},
		{20, 0, "river", true}, {30, 0, "river", true}, {31, 0, "deep_sea", false},
		{40, 0, "river_ford", true}, {41, 0, "river_ford", false},
		{60, 0, "river", false}, {61, 0, "river", false},
	}
	for _, s := range specs {
		if _, err := pool.Exec(ctx, `INSERT INTO map_tiles(world_id,q,r,terrain,coastal,fertility,mineral,copper_deposit,tin_deposit,silver_deposit,cedar_deposit,incense_deposit) VALUES($1,$2,$3,$4,true,0.987,0.876,true,true,true,true,true)`, wid, s.q, s.r, s.terrain); err != nil {
			t.Fatal(err)
		}
		if s.known {
			if _, err := pool.Exec(ctx, `INSERT INTO player_scouted_tiles(world_id,player_id,q,r) VALUES($1,$2,$3,$4)`, wid, viewer, s.q, s.r); err != nil {
				t.Fatal(err)
			}
		}
	}
	if _, err := pool.Exec(ctx, `INSERT INTO units(world_id,owner_id,type,category,size,crew,status,q,r) VALUES($1,$2,'galley','naval',1,20,'positioned',60,0)`, wid, viewer); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO place_names(world_id,kind,anchor_q,anchor_r,name) VALUES($1,'river',40,0,'Hidden Ford Name')`, wid); err != nil {
		t.Fatal(err)
	}
	if err := world.CompleteRiverMetadata(ctx, pool); err != nil {
		t.Fatal(err)
	}
	wh := NewWorldHandler(pool, svc, clock.NewTestClock(time.Now()))
	router := chi.NewRouter()
	router.With(auth.Middleware(svc)).Get("/worlds/{worldID}/map", wh.Map)
	fetch := func(token string) map[[2]int]map[string]any {
		req := httptest.NewRequest(http.MethodGet, "/worlds/"+wid.String()+"/map", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		if rec.Code != 200 {
			t.Fatalf("map=%d %s", rec.Code, rec.Body)
		}
		var tiles []map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &tiles); err != nil {
			t.Fatal(err)
		}
		out := map[[2]int]map[string]any{}
		for _, tile := range tiles {
			out[[2]int{int(tile["q"].(float64)), int(tile["r"].(float64))}] = tile
		}
		return out
	}
	got := fetch(token)
	if got[[2]int{0, 0}]["river_connections"] == nil {
		t.Fatal("known river lacks sanctioned neighbour kinds")
	}
	connections := got[[2]int{0, 0}]["river_connections"].([]any)
	wantKinds := map[float64]string{0: "river", 2: "coastal_sea", 5: "river_ford"}
	for _, raw := range connections {
		c := raw.(map[string]any)
		if len(c) != 2 || c["terrain"] != wantKinds[c["edge"].(float64)] {
			t.Errorf("connection leaked details or wrong kind: %v", c)
		}
	}

	for xy, want := range map[[2]int][]any{{0, 0}: {float64(0), float64(2), float64(5)}, {20, 0}: {}, {30, 0}: {float64(0)}, {40, 0}: {float64(0)}, {60, 0}: {float64(0)}} {
		if !reflect.DeepEqual(got[xy]["river_edges"], want) {
			t.Errorf("known river %v edges=%v want%v", xy, got[xy]["river_edges"], want)
		}
	}
	for _, xy := range [][2]int{{1, 0}, {0, 1}, {-1, 1}, {31, 0}, {41, 0}} {
		tile := got[xy]
		if tile["terrain"] != "fog" || tile["tier"] != "fog" || tile["visible"] != false {
			t.Errorf("hidden neighbour %v revealed terrain/tier: %v", xy, tile)
		}
		for _, field := range []string{"river_connections", "river_edges", "river_flow_known", "river_upstream_edges", "river_downstream_edges", "river_name", "place_name", "place_kind", "place_id", "coastal", "fertility", "mineral", "copper_deposit", "tin_deposit", "silver_deposit", "cedar_deposit", "incense_deposit"} {
			if _, ok := tile[field]; ok {
				t.Errorf("hidden neighbour %v leaked %s=%v", xy, field, tile[field])
			}
		}
	}
	for xy, tile := range fetch(other) {
		for _, field := range []string{"river_connections", "river_edges", "river_flow_known", "river_upstream_edges", "river_downstream_edges"} {
			if _, ok := tile[field]; ok {
				t.Errorf("uninformed viewer gained %s on %v: %v", field, xy, tile)
			}
		}
	}
}

func TestMapRiverFlowUsesFullSavedGeographyButOnlyExposesKnownTile(t *testing.T) {
	pool := citiesTestPool(t)
	ctx := context.Background()
	var wid uuid.UUID
	if err := pool.QueryRow(ctx, `INSERT INTO worlds(name,status,map_width,map_height) VALUES('River direction','archived',9,9) RETURNING id`).Scan(&wid); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { pool.Exec(ctx, `DELETE FROM worlds WHERE id=$1`, wid) })
	svc := auth.NewService(pool, "river-flow-test")
	viewer, token := registerViewer(t, ctx, svc, "river-direction-viewer")
	for q := 0; q < 9; q++ {
		for r := 4 - q/2; r < 4-q/2+9; r++ {
			terrain := "plains"
			if q == 8 {
				terrain = "coastal_sea"
			}
			if q == 1 && r == 7 {
				terrain = "coastal_sea"
			}
			if q >= 2 && q <= 7 && r == 7 {
				terrain = "river"
			}
			if q == 7 && r == 8 {
				terrain = "river_delta"
			}
			if _, err := pool.Exec(ctx, `INSERT INTO map_tiles(world_id,q,r,terrain) VALUES($1,$2,$3,$4)`, wid, q, r, terrain); err != nil {
				t.Fatal(err)
			}
		}
	}
	if _, err := pool.Exec(ctx, `INSERT INTO player_scouted_tiles(world_id,player_id,q,r) VALUES($1,$2,3,7)`, wid, viewer); err != nil {
		t.Fatal(err)
	}
	if err := world.CompleteRiverMetadata(ctx, pool); err != nil {
		t.Fatal(err)
	}
	wh := NewWorldHandler(pool, svc, clock.NewTestClock(time.Now()))
	router := chi.NewRouter()
	router.With(auth.Middleware(svc)).Get("/worlds/{worldID}/map", wh.Map)
	req := httptest.NewRequest(http.MethodGet, "/worlds/"+wid.String()+"/map", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("map=%d %s", rec.Code, rec.Body)
	}
	var tiles []map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &tiles); err != nil {
		t.Fatal(err)
	}
	known := 0
	for _, tile := range tiles {
		if tile["q"] == float64(3) && tile["r"] == float64(7) {
			known++
			if tile["river_flow_known"] != true || !reflect.DeepEqual(tile["river_upstream_edges"], []any{float64(1)}) || !reflect.DeepEqual(tile["river_downstream_edges"], []any{float64(0)}) {
				t.Errorf("known river direction wrong: %v", tile)
			}
		} else {
			if tile["terrain"] != "fog" {
				t.Errorf("direction leaked neighbour terrain: %v", tile)
			}
			for _, field := range []string{"river_connections", "river_edges", "river_flow_known", "river_upstream_edges", "river_downstream_edges"} {
				if _, ok := tile[field]; ok {
					t.Errorf("unknown neighbour gained %s: %v", field, tile)
				}
			}
		}
	}
	if _, err := pool.Exec(ctx, `UPDATE map_tiles SET river_upstream_edges=1,river_downstream_edges=2 WHERE world_id=$1 AND q=3 AND r=7`, wid); err != nil {
		t.Fatal(err)
	}
	again := httptest.NewRecorder()
	router.ServeHTTP(again, req)
	var saved []map[string]any
	if err := json.Unmarshal(again.Body.Bytes(), &saved); err != nil {
		t.Fatal(err)
	}
	for _, tile := range saved {
		if tile["q"] == float64(3) && tile["r"] == float64(7) && !reflect.DeepEqual(tile["river_downstream_edges"], []any{float64(1)}) {
			t.Errorf("FOV read recalculated saved direction: %v", tile)
		}
	}
	if known != 1 {
		t.Fatalf("known river records=%d want1", known)
	}
}

func TestStoreTilesPersistsGeneratedRiverGeography(t *testing.T) {
	pool := citiesTestPool(t)
	ctx := context.Background()
	var wid uuid.UUID
	if err := pool.QueryRow(ctx, `INSERT INTO worlds(name,status,map_width,map_height) VALUES('River tile writer','archived',40,30) RETURNING id`).Scan(&wid); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { pool.Exec(ctx, `DELETE FROM worlds WHERE id=$1`, wid) })
	tiles, _ := world.GenerateMap(wid, 12345, 40, 30)
	want := 0
	for _, tile := range tiles {
		if tile.RiverMetadataReady {
			want++
		}
	}
	if want == 0 {
		t.Fatal("generated fixture has no rivers")
	}
	h := NewWorldHandler(pool, nil, clock.NewTestClock(time.Now()))
	if err := h.storeTiles(ctx, wid, tiles); err != nil {
		t.Fatalf("store generated geography: %v", err)
	}
	var total, saved int
	if err := pool.QueryRow(ctx, `SELECT count(*),count(river_edges) FROM map_tiles WHERE world_id=$1`, wid).Scan(&total, &saved); err != nil {
		t.Fatal(err)
	}
	if total != len(tiles) || saved != want {
		t.Fatalf("writer saved %d/%d tiles and %d/%d river records", total, len(tiles), saved, want)
	}
	for _, tile := range tiles {
		if !tile.RiverMetadataReady {
			continue
		}
		var edges, up, down int16
		var known bool
		if err := pool.QueryRow(ctx, `SELECT river_edges,river_upstream_edges,river_downstream_edges,river_flow_known FROM map_tiles WHERE world_id=$1 AND q=$2 AND r=$3`, wid, tile.Q, tile.R).Scan(&edges, &up, &down, &known); err != nil {
			t.Fatal(err)
		}
		if edges != tile.RiverEdgesMask || up != tile.RiverUpstreamMask || down != tile.RiverDownstreamMask || known != tile.RiverFlowKnown {
			t.Fatalf("writer changed river at %d,%d", tile.Q, tile.R)
		}
	}
}
