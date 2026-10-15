package handlers

// Vägar V2 (megaron_plan_vagar.md): Build Road end to end through the HTTP
// handlers — the order (202, stone out, site, caravan, Runner), its refusals
// (4xx + a refusals row, nothing happens), the Runner's delivery, the owner's
// road-site list, and the FOW rule for GET /roads (F5).

import (
	"bytes"
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
	"formatet/megaron/server/internal/messenger"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type roadAPI struct {
	pool            *pgxpool.Pool
	world           uuid.UUID
	player, other   uuid.UUID
	token, otherTok string
	city            uuid.UUID
	unit            uuid.UUID
	r               chi.Router
	clk             clock.Clock
}

func newRoadAPI(t *testing.T, stone float64) *roadAPI {
	t.Helper()
	pool := unitLoadTestPool(t)
	ctx := context.Background()
	if _, err := pool.Exec(ctx, `UPDATE worlds SET status = 'archived' WHERE status = 'active'`); err != nil {
		t.Fatal(err)
	}
	a := &roadAPI{pool: pool}
	if err := pool.QueryRow(ctx, `INSERT INTO worlds (name, status) VALUES ($1, 'active') RETURNING id`, "test-world-"+uuid.New().String()).Scan(&a.world); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = pool.Exec(ctx, `UPDATE worlds SET status = 'archived' WHERE id = $1`, a.world) })

	authSvc := auth.NewService(pool, "test-secret")
	reg := func(tag string) (string, uuid.UUID) {
		tok, _, err := authSvc.Register(ctx, tag+"-"+uuid.New().String(), "x")
		if err != nil {
			t.Fatal(err)
		}
		c, err := authSvc.ValidateAccessToken(tok)
		if err != nil {
			t.Fatal(err)
		}
		return tok, c.PlayerID
	}
	a.token, a.player = reg("road")
	a.otherTok, a.other = reg("roadother")

	var prov uuid.UUID
	if err := pool.QueryRow(ctx, `INSERT INTO provinces (world_id, map_q, map_r, terrain_type) VALUES ($1, 0, 0, 'plains') RETURNING id`, a.world).Scan(&prov); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx,
		`INSERT INTO settlements (world_id, province_id, name, culture_id, owner_id, control_type, is_capital)
		 VALUES ($1, $2, 'Capital', 'achaean', $3, 'capital', true) RETURNING id`, a.world, prov, a.player).Scan(&a.city); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO settlement_goods (settlement_id, good_key, amount, rate, cap, calc_tick)
		 VALUES ($1, 'stone', $2, 0, 1000000, current_world_tick())
		 ON CONFLICT (settlement_id, good_key) DO UPDATE SET amount = $2, rate = 0, calc_tick = current_world_tick()`, a.city, stone); err != nil {
		t.Fatal(err)
	}
	for q := -1; q <= 6; q++ {
		for r := -1; r <= 1; r++ {
			if _, err := pool.Exec(ctx, `INSERT INTO map_tiles (world_id, q, r, terrain) VALUES ($1,$2,$3,'plains') ON CONFLICT DO NOTHING`, a.world, q, r); err != nil {
				t.Fatal(err)
			}
		}
	}
	// A far-away stretch the player has never seen, and a ford beside the field.
	for _, tl := range [][3]any{{40, 0, "plains"}, {41, 0, "plains"}, {6, 1, "river_ford"}} {
		if _, err := pool.Exec(ctx, `INSERT INTO map_tiles (world_id, q, r, terrain) VALUES ($1,$2,$3,$4) ON CONFLICT (world_id,q,r) DO UPDATE SET terrain=$4`, a.world, tl[0], tl[1], tl[2]); err != nil {
			t.Fatal(err)
		}
	}
	if err := pool.QueryRow(ctx,
		`INSERT INTO units (world_id, owner_id, type, category, size, status, q, r)
		 VALUES ($1, $2, 'spearman', 'land', 100, 'positioned', 5, 0) RETURNING id`, a.world, a.player).Scan(&a.unit); err != nil {
		t.Fatal(err)
	}

	a.clk = clock.NewTestClock(time.Now())
	uh := NewUnitHandler(pool, events.NewScheduler(pool, a.clk), events.NewStore(pool), a.clk)
	a.r = chi.NewRouter()
	a.r.Use(auth.Middleware(authSvc))
	a.r.Use(RecordRefusals(pool))
	a.r.Post("/worlds/{worldID}/units/{unitID}/build-road", uh.BuildRoad)
	a.r.Get("/worlds/{worldID}/road-sites", uh.RoadSites)
	a.r.Get("/worlds/{worldID}/roads", uh.Roads)
	return a
}

func (a *roadAPI) do(t *testing.T, method, path, token string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var rd *bytes.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
	} else {
		rd = bytes.NewReader(nil)
	}
	req := httptest.NewRequest(method, path, rd)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	a.r.ServeHTTP(rec, req)
	return rec
}

func (a *roadAPI) stone(t *testing.T) float64 {
	var s float64
	if err := a.pool.QueryRow(context.Background(),
		`SELECT settled(amount, rate, calc_tick) FROM settlement_goods WHERE settlement_id = $1 AND good_key = 'stone'`, a.city).Scan(&s); err != nil {
		t.Fatal(err)
	}
	return s
}

func (a *roadAPI) buildURL() string {
	return "/worlds/" + a.world.String() + "/units/" + a.unit.String() + "/build-road"
}

func TestBuildRoad_OrderDeliveryAndSiteList(t *testing.T) {
	a := newRoadAPI(t, 100)
	ctx := context.Background()

	rec := a.do(t, http.MethodPost, a.buildURL(), a.token, map[string]int{"from_q": 5, "from_r": 0, "to_q": 6, "to_r": 0})
	if rec.Code != http.StatusAccepted {
		t.Fatalf("build-road = %d %s, want 202", rec.Code, rec.Body.String())
	}
	var resp struct {
		Status      string    `json:"status"`
		Verb        string    `json:"verb"`
		MessengerID uuid.UUID `json:"messenger_id"`
		SiteID      uuid.UUID `json:"site_id"`
		Stone       float64   `json:"stone"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Status != "order_dispatched" || resp.Verb != "build_road" || resp.Stone != 8 || resp.SiteID == uuid.Nil {
		t.Fatalf("response = %+v, want order_dispatched/build_road, 8 stone, a site id", resp)
	}
	if got := a.stone(t); got != 92 {
		t.Fatalf("stone = %v, want 92 (taken when the order is given)", got)
	}
	var stoneKind string
	if err := a.pool.QueryRow(ctx, `SELECT t.kind FROM road_sites rs JOIN transports t ON t.id = rs.stone_transport_id WHERE rs.id = $1`, resp.SiteID).Scan(&stoneKind); err != nil || stoneKind != "road_stone" {
		t.Fatalf("stone caravan kind = %q err=%v", stoneKind, err)
	}
	// Command is never instant: the army has not been told yet.
	var armyKnows bool
	if err := a.pool.QueryRow(ctx, `SELECT army_arrived FROM road_sites WHERE id = $1`, resp.SiteID).Scan(&armyKnows); err != nil || armyKnows {
		t.Fatalf("army_arrived before the Runner arrived = %v (err %v)", armyKnows, err)
	}

	// The owner's list names what the road waits for.
	rec = a.do(t, http.MethodGet, "/worlds/"+a.world.String()+"/road-sites", a.token, nil)
	var list struct {
		Sites []struct {
			ID      uuid.UUID `json:"id"`
			State   string    `json:"state"`
			Waiting []string  `json:"waiting_for"`
		} `json:"sites"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil || len(list.Sites) != 1 {
		t.Fatalf("road-sites = %s", rec.Body.String())
	}
	if list.Sites[0].State != "waiting" || len(list.Sites[0].Waiting) != 2 {
		t.Fatalf("site = %+v, want waiting for the stone caravan and the army", list.Sites[0])
	}
	// Another player sees none of it.
	rec = a.do(t, http.MethodGet, "/worlds/"+a.world.String()+"/road-sites", a.otherTok, nil)
	if strings.Contains(rec.Body.String(), resp.SiteID.String()) {
		t.Fatal("a road site leaked to another player")
	}

	// Deliver the Runner: the army (already at the from hex) is flagged.
	var raw []byte
	if err := a.pool.QueryRow(ctx,
		`SELECT payload FROM scheduled_events WHERE event_type = 'OrderDelivery' AND (payload->>'messenger_id')::uuid = $1 AND processed_at IS NULL`,
		resp.MessengerID).Scan(&raw); err != nil {
		t.Fatalf("load scheduled OrderDelivery: %v", err)
	}
	odh := messenger.NewOrderDeliveryHandler(a.pool, events.NewScheduler(a.pool, a.clk), events.NewStore(a.pool), nil, a.clk)
	if err := odh.Handle(ctx, events.ScheduledEvent{Payload: raw}); err != nil {
		t.Fatalf("deliver build_road: %v", err)
	}
	if err := a.pool.QueryRow(ctx, `SELECT army_arrived FROM road_sites WHERE id = $1`, resp.SiteID).Scan(&armyKnows); err != nil || !armyKnows {
		t.Fatalf("army_arrived after delivery = %v (err %v), want true", armyKnows, err)
	}
	var checks int
	if err := a.pool.QueryRow(ctx, `SELECT count(*) FROM scheduled_events WHERE event_type = 'RoadSite' AND payload->>'phase' = 'check' AND payload->>'site_id' = $1`, resp.SiteID.String()).Scan(&checks); err != nil || checks != 1 {
		t.Fatalf("queued site checks = %d err=%v, want 1", checks, err)
	}
}

func TestBuildRoad_RefusalIsRecordedAndChangesNothing(t *testing.T) {
	ctx := context.Background()
	a2 := newRoadAPI(t, 3)
	rec := a2.do(t, http.MethodPost, a2.buildURL(), a2.token, map[string]int{"from_q": 5, "from_r": 0, "to_q": 6, "to_r": 0})
	if rec.Code != http.StatusUnprocessableEntity || !strings.Contains(rec.Body.String(), "3 of 8 stone") {
		t.Fatalf("too little stone: %d %s, want 422 naming '3 of 8 stone'", rec.Code, rec.Body.String())
	}
	if a2.stone(t) != 3 {
		t.Fatal("a refused order must not touch the stone")
	}
	var n int
	if err := a2.pool.QueryRow(ctx, `SELECT count(*) FROM road_sites WHERE world_id = $1`, a2.world).Scan(&n); err != nil || n != 0 {
		t.Fatalf("sites after refusal = %d", n)
	}
	if err := a2.pool.QueryRow(ctx, `SELECT count(*) FROM messengers WHERE world_id = $1 AND kind = 'order'`, a2.world).Scan(&n); err != nil || n != 0 {
		t.Fatalf("a refused order must not send a Runner, found %d", n)
	}
	if err := a2.pool.QueryRow(ctx, `SELECT count(*) FROM refusals WHERE world_id = $1 AND status = 422`, a2.world).Scan(&n); err != nil || n != 1 {
		t.Fatalf("refusal rows = %d err=%v, want 1 (the avslagslogg)", n, err)
	}
	// Ford.
	rec = a2.do(t, http.MethodPost, a2.buildURL(), a2.token, map[string]int{"from_q": 6, "from_r": 0, "to_q": 6, "to_r": 1})
	if rec.Code != http.StatusUnprocessableEntity || !strings.Contains(rec.Body.String(), "ford") {
		t.Fatalf("ford: %d %s", rec.Code, rec.Body.String())
	}
	// Somebody else's army.
	rec = a2.do(t, http.MethodPost, a2.buildURL(), a2.otherTok, map[string]int{"from_q": 5, "from_r": 0, "to_q": 6, "to_r": 0})
	if rec.Code != http.StatusNotFound {
		t.Fatalf("foreign army: %d, want 404", rec.Code)
	}
	// Missing fields.
	rec = a2.do(t, http.MethodPost, a2.buildURL(), a2.token, map[string]int{"from_q": 5})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("missing fields: %d, want 400", rec.Code)
	}
}

// F5: GET /roads only returns edges whose BOTH hexes the caller has seen or
// remembers; a player who has seen nothing gets none, and an anonymous caller is refused (401).
func TestRoads_FogOfWar(t *testing.T) {
	a := newRoadAPI(t, 100)
	ctx := context.Background()
	for _, e := range [][4]int{{5, 0, 6, 0}, {40, 0, 41, 0}} {
		if _, err := a.pool.Exec(ctx, `INSERT INTO road_edges (world_id,q1,r1,q2,r2,built_tick) VALUES ($1,$2,$3,$4,$5,0)`, a.world, e[0], e[1], e[2], e[3]); err != nil {
			t.Fatal(err)
		}
	}
	type resp struct {
		Edges []struct{ Q1, R1, Q2, R2 int } `json:"edges"`
	}
	get := func(token string) resp {
		rec := a.do(t, http.MethodGet, "/worlds/"+a.world.String()+"/roads", token, nil)
		if rec.Code != http.StatusOK {
			t.Fatalf("GET /roads = %d %s", rec.Code, rec.Body.String())
		}
		var r resp
		_ = json.Unmarshal(rec.Body.Bytes(), &r)
		return r
	}
	own := get(a.token)
	if len(own.Edges) != 1 || own.Edges[0].Q1 != 5 {
		t.Fatalf("own view = %+v, want only the road beside the army (the far road is unseen)", own.Edges)
	}
	if got := get(a.otherTok); len(got.Edges) != 0 {
		t.Fatalf("a player who has seen nothing got %+v", got.Edges)
	}
	if rec := a.do(t, http.MethodGet, "/worlds/"+a.world.String()+"/roads", "", nil); rec.Code != http.StatusUnauthorized || strings.Contains(rec.Body.String(), "edges") {
		t.Fatalf("anonymous = %d %s, want 401 and no edges", rec.Code, rec.Body.String())
	}
	// Having once scouted the far hexes, the player remembers them.
	for _, h := range [][2]int{{40, 0}, {41, 0}} {
		if _, err := a.pool.Exec(ctx, `INSERT INTO player_scouted_tiles (world_id, player_id, q, r) VALUES ($1,$2,$3,$4)`, a.world, a.player, h[0], h[1]); err != nil {
			t.Fatal(err)
		}
	}
	if got := get(a.token); len(got.Edges) != 2 {
		t.Fatalf("after scouting both ends: %+v, want both roads", got.Edges)
	}
	// One known end is not enough.
	if _, err := a.pool.Exec(ctx, `DELETE FROM player_scouted_tiles WHERE world_id=$1 AND player_id=$2 AND q=41`, a.world, a.player); err != nil {
		t.Fatal(err)
	}
	if got := get(a.token); len(got.Edges) != 1 {
		t.Fatalf("one end known: %+v, want the far road hidden", got.Edges)
	}
}
