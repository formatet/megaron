package handlers

// DB integration tests for megaron_plan_disband_returnerar_folket.md: disband
// is recruitment's mirror. Recruitment draws men out of settlements.population
// physically (C2, 52fa1c5, 2026-06-15); disband must give them back — land
// units by their size-delta, naval units by their crew (size is always 1, one
// vessel, so a naval unit's men live in crew, never size). Real Postgres,
// gated by DATABASE_URL (armyDisplayTestPool).

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
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type disbandFixture struct {
	pool         *pgxpool.Pool
	worldID      uuid.UUID
	provinceID   uuid.UUID
	settlementID uuid.UUID
	accessToken  string
	router       *chi.Mux
}

// setupDisbandFixture creates a world/player/settlement with the given
// starting population and wires ProvinceHandler.Disband behind auth
// middleware, mirroring setupRecruitShipFixture's minimal shape (no
// buildings/goods/map_tiles rows — RecomputeProduction tolerates an
// ungenerated catchment, same as the recruit fixture).
func setupDisbandFixture(t *testing.T, population int) *disbandFixture {
	t.Helper()
	pool := armyDisplayTestPool(t)
	ctx := context.Background()

	if _, err := pool.Exec(ctx,
		`UPDATE worlds SET status = 'archived' WHERE status = 'active'`,
	); err != nil {
		t.Fatalf("archive leftover active test worlds: %v", err)
	}
	var f disbandFixture
	f.pool = pool
	if err := pool.QueryRow(ctx,
		`INSERT INTO worlds (name, status) VALUES ($1, 'active') RETURNING id`,
		"test-world-"+uuid.New().String(),
	).Scan(&f.worldID); err != nil {
		t.Fatalf("create test world: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `UPDATE worlds SET status = 'archived' WHERE id = $1`, f.worldID)
	})

	authSvc := auth.NewService(pool, "test-secret")
	username := "disband-" + uuid.New().String()
	accessToken, _, err := authSvc.Register(ctx, username, "x")
	if err != nil {
		t.Fatalf("register test player: %v", err)
	}
	claims, err := authSvc.ValidateAccessToken(accessToken)
	if err != nil {
		t.Fatalf("validate minted token: %v", err)
	}
	f.accessToken = accessToken

	if err := pool.QueryRow(ctx,
		`INSERT INTO provinces (world_id, map_q, map_r, terrain_type, coastal) VALUES ($1, 0, 0, 'plains', true) RETURNING id`,
		f.worldID,
	).Scan(&f.provinceID); err != nil {
		t.Fatalf("create province: %v", err)
	}
	if err := pool.QueryRow(ctx,
		`INSERT INTO settlements (world_id, province_id, name, culture_id, owner_id, control_type, is_capital, population)
		 VALUES ($1, $2, 'Phaistos', 'minoan', $3, 'capital', true, $4) RETURNING id`,
		f.worldID, f.provinceID, claims.PlayerID, population,
	).Scan(&f.settlementID); err != nil {
		t.Fatalf("create settlement: %v", err)
	}

	clk := clock.NewTestClock(time.Now())
	ph := NewProvinceHandler(pool, nil, clk, economy.SitosConfig{}, nil, nil)
	r := chi.NewRouter()
	r.Use(auth.Middleware(authSvc))
	r.Post("/worlds/{worldID}/provinces/{provinceID}/disband", ph.Disband)
	f.router = r
	return &f
}

func (f *disbandFixture) seedUnit(t *testing.T, utype, category string, size, crew int) {
	t.Helper()
	if _, err := f.pool.Exec(context.Background(),
		`INSERT INTO units (world_id, owner_id, type, category, size, crew, status, settlement_id)
		 SELECT $1, s.owner_id, $2, $3, $4, $5, 'garrison', $6
		 FROM settlements s WHERE s.id = $6`,
		f.worldID, utype, category, size, crew, f.settlementID,
	); err != nil {
		t.Fatalf("seed unit %s: %v", utype, err)
	}
}

func (f *disbandFixture) population(t *testing.T) int {
	t.Helper()
	var pop int
	if err := f.pool.QueryRow(context.Background(),
		`SELECT population FROM settlements WHERE id = $1`, f.settlementID,
	).Scan(&pop); err != nil {
		t.Fatalf("read population: %v", err)
	}
	return pop
}

func (f *disbandFixture) disband(t *testing.T, body string) (*httptest.ResponseRecorder, map[string]any) {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost,
		"/worlds/"+f.worldID.String()+"/provinces/"+f.provinceID.String()+"/disband",
		strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+f.accessToken)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	f.router.ServeHTTP(rec, req)
	var resp map[string]any
	if rec.Body.Len() > 0 {
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("decode disband response %q: %v", rec.Body.String(), err)
		}
	}
	return rec, resp
}

func TestDisband_RestoresPopulationForLandUnits(t *testing.T) {
	f := setupDisbandFixture(t, 1000)
	f.seedUnit(t, "spearman", "land", 200, 0)

	rec, resp := f.disband(t, `{"spearman":200}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("Disband = %d %q, want 200", rec.Code, rec.Body.String())
	}
	if got := resp["pop_restored"]; got != 200.0 {
		t.Errorf("pop_restored = %v, want 200", got)
	}
	if got := resp["population"]; got != 1200.0 {
		t.Errorf("response population = %v, want 1200", got)
	}
	if got := f.population(t); got != 1200 {
		t.Errorf("settlements.population = %d, want 1200 (1000 + 200 disbanded spearmen)", got)
	}
}

func TestDisband_RestoresCrewForNavalUnits(t *testing.T) {
	f := setupDisbandFixture(t, 1000)
	// Two galleys, size=1 each (one vessel), crew=30 each — the men left
	// population as crew at recruit time, never as size.
	f.seedUnit(t, "galley", "naval", 1, 30)
	f.seedUnit(t, "galley", "naval", 1, 30)

	rec, resp := f.disband(t, `{"ship":2}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("Disband = %d %q, want 200", rec.Code, rec.Body.String())
	}
	if got := resp["pop_restored"]; got != 60.0 {
		t.Errorf("pop_restored = %v, want 60 (2 galleys × 30 crew — NOT 2, the ship count)", got)
	}
	if got := f.population(t); got != 1060 {
		t.Errorf("settlements.population = %d, want 1060", got)
	}
}

func TestDisband_RecruitDisbandRoundTripIsPopulationNeutral(t *testing.T) {
	f := setupDisbandFixture(t, 1000)
	before := f.population(t)

	// Simulate the C2 recruit-time draw directly (this test locks disband's
	// side of the mirror, not the recruit endpoint itself).
	if _, err := f.pool.Exec(context.Background(),
		`UPDATE settlements SET population = population - 100 WHERE id = $1`, f.settlementID,
	); err != nil {
		t.Fatalf("simulate recruit draw: %v", err)
	}
	f.seedUnit(t, "spearman", "land", 100, 0)

	rec, resp := f.disband(t, `{"spearman":100}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("Disband = %d %q, want 200", rec.Code, rec.Body.String())
	}
	if got := resp["pop_restored"]; got != 100.0 {
		t.Errorf("pop_restored = %v, want 100", got)
	}
	if after := f.population(t); after != before {
		t.Errorf("population after recruit(100)+disband(100) = %d, want %d (round-trip neutral)", after, before)
	}
}
