package handlers

// R2 (megaron_plan_skeppsuppdrag_landsatt.md): the P7 field-landing fall (b) —
// a ship field-positioned at sea could drop its cargo on adjacent unclaimed
// ground immediately, with no courier — is retired. A ship at sea takes no
// orders (R3); putting troops ashore away from a friendly port is now a
// MISSION given in port (march intent=land, R1: see
// unit_land_mission_test.go), never an instant command reaching a ship
// already out on the water. This test proves Unload now refuses a
// field-positioned ship outright, regardless of what stands next to it —
// unlike the retired fall (b), the neighbouring terrain no longer matters at
// all, so one test covers what used to be two.

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"formatet/megaron/server/internal/auth"
	"formatet/megaron/server/internal/clock"
	"formatet/megaron/server/internal/events"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

func TestUnitUnload_FieldPositionedShipRejected(t *testing.T) {
	pool := unitLoadTestPool(t)
	ctx := context.Background()

	if _, err := pool.Exec(ctx,
		`UPDATE worlds SET status = 'archived' WHERE status = 'active'`,
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
	accessToken, _, err := authSvc.Register(ctx, username, "x")
	if err != nil {
		t.Fatalf("register test player: %v", err)
	}
	claims, err := authSvc.ValidateAccessToken(accessToken)
	if err != nil {
		t.Fatalf("validate minted token: %v", err)
	}
	playerID := claims.PlayerID

	// The ship sits at a sea hex (0,0), next to dry unclaimed land (1,0) — the
	// exact geography fall (b) used to accept. It must be refused all the same.
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

	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("Unload(field-positioned ship) = %d %q, want 422 — R2 retires the P7 field-landing fall (b): a ship at sea cannot unload, it needs a \"land\" mission from port",
			rec.Code, rec.Body.String())
	}

	// Nothing must have moved — a rejected order changes nothing.
	var cargoStatus string
	if err := pool.QueryRow(ctx, `SELECT status FROM units WHERE id = $1`, cargoID).Scan(&cargoStatus); err != nil {
		t.Fatalf("read cargo unit after rejected unload: %v", err)
	}
	if cargoStatus != "embarked" {
		t.Errorf("cargo unit status = %q, want still \"embarked\" — the rejected order must not have moved it", cargoStatus)
	}
	var shipCargo *uuid.UUID
	if err := pool.QueryRow(ctx, `SELECT cargo_unit_id FROM units WHERE id = $1`, shipID).Scan(&shipCargo); err != nil {
		t.Fatalf("read ship after rejected unload: %v", err)
	}
	if shipCargo == nil || *shipCargo != cargoID {
		t.Errorf("ship.cargo_unit_id = %v, want still %v — the rejected order must not have cleared it", shipCargo, cargoID)
	}
}
