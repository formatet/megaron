package handlers

// KR3 §5: a retreat order to a unit fighting in the FIELD travels by Runner
// from the nearest own city and applies only on delivery — full E2E through
// the HTTP handler, same shape as unit_stance_courier_test.go's stance test.
//
// DB integration test (real Postgres, gated by DATABASE_URL).

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
	"formatet/megaron/server/internal/events"
	"formatet/megaron/server/internal/messenger"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

func TestStandingOrders_FieldUnitOrderTravelsByCourierAndAppliesOnlyToActiveBattle(t *testing.T) {
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
	username := "standing-orders-" + uuid.New().String()
	accessToken, _, err := authSvc.Register(ctx, username, username+"@test.invalid", "x")
	if err != nil {
		t.Fatalf("register test player: %v", err)
	}
	claims, err := authSvc.ValidateAccessToken(accessToken)
	if err != nil {
		t.Fatalf("validate minted token: %v", err)
	}
	playerID := claims.PlayerID

	// The nearest own city at (0,0) — the runner's origin.
	var capProvID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO provinces (world_id, map_q, map_r, terrain_type) VALUES ($1, 0, 0, 'plains') RETURNING id`,
		worldID,
	).Scan(&capProvID); err != nil {
		t.Fatalf("create capital province: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO settlements (world_id, province_id, name, culture_id, owner_id, control_type, is_capital)
		 VALUES ($1, $2, 'Capital', 'achaean', $3, 'capital', true)`,
		worldID, capProvID, playerID,
	); err != nil {
		t.Fatalf("create capital settlement: %v", err)
	}

	// A field unit 5 hexes out, NOT in any battle yet.
	var unitID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO units (world_id, owner_id, type, category, size, status, q, r)
		 VALUES ($1, $2, 'spearman', 'land', 100, 'positioned', 5, 0) RETURNING id`,
		worldID, playerID,
	).Scan(&unitID); err != nil {
		t.Fatalf("create positioned unit: %v", err)
	}

	clk := clock.NewTestClock(time.Now())
	uh := NewUnitHandler(pool, events.NewScheduler(pool, clk), events.NewStore(pool), clk)
	r := chi.NewRouter()
	r.Use(auth.Middleware(authSvc))
	r.Post("/worlds/{worldID}/units/{unitID}/standing-orders", uh.SetStandingOrders)

	body, _ := json.Marshal(map[string]any{"retreat_at_loss": 0.6})
	req := httptest.NewRequest(http.MethodPost,
		"/worlds/"+worldID.String()+"/units/"+unitID.String()+"/standing-orders", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+accessToken)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("SetStandingOrders(field unit) = %d %q, want 202 order_dispatched", rec.Code, rec.Body.String())
	}
	var resp struct {
		Status      string    `json:"status"`
		Verb        string    `json:"verb"`
		MessengerID uuid.UUID `json:"messenger_id"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode dispatch response: %v", err)
	}
	if resp.Status != "order_dispatched" || resp.Verb != "standing_orders" {
		t.Fatalf("dispatch = %s/%s, want order_dispatched/standing_orders", resp.Status, resp.Verb)
	}

	// Deliver the courier BEFORE the unit is in any battle: the order must
	// fail final (OrderReject → notifyOrderFailed), not silently no-op or crash.
	var rawPayload []byte
	if err := pool.QueryRow(ctx,
		`SELECT payload FROM scheduled_events
		 WHERE event_type = 'OrderDelivery' AND (payload->>'messenger_id')::uuid = $1
		   AND processed_at IS NULL AND failed_at IS NULL`,
		resp.MessengerID,
	).Scan(&rawPayload); err != nil {
		t.Fatalf("load scheduled OrderDelivery: %v", err)
	}
	odh := messenger.NewOrderDeliveryHandler(pool, events.NewScheduler(pool, clk), events.NewStore(pool), nil, clk)
	if err := odh.Handle(ctx, events.ScheduledEvent{Payload: rawPayload}); err != nil {
		t.Fatalf("deliver standing-orders order (unit not in battle): %v", err)
	}

	// Now put the unit into an active battle and dispatch a second retreat
	// order — this one must apply once delivered.
	seed := int64(12345)
	var battleID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO battles (world_id, q, r, started_tick, current_tick, status, seed)
		 VALUES ($1, 5, 0, 1, 1, 'active', $2) RETURNING id`,
		worldID, seed,
	).Scan(&battleID); err != nil {
		t.Fatalf("create active battle: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO battle_participants (battle_id, unit_id, owner_id, side, joined_tick, initial_size, current_size)
		 VALUES ($1, $2, $3, 'attacker', 1, 100, 100)`,
		battleID, unitID, playerID,
	); err != nil {
		t.Fatalf("insert battle participant: %v", err)
	}

	body2, _ := json.Marshal(map[string]any{"hold_to_last_man": true})
	req2 := httptest.NewRequest(http.MethodPost,
		"/worlds/"+worldID.String()+"/units/"+unitID.String()+"/standing-orders", bytes.NewReader(body2))
	req2.Header.Set("Authorization", "Bearer "+accessToken)
	rec2 := httptest.NewRecorder()
	r.ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusAccepted {
		t.Fatalf("second SetStandingOrders = %d %q, want 202", rec2.Code, rec2.Body.String())
	}
	var resp2 struct {
		MessengerID uuid.UUID `json:"messenger_id"`
	}
	if err := json.Unmarshal(rec2.Body.Bytes(), &resp2); err != nil {
		t.Fatalf("decode second dispatch response: %v", err)
	}

	var rawPayload2 []byte
	if err := pool.QueryRow(ctx,
		`SELECT payload FROM scheduled_events
		 WHERE event_type = 'OrderDelivery' AND (payload->>'messenger_id')::uuid = $1
		   AND processed_at IS NULL AND failed_at IS NULL`,
		resp2.MessengerID,
	).Scan(&rawPayload2); err != nil {
		t.Fatalf("load second scheduled OrderDelivery: %v", err)
	}

	// Standing orders must NOT be applied before delivery — command is never instant.
	var soBefore []byte
	if err := pool.QueryRow(ctx,
		`SELECT standing_orders FROM battle_participants WHERE battle_id = $1 AND unit_id = $2`,
		battleID, unitID,
	).Scan(&soBefore); err != nil {
		t.Fatalf("read participant pre-delivery: %v", err)
	}
	if string(soBefore) != "{}" {
		t.Fatalf("standing_orders applied before delivery: %s (command must not be instant)", soBefore)
	}

	if err := odh.Handle(ctx, events.ScheduledEvent{Payload: rawPayload2}); err != nil {
		t.Fatalf("deliver second standing-orders order: %v", err)
	}
	var soAfter []byte
	if err := pool.QueryRow(ctx,
		`SELECT standing_orders FROM battle_participants WHERE battle_id = $1 AND unit_id = $2`,
		battleID, unitID,
	).Scan(&soAfter); err != nil {
		t.Fatalf("read participant post-delivery: %v", err)
	}
	var fields struct {
		HoldToLastMan bool `json:"hold_to_last_man"`
	}
	if err := json.Unmarshal(soAfter, &fields); err != nil {
		t.Fatalf("unmarshal standing_orders: %v", err)
	}
	if !fields.HoldToLastMan {
		t.Fatalf("standing_orders after delivery = %s, want hold_to_last_man=true", soAfter)
	}
}
