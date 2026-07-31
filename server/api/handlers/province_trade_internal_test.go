package handlers

// DB integration tests for the internal-transfer physical caravan (Del 3-fas-5,
// fix/intern-overforing-karavan, 2026-07-30): ProvinceHandler.Trade's own→own
// branch used to schedule a ScheduledTradeDelivery with no transports row at
// all — the goods had no map position, no route, and could not be intercepted,
// even though CLAUDE.md's trade-lagret invariant #3 says an internal transfer
// is "logistik utan samtycke, men en fysisk karavan". Proven here with silver
// (AK5 — the tricky recently-became-a-normal-good case), not grain: exactly one
// transports+transport_goods row is created (AK1), the destination is credited
// exactly once with no ScheduledTransportArrival ever scheduled (AK2 — the
// Dispatch-vs-CreateShadow double-credit trap), an intercepted caravan is never
// credited (AK3), and the trade_routes bookkeeping row survives untouched (AK4).
//
// Real Postgres, gated by DATABASE_URL — same harness as recruit_ship_test.go.

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"formatet/megaron/server/internal/auth"
	"formatet/megaron/server/internal/clock"
	"formatet/megaron/server/internal/economy"
	"formatet/megaron/server/internal/events"
	"formatet/megaron/server/internal/notify"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type tradeInternalFixture struct {
	pool           *pgxpool.Pool
	worldID        uuid.UUID
	originID       uuid.UUID
	originProvince uuid.UUID
	destID         uuid.UUID
	playerID       uuid.UUID
	token          string
	router         *chi.Mux
	scheduler      *events.Scheduler
	clk            clock.Clock
}

// setupTradeInternalFixture builds one player owning two settlements (origin at
// hex (0,0), destination at hex (3,0) — distance 3, so the caravan has a real
// route to interpolate a position along) with 1000 silver seeded at the origin.
func setupTradeInternalFixture(t *testing.T) *tradeInternalFixture {
	t.Helper()
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
	username := "trader-" + uuid.New().String()
	token, _, err := authSvc.Register(ctx, username, username+"@test.invalid", "x")
	if err != nil {
		t.Fatalf("register test player: %v", err)
	}
	claims, err := authSvc.ValidateAccessToken(token)
	if err != nil {
		t.Fatalf("validate minted token: %v", err)
	}
	playerID := claims.PlayerID

	mkSettlement := func(name string, q int, capital bool) (settlementID, provinceID uuid.UUID) {
		if err := pool.QueryRow(ctx,
			`INSERT INTO provinces (world_id, map_q, map_r, terrain_type) VALUES ($1, $2, 0, 'plains') RETURNING id`,
			worldID, q,
		).Scan(&provinceID); err != nil {
			t.Fatalf("create province %s: %v", name, err)
		}
		if err := pool.QueryRow(ctx,
			`INSERT INTO settlements (world_id, province_id, name, culture_id, owner_id, control_type, is_capital, state, population)
			 VALUES ($1, $2, $3, 'achaean', $4, 'capital', $5, 'active', 5000) RETURNING id`,
			worldID, provinceID, name, playerID, capital,
		).Scan(&settlementID); err != nil {
			t.Fatalf("create settlement %s: %v", name, err)
		}
		return settlementID, provinceID
	}
	originID, originProvince := mkSettlement("Origin", 0, true)
	destID, _ := mkSettlement("Dest", 3, false)

	if _, err := pool.Exec(ctx,
		`INSERT INTO settlement_goods (settlement_id, good_key, amount, rate, cap, calc_tick)
		 VALUES ($1, 'silver', 100000, 0, 1000000, 0)`,
		originID,
	); err != nil {
		t.Fatalf("seed origin silver: %v", err)
	}

	clk := clock.NewTestClock(time.Now())
	scheduler := events.NewScheduler(pool, clk)
	eventStore := events.NewStore(pool)
	hub := notify.New()
	hub.SetPool(pool)
	ph := NewProvinceHandler(pool, scheduler, clk, economy.SitosConfig{}, eventStore, hub)

	r := chi.NewRouter()
	r.Use(auth.Middleware(authSvc))
	r.Post("/worlds/{worldID}/provinces/{provinceID}/trade", ph.Trade)

	return &tradeInternalFixture{
		pool: pool, worldID: worldID, originID: originID, originProvince: originProvince, destID: destID,
		playerID: playerID, token: token, router: r, scheduler: scheduler, clk: clk,
	}
}

func (f *tradeInternalFixture) post(t *testing.T, path string, body any) (int, map[string]any) {
	t.Helper()
	rf := &recruitShipFixture{accessToken: f.token, router: f.router}
	rec, resp := rf.post(t, path, body)
	return rec.Code, resp
}

// lastTradeDeliveryEvent fetches the payload of the most recently scheduled
// TradeDelivery event for this world, as an events.ScheduledEvent ready to feed
// into economy.DeliveryHandler.Handle — this exercises the exact payload shape
// ProvinceHandler.Trade produced, not a hand-built stand-in.
func (f *tradeInternalFixture) lastTradeDeliveryEvent(t *testing.T) events.ScheduledEvent {
	t.Helper()
	var id int64
	var payload []byte
	if err := f.pool.QueryRow(context.Background(),
		`SELECT id, payload FROM scheduled_events
		  WHERE world_id = $1 AND event_type = 'TradeDelivery'
		  ORDER BY id DESC LIMIT 1`,
		f.worldID,
	).Scan(&id, &payload); err != nil {
		t.Fatalf("no TradeDelivery scheduled: %v", err)
	}
	return events.ScheduledEvent{ID: id, WorldID: f.worldID, Payload: payload}
}

// TestTrade_InternalTransferCreatesPhysicalCaravan is AK1 + AK4 + AK5: the
// own→own /trade branch must leave exactly one transports row (kind='transfer',
// status='in_transit', interceptable, correct origin/dest hex) with exactly one
// transport_goods row for the silver manifest, and the trade_routes bookkeeping
// row must still exist alongside it (the route is bookkeeping, not the mover).
func TestTrade_InternalTransferCreatesPhysicalCaravan(t *testing.T) {
	f := setupTradeInternalFixture(t)
	ctx := context.Background()

	code, resp := f.post(t, "/worlds/"+f.worldID.String()+"/provinces/"+f.originProvince.String()+"/trade",
		map[string]any{"destination_id": f.destID.String(), "good_key": "silver", "quantity": 250.0})
	if code != 201 {
		t.Fatalf("trade returned %d: %v", code, resp)
	}
	routeID, _ := resp["route_id"].(string)
	if routeID == "" {
		t.Fatalf("no route_id in response: %v", resp)
	}

	// AK4: trade_routes row exists, offered quantity unchanged.
	var routeQty float64
	if err := f.pool.QueryRow(ctx,
		`SELECT quantity FROM trade_routes WHERE id = $1 AND origin_id = $2 AND destination_id = $3`,
		routeID, f.originID, f.destID,
	).Scan(&routeQty); err != nil {
		t.Fatalf("no trade_routes row: %v", err)
	}
	if routeQty != 250 {
		t.Fatalf("trade_routes.quantity = %v, want 250", routeQty)
	}

	// AK1: exactly one transport row, kind='transfer', in_transit, interceptable,
	// hexes matching origin (0,0) → dest (3,0).
	var transportID uuid.UUID
	var kind, status string
	var originQ, originR, destQ, destR int
	var interceptable bool
	if err := f.pool.QueryRow(ctx,
		`SELECT id, kind, status, origin_q, origin_r, dest_q, dest_r, interceptable
		   FROM transports WHERE world_id = $1 AND origin_id = $2 AND dest_id = $3`,
		f.worldID, f.originID, f.destID,
	).Scan(&transportID, &kind, &status, &originQ, &originR, &destQ, &destR, &interceptable); err != nil {
		t.Fatalf("no transports row created for internal transfer: %v", err)
	}
	if kind != "transfer" {
		t.Errorf("transport kind = %q, want transfer", kind)
	}
	if status != "in_transit" {
		t.Errorf("transport status = %q, want in_transit", status)
	}
	if originQ != 0 || originR != 0 || destQ != 3 || destR != 0 {
		t.Errorf("transport hexes = (%d,%d)->(%d,%d), want (0,0)->(3,0)", originQ, originR, destQ, destR)
	}
	if !interceptable {
		t.Errorf("transport interceptable = false, want true (Timothy must decide if internal logistics should be exempt — see report)")
	}

	var count int
	if err := f.pool.QueryRow(ctx,
		`SELECT count(*) FROM transports WHERE world_id = $1 AND origin_id = $2 AND dest_id = $3`,
		f.worldID, f.originID, f.destID,
	).Scan(&count); err != nil || count != 1 {
		t.Fatalf("transports row count = %d (err %v), want exactly 1", count, err)
	}

	// AK1: manifest carries the silver.
	var manifestQty float64
	if err := f.pool.QueryRow(ctx,
		`SELECT quantity FROM transport_goods WHERE transport_id = $1 AND good_key = 'silver'`,
		transportID,
	).Scan(&manifestQty); err != nil {
		t.Fatalf("no transport_goods manifest row: %v", err)
	}
	if manifestQty != 250 {
		t.Fatalf("transport_goods.quantity = %v, want 250", manifestQty)
	}

	// The scheduled delivery payload must carry this exact transport's id, else
	// the interception veto in economy.DeliveryHandler can never see it.
	ev := f.lastTradeDeliveryEvent(t)
	var payload struct {
		TransportID uuid.UUID `json:"transport_id"`
	}
	if err := json.Unmarshal(ev.Payload, &payload); err != nil {
		t.Fatalf("unmarshal scheduled payload: %v", err)
	}
	if payload.TransportID != transportID {
		t.Errorf("scheduled event transport_id = %s, want %s", payload.TransportID, transportID)
	}
}

// TestTrade_InternalTransferCreditsDestinationExactlyOnce is AK2 + AK5: this is
// the Dispatch-vs-CreateShadow trap the plan warned about. Dispatch would ALSO
// schedule a ScheduledTransportArrival, crediting the destination a second time
// when that generic handler runs. We assert structurally that no such event was
// ever scheduled (ruling out the double-credit path regardless of the delivery
// handler's independent 5% storm-loss roll), then drive the actual
// ScheduledTradeDelivery event through economy.DeliveryHandler — retrying with
// fresh transfers the way trade_transport_test.go does, since that roll is
// pre-existing, unrelated behaviour this slice does not touch — and check the
// destination gained exactly the one delivered amount, not double.
func TestTrade_InternalTransferCreditsDestinationExactlyOnce(t *testing.T) {
	f := setupTradeInternalFixture(t)
	ctx := context.Background()

	deliveryHandler := economy.NewDeliveryHandler(f.pool, events.NewStore(f.pool), nil, f.scheduler)

	var lastTransportID uuid.UUID
	delivered := false
	for i := 0; i < 25 && !delivered; i++ {
		code, resp := f.post(t, "/worlds/"+f.worldID.String()+"/provinces/"+f.originProvince.String()+"/trade",
			map[string]any{"destination_id": f.destID.String(), "good_key": "silver", "quantity": 50.0})
		if code != 201 {
			t.Fatalf("trade returned %d: %v", code, resp)
		}
		ev := f.lastTradeDeliveryEvent(t)
		var payload struct {
			TransportID uuid.UUID `json:"transport_id"`
		}
		_ = json.Unmarshal(ev.Payload, &payload)
		lastTransportID = payload.TransportID

		if err := deliveryHandler.Handle(ctx, ev); err != nil {
			t.Fatalf("delivery handle: %v", err)
		}
		var status string
		_ = f.pool.QueryRow(ctx, `SELECT status FROM transports WHERE id = $1`, lastTransportID).Scan(&status)
		delivered = status == "delivered"
	}
	if !delivered {
		t.Fatalf("internal transfer never delivered across %d retries (all lost to storm?)", 25)
	}

	// Structural guard against the Dispatch trap: CreateShadow must never have
	// scheduled a ScheduledTransportArrival for this (or any) transport in this
	// test — that event type is what a second, generic credit would ride in on.
	var arrivalCount int
	if err := f.pool.QueryRow(ctx,
		`SELECT count(*) FROM scheduled_events WHERE world_id = $1 AND event_type = 'TransportArrival'`,
		f.worldID,
	).Scan(&arrivalCount); err != nil {
		t.Fatalf("count TransportArrival events: %v", err)
	}
	if arrivalCount != 0 {
		t.Fatalf("TransportArrival events = %d, want 0 (CreateShadow must not schedule a generic arrival — that's the Dispatch double-credit trap)", arrivalCount)
	}

	// Destination credited exactly the one delivered leg's amount (50), never 100.
	var destSilver float64
	_ = f.pool.QueryRow(ctx,
		`SELECT COALESCE(settled(amount, rate, calc_tick), 0) FROM settlement_goods
		 WHERE settlement_id = $1 AND good_key = 'silver'`, f.destID).Scan(&destSilver)
	if destSilver != 50 {
		t.Fatalf("dest silver = %v, want exactly 50 (single credit, no double-credit)", destSilver)
	}
}

// TestTrade_InterceptedInternalTransferNotCredited is AK3 + AK5: today the
// internal-transfer branch schedules no transport at all, so it has no
// transport_id and the interception veto in economy.DeliveryHandler (trade.go
// ~218-240) can never see it — an enemy sentry can never seize a wanax's own
// supply line because there is nothing physical to seize. This proves the veto
// now fires: an intercepted caravan must never credit the destination, and the
// transport must stay 'intercepted', not flip to delivered.
func TestTrade_InterceptedInternalTransferNotCredited(t *testing.T) {
	f := setupTradeInternalFixture(t)
	ctx := context.Background()

	code, resp := f.post(t, "/worlds/"+f.worldID.String()+"/provinces/"+f.originProvince.String()+"/trade",
		map[string]any{"destination_id": f.destID.String(), "good_key": "silver", "quantity": 400.0})
	if code != 201 {
		t.Fatalf("trade returned %d: %v", code, resp)
	}

	var transportID uuid.UUID
	if err := f.pool.QueryRow(ctx,
		`SELECT id FROM transports WHERE world_id = $1 AND origin_id = $2 AND dest_id = $3`,
		f.worldID, f.originID, f.destID,
	).Scan(&transportID); err != nil {
		t.Fatalf("no transports row: %v", err)
	}

	// Simulate what internal/transport/intercept.go's scan does on seizure —
	// deterministic, no random roll (the veto check runs before the storm roll).
	if _, err := f.pool.Exec(ctx,
		`UPDATE transports SET status = 'intercepted', updated_at = now() WHERE id = $1`, transportID,
	); err != nil {
		t.Fatalf("simulate interception: %v", err)
	}

	deliveryHandler := economy.NewDeliveryHandler(f.pool, events.NewStore(f.pool), nil, f.scheduler)
	ev := f.lastTradeDeliveryEvent(t)
	if err := deliveryHandler.Handle(ctx, ev); err != nil {
		t.Fatalf("delivery handle: %v", err)
	}

	var destSilver float64
	_ = f.pool.QueryRow(ctx,
		`SELECT COALESCE(settled(amount, rate, calc_tick), 0) FROM settlement_goods
		 WHERE settlement_id = $1 AND good_key = 'silver'`, f.destID).Scan(&destSilver)
	if destSilver != 0 {
		t.Fatalf("dest silver = %v, want 0 (intercepted internal transfer must not deliver)", destSilver)
	}

	var status string
	_ = f.pool.QueryRow(ctx, `SELECT status FROM transports WHERE id = $1`, transportID).Scan(&status)
	if status != "intercepted" {
		t.Errorf("transport status = %q, want intercepted (must not be overwritten to delivered)", status)
	}

	// AK4: trade_routes row survives, marked resolved so it isn't retried.
	var resolved bool
	if err := f.pool.QueryRow(ctx,
		`SELECT resolved FROM trade_routes WHERE origin_id = $1 AND destination_id = $2 ORDER BY id DESC LIMIT 1`,
		f.originID, f.destID,
	).Scan(&resolved); err != nil {
		t.Fatalf("no trade_routes row survives interception: %v", err)
	}
	if !resolved {
		t.Errorf("trade_routes.resolved = false, want true after interception")
	}
}
