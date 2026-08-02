package handlers

// Recall/redirect→kuvert-unifiering (temenos_orderlopare_plan.md): recall and
// redirect now dispatch through the same order envelope march/stance already
// use — resolveOrderOrigin + sendOrderCourier, verb-based delivery in
// messenger.OrderDeliveryHandler — instead of the old bespoke path that wrote
// a kind='recall' messenger and a ScheduledMarchRecall event directly. Full
// E2E through the HTTP handler: dispatch (202 order_dispatched, messenger
// kind='order', a ScheduledOrderDelivery — NOT ScheduledMarchRecall — with
// verb recall|redirect) → deliver → unit turned exactly as the frozen
// ScheduledMarchRecall path used to turn it.
//
// DB integration tests (real Postgres, gated by DATABASE_URL).

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

// recallCourierFixture builds a world with a capital at (0,0), a straight
// 5-hex plains line (0,0)..(4,0), and a unit marching that line — mirrors
// internal/messenger's marchRecallFixture, but through the real HTTP router
// so the dispatch-time envelope wiring (resolveOrderOrigin, sendOrderCourier)
// is exercised too, not just the delivery handler.
type recallCourierFixture struct {
	worldID     uuid.UUID
	playerID    uuid.UUID
	accessToken string
	unitID      uuid.UUID
	departsAt   time.Time
	arrivesAt   time.Time
}

func setupRecallCourierWorld(t *testing.T) (recallCourierFixture, *chi.Mux) {
	t.Helper()
	pool := unitLoadTestPool(t)
	ctx := context.Background()

	if _, err := pool.Exec(ctx,
		`UPDATE worlds SET status = 'archived' WHERE status = 'active'`,
	); err != nil {
		t.Fatalf("archive leftover active test worlds: %v", err)
	}

	var f recallCourierFixture
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
	username := "recall-courier-" + uuid.New().String()
	accessToken, _, err := authSvc.Register(ctx, username, username+"@test.invalid", "x")
	if err != nil {
		t.Fatalf("register test player: %v", err)
	}
	claims, err := authSvc.ValidateAccessToken(accessToken)
	if err != nil {
		t.Fatalf("validate minted token: %v", err)
	}
	f.playerID = claims.PlayerID
	f.accessToken = accessToken

	// Capital at (0,0) — the runner's origin (nearest own settlement to
	// the unit's CURRENT position, beslut 4) and, at radius 3, the eye that
	// keeps the redirect target within FOW.
	var capProvID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO provinces (world_id, map_q, map_r, terrain_type) VALUES ($1, 0, 0, 'plains') RETURNING id`,
		f.worldID,
	).Scan(&capProvID); err != nil {
		t.Fatalf("create capital province: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO settlements (world_id, province_id, name, culture_id, owner_id, control_type, is_capital)
		 VALUES ($1, $2, 'Capital', 'achaean', $3, 'capital', true)`,
		f.worldID, capProvID, f.playerID,
	); err != nil {
		t.Fatalf("create capital settlement: %v", err)
	}

	for q := 0; q <= 4; q++ {
		if _, err := pool.Exec(ctx,
			`INSERT INTO map_tiles (world_id, q, r, terrain) VALUES ($1, $2, 0, 'plains')`,
			f.worldID, q,
		); err != nil {
			t.Fatalf("create map_tiles(%d,0): %v", q, err)
		}
	}
	// Redirect target tile, within the capital's live vision (radius 3 from (0,0)).
	if _, err := pool.Exec(ctx,
		`INSERT INTO map_tiles (world_id, q, r, terrain) VALUES ($1, 2, 1, 'plains') ON CONFLICT DO NOTHING`,
		f.worldID,
	); err != nil {
		t.Fatalf("create redirect target tile: %v", err)
	}

	// 25% through a 3h march (not the exact halfway point — temenos_orderlopare_plan.md
	// interception fix, 2026-07-30: a courier dispatched from the unit's own
	// departure city always takes exactly half the march's total duration to
	// reach its destination — at precisely 50% elapsed that ties the unit's own
	// remaining time exactly, and CourierTravel's round-to-the-nearest-tick can
	// tip an exact tie either way (1.5h rounds up to 2 ticks here) depending on
	// incidental fractions. 25% elapsed leaves real headroom so this fixture
	// proves ordinary delivery, not a coin flip on rounding.)
	f.departsAt = time.Now().Add(-45 * time.Minute) // 45 min into a 3h march
	f.arrivesAt = f.departsAt.Add(3 * time.Hour)    // 4 hexes × 0.75h/hex plains
	if err := pool.QueryRow(ctx,
		`INSERT INTO units (world_id, owner_id, type, category, size, status, q, r, target_q, target_r, departs_at, arrives_at)
		 VALUES ($1, $2, 'spearman', 'land', 100, 'marching', 0, 0, 4, 0, $3, $4) RETURNING id`,
		f.worldID, f.playerID, f.departsAt, f.arrivesAt,
	).Scan(&f.unitID); err != nil {
		t.Fatalf("create marching unit: %v", err)
	}

	clk := clock.NewTestClock(time.Now())
	uh := NewUnitHandler(pool, events.NewScheduler(pool, clk), events.NewStore(pool), clk)
	r := chi.NewRouter()
	r.Use(auth.Middleware(authSvc))
	r.Post("/worlds/{worldID}/units/{unitID}/recall", uh.Recall)
	return f, r
}

func TestRecall_FieldUnitOrderTravelsByCourier(t *testing.T) {
	pool := unitLoadTestPool(t)
	ctx := context.Background()
	f, router := setupRecallCourierWorld(t)

	req := httptest.NewRequest(http.MethodPost,
		"/worlds/"+f.worldID.String()+"/units/"+f.unitID.String()+"/recall", bytes.NewReader([]byte("{}")))
	req.Header.Set("Authorization", "Bearer "+f.accessToken)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("Recall(marching unit) = %d %q, want 202 order_dispatched", rec.Code, rec.Body.String())
	}
	var resp struct {
		Status      string    `json:"status"`
		Verb        string    `json:"verb"`
		MessengerID uuid.UUID `json:"messenger_id"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode dispatch response: %v", err)
	}
	if resp.Status != "order_dispatched" || resp.Verb != "recall" {
		t.Fatalf("dispatch = %s/%s, want order_dispatched/recall", resp.Status, resp.Verb)
	}

	var kind string
	if err := pool.QueryRow(ctx, `SELECT kind FROM messengers WHERE id = $1`, resp.MessengerID).Scan(&kind); err != nil {
		t.Fatalf("read messenger: %v", err)
	}
	if kind != "order" {
		t.Errorf("messenger kind = %q, want order (not the old 'recall' kind)", kind)
	}

	// The scheduled event must be the NEW ScheduledOrderDelivery type, not the
	// frozen ScheduledMarchRecall one — a fresh dispatch must never write the
	// old event type.
	var eventType, verb string
	if err := pool.QueryRow(ctx,
		`SELECT event_type, payload->>'verb' FROM scheduled_events
		 WHERE (payload->>'messenger_id')::uuid = $1 AND processed_at IS NULL AND failed_at IS NULL`,
		resp.MessengerID,
	).Scan(&eventType, &verb); err != nil {
		t.Fatalf("load scheduled event: %v", err)
	}
	if eventType != string(events.ScheduledOrderDelivery) {
		t.Errorf("scheduled event_type = %q, want %q", eventType, events.ScheduledOrderDelivery)
	}
	if verb != "recall" {
		t.Errorf("scheduled event verb = %q, want recall", verb)
	}

	// The unit must be untouched until the runner arrives.
	var targetQ int
	var status string
	if err := pool.QueryRow(ctx, `SELECT status, target_q FROM units WHERE id = $1`, f.unitID).Scan(&status, &targetQ); err != nil {
		t.Fatalf("read unit pre-delivery: %v", err)
	}
	if status != "marching" || targetQ != 4 {
		t.Fatalf("unit disturbed before delivery: status=%s target_q=%d (command must not be instant)", status, targetQ)
	}

	// A second recall while one is already in flight must 409 — the pending
	// guard now checks ScheduledOrderDelivery (verb recall|redirect), not the
	// no-longer-written ScheduledMarchRecall.
	req2 := httptest.NewRequest(http.MethodPost,
		"/worlds/"+f.worldID.String()+"/units/"+f.unitID.String()+"/recall", bytes.NewReader([]byte("{}")))
	req2.Header.Set("Authorization", "Bearer "+f.accessToken)
	rec2 := httptest.NewRecorder()
	router.ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusConflict {
		t.Fatalf("second recall while one in flight = %d %q, want 409", rec2.Code, rec2.Body.String())
	}

	// Deliver the courier: the unit turns for home, exactly like the frozen
	// MarchRecallHandler used to turn it.
	var rawPayload []byte
	if err := pool.QueryRow(ctx,
		`SELECT payload FROM scheduled_events
		 WHERE event_type = 'OrderDelivery' AND (payload->>'messenger_id')::uuid = $1
		   AND processed_at IS NULL AND failed_at IS NULL`,
		resp.MessengerID,
	).Scan(&rawPayload); err != nil {
		t.Fatalf("load scheduled OrderDelivery: %v", err)
	}
	clk := clock.NewTestClock(time.Now())
	odh := messenger.NewOrderDeliveryHandler(pool, events.NewScheduler(pool, clk), events.NewStore(pool), nil, clk)
	if err := odh.Handle(ctx, events.ScheduledEvent{Payload: rawPayload}); err != nil {
		t.Fatalf("deliver recall order: %v", err)
	}

	var newTargetQ, newTargetR int
	if err := pool.QueryRow(ctx, `SELECT status, target_q, target_r FROM units WHERE id = $1`, f.unitID).
		Scan(&status, &newTargetQ, &newTargetR); err != nil {
		t.Fatalf("read unit post-delivery: %v", err)
	}
	if status != "marching" {
		t.Errorf("expected unit still marching (turning takes time), got %q", status)
	}
	if newTargetQ != 0 || newTargetR != 0 {
		t.Errorf("expected new target = origin (0,0), got (%d,%d)", newTargetQ, newTargetR)
	}
	var msgStatus string
	if err := pool.QueryRow(ctx, `SELECT status FROM messengers WHERE id = $1`, resp.MessengerID).Scan(&msgStatus); err != nil {
		t.Fatalf("load messenger: %v", err)
	}
	if msgStatus != "arrived" {
		t.Errorf("expected messenger marked arrived, got %q", msgStatus)
	}
}

func TestRedirect_FieldUnitOrderTravelsByCourier(t *testing.T) {
	pool := unitLoadTestPool(t)
	ctx := context.Background()
	f, router := setupRecallCourierWorld(t)

	body, _ := json.Marshal(map[string]any{"target_q": 2, "target_r": 1})
	req := httptest.NewRequest(http.MethodPost,
		"/worlds/"+f.worldID.String()+"/units/"+f.unitID.String()+"/recall", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+f.accessToken)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("Recall(redirect) = %d %q, want 202 order_dispatched", rec.Code, rec.Body.String())
	}
	var resp struct {
		Status      string    `json:"status"`
		Verb        string    `json:"verb"`
		MessengerID uuid.UUID `json:"messenger_id"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode dispatch response: %v", err)
	}
	if resp.Status != "order_dispatched" || resp.Verb != "redirect" {
		t.Fatalf("dispatch = %s/%s, want order_dispatched/redirect", resp.Status, resp.Verb)
	}

	var eventType, verb string
	if err := pool.QueryRow(ctx,
		`SELECT event_type, payload->>'verb' FROM scheduled_events
		 WHERE (payload->>'messenger_id')::uuid = $1 AND processed_at IS NULL AND failed_at IS NULL`,
		resp.MessengerID,
	).Scan(&eventType, &verb); err != nil {
		t.Fatalf("load scheduled event: %v", err)
	}
	if eventType != string(events.ScheduledOrderDelivery) || verb != "redirect" {
		t.Fatalf("scheduled event = %s/%s, want %s/redirect", eventType, verb, events.ScheduledOrderDelivery)
	}

	var rawPayload []byte
	if err := pool.QueryRow(ctx,
		`SELECT payload FROM scheduled_events
		 WHERE event_type = 'OrderDelivery' AND (payload->>'messenger_id')::uuid = $1
		   AND processed_at IS NULL AND failed_at IS NULL`,
		resp.MessengerID,
	).Scan(&rawPayload); err != nil {
		t.Fatalf("load scheduled OrderDelivery: %v", err)
	}
	clk := clock.NewTestClock(time.Now())
	odh := messenger.NewOrderDeliveryHandler(pool, events.NewScheduler(pool, clk), events.NewStore(pool), nil, clk)
	if err := odh.Handle(ctx, events.ScheduledEvent{Payload: rawPayload}); err != nil {
		t.Fatalf("deliver redirect order: %v", err)
	}

	var targetQ, targetR int
	if err := pool.QueryRow(ctx, `SELECT target_q, target_r FROM units WHERE id = $1`, f.unitID).Scan(&targetQ, &targetR); err != nil {
		t.Fatalf("read unit post-delivery: %v", err)
	}
	if targetQ != 2 || targetR != 1 {
		t.Errorf("expected redirected target (2,1), got (%d,%d)", targetQ, targetR)
	}
}
