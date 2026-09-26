package handlers

// HTTP-level tests for the standing-order CRUD surface (StandingOrderHandler).
// The dispatch/pull LOGIC (the two named traps in
// megaron_plan_staende_leverans.md) is proven in
// internal/combat/standing_orders_test.go against the sweep directly; this
// file only proves the player-facing surface: ownership is enforced (egen→egen,
// CLAUDE.md trade-lagret punkt 3), unshippable/unknown goods are rejected
// before a row is written, and create/list/pause/resume/delete round-trip.

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"formatet/megaron/server/internal/auth"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type standingOrderFixture struct {
	pool        *pgxpool.Pool
	worldID     uuid.UUID
	fromID      uuid.UUID
	toID        uuid.UUID
	otherID     uuid.UUID // a settlement NOT owned by the caller
	accessToken string
	router      *chi.Mux
}

func setupStandingOrderFixture(t *testing.T) *standingOrderFixture {
	t.Helper()
	pool := recruitShipTestPool(t)
	ctx := context.Background()

	if _, err := pool.Exec(ctx, `UPDATE worlds SET status = 'archived' WHERE status = 'active'`); err != nil {
		t.Fatalf("archive leftover active test worlds: %v", err)
	}
	var worldID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO worlds (name, status) VALUES ($1, 'active') RETURNING id`,
		"test-world-"+uuid.New().String(),
	).Scan(&worldID); err != nil {
		t.Fatalf("create world: %v", err)
	}
	t.Cleanup(func() { _, _ = pool.Exec(ctx, `UPDATE worlds SET status = 'archived' WHERE id = $1`, worldID) })

	authSvc := auth.NewService(pool, "test-secret")
	token, _, err := authSvc.Register(ctx, "so-owner-"+uuid.New().String(), "x")
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	claims, err := authSvc.ValidateAccessToken(token)
	if err != nil {
		t.Fatalf("validate token: %v", err)
	}
	ownerID := claims.PlayerID

	otherToken, _, err := authSvc.Register(ctx, "so-other-"+uuid.New().String(), "x")
	if err != nil {
		t.Fatalf("register other player: %v", err)
	}
	otherClaims, err := authSvc.ValidateAccessToken(otherToken)
	if err != nil {
		t.Fatalf("validate other token: %v", err)
	}

	mk := func(name string, q int, owner uuid.UUID) uuid.UUID {
		var prov, sid uuid.UUID
		if err := pool.QueryRow(ctx,
			`INSERT INTO provinces (world_id, map_q, map_r, terrain_type) VALUES ($1, $2, 0, 'plains') RETURNING id`,
			worldID, q,
		).Scan(&prov); err != nil {
			t.Fatalf("create province: %v", err)
		}
		if err := pool.QueryRow(ctx,
			`INSERT INTO settlements (world_id, province_id, name, culture_id, owner_id, control_type, is_capital, state, population)
			 VALUES ($1, $2, $3, 'achaean', $4, 'capital', true, 'active', 1000) RETURNING id`,
			worldID, prov, name, owner,
		).Scan(&sid); err != nil {
			t.Fatalf("create settlement: %v", err)
		}
		return sid
	}
	fromID := mk("Petras", 0, ownerID)
	toID := mk("Colony", 4, ownerID)
	otherID := mk("Rival", 8, otherClaims.PlayerID)

	r := chi.NewRouter()
	r.Use(auth.Middleware(authSvc))
	soh := NewStandingOrderHandler(pool)
	r.Post("/worlds/{worldID}/standing-orders", soh.Create)
	r.Get("/worlds/{worldID}/standing-orders", soh.List)
	r.Post("/worlds/{worldID}/standing-orders/{orderID}/pause", soh.Pause)
	r.Post("/worlds/{worldID}/standing-orders/{orderID}/resume", soh.Resume)
	r.Delete("/worlds/{worldID}/standing-orders/{orderID}", soh.Delete)

	return &standingOrderFixture{pool: pool, worldID: worldID, fromID: fromID, toID: toID, otherID: otherID,
		accessToken: token, router: r}
}

func (f *standingOrderFixture) do(t *testing.T, method, path string, body any) (int, map[string]any) {
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

func TestStandingOrderAPI_CreateListPauseResumeDelete(t *testing.T) {
	f := setupStandingOrderFixture(t)
	base := "/worlds/" + f.worldID.String() + "/standing-orders"

	code, resp := f.do(t, http.MethodPost, base, map[string]any{
		"from_settlement_id":      f.fromID,
		"to_settlement_id":        f.toID,
		"crewed_by_settlement_id": f.fromID,
		"outbound":                []map[string]any{{"good_key": "grain", "threshold": 200}},
		"return":                  []map[string]any{{"good_key": "stone", "floor": 20}},
	})
	if code != http.StatusCreated {
		t.Fatalf("Create = %d %v", code, resp)
	}
	orderID, _ := resp["id"].(string)
	if orderID == "" {
		t.Fatalf("no id in create response: %v", resp)
	}

	// List returns a top-level JSON array, so f.do's map[string]any decode
	// target can't hold it — build the GET directly instead.
	req := httptest.NewRequest(http.MethodGet, base, nil)
	req.Header.Set("Authorization", "Bearer "+f.accessToken)
	rec := httptest.NewRecorder()
	f.router.ServeHTTP(rec, req)
	var orders []map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &orders); err != nil {
		t.Fatalf("unmarshal list: %v", err)
	}
	if len(orders) != 1 {
		t.Fatalf("orders = %d, want 1", len(orders))
	}
	if orders[0]["status"] != "active" {
		t.Errorf("status = %v, want active", orders[0]["status"])
	}
	outbound, _ := orders[0]["outbound"].([]any)
	if len(outbound) != 1 {
		t.Fatalf("outbound goods = %d, want 1", len(outbound))
	}
	ret, _ := orders[0]["return"].([]any)
	if len(ret) != 1 {
		t.Fatalf("return goods = %d, want 1", len(ret))
	}

	code, _ = f.do(t, http.MethodPost, base+"/"+orderID+"/pause", nil)
	if code != http.StatusOK {
		t.Fatalf("Pause = %d", code)
	}
	var status string
	var reason *string
	if err := f.pool.QueryRow(context.Background(),
		`SELECT status, pause_reason FROM standing_orders WHERE id = $1`, orderID,
	).Scan(&status, &reason); err != nil {
		t.Fatalf("read status: %v", err)
	}
	if status != "paused" || reason == nil {
		t.Errorf("after Pause: status=%q reason=%v, want paused with a reason", status, reason)
	}

	code, _ = f.do(t, http.MethodPost, base+"/"+orderID+"/resume", nil)
	if code != http.StatusOK {
		t.Fatalf("Resume = %d", code)
	}
	if err := f.pool.QueryRow(context.Background(),
		`SELECT status, pause_reason FROM standing_orders WHERE id = $1`, orderID,
	).Scan(&status, &reason); err != nil {
		t.Fatalf("read status: %v", err)
	}
	if status != "active" || reason != nil {
		t.Errorf("after Resume: status=%q reason=%v, want active with no reason", status, reason)
	}

	code, _ = f.do(t, http.MethodDelete, base+"/"+orderID, nil)
	if code != http.StatusNoContent {
		t.Fatalf("Delete = %d", code)
	}
	var count int
	_ = f.pool.QueryRow(context.Background(), `SELECT count(*) FROM standing_orders WHERE id = $1`, orderID).Scan(&count)
	if count != 0 {
		t.Errorf("standing_orders row survives delete")
	}
}

// Egen→egen only: a route where the destination belongs to someone else must
// be rejected before anything is written (CLAUDE.md trade-lagret punkt 3).
func TestStandingOrderAPI_RejectsForeignSettlement(t *testing.T) {
	f := setupStandingOrderFixture(t)
	base := "/worlds/" + f.worldID.String() + "/standing-orders"

	code, resp := f.do(t, http.MethodPost, base, map[string]any{
		"from_settlement_id":      f.fromID,
		"to_settlement_id":        f.otherID,
		"crewed_by_settlement_id": f.fromID,
		"outbound":                []map[string]any{{"good_key": "grain", "threshold": 200}},
	})
	if code != http.StatusForbidden {
		t.Fatalf("Create with a foreign destination = %d %v, want 403", code, resp)
	}
}

// An unknown/unshippable good must be rejected before the order is created —
// mirrors the trade-offer good-key validation gap
// (messenger_trade_offer_good_validation_test.go): a stuck route that could
// never fulfil its own threshold is worse than a 400 up front.
func TestStandingOrderAPI_RejectsUnshippableGood(t *testing.T) {
	f := setupStandingOrderFixture(t)
	base := "/worlds/" + f.worldID.String() + "/standing-orders"

	code, resp := f.do(t, http.MethodPost, base, map[string]any{
		"from_settlement_id":      f.fromID,
		"to_settlement_id":        f.toID,
		"crewed_by_settlement_id": f.fromID,
		"outbound":                []map[string]any{{"good_key": "cult", "threshold": 10}},
	})
	if code != http.StatusBadRequest {
		t.Fatalf("Create with good_key=cult = %d %v, want 400 (cult is temple labor, never shippable)", code, resp)
	}

	var count int
	_ = f.pool.QueryRow(context.Background(), `SELECT count(*) FROM standing_orders WHERE world_id = $1`, f.worldID).Scan(&count)
	if count != 0 {
		t.Errorf("a standing_orders row was written despite the rejected good: %d rows", count)
	}
}

// shipForOrder inserts a garrison ship at settlementID, owned by whoever owns
// that settlement, for R4 Delete tests below.
func (f *standingOrderFixture) shipForOrder(t *testing.T, settlementID uuid.UUID) uuid.UUID {
	t.Helper()
	ctx := context.Background()
	var ownerID uuid.UUID
	if err := f.pool.QueryRow(ctx, `SELECT owner_id FROM settlements WHERE id = $1`, settlementID).Scan(&ownerID); err != nil {
		t.Fatalf("look up settlement owner: %v", err)
	}
	var shipID uuid.UUID
	if err := f.pool.QueryRow(ctx,
		`INSERT INTO units (world_id, owner_id, type, category, size, crew, status, settlement_id)
		 VALUES ($1, $2, 'merchantman', 'naval', 1, 10, 'garrison', $3) RETURNING id`,
		f.worldID, ownerID, settlementID,
	).Scan(&shipID); err != nil {
		t.Fatalf("create ship: %v", err)
	}
	return shipID
}

// R4 (megaron_plan_sjohandel_kraver_skepp.md), Delete case 1: a route whose
// bound ship is idle (no leg in flight — here, no leg ever dispatched) is
// released the instant the Wanax deletes the route, not left bound forever
// with no order left to manage it.
func TestStandingOrderAPI_DeleteReleasesIdleShip(t *testing.T) {
	f := setupStandingOrderFixture(t)
	ctx := context.Background()
	base := "/worlds/" + f.worldID.String() + "/standing-orders"

	code, resp := f.do(t, http.MethodPost, base, map[string]any{
		"from_settlement_id":      f.fromID,
		"to_settlement_id":        f.toID,
		"crewed_by_settlement_id": f.fromID,
		"outbound":                []map[string]any{{"good_key": "grain", "threshold": 200}},
	})
	if code != http.StatusCreated {
		t.Fatalf("Create = %d %v", code, resp)
	}
	orderID, _ := resp["id"].(string)

	shipID := f.shipForOrder(t, f.fromID)
	if _, err := f.pool.Exec(ctx,
		`UPDATE standing_orders SET ship_unit_id = $2 WHERE id = $1`, orderID, shipID,
	); err != nil {
		t.Fatalf("bind ship to order: %v", err)
	}
	if _, err := f.pool.Exec(ctx, `UPDATE units SET status = 'freighting' WHERE id = $1`, shipID); err != nil {
		t.Fatalf("mark ship freighting: %v", err)
	}

	code, resp = f.do(t, http.MethodDelete, base+"/"+orderID, nil)
	if code != http.StatusNoContent {
		t.Fatalf("Delete = %d %v", code, resp)
	}

	var status string
	var settlementID *uuid.UUID
	if err := f.pool.QueryRow(ctx, `SELECT status, settlement_id FROM units WHERE id = $1`, shipID).
		Scan(&status, &settlementID); err != nil {
		t.Fatalf("read ship state: %v", err)
	}
	if status != "garrison" {
		t.Errorf("ship status after delete = %q, want garrison", status)
	}
	if settlementID == nil || *settlementID != f.fromID {
		t.Errorf("ship settlement after delete = %v, want %s", settlementID, f.fromID)
	}
}

// R4 Delete case 2 ("ute"): a route whose ship is mid-leg when the Wanax
// deletes it keeps its ship bound (the leg must not be yanked out from under
// it) — transport.ArrivalHandler releases it once that orphaned leg lands
// (proven directly against the handler in internal/transport; here we only
// check the API call itself doesn't touch a ship that's still moving, and
// that the cascade actually orphans the leg).
func TestStandingOrderAPI_DeleteLeavesInFlightShipBound(t *testing.T) {
	f := setupStandingOrderFixture(t)
	ctx := context.Background()
	base := "/worlds/" + f.worldID.String() + "/standing-orders"

	code, resp := f.do(t, http.MethodPost, base, map[string]any{
		"from_settlement_id":      f.fromID,
		"to_settlement_id":        f.toID,
		"crewed_by_settlement_id": f.fromID,
		"outbound":                []map[string]any{{"good_key": "grain", "threshold": 200}},
	})
	if code != http.StatusCreated {
		t.Fatalf("Create = %d %v", code, resp)
	}
	orderID, _ := resp["id"].(string)

	shipID := f.shipForOrder(t, f.fromID)
	if _, err := f.pool.Exec(ctx,
		`UPDATE standing_orders SET ship_unit_id = $2 WHERE id = $1`, orderID, shipID,
	); err != nil {
		t.Fatalf("bind ship to order: %v", err)
	}
	if _, err := f.pool.Exec(ctx, `UPDATE units SET status = 'freighting' WHERE id = $1`, shipID); err != nil {
		t.Fatalf("mark ship freighting: %v", err)
	}
	var transportID uuid.UUID
	if err := f.pool.QueryRow(ctx,
		`INSERT INTO transports
		   (world_id, owner_id, kind, origin_id, dest_id, category,
		    origin_q, origin_r, dest_q, dest_r, departs_at, arrives_at, due_tick,
		    status, interceptable, standing_order_id, ship_unit_id)
		 VALUES ($1, (SELECT owner_id FROM settlements WHERE id = $2), 'standing_order_out', $2, $3, 'naval',
		         0, 0, 4, 0, now(), now() + interval '1 hour', 1,
		         'in_transit', true, $4, $5)
		 RETURNING id`,
		f.worldID, f.fromID, f.toID, orderID, shipID,
	).Scan(&transportID); err != nil {
		t.Fatalf("insert in-flight leg: %v", err)
	}

	code, resp = f.do(t, http.MethodDelete, base+"/"+orderID, nil)
	if code != http.StatusNoContent {
		t.Fatalf("Delete = %d %v", code, resp)
	}

	var status string
	if err := f.pool.QueryRow(ctx, `SELECT status FROM units WHERE id = $1`, shipID).Scan(&status); err != nil {
		t.Fatalf("read ship status: %v", err)
	}
	if status != "freighting" {
		t.Errorf("ship status right after delete = %q, want still freighting (leg not landed yet)", status)
	}

	var orphanedOrderID *uuid.UUID
	if err := f.pool.QueryRow(ctx, `SELECT standing_order_id FROM transports WHERE id = $1`, transportID).
		Scan(&orphanedOrderID); err != nil {
		t.Fatalf("read transport standing_order_id: %v", err)
	}
	if orphanedOrderID != nil {
		t.Errorf("transport.standing_order_id after delete = %v, want NULL (ON DELETE SET NULL)", orphanedOrderID)
	}
}
