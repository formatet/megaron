package handlers

// Slice A red-before/green-after (megaron_plan_skeppsreparation.md, §Beslut
// B1): ship recruiting must gate on a SHIPYARD, not a harbour — the harbour
// keeps fish and loses the shipbuilding role. Before this slice, a harbour
// alone was sufficient (RequiresHarbour) and this test is RED (the recruit
// succeeds). After the slice, galley/war_galley/merchantman require
// RequiresShipyard instead, and this settlement (harbour + barracks, but no
// shipyard) must be rejected with "shipyard required".
//
// Barracks is seeded alongside harbour so the settlement has at least one
// affordable unit type (spearman) — otherwise capabilities.CanRecruit's
// aggregate "at least one type affordable" gate would reject the request
// before reaching the per-type building check this test targets, and the
// error message would be the wrong one.
//
// DB integration test, gated by DATABASE_URL — same rig as recruit_ship_test.go.

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"formatet/megaron/server/internal/auth"
	"formatet/megaron/server/internal/clock"
	"formatet/megaron/server/internal/economy"
	"formatet/megaron/server/internal/events"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

func TestRecruit_GalleyRequiresShipyardNotJustHarbour(t *testing.T) {
	pool := recruitShipTestPool(t)
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
	username := "shipyardgate-" + uuid.New().String()
	accessToken, _, err := authSvc.Register(ctx, username, username+"@test.invalid", "x")
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
		`INSERT INTO provinces (world_id, map_q, map_r, terrain_type, coastal) VALUES ($1, 0, 0, 'plains', true) RETURNING id`,
		worldID,
	).Scan(&provinceID); err != nil {
		t.Fatalf("create province: %v", err)
	}
	var settlementID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO settlements (world_id, province_id, name, culture_id, owner_id, control_type, is_capital, population)
		 VALUES ($1, $2, 'HarbourNoYard', 'minoan', $3, 'capital', true, 5000) RETURNING id`,
		worldID, provinceID, playerID,
	).Scan(&settlementID); err != nil {
		t.Fatalf("create settlement: %v", err)
	}

	// Harbour + barracks — NO shipyard. Exactly the pre-slice-sufficient setup
	// for ships, plus barracks so the aggregate can_recruit gate (spearman)
	// still passes and doesn't mask the per-type message this test checks.
	for _, bt := range []string{"harbour", "barracks"} {
		if _, err := pool.Exec(ctx,
			`INSERT INTO buildings (settlement_id, building_type, level) VALUES ($1, $2, 1)`,
			settlementID, bt,
		); err != nil {
			t.Fatalf("create %s building: %v", bt, err)
		}
	}

	for good, amount := range map[string]float64{"timber": 1000, "silver": 1000, "grain": 1000, "cedar": 1000} {
		if _, err := pool.Exec(ctx,
			`INSERT INTO settlement_goods (settlement_id, good_key, amount, rate, cap, calc_tick)
			 VALUES ($1, $2, $3, 0, $3, 0)`,
			settlementID, good, amount,
		); err != nil {
			t.Fatalf("seed %s: %v", good, err)
		}
	}

	clk := clock.NewTestClock(time.Now())
	scheduler := events.NewScheduler(pool, clk)
	eventStore := events.NewStore(pool)
	ph := NewProvinceHandler(pool, scheduler, clk, economy.SitosConfig{}, eventStore, nil)

	r := chi.NewRouter()
	r.Use(auth.Middleware(authSvc))
	r.Post("/worlds/{worldID}/provinces/{provinceID}/recruit", ph.Recruit)

	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(map[string]any{"unit_type": "galley"}); err != nil {
		t.Fatalf("encode body: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost,
		"/worlds/"+worldID.String()+"/provinces/"+provinceID.String()+"/recruit", &buf)
	req.Header.Set("Authorization", "Bearer "+accessToken)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("Recruit(galley) with harbour but no shipyard = %d %q, want 422 (shipyard required)",
			rec.Code, rec.Body.String())
	}
	var resp map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	errMsg, _ := resp["error"].(string)
	if errMsg != "shipyard required" {
		t.Errorf(`error = %q, want "shipyard required"`, errMsg)
	}
}
