package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"formatet/megaron/server/internal/auth"
	"formatet/megaron/server/internal/clock"
	"formatet/megaron/server/internal/combat"
	"formatet/megaron/server/internal/events"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type placeRig struct {
	t             *testing.T
	pool          *pgxpool.Pool
	world, viewer uuid.UUID
	token, other  string
	router        *chi.Mux
}

func newPlaceRig(t *testing.T) *placeRig {
	pool := citiesTestPool(t)
	ctx := context.Background()
	var wid uuid.UUID
	if err := pool.QueryRow(ctx, `INSERT INTO worlds(name,status,current_tick) VALUES('Place Names','archived',12) RETURNING id`).Scan(&wid); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { pool.Exec(ctx, `DELETE FROM worlds WHERE id=$1`, wid) })
	svc := auth.NewService(pool, "place-test")
	viewer, token := registerViewer(t, ctx, svc, "place-viewer")
	_, other := registerViewer(t, ctx, svc, "place-other")
	for _, spec := range []struct {
		q       int
		terrain string
	}{{0, "river"}, {10, "mountain_limestone"}, {20, "semi_desert"}, {30, "forest_cedar"}} {
		for i := 0; i < 5; i++ {
			if _, err := pool.Exec(ctx, `INSERT INTO map_tiles(world_id,q,r,terrain) VALUES($1,$2,0,$3)`, wid, spec.q+i, spec.terrain); err != nil {
				t.Fatal(err)
			}
			if _, err := pool.Exec(ctx, `INSERT INTO player_scouted_tiles(world_id,player_id,q,r) VALUES($1,$2,$3,0)`, wid, viewer, spec.q+i); err != nil {
				t.Fatal(err)
			}
		}
	}
	for _, spec := range []struct {
		kind string
		q    int
	}{{"river", 0}, {"massif", 10}, {"desert", 20}, {"forest", 30}} {
		if _, err := pool.Exec(ctx, `INSERT INTO place_names(world_id,kind,anchor_q,anchor_r,claimed_by,claimed_tick) VALUES($1,$2,$3,0,$4,7)`, wid, spec.kind, spec.q, viewer); err != nil {
			t.Fatal(err)
		}
	}
	wh := NewWorldHandler(pool, svc, clock.NewTestClock(time.Date(2026, 10, 12, 0, 0, 0, 0, time.UTC)))
	r := chi.NewRouter()
	r.With(auth.Middleware(svc)).Get("/worlds/{worldID}/places", wh.Places)
	r.With(auth.Middleware(svc)).Post("/worlds/{worldID}/places/{kind}/{placeID}/name", wh.NamePlace)
	r.Put("/admin/worlds/{worldID}/places/{kind}/{placeID}/name", wh.AdminNamePlace)
	r.Delete("/admin/worlds/{worldID}/places/{kind}/{placeID}/name", wh.AdminClearPlaceName)
	t.Setenv("POLEIA_ADMIN_KEY", "place-k")
	return &placeRig{t, pool, wid, viewer, token, other, r}
}
func (n *placeRig) do(method, kind, id, body, token, key string) *httptest.ResponseRecorder {
	prefix := "/worlds/"
	if key != "" {
		prefix = "/admin/worlds/"
	}
	path := prefix + n.world.String() + "/places"
	if method != "GET" {
		path += "/" + kind + "/" + id + "/name"
	}
	req := httptest.NewRequest(method, path, bytes.NewBufferString(body))
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	if key != "" {
		req.Header.Set("X-Admin-Key", key)
	}
	rec := httptest.NewRecorder()
	n.router.ServeHTTP(rec, req)
	return rec
}
func TestPlaceNameOnlyFirstToReachAndOnce(t *testing.T) {
	n := newPlaceRig(t)
	if got := n.do("POST", "river", "0,0", `{"name":"Thaleia"}`, n.other, ""); got.Code != 403 {
		t.Fatalf("nonclaimant naming = %d, want403: %s", got.Code, got.Body)
	}
	for _, kindID := range [][2]string{{"river", "0,0"}, {"massif", "10,0"}, {"desert", "20,0"}, {"forest", "30,0"}} {
		kind, id := kindID[0], kindID[1]
		name := kind + " of Thaleia"
		got := n.do("POST", kind, id, `{"name":"  `+name+`   "}`, n.token, "")
		if got.Code != 200 {
			t.Fatalf("claimant %s naming=%d: %s", kind, got.Code, got.Body)
		}
		got = n.do("POST", kind, id, `{"name":"Second Try"}`, n.token, "")
		if got.Code != 409 {
			t.Fatalf("%s right reused, got%d", kind, got.Code)
		}
	}
	got := n.do("GET", "", "", "", n.token, "")
	if got.Code != 200 {
		t.Fatal(got.Code)
	}
	var v struct {
		Places []struct {
			Name    string `json:"name"`
			CanName bool   `json:"can_name"`
		}
	}
	if err := json.Unmarshal(got.Body.Bytes(), &v); err != nil {
		t.Fatal(err)
	}
	if len(v.Places) != 4 {
		t.Fatal(got.Body)
	}
	for _, p := range v.Places {
		if p.Name == "" || p.CanName {
			t.Fatalf("named place still nameable: %+v", p)
		}
	}
}
func TestPlaceNameValidationUniqueAcrossKindsAndAdminSpent(t *testing.T) {
	n := newPlaceRig(t)
	for _, bad := range []string{`{"name":"x"}`, `{"name":"` + strings.Repeat("a", 31) + `"}`, `{"name":"<b>hej</b>"}`, `{"name":" "}`, `nope`} {
		if got := n.do("POST", "river", "0,0", bad, n.token, ""); got.Code != 400 {
			t.Fatalf("badname accepted:%d %s", got.Code, got.Body)
		}
	}
	if got := n.do("POST", "river", "0,0", `{"name":"Poseidon's Gift"}`, n.token, ""); got.Code != 200 {
		t.Fatal(got.Body)
	}
	if got := n.do("POST", "forest", "30,0", `{"name":"poseidon's gift"}`, n.token, ""); got.Code != 409 {
		t.Fatalf("cross-kind duplicate accepted:%d", got.Code)
	}
	if got := n.do("PUT", "river", "0,0", `{"name":"Calm Mother"}`, "", "bad-key"); got.Code != 403 {
		t.Fatal(got.Code)
	}
	if got := n.do("PUT", "river", "0,0", `{"name":"Calm Mother"}`, "", "place-k"); got.Code != 200 {
		t.Fatal(got.Body)
	}
	if got := n.do("DELETE", "river", "0,0", "", "", "place-k"); got.Code != 204 {
		t.Fatal(got.Body)
	}
	if got := n.do("POST", "river", "0,0", `{"name":"Again"}`, n.token, ""); got.Code != 409 {
		t.Fatalf("admin-cleared right reusable:%d", got.Code)
	}
}
func TestPlacesUnknownObjectDoesNotLeakNameOrRight(t *testing.T) {
	n := newPlaceRig(t)
	got := n.do("GET", "", "", "", n.other, "")
	if got.Code != http.StatusOK {
		t.Fatal(got.Body)
	}
	var v struct {
		Places []any `json:"places"`
	}
	if err := json.Unmarshal(got.Body.Bytes(), &v); err != nil {
		t.Fatal(err)
	}
	if len(v.Places) != 0 {
		t.Fatalf("unknown places leaked name/right: %s", got.Body)
	}
}

func TestPlaceMassifPhysicalBorderClaimCanBeNamedThroughAPI(t *testing.T) {
	n := newPlaceRig(t)
	ctx := context.Background()
	if _, err := n.pool.Exec(ctx, `UPDATE worlds SET status='archived' WHERE status='active'`); err != nil {
		t.Fatal(err)
	}
	if _, err := n.pool.Exec(ctx, `UPDATE worlds SET status='active' WHERE id=$1`, n.world); err != nil {
		t.Fatal(err)
	}
	// Remove the API fixture's synthetic rights and memory: only the scan may
	// grant this right and retain a visible mountain witness.
	if _, err := n.pool.Exec(ctx, `DELETE FROM place_names WHERE world_id=$1`, n.world); err != nil {
		t.Fatal(err)
	}
	if _, err := n.pool.Exec(ctx, `DELETE FROM player_scouted_tiles WHERE world_id=$1`, n.world); err != nil {
		t.Fatal(err)
	}
	for q := 9; q <= 10; q++ {
		if _, err := n.pool.Exec(ctx, `INSERT INTO map_tiles(world_id,q,r,terrain) VALUES($1,$2,1,'plains')`, n.world, q); err != nil {
			t.Fatal(err)
		}
	}
	clk := clock.NewTestClock(time.Date(2026, 10, 12, 0, 0, 0, 0, time.UTC))
	route, _ := json.Marshal(combat.StoredRoute{StartTick: 12, EndTick: 13, Hexes: [][2]int{{9, 1}, {10, 1}}, Costs: []int64{1000}})
	if _, err := n.pool.Exec(ctx, `INSERT INTO units(world_id,owner_id,type,category,size,status,q,r,target_q,target_r,depart_tick,arrive_tick,march_route,departs_at,arrives_at) VALUES($1,$2,'infantry','land',100,'marching',9,1,10,1,12,13,$3,$4,$5)`, n.world, n.viewer, route, clk.Now(), clk.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	h := combat.NewPlaceReachScanHandler(n.pool, events.NewScheduler(n.pool, clk), events.NewStore(n.pool), nil)
	if err := h.Handle(ctx, events.ScheduledEvent{WorldID: n.world, DueTick: 12}); err != nil {
		t.Fatal(err)
	}
	// After the unit is gone, its persistent right and border witness remain;
	// the live eye must not mask a broken memory/action link.
	if _, err := n.pool.Exec(ctx, `UPDATE units SET status='disbanded',size=0 WHERE world_id=$1`, n.world); err != nil {
		t.Fatal(err)
	}
	got := n.do("GET", "", "", "", n.token, "")
	var body struct {
		Places []struct {
			Kind    string               `json:"kind"`
			ID      string               `json:"id"`
			CanName bool                 `json:"can_name"`
			Hexes   []struct{ Q, R int } `json:"hexes"`
		} `json:"places"`
	}
	if got.Code != 200 {
		t.Fatalf("massif catalogue HTTP%d: %s", got.Code, got.Body)
	}
	if err := json.Unmarshal(got.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Places) != 1 || body.Places[0].Kind != "massif" || body.Places[0].ID != "10,0" || !body.Places[0].CanName || len(body.Places[0].Hexes) != 1 {
		t.Fatalf("physical massif right not actionable without hidden component: %s", got.Body)
	}
	if got := n.do("POST", "massif", "10,0", `{"name":"Mount Ida"}`, n.other, ""); got.Code != 403 {
		t.Fatalf("other Wanax named reached massif: HTTP%d", got.Code)
	}
	if got := n.do("POST", "massif", "10,0", `{"name":"Mount Ida"}`, n.token, ""); got.Code != 200 {
		t.Fatalf("physical massif claimant cannot name it: HTTP%d %s", got.Code, got.Body)
	}
	if got := n.do("POST", "massif", "10,0", `{"name":"Second Try"}`, n.token, ""); got.Code != 409 {
		t.Fatalf("massif right reused: HTTP%d", got.Code)
	}
}
