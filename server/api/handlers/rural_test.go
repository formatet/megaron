package handlers

// DB integration tests for RuralProjections under megaron_plan_byggnad_pa_hex.md
// §A2: buildings are hex-bound now, so the endpoint reads each hex-bound
// building row's REAL hex_q/hex_r instead of inventing a plausible one per
// building TYPE. Rewritten 2026-09-28 — the old placeRural/ruralCity/
// ruralCandidate placement heuristic (specific-match preference, hash tie-
// break, one-projection-per-type collision avoidance) is gone along with the
// fake-hex problem it existed to solve; a real hex can never collide with
// another of its own type on the same settlement (buildings' partial unique
// index already forbids that), so there is nothing left to arbitrate.
//
// Real Postgres, gated by DATABASE_URL — same harness as map_trades_mine_test.go.

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

type ruralFixture struct {
	pool        *pgxpool.Pool
	worldID     uuid.UUID
	settlementA uuid.UUID
	tokenA      string
	router      *chi.Mux
}

// setupRuralFixture builds one world with player A's capital at (0,0). Callers
// seed `buildings` rows directly (hex_q/hex_r set) rather than going through
// POST .../build — this test is about how RuralProjections READS building
// rows, not about the build verb's own validation (covered by
// province_build_hex_test.go).
func setupRuralFixture(t *testing.T) *ruralFixture {
	t.Helper()
	pool := recruitShipTestPool(t)
	ctx := context.Background()

	if _, err := pool.Exec(ctx,
		`UPDATE worlds SET status = 'archived' WHERE status = 'active' AND name LIKE 'test-rural-%'`,
	); err != nil {
		t.Fatalf("archive leftover active test worlds: %v", err)
	}
	var worldID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO worlds (name, status) VALUES ($1, 'active') RETURNING id`,
		"test-rural-"+uuid.New().String(),
	).Scan(&worldID); err != nil {
		t.Fatalf("create test world: %v", err)
	}
	t.Cleanup(func() { _, _ = pool.Exec(ctx, `UPDATE worlds SET status = 'archived' WHERE id = $1`, worldID) })

	authSvc := auth.NewService(pool, "test-secret")
	tokenA, _, err := authSvc.Register(ctx, "rural-a-"+uuid.New().String(), "x")
	if err != nil {
		t.Fatalf("register player A: %v", err)
	}
	claimsA, err := authSvc.ValidateAccessToken(tokenA)
	if err != nil {
		t.Fatalf("validate A token: %v", err)
	}
	playerA := claimsA.PlayerID

	var provinceID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO provinces (world_id, map_q, map_r, terrain_type) VALUES ($1, 0, 0, 'plains') RETURNING id`,
		worldID,
	).Scan(&provinceID); err != nil {
		t.Fatalf("create province: %v", err)
	}
	var settlementA uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO settlements (world_id, province_id, name, culture_id, owner_id, control_type, is_capital, state, population)
		 VALUES ($1, $2, 'Pylos', 'achaean', $3, 'capital', true, 'active', 5000) RETURNING id`,
		worldID, provinceID, playerA,
	).Scan(&settlementA); err != nil {
		t.Fatalf("create settlement A: %v", err)
	}

	clk := clock.NewTestClock(time.Now())
	wh := NewWorldHandler(pool, authSvc, clk)

	r := chi.NewRouter()
	r.Use(auth.OptionalMiddleware(authSvc))
	r.Get("/worlds/{worldID}/rural-projections", wh.RuralProjections)

	return &ruralFixture{pool: pool, worldID: worldID, settlementA: settlementA, tokenA: tokenA, router: r}
}

// seedBuilding inserts a `buildings` row directly. hexQ/hexR nil = a city
// building (NULL hex, never projected).
func (f *ruralFixture) seedBuilding(t *testing.T, settlementID uuid.UUID, buildingType string, hexQ, hexR *int) {
	t.Helper()
	if _, err := f.pool.Exec(context.Background(),
		`INSERT INTO buildings (settlement_id, building_type, level, hex_q, hex_r) VALUES ($1, $2, 1, $3, $4)`,
		settlementID, buildingType, hexQ, hexR,
	); err != nil {
		t.Fatalf("seed building %s: %v", buildingType, err)
	}
}

func intp(v int) *int { return &v }

func (f *ruralFixture) get(t *testing.T, token string) []map[string]any {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/worlds/"+f.worldID.String()+"/rural-projections", nil)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	f.router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /rural-projections = %d %q", rec.Code, rec.Body.String())
	}
	var out []map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("unmarshal response: %v (body: %s)", err, rec.Body.String())
	}
	return out
}

// TestRuralProjections_OneRowPerHexBoundBuilding is the core acceptance: two
// farms on two different hexes yield two sprites, each at its OWN real
// hex_q/hex_r — not the old one-per-type heuristic collapsing them to one.
func TestRuralProjections_OneRowPerHexBoundBuilding(t *testing.T) {
	f := setupRuralFixture(t)
	f.seedBuilding(t, f.settlementA, "farm", intp(1), intp(0))
	f.seedBuilding(t, f.settlementA, "farm", intp(-1), intp(1))

	out := f.get(t, f.tokenA)
	if len(out) != 2 {
		t.Fatalf("want 2 farm projections, got %d: %v", len(out), out)
	}
	seen := map[[2]int]bool{}
	for _, p := range out {
		if p["building_type"] != "farm" {
			t.Errorf("building_type = %v, want farm", p["building_type"])
		}
		q, r := int(p["q"].(float64)), int(p["r"].(float64))
		seen[[2]int{q, r}] = true
	}
	if !seen[[2]int{1, 0}] || !seen[[2]int{-1, 1}] {
		t.Fatalf("projections did not land on the two real hexes: %v", out)
	}
}

// TestRuralProjections_CityBuildingNeverProjected: a NULL-hex (city) building
// row is never turned into a rural sprite.
func TestRuralProjections_CityBuildingNeverProjected(t *testing.T) {
	f := setupRuralFixture(t)
	f.seedBuilding(t, f.settlementA, "market", nil, nil)

	out := f.get(t, f.tokenA)
	if len(out) != 0 {
		t.Fatalf("want 0 projections (market is a city building), got %d: %v", len(out), out)
	}
}

// TestRuralProjections_NonSpriteHexBoundTypeOmitted: stonequarry is hex-bound
// (province.HexBoundBuildings) but has no sprite yet (ruralProjectionTypes) —
// it must not appear here even though it carries a real hex.
func TestRuralProjections_NonSpriteHexBoundTypeOmitted(t *testing.T) {
	f := setupRuralFixture(t)
	f.seedBuilding(t, f.settlementA, "stonequarry", intp(1), intp(0))

	out := f.get(t, f.tokenA)
	if len(out) != 0 {
		t.Fatalf("want 0 projections (stonequarry has no sprite), got %d: %v", len(out), out)
	}
}

// TestRuralProjections_MineOnDepositHexReportsMine: mine is the retired
// silver_mine's replacement — its projection still names the building_type
// "mine" regardless of which deposit the hex holds (the deposit decides what
// it PRODUCES, not what it's CALLED).
func TestRuralProjections_MineOnDepositHexReportsMine(t *testing.T) {
	f := setupRuralFixture(t)
	f.seedBuilding(t, f.settlementA, "mine", intp(2), intp(-1))

	out := f.get(t, f.tokenA)
	if len(out) != 1 || out[0]["building_type"] != "mine" {
		t.Fatalf("want 1 mine projection, got %v", out)
	}
}

// TestRuralProjections_ExcludesHexHeldByAnotherProvince: a hex-bound building
// whose hex coincides with a SECOND province's map position is never
// projected there — a rural sprite must never sit on top of another city's
// own marker.
func TestRuralProjections_ExcludesHexHeldByAnotherProvince(t *testing.T) {
	f := setupRuralFixture(t)
	if _, err := f.pool.Exec(context.Background(),
		`INSERT INTO provinces (world_id, map_q, map_r, terrain_type) VALUES ($1, 3, 3, 'plains')`,
		f.worldID,
	); err != nil {
		t.Fatalf("seed second province: %v", err)
	}
	f.seedBuilding(t, f.settlementA, "farm", intp(3), intp(3))

	out := f.get(t, f.tokenA)
	if len(out) != 0 {
		t.Fatalf("want 0 projections (hex is another province), got %d: %v", len(out), out)
	}
}

// TestRuralProjections_UnauthenticatedReturnsEmpty: no player context, no
// projections — RuralProjections never enumerates buildings world-wide.
func TestRuralProjections_UnauthenticatedReturnsEmpty(t *testing.T) {
	f := setupRuralFixture(t)
	f.seedBuilding(t, f.settlementA, "farm", intp(1), intp(0))

	out := f.get(t, "")
	if len(out) != 0 {
		t.Fatalf("want 0 projections unauthenticated, got %d: %v", len(out), out)
	}
}
