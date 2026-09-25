package handlers

// No orders before the world has begun (Timothy 2026-09-25). While
// worlds.state = 'forming' every write in the authenticated game group is
// refused by RequireStartedWorld with the live "N of M" — except join,
// reports and the notification inbox. The start threshold is a server setting
// (POLEIA_WORLD_START_WANAXES), injected into join, GET /worlds/{id} and the
// gate; these tests run it at 2 so the Nth join is observable without four
// accounts.

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
	"formatet/megaron/server/internal/economy"
	"formatet/megaron/server/internal/events"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

const gateTestThreshold = 2

// startGateRouter wires the gate exactly as cmd/server does: inside a chi
// Group under /api/v1 (so the route pattern the exempt list matches on is
// the production one), after auth and RequireActiveWorld.
func startGateRouter(pool *pgxpool.Pool, authSvc *auth.Service) *chi.Mux {
	clk := clock.NewTestClock(time.Now())
	scheduler := events.NewScheduler(pool, clk)
	store := events.NewStore(pool)
	jh := NewJoinHandler(pool, store, economy.LoadSitosConfig(), clk, nil)
	jh.SetWorldStartWanaxes(gateTestThreshold)
	wh := NewWorldHandler(pool, authSvc, clk)
	wh.SetWorldStartWanaxes(gateTestThreshold)
	uh := NewUnitHandler(pool, scheduler, store, clk)
	rh := NewReportsHandler(pool)

	r := chi.NewRouter()
	r.Route("/api/v1", func(r chi.Router) {
		r.Get("/worlds/{worldID}", wh.Get)
		r.Group(func(r chi.Router) {
			r.Use(auth.Middleware(authSvc))
			r.Use(RequireActiveWorld(pool))
			r.Use(RequireStartedWorld(pool, gateTestThreshold))
			r.Post("/worlds/{worldID}/join", jh.Join)
			r.Post("/worlds/{worldID}/reports", rh.Create)
			r.Get("/worlds/{worldID}/units", uh.ListUnits)
			r.Post("/worlds/{worldID}/units/{unitID}/stance", uh.SetStance)
		})
	})
	return r
}

func gateDo(t *testing.T, r http.Handler, method, path, token, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec
}

func worldState(t *testing.T, pool *pgxpool.Pool, worldID uuid.UUID) string {
	t.Helper()
	var s string
	if err := pool.QueryRow(context.Background(), `SELECT state FROM worlds WHERE id = $1`, worldID).Scan(&s); err != nil {
		t.Fatalf("read world state: %v", err)
	}
	return s
}

func TestRequireStartedWorld_NoOrdersUntilThresholdJoins(t *testing.T) {
	pool := citiesTestPool(t)
	ctx := context.Background()
	worldID := seedJoinableWorld(t, pool)
	// Room for more than one host: seedJoinableWorld gives a single spawn tile.
	for q := 8; q <= 40; q += 8 {
		if _, err := pool.Exec(ctx,
			`INSERT INTO map_tiles (world_id, q, r, terrain) VALUES ($1, $2, 0, 'plains')`,
			worldID, q,
		); err != nil {
			t.Fatalf("seed spawn tile q=%d: %v", q, err)
		}
	}
	if s := worldState(t, pool, worldID); s != "forming" {
		t.Fatalf("fixture world state = %q, want forming", s)
	}

	authSvc := auth.NewService(pool, "test-secret")
	_, tokA := registerViewer(t, ctx, authSvc, "gate-a")
	_, tokB := registerViewer(t, ctx, authSvc, "gate-b")
	r := startGateRouter(pool, authSvc)
	base := "/api/v1/worlds/" + worldID.String()
	stancePath := base + "/units/" + uuid.New().String() + "/stance"

	// Exempt write: the first Wanax joins a forming world.
	if rec := gateDo(t, r, http.MethodPost, base+"/join", tokA, "{}"); rec.Code != http.StatusCreated {
		t.Fatalf("first join = %d %q, want 201", rec.Code, rec.Body.String())
	}
	if s := worldState(t, pool, worldID); s != "forming" {
		t.Fatalf("after 1 of %d joins state = %q, want forming", gateTestThreshold, s)
	}

	// (1) A game verb is refused with the live N of M.
	rec := gateDo(t, r, http.MethodPost, stancePath, tokA, `{"stance":"sentry"}`)
	if rec.Code != http.StatusConflict {
		t.Fatalf("stance while forming = %d %q, want 409", rec.Code, rec.Body.String())
	}
	var body struct {
		Error string `json:"error"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	want := "The world has not begun — 1 of 2 Wanaxes have arrived. Orders can be given once it starts."
	if body.Error != want {
		t.Fatalf("refusal = %q, want %q", body.Error, want)
	}

	// Reads pass while forming.
	if rec := gateDo(t, r, http.MethodGet, base+"/units", tokA, ""); rec.Code != http.StatusOK {
		t.Fatalf("GET units while forming = %d %q, want 200", rec.Code, rec.Body.String())
	}
	// (3) An exempt write works while forming: a bug report.
	if rec := gateDo(t, r, http.MethodPost, base+"/reports", tokA,
		`{"kind":"bug","body":"the world has not begun"}`); rec.Code >= 300 {
		t.Fatalf("POST report while forming = %d %q, want 2xx", rec.Code, rec.Body.String())
	}
	// GET /worlds/{id} reports the injected threshold, not the default.
	var wbody struct {
		Needed int `json:"wanaxes_needed"`
		Joined int `json:"wanaxes_joined"`
	}
	wrec := gateDo(t, r, http.MethodGet, base, "", "")
	_ = json.Unmarshal(wrec.Body.Bytes(), &wbody)
	if wbody.Needed != gateTestThreshold || wbody.Joined != 1 {
		t.Fatalf("GET world = needed %d joined %d, want %d / 1 (%s)", wbody.Needed, wbody.Joined, gateTestThreshold, wrec.Body.String())
	}

	// (4) The Nth join (N = the injected threshold, not the default 4) starts the world.
	if rec := gateDo(t, r, http.MethodPost, base+"/join", tokB, "{}"); rec.Code != http.StatusCreated {
		t.Fatalf("second join = %d %q, want 201", rec.Code, rec.Body.String())
	}
	if s := worldState(t, pool, worldID); s != "active" {
		t.Fatalf("after %d of %d joins state = %q, want active", gateTestThreshold, gateTestThreshold, s)
	}

	// (2) Once active the gate lets the verb through to its handler — which
	// answers for itself (the unit does not exist), not with the gate's refusal.
	rec = gateDo(t, r, http.MethodPost, stancePath, tokA, `{"stance":"sentry"}`)
	if strings.Contains(rec.Body.String(), "has not begun") {
		t.Fatalf("stance once active still refused by the gate: %d %q", rec.Code, rec.Body.String())
	}
	if rec.Code == http.StatusConflict {
		t.Fatalf("stance once active = 409 %q — gate or guard still in the way", rec.Body.String())
	}
}
