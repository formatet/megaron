package handlers

// Fas 3 (temenos_orderlopare_plan.md): a stance order to a FIELD unit travels
// by hemerodromos from the nearest own city and applies only on delivery —
// full E2E through the HTTP handler: dispatch (202 order_dispatched, messenger
// kind='order') → deliver the scheduled OrderDelivery → stance applied.
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

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/poleia/server/internal/auth"
	"github.com/poleia/server/internal/clock"
	"github.com/poleia/server/internal/events"
	"github.com/poleia/server/internal/messenger"
)

func TestStance_FieldUnitOrderTravelsByCourier(t *testing.T) {
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
	username := "stance-" + uuid.New().String()
	accessToken, _, err := authSvc.Register(ctx, username, username+"@test.invalid", "x")
	if err != nil {
		t.Fatalf("register test player: %v", err)
	}
	claims, err := authSvc.ValidateAccessToken(accessToken)
	if err != nil {
		t.Fatalf("validate minted token: %v", err)
	}
	playerID := claims.PlayerID

	// The nearest own city at (0,0) — the hemerodromos' origin.
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

	// A field unit 5 hexes out.
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
	r.Post("/worlds/{worldID}/units/{unitID}/stance", uh.SetStance)

	body, _ := json.Marshal(map[string]any{"stance": "fortify"})
	req := httptest.NewRequest(http.MethodPost,
		"/worlds/"+worldID.String()+"/units/"+unitID.String()+"/stance", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+accessToken)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("SetStance(field unit) = %d %q, want 202 order_dispatched", rec.Code, rec.Body.String())
	}
	var resp struct {
		Status      string    `json:"status"`
		Verb        string    `json:"verb"`
		MessengerID uuid.UUID `json:"messenger_id"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode dispatch response: %v", err)
	}
	if resp.Status != "order_dispatched" || resp.Verb != "stance" {
		t.Fatalf("dispatch = %s/%s, want order_dispatched/stance", resp.Status, resp.Verb)
	}

	// The unit must be untouched until the runner arrives.
	var stance *string
	if err := pool.QueryRow(ctx, `SELECT stance FROM units WHERE id = $1`, unitID).Scan(&stance); err != nil {
		t.Fatalf("read unit pre-delivery: %v", err)
	}
	if stance != nil {
		t.Fatalf("stance applied before delivery: %v (command must not be instant)", *stance)
	}
	var kind string
	if err := pool.QueryRow(ctx, `SELECT kind FROM messengers WHERE id = $1`, resp.MessengerID).Scan(&kind); err != nil {
		t.Fatalf("read messenger: %v", err)
	}
	if kind != "order" {
		t.Errorf("messenger kind = %q, want order", kind)
	}

	// Deliver the courier: stance applies.
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
		t.Fatalf("deliver stance order: %v", err)
	}
	if err := pool.QueryRow(ctx, `SELECT stance FROM units WHERE id = $1`, unitID).Scan(&stance); err != nil {
		t.Fatalf("read unit post-delivery: %v", err)
	}
	if stance == nil || *stance != "fortify" {
		t.Fatalf("stance after delivery = %v, want fortify", stance)
	}
}
