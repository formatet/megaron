package handlers

// DB integration tests for megaron_plan_gruvgrinden.md Slice A (the mine build
// gate's catchment radius) AND megaron_plan_byggnad_pa_hex.md §A (mine became
// hex-bound, silver_mine retired — a mine on a silver-deposit hex now
// produces silver, exactly like copper/tin). Rewritten 2026-09-28: every
// build request now names hex_q/hex_r, and the two former "silver_mine hint"
// tests (TestBuildMine_SilverOnlyCatchmentHintsSilverMine /
// TestBuildMine_NoDepositAtAllGivesNoSilverHint) are gone along with the
// mechanic they tested — there is only one mine building now, so there is no
// second type left to hint the player toward.
//
// Real Postgres, gated by DATABASE_URL — same harness as recruit_shipyard_gate_test.go.

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"formatet/megaron/server/internal/auth"
	"formatet/megaron/server/internal/clock"
	"formatet/megaron/server/internal/economy"
	"formatet/megaron/server/internal/events"
	"formatet/megaron/server/internal/hexgrid"
	"formatet/megaron/server/internal/notify"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type mineGateFixture struct {
	pool         *pgxpool.Pool
	worldID      uuid.UUID
	provinceID   uuid.UUID
	settlementID uuid.UUID
	accessToken  string
	router       *chi.Mux
}

// setupMineGateFixture creates a world with a capital settlement at (0,0) and, if
// depositAt is non-nil, a silver-bearing hills tile at that coordinate (must be
// within the settlement's catchment for the deposit to matter to the test). The
// settlement is seeded with enough timber/stone to afford a mine (province.BuildingSpecs)
// and enough population for one placeable gubbe.
func setupMineGateFixture(t *testing.T, depositAt *hexgrid.Coord) *mineGateFixture {
	t.Helper()
	pool := recruitShipTestPool(t)
	ctx := context.Background()

	if _, err := pool.Exec(ctx,
		`UPDATE worlds SET status = 'archived' WHERE status = 'active' AND name LIKE 'test-minegate-%'`,
	); err != nil {
		t.Fatalf("archive leftover active test worlds: %v", err)
	}
	var worldID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO worlds (name, status, current_tick) VALUES ($1, 'active', 100) RETURNING id`,
		"test-minegate-"+uuid.New().String(),
	).Scan(&worldID); err != nil {
		t.Fatalf("create test world: %v", err)
	}
	t.Cleanup(func() { _, _ = pool.Exec(ctx, `UPDATE worlds SET status = 'archived' WHERE id = $1`, worldID) })

	authSvc := auth.NewService(pool, "test-secret")
	username := "minegate-" + uuid.New().String()
	accessToken, _, err := authSvc.Register(ctx, username, "x")
	if err != nil {
		t.Fatalf("register test player: %v", err)
	}
	claims, err := authSvc.ValidateAccessToken(accessToken)
	if err != nil {
		t.Fatalf("validate minted token: %v", err)
	}
	playerID := claims.PlayerID

	var provinceID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO provinces (world_id, map_q, map_r, terrain_type) VALUES ($1, 0, 0, 'plains') RETURNING id`,
		worldID,
	).Scan(&provinceID); err != nil {
		t.Fatalf("create province: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO map_tiles (world_id, q, r, terrain) VALUES ($1, 0, 0, 'plains')`,
		worldID,
	); err != nil {
		t.Fatalf("seed centre tile: %v", err)
	}
	if depositAt != nil {
		if _, err := pool.Exec(ctx,
			`INSERT INTO map_tiles (world_id, q, r, terrain, silver_deposit) VALUES ($1, $2, $3, 'hills', true)`,
			worldID, depositAt.Q, depositAt.R,
		); err != nil {
			t.Fatalf("seed deposit tile: %v", err)
		}
	}

	var settlementID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO settlements (world_id, province_id, name, culture_id, owner_id, control_type, is_capital, population)
		 VALUES ($1, $2, 'Argentopolis', 'achaean', $3, 'capital', true, 500) RETURNING id`,
		worldID, provinceID, playerID,
	).Scan(&settlementID); err != nil {
		t.Fatalf("create settlement: %v", err)
	}

	for good, amount := range map[string]float64{"timber": 200, "stone": 200} {
		if _, err := pool.Exec(ctx,
			`INSERT INTO settlement_goods (settlement_id, good_key, amount, rate, cap, calc_tick)
			 VALUES ($1, $2, $3, 0, $3, 0)`,
			settlementID, good, amount,
		); err != nil {
			t.Fatalf("seed %s: %v", good, err)
		}
	}

	if err := economy.RecomputeProduction(ctx, pool, settlementID); err != nil {
		t.Fatalf("initial RecomputeProduction: %v", err)
	}

	clk := clock.NewTestClock(time.Now())
	scheduler := events.NewScheduler(pool, clk)
	eventStore := events.NewStore(pool)
	hub := notify.New()
	hub.SetPool(pool)
	ph := NewProvinceHandler(pool, scheduler, clk, economy.SitosConfig{}, eventStore, hub)

	r := chi.NewRouter()
	r.Use(auth.Middleware(authSvc))
	r.Post("/worlds/{worldID}/provinces/{provinceID}/build", ph.Build)
	r.Post("/worlds/{worldID}/provinces/{provinceID}/placements", ph.PlaceGubbe)

	return &mineGateFixture{pool: pool, worldID: worldID, provinceID: provinceID, settlementID: settlementID, accessToken: accessToken, router: r}
}

func (f *mineGateFixture) buildPath() string {
	return "/worlds/" + f.worldID.String() + "/provinces/" + f.provinceID.String() + "/build"
}

func (f *mineGateFixture) placementsPath() string {
	return "/worlds/" + f.worldID.String() + "/provinces/" + f.provinceID.String() + "/placements"
}

func (f *mineGateFixture) do(t *testing.T, method, path string, body any) (int, map[string]any) {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			t.Fatalf("encode body: %v", err)
		}
	}
	req := httptest.NewRequest(method, path, &buf)
	req.Header.Set("Authorization", "Bearer "+f.accessToken)
	rec := httptest.NewRecorder()
	f.router.ServeHTTP(rec, req)
	var resp map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	return rec.Code, resp
}

// TestBuildMine_DepositAtRing2WasRejectedPreFix is the acceptance criterion 1
// red-before/green-after (megaron_plan_gruvgrinden.md): a silver deposit at
// hex distance EXACTLY 2 from the settlement (hexgrid.CatchmentRadius) is
// inside the production catchment — building the mine ON that exact hex
// must succeed.
func TestBuildMine_DepositAtRing2WasRejectedPreFix(t *testing.T) {
	deposit := hexgrid.Coord{Q: 2, R: 0} // hex distance 2 from (0,0) == hexgrid.CatchmentRadius
	f := setupMineGateFixture(t, &deposit)

	code, resp := f.do(t, http.MethodPost, f.buildPath(), map[string]any{"building_type": "mine", "hex_q": deposit.Q, "hex_r": deposit.R})
	if code != http.StatusCreated {
		t.Fatalf("build mine on ring-2 deposit = %d: %v, want %d (production reads hexgrid.CatchmentRadius, this deposit is inside it)",
			code, resp, http.StatusCreated)
	}
}

// TestBuildMine_DepositAtRing3StillRejected is acceptance criterion 2: a hex
// ONE past the catchment (distance 3, outside hexgrid.CatchmentRadius) must
// still be rejected — the fix widens the gate to match production, it does
// not remove it. The error string must name the real reachable radius
// (derived from hexgrid.CatchmentRadius, not a hardcoded literal that would
// silently go stale again the next time the radius changes).
func TestBuildMine_DepositAtRing3StillRejected(t *testing.T) {
	deposit := hexgrid.Coord{Q: 3, R: 0} // hex distance 3 from (0,0) — one past CatchmentRadius
	f := setupMineGateFixture(t, &deposit)

	code, resp := f.do(t, http.MethodPost, f.buildPath(), map[string]any{"building_type": "mine", "hex_q": deposit.Q, "hex_r": deposit.R})
	if code != http.StatusUnprocessableEntity {
		t.Fatalf("build mine on ring-3 deposit (outside catchment) = %d: %v, want 422", code, resp)
	}
	errMsg, _ := resp["error"].(string)
	wantSubstr := fmt.Sprintf("within %d steps", hexgrid.CatchmentRadius)
	if !strings.Contains(errMsg, wantSubstr) {
		t.Errorf("error = %q, want it to name the real radius (contains %q)", errMsg, wantSubstr)
	}
}

// TestBuildMine_Ring2DepositActuallyProduces is acceptance criterion 3 — the
// proof that this is a reachability fix, not just a gate test: build the mine
// on the ring-2 deposit, place a gubbe on that exact hex for silver, run
// economy.RecomputeProduction, and check the settlement's silver rate is > 0.
//
// The build queue itself (build_queue -> buildings on completion) is a separate
// worker concern outside this slice's scope, so the queued build's completion is
// simulated directly (INSERT INTO buildings) — Slice A only claims the GATE is
// fixed; this test proves that once built, the ring-2 mine is not a dead end.
func TestBuildMine_Ring2DepositActuallyProduces(t *testing.T) {
	deposit := hexgrid.Coord{Q: 2, R: 0}
	f := setupMineGateFixture(t, &deposit)
	ctx := context.Background()

	code, resp := f.do(t, http.MethodPost, f.buildPath(), map[string]any{"building_type": "mine", "hex_q": deposit.Q, "hex_r": deposit.R})
	if code != http.StatusCreated {
		t.Fatalf("build mine on ring-2 deposit = %d: %v, want 201", code, resp)
	}

	// Simulate the build queue completing (out of scope for this slice).
	if _, err := f.pool.Exec(ctx,
		`INSERT INTO buildings (settlement_id, building_type, level, hex_q, hex_r) VALUES ($1, 'mine', 1, $2, $3)`,
		f.settlementID, deposit.Q, deposit.R,
	); err != nil {
		t.Fatalf("simulate build completion: %v", err)
	}

	code, placeResp := f.do(t, http.MethodPost, f.placementsPath(),
		map[string]any{"target_kind": "hex", "hex_q": deposit.Q, "hex_r": deposit.R, "good_key": "silver"})
	if code != http.StatusCreated {
		t.Fatalf("place gubbe on ring-2 silver deposit = %d: %v, want 201", code, placeResp)
	}

	if err := economy.RecomputeProduction(ctx, f.pool, f.settlementID); err != nil {
		t.Fatalf("RecomputeProduction: %v", err)
	}

	var rate float64
	if err := f.pool.QueryRow(ctx,
		`SELECT rate FROM settlement_goods WHERE settlement_id = $1 AND good_key = 'silver'`,
		f.settlementID,
	).Scan(&rate); err != nil {
		t.Fatalf("read silver rate: %v", err)
	}
	if rate <= 0 {
		t.Errorf("silver rate after placing a gubbe on the ring-2 deposit and recomputing = %v, want > 0 (the mine on ring 2 must actually produce)", rate)
	}
}

// TestBuildMine_NoDepositAtChosenHexRejected is the hex-bound acceptance
// criterion 4 (megaron_plan_byggnad_pa_hex.md §A): a hex WITHIN the
// catchment but carrying no ore deposit at all must reject "mine" there,
// naming the condition (terrain/deposit), even though the settlement DOES
// have a silver deposit somewhere else in reach — the deposit must be on the
// CHOSEN hex, not merely "in the catchment".
func TestBuildMine_NoDepositAtChosenHexRejected(t *testing.T) {
	deposit := hexgrid.Coord{Q: 1, R: 0}
	f := setupMineGateFixture(t, &deposit)

	barren := hexgrid.Coord{Q: -1, R: 0} // plains, within catchment, no deposit
	code, resp := f.do(t, http.MethodPost, f.buildPath(), map[string]any{"building_type": "mine", "hex_q": barren.Q, "hex_r": barren.R})
	if code != http.StatusUnprocessableEntity {
		t.Fatalf("build mine on a deposit-free hex = %d: %v, want 422", code, resp)
	}
	errMsg, _ := resp["error"].(string)
	if !strings.Contains(errMsg, "terrain/deposit") {
		t.Errorf("error = %q, want it to name the terrain/deposit condition", errMsg)
	}
}
