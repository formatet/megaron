package handlers

// P7 soak fix (2026-07-19, "unit unload kräver hamn/garrison — embark kan
// aldrig etablera fotfäste på ny mark"): before this fix, Unload required the
// ship to be status='garrison' at a friendly settlement — a ship that had
// sailed out into the field (status='positioned', no settlement) to scout
// genuinely new coastline had no way whatsoever to put its cargo ashore there.
// A sea-enclosed start could dispatch ships but could never use them to expand
// territorially. This test drives the real HTTP handler (route-param
// correctness matters here too, per unit_load_test.go) and proves a
// field-positioned ship can now land its cargo on adjacent unclaimed ground as
// a field-positioned unit — ready for a follow-up `march --intent colonize`.

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/poleia/server/internal/auth"
	"github.com/poleia/server/internal/clock"
	"github.com/poleia/server/internal/events"
)

func TestUnitUnload_FieldPositionedShipLandsCargoOnUnclaimedGround(t *testing.T) {
	pool := unitLoadTestPool(t)
	ctx := context.Background()

	if _, err := pool.Exec(ctx,
		`UPDATE worlds SET status = 'archived' WHERE status = 'active' AND name LIKE 'test-world-%'`,
	); err != nil {
		t.Fatalf("archive leftover active test worlds: %v", err)
	}
	var worldID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO worlds (name, status) VALUES ($1, 'active') RETURNING id`,
		"test-world-"+uuid.New().String(),
	).Scan(&worldID); err != nil {
		t.Fatalf("create test world: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `UPDATE worlds SET status = 'archived' WHERE id = $1`, worldID)
	})

	authSvc := auth.NewService(pool, "test-secret")
	username := "shore-lander-" + uuid.New().String()
	accessToken, _, err := authSvc.Register(ctx, username, username+"@test.invalid", "x")
	if err != nil {
		t.Fatalf("register test player: %v", err)
	}
	claims, err := authSvc.ValidateAccessToken(accessToken)
	if err != nil {
		t.Fatalf("validate minted token: %v", err)
	}
	playerID := claims.PlayerID

	// The ship sits at a sea hex (0,0). Its neighbour (1,0) — the first hex
	// axialDirs checks — is dry, unclaimed plains: no provinces row exists for
	// it at all yet (the common case; provinces are sparse, see foundColony's
	// own comment in unit_arrival.go). The fix must handle that, not just an
	// already-provisioned-but-unsettled hex.
	if _, err := pool.Exec(ctx,
		`INSERT INTO map_tiles (world_id, q, r, terrain) VALUES ($1, 0, 0, 'coastal_sea'), ($1, 1, 0, 'plains')`,
		worldID,
	); err != nil {
		t.Fatalf("create map tiles: %v", err)
	}

	var shipID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO units (world_id, owner_id, type, category, size, status, q, r)
		 VALUES ($1, $2, 'merchantman', 'naval', 1, 'positioned', 0, 0) RETURNING id`,
		worldID, playerID,
	).Scan(&shipID); err != nil {
		t.Fatalf("create field-positioned ship: %v", err)
	}
	var cargoID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO units (world_id, owner_id, type, category, size, status)
		 VALUES ($1, $2, 'spearman', 'land', 100, 'embarked') RETURNING id`,
		worldID, playerID,
	).Scan(&cargoID); err != nil {
		t.Fatalf("create embarked cargo unit: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`UPDATE units SET cargo_unit_id = $2 WHERE id = $1`, shipID, cargoID,
	); err != nil {
		t.Fatalf("load cargo onto ship: %v", err)
	}

	clk := clock.NewTestClock(time.Now())
	uh := NewUnitHandler(pool, events.NewScheduler(pool, clk), events.NewStore(pool), clk)

	r := chi.NewRouter()
	r.Use(auth.Middleware(authSvc))
	r.Post("/worlds/{worldID}/units/{unitID}/unload", uh.Unload)

	req := httptest.NewRequest(http.MethodPost,
		"/worlds/"+worldID.String()+"/units/"+shipID.String()+"/unload", nil)
	req.Header.Set("Authorization", "Bearer "+accessToken)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("Unload(field-positioned ship next to unclaimed land) = %d %q, want 200 — the P7 bug is back: a ship out at sea can no longer land its cargo anywhere but a friendly harbour",
			rec.Code, rec.Body.String())
	}
	var resp struct {
		Q      int    `json:"q"`
		R      int    `json:"r"`
		Status string `json:"status"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode unload response: %v", err)
	}
	if resp.Q != 1 || resp.R != 0 {
		t.Errorf("landed cargo at (%d,%d), want (1,0) — the first unclaimed land neighbour", resp.Q, resp.R)
	}
	if resp.Status != "positioned" {
		t.Errorf("response status = %q, want \"positioned\" (no settlement at the landing hex)", resp.Status)
	}

	var cargoStatus string
	var cargoQ, cargoR int
	var cargoSettlement *uuid.UUID
	if err := pool.QueryRow(ctx,
		`SELECT status, q, r, settlement_id FROM units WHERE id = $1`, cargoID,
	).Scan(&cargoStatus, &cargoQ, &cargoR, &cargoSettlement); err != nil {
		t.Fatalf("read cargo unit after unload: %v", err)
	}
	if cargoStatus != "positioned" {
		t.Errorf("cargo unit status = %q, want \"positioned\"", cargoStatus)
	}
	if cargoQ != 1 || cargoR != 0 {
		t.Errorf("cargo unit position = (%d,%d), want (1,0)", cargoQ, cargoR)
	}
	if cargoSettlement != nil {
		t.Errorf("cargo unit settlement_id = %v, want nil (bare land, no settlement)", *cargoSettlement)
	}

	var shipCargo *uuid.UUID
	if err := pool.QueryRow(ctx, `SELECT cargo_unit_id FROM units WHERE id = $1`, shipID).Scan(&shipCargo); err != nil {
		t.Fatalf("read ship after unload: %v", err)
	}
	if shipCargo != nil {
		t.Errorf("ship.cargo_unit_id = %v, want nil (cargo disembarked)", *shipCargo)
	}
}

// TestUnitUnload_FieldPositionedShipWithNoUnclaimedNeighborRejected verifies the
// explicit, actionable rejection (not a silent no-op) when a field-positioned
// ship has nowhere dry and unclaimed to land — every neighbour here is sea.
func TestUnitUnload_FieldPositionedShipWithNoUnclaimedNeighborRejected(t *testing.T) {
	pool := unitLoadTestPool(t)
	ctx := context.Background()

	if _, err := pool.Exec(ctx,
		`UPDATE worlds SET status = 'archived' WHERE status = 'active' AND name LIKE 'test-world-%'`,
	); err != nil {
		t.Fatalf("archive leftover active test worlds: %v", err)
	}
	var worldID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO worlds (name, status) VALUES ($1, 'active') RETURNING id`,
		"test-world-"+uuid.New().String(),
	).Scan(&worldID); err != nil {
		t.Fatalf("create test world: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `UPDATE worlds SET status = 'archived' WHERE id = $1`, worldID)
	})

	authSvc := auth.NewService(pool, "test-secret")
	username := "stranded-sailor-" + uuid.New().String()
	accessToken, _, err := authSvc.Register(ctx, username, username+"@test.invalid", "x")
	if err != nil {
		t.Fatalf("register test player: %v", err)
	}
	claims, err := authSvc.ValidateAccessToken(accessToken)
	if err != nil {
		t.Fatalf("validate minted token: %v", err)
	}
	playerID := claims.PlayerID

	// Open ocean: only the ship's own hex exists, no neighbours at all (deep sea
	// on every side in practice — absent map_tiles rows are treated identically
	// to sea/mountain by NearestUnclaimedLandNeighbor: not landable).
	if _, err := pool.Exec(ctx,
		`INSERT INTO map_tiles (world_id, q, r, terrain) VALUES ($1, 0, 0, 'deep_sea')`,
		worldID,
	); err != nil {
		t.Fatalf("create map tile: %v", err)
	}

	var shipID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO units (world_id, owner_id, type, category, size, status, q, r)
		 VALUES ($1, $2, 'merchantman', 'naval', 1, 'positioned', 0, 0) RETURNING id`,
		worldID, playerID,
	).Scan(&shipID); err != nil {
		t.Fatalf("create field-positioned ship: %v", err)
	}
	var cargoID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO units (world_id, owner_id, type, category, size, status)
		 VALUES ($1, $2, 'spearman', 'land', 100, 'embarked') RETURNING id`,
		worldID, playerID,
	).Scan(&cargoID); err != nil {
		t.Fatalf("create embarked cargo unit: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`UPDATE units SET cargo_unit_id = $2 WHERE id = $1`, shipID, cargoID,
	); err != nil {
		t.Fatalf("load cargo onto ship: %v", err)
	}

	clk := clock.NewTestClock(time.Now())
	uh := NewUnitHandler(pool, events.NewScheduler(pool, clk), events.NewStore(pool), clk)

	r := chi.NewRouter()
	r.Use(auth.Middleware(authSvc))
	r.Post("/worlds/{worldID}/units/{unitID}/unload", uh.Unload)

	req := httptest.NewRequest(http.MethodPost,
		"/worlds/"+worldID.String()+"/units/"+shipID.String()+"/unload", nil)
	req.Header.Set("Authorization", "Bearer "+accessToken)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("Unload(no unclaimed neighbour) = %d %q, want 422 with an actionable reason",
			rec.Code, rec.Body.String())
	}
}
