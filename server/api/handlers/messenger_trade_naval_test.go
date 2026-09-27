package handlers

// DB integration tests for slice 1b (megaron_plan_sjohandel_mellan_spelare.md):
// a negotiated trade offer between two coastal-or-harboured Wanaxes goes to
// sea on the INITIATOR's own ship. Real Postgres, gated by DATABASE_URL — same
// harness as province_trade_naval_test.go (internal transfers) and
// goodValidationFixture (two distinct owners, messenger.go's Send/TradeAccept).
//
// §5 acceptance covered here:
//  1. Sell/buy naval round trip: ship bound at accept, response names it.
//  2. Send with no free ship and no land route -> 422; with a land route ->
//     lands as an ordinary (unbound) offer.
//  3. Accept when the initiator's ship was taken since Send -> 422, offer
//     stays pending.
//  (R4/R5 — same ship carries leg 2, ben-2 capture — are proven directly at
//  the economy/transport layer: trade_naval_player_test.go,
//  intercept_naval_test.go.)

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"formatet/megaron/server/internal/auth"
	"formatet/megaron/server/internal/clock"
	"formatet/megaron/server/internal/events"
	"formatet/megaron/server/internal/notify"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func navalPlayerTestPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL not set — skipping DB integration test")
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatalf("connect to test database: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

type navalPlayerFixture struct {
	pool                              *pgxpool.Pool
	worldID                           uuid.UUID
	initiatorID, counterpartyID       uuid.UUID
	initiatorToken, counterpartyToken string
	router                            *chi.Mux
	clk                               clock.Clock
}

func setupNavalPlayerFixture(t *testing.T) *navalPlayerFixture {
	t.Helper()
	pool := navalPlayerTestPool(t)
	ctx := context.Background()

	if _, err := pool.Exec(ctx, `UPDATE worlds SET status = 'archived' WHERE status = 'active'`); err != nil {
		t.Fatalf("archive leftover active test worlds: %v", err)
	}
	var worldID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO worlds (name, status) VALUES ($1, 'active') RETURNING id`,
		"test-1b-h-"+uuid.New().String(),
	).Scan(&worldID); err != nil {
		t.Fatalf("create test world: %v", err)
	}
	t.Cleanup(func() { _, _ = pool.Exec(ctx, `UPDATE worlds SET status = 'archived' WHERE id = $1`, worldID) })

	authSvc := auth.NewService(pool, "test-secret")
	initiatorToken, _, err := authSvc.Register(ctx, "initiator-"+uuid.New().String(), "x")
	if err != nil {
		t.Fatalf("register initiator: %v", err)
	}
	initiatorClaims, err := authSvc.ValidateAccessToken(initiatorToken)
	if err != nil {
		t.Fatalf("validate initiator token: %v", err)
	}
	counterpartyToken, _, err := authSvc.Register(ctx, "counterparty-"+uuid.New().String(), "x")
	if err != nil {
		t.Fatalf("register counterparty: %v", err)
	}
	counterpartyClaims, err := authSvc.ValidateAccessToken(counterpartyToken)
	if err != nil {
		t.Fatalf("validate counterparty token: %v", err)
	}

	clk := clock.NewTestClock(time.Now())
	scheduler := events.NewScheduler(pool, clk)
	hub := notify.New()
	hub.SetPool(pool)
	mh := NewMessengerHandler(pool, scheduler, clk, hub)
	// eventStore + pah (megaron_plan_ordna_passage.md 3b-3): added for the
	// "Arrange passage" verb, which needs an *events.Store for combat.
	// StartMarch — PassageHandler is a separate handler precisely so this
	// fixture's own mh construction above never has to change.
	eventStore := events.NewStore(pool)
	pah := NewPassageHandler(pool, scheduler, eventStore, clk)

	r := chi.NewRouter()
	r.Use(auth.Middleware(authSvc))
	r.Post("/worlds/{worldID}/settlements/{settlementID}/messengers", mh.Send)
	r.Post("/worlds/{worldID}/messengers/{messengerID}/trade-accept", mh.TradeAccept)
	r.Post("/worlds/{worldID}/messengers/{messengerID}/reply", mh.Reply)
	r.Post("/worlds/{worldID}/messengers/{messengerID}/passage", pah.Arrange)

	return &navalPlayerFixture{
		pool: pool, worldID: worldID,
		initiatorID: initiatorClaims.PlayerID, counterpartyID: counterpartyClaims.PlayerID,
		initiatorToken: initiatorToken, counterpartyToken: counterpartyToken,
		router: r, clk: clk,
	}
}

func (f *navalPlayerFixture) mapTile(t *testing.T, q, r int, terrain string) {
	t.Helper()
	if _, err := f.pool.Exec(context.Background(),
		`INSERT INTO map_tiles (world_id, q, r, terrain) VALUES ($1, $2, $3, $4)`,
		f.worldID, q, r, terrain,
	); err != nil {
		t.Fatalf("insert map tile (%d,%d)=%s: %v", q, r, terrain, err)
	}
}

// settlement creates a settlement at (q,0) owned by ownerID, with the given
// coastal flag, and seeds its own plains hex.
func (f *navalPlayerFixture) settlement(t *testing.T, name string, q int, ownerID uuid.UUID, coastal bool) uuid.UUID {
	t.Helper()
	ctx := context.Background()
	f.mapTile(t, q, 0, "plains")
	var provinceID uuid.UUID
	if err := f.pool.QueryRow(ctx,
		`INSERT INTO provinces (world_id, map_q, map_r, terrain_type, coastal) VALUES ($1, $2, 0, 'plains', $3) RETURNING id`,
		f.worldID, q, coastal,
	).Scan(&provinceID); err != nil {
		t.Fatalf("create province %s: %v", name, err)
	}
	var settlementID uuid.UUID
	if err := f.pool.QueryRow(ctx,
		`INSERT INTO settlements (world_id, province_id, name, culture_id, owner_id, control_type, is_capital, state, population)
		 VALUES ($1, $2, $3, 'achaean', $4, 'capital', true, 'active', 5000) RETURNING id`,
		f.worldID, provinceID, name, ownerID,
	).Scan(&settlementID); err != nil {
		t.Fatalf("create settlement %s: %v", name, err)
	}
	return settlementID
}

func (f *navalPlayerFixture) good(t *testing.T, settlementID uuid.UUID, key string, amount float64) {
	t.Helper()
	if _, err := f.pool.Exec(context.Background(),
		`INSERT INTO settlement_goods (settlement_id, good_key, amount, rate, cap, calc_tick)
		 VALUES ($1, $2, $3, 0, 1000000, 0)`,
		settlementID, key, amount,
	); err != nil {
		t.Fatalf("seed %s=%v at %s: %v", key, amount, settlementID, err)
	}
}

func (f *navalPlayerFixture) ship(t *testing.T, settlementID, ownerID uuid.UUID, shipType string) uuid.UUID {
	t.Helper()
	var id uuid.UUID
	if err := f.pool.QueryRow(context.Background(),
		`INSERT INTO units (world_id, owner_id, type, category, size, crew, status, settlement_id)
		 VALUES ($1, $2, $3, 'naval', 1, 10, 'garrison', $4) RETURNING id`,
		f.worldID, ownerID, shipType, settlementID,
	).Scan(&id); err != nil {
		t.Fatalf("create ship %s at %s: %v", shipType, settlementID, err)
	}
	return id
}

func (f *navalPlayerFixture) post(t *testing.T, token, path string, body any) (int, map[string]any) {
	t.Helper()
	b, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal request body: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(string(b)))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	f.router.ServeHTTP(rec, req)
	var resp map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	return rec.Code, resp
}

// deliverMessenger marks a just-sent messenger 'delivered' directly (skipping
// the real courier travel time, same shortcut messenger_inbox_insolvent_test.go
// uses) so TradeAccept's own "status = 'delivered'" precondition is met.
func (f *navalPlayerFixture) deliverMessenger(t *testing.T, messengerID uuid.UUID) {
	t.Helper()
	if _, err := f.pool.Exec(context.Background(),
		`UPDATE messengers SET status = 'delivered' WHERE id = $1`, messengerID,
	); err != nil {
		t.Fatalf("mark messenger delivered: %v", err)
	}
}

// TestMessengerTrade_SellNavalRoundTrip is acceptance criterion 1 (sell kind):
// two coastal cities with a sea lane, the initiator (seller) owns a ship —
// TradeAccept must bind it, dispatch leg 1 as naval, and name the ship in the
// response.
func TestMessengerTrade_SellNavalRoundTrip(t *testing.T) {
	f := setupNavalPlayerFixture(t)

	sellerID := f.settlement(t, "Byblos", 0, f.initiatorID, true)
	buyerID := f.settlement(t, "Ugarit", 5, f.counterpartyID, true)
	for q := 1; q <= 4; q++ {
		f.mapTile(t, q, 0, "coastal_sea")
	}
	shipID := f.ship(t, sellerID, f.initiatorID, "merchantman")
	f.good(t, sellerID, "copper", 100)
	f.good(t, buyerID, "silver", 500)

	code, resp := f.post(t, f.initiatorToken,
		"/worlds/"+f.worldID.String()+"/settlements/"+sellerID.String()+"/messengers",
		map[string]any{
			"destination_id": buyerID.String(),
			"message":        "copper for sale",
			"trade_offer": map[string]any{
				"kind": "sell", "offer_good": "copper", "offer_qty": 20.0, "want_silver": 80.0,
			},
		})
	if code != http.StatusCreated {
		t.Fatalf("Send = %d: %v", code, resp)
	}
	messengerIDStr, _ := resp["id"].(string)
	messengerID, err := uuid.Parse(messengerIDStr)
	if err != nil {
		t.Fatalf("parse messenger id %q: %v", messengerIDStr, err)
	}

	// The ship must NOT be bound yet (R2 — Send never binds).
	var shipStatus string
	if err := f.pool.QueryRow(context.Background(), `SELECT status FROM units WHERE id = $1`, shipID).Scan(&shipStatus); err != nil {
		t.Fatalf("read ship status after Send: %v", err)
	}
	if shipStatus != "garrison" {
		t.Errorf("ship status after Send = %q, want garrison (R2: never bound at send)", shipStatus)
	}

	f.deliverMessenger(t, messengerID)

	code, resp = f.post(t, f.counterpartyToken,
		"/worlds/"+f.worldID.String()+"/messengers/"+messengerID.String()+"/trade-accept", nil)
	if code != http.StatusOK {
		t.Fatalf("TradeAccept = %d: %v", code, resp)
	}
	if respShipID, _ := resp["ship_id"].(string); respShipID != shipID.String() {
		t.Errorf("TradeAccept response ship_id = %v, want %s", resp["ship_id"], shipID)
	}
	if shipName, _ := resp["ship_name"].(string); shipName == "" {
		t.Error("TradeAccept response ship_name is empty, want the ship's display name")
	}

	if err := f.pool.QueryRow(context.Background(), `SELECT status FROM units WHERE id = $1`, shipID).Scan(&shipStatus); err != nil {
		t.Fatalf("read ship status after accept: %v", err)
	}
	if shipStatus != "freighting" {
		t.Errorf("ship status after accept = %q, want freighting", shipStatus)
	}

	var leg1Category string
	var leg1ShipUnitID *uuid.UUID
	if err := f.pool.QueryRow(context.Background(),
		`SELECT category, ship_unit_id FROM transports WHERE world_id = $1 AND kind = 'trade' ORDER BY created_at DESC LIMIT 1`,
		f.worldID,
	).Scan(&leg1Category, &leg1ShipUnitID); err != nil {
		t.Fatalf("no leg1 trade transport found: %v", err)
	}
	if leg1Category != "naval" {
		t.Errorf("leg1 category = %q, want naval", leg1Category)
	}
	if leg1ShipUnitID == nil || *leg1ShipUnitID != shipID {
		t.Errorf("leg1 ship_unit_id = %v, want %s", leg1ShipUnitID, shipID)
	}
}

// TestMessengerTrade_SendNoShipNoLandRejects and
// TestMessengerTrade_SendNoShipButLandFallsBack cover R2's two branches.
func TestMessengerTrade_SendNoShipNoLandRejects(t *testing.T) {
	f := setupNavalPlayerFixture(t)

	sellerID := f.settlement(t, "Byblos", 0, f.initiatorID, true)
	buyerID := f.settlement(t, "Ugarit", 5, f.counterpartyID, true)
	for q := 1; q <= 4; q++ {
		f.mapTile(t, q, 0, "coastal_sea")
	}
	// No ship, no land bridge — two islands.
	f.good(t, sellerID, "copper", 100)
	f.good(t, buyerID, "silver", 500)

	code, resp := f.post(t, f.initiatorToken,
		"/worlds/"+f.worldID.String()+"/settlements/"+sellerID.String()+"/messengers",
		map[string]any{
			"destination_id": buyerID.String(),
			"message":        "copper for sale",
			"trade_offer": map[string]any{
				"kind": "sell", "offer_good": "copper", "offer_qty": 20.0, "want_silver": 80.0,
			},
		})
	if code != http.StatusUnprocessableEntity {
		t.Fatalf("Send = %d, want 422 (no ship, no land route): %v", code, resp)
	}
	errMsg, _ := resp["error"].(string)
	if !strings.Contains(errMsg, "no free galley or merchantman") || !strings.Contains(errMsg, "Byblos") {
		t.Errorf("error = %q, want it to name Byblos and explain no free ship", errMsg)
	}

	// No escrow ran on a rejected Send.
	var copper float64
	_ = f.pool.QueryRow(context.Background(),
		`SELECT settled(amount, rate, calc_tick) FROM settlement_goods WHERE settlement_id = $1 AND good_key = 'copper'`,
		sellerID,
	).Scan(&copper)
	if copper != 100 {
		t.Errorf("seller copper = %v, want unchanged 100 (Send rejected before escrow)", copper)
	}
}

func TestMessengerTrade_SendNoShipButLandFallsBack(t *testing.T) {
	f := setupNavalPlayerFixture(t)

	sellerID := f.settlement(t, "Tyre", 0, f.initiatorID, true)
	buyerID := f.settlement(t, "Sidon", 3, f.counterpartyID, true)
	f.mapTile(t, 1, 0, "plains")
	f.mapTile(t, 2, 0, "plains")
	f.good(t, sellerID, "copper", 100)
	f.good(t, buyerID, "silver", 500)

	code, resp := f.post(t, f.initiatorToken,
		"/worlds/"+f.worldID.String()+"/settlements/"+sellerID.String()+"/messengers",
		map[string]any{
			"destination_id": buyerID.String(),
			"message":        "copper for sale",
			"trade_offer": map[string]any{
				"kind": "sell", "offer_good": "copper", "offer_qty": 20.0, "want_silver": 80.0,
			},
		})
	if code != http.StatusCreated {
		t.Fatalf("Send = %d, want 201 (no ship, but a land bridge exists): %v", code, resp)
	}
}

// TestMessengerTrade_AcceptShipTakenMeanwhile_Returns422AndStaysPending is
// acceptance criterion 3: the initiator's only ship is bound to something
// else between Send and Accept — TradeAccept must reject with 422 and the
// offer must remain 'pending' (not flipped, not consumed).
func TestMessengerTrade_AcceptShipTakenMeanwhile_Returns422AndStaysPending(t *testing.T) {
	f := setupNavalPlayerFixture(t)

	sellerID := f.settlement(t, "Byblos", 0, f.initiatorID, true)
	buyerID := f.settlement(t, "Ugarit", 5, f.counterpartyID, true)
	for q := 1; q <= 4; q++ {
		f.mapTile(t, q, 0, "coastal_sea")
	}
	shipID := f.ship(t, sellerID, f.initiatorID, "merchantman")
	f.good(t, sellerID, "copper", 100)
	f.good(t, buyerID, "silver", 500)

	code, resp := f.post(t, f.initiatorToken,
		"/worlds/"+f.worldID.String()+"/settlements/"+sellerID.String()+"/messengers",
		map[string]any{
			"destination_id": buyerID.String(),
			"message":        "copper for sale",
			"trade_offer": map[string]any{
				"kind": "sell", "offer_good": "copper", "offer_qty": 20.0, "want_silver": 80.0,
			},
		})
	if code != http.StatusCreated {
		t.Fatalf("Send = %d: %v", code, resp)
	}
	messengerID, _ := uuid.Parse(resp["id"].(string))
	f.deliverMessenger(t, messengerID)

	// The ship is taken by something else (e.g. a standing route) before the
	// counterparty gets to accept.
	if _, err := f.pool.Exec(context.Background(),
		`UPDATE units SET status = 'freighting' WHERE id = $1`, shipID,
	); err != nil {
		t.Fatalf("simulate ship taken: %v", err)
	}

	code, resp = f.post(t, f.counterpartyToken,
		"/worlds/"+f.worldID.String()+"/messengers/"+messengerID.String()+"/trade-accept", nil)
	if code != http.StatusUnprocessableEntity {
		t.Fatalf("TradeAccept = %d, want 422 (ship taken since send): %v", code, resp)
	}
	errMsg, _ := resp["error"].(string)
	if !strings.Contains(errMsg, "Byblos") || !strings.Contains(errMsg, "offer stays open") {
		t.Errorf("error = %q, want it to name Byblos and say the offer stays open", errMsg)
	}

	var offerStatus string
	if err := f.pool.QueryRow(context.Background(),
		`SELECT trade_offer->>'status' FROM messengers WHERE id = $1`, messengerID,
	).Scan(&offerStatus); err != nil {
		t.Fatalf("read offer status: %v", err)
	}
	if offerStatus != "pending" {
		t.Errorf("offer status after failed accept = %q, want still pending", offerStatus)
	}

	// Buyer's silver must NOT have been deducted (accept aborted before any flip).
	var buyerSilver float64
	_ = f.pool.QueryRow(context.Background(),
		`SELECT settled(amount, rate, calc_tick) FROM settlement_goods WHERE settlement_id = $1 AND good_key = 'silver'`,
		buyerID,
	).Scan(&buyerSilver)
	if buyerSilver != 500 {
		t.Errorf("buyer silver = %v, want unchanged 500 (accept aborted before any deduction)", buyerSilver)
	}
}

// TestMessengerTrade_SendCapacityCheckRejectsOverloadedManifest is R2's
// capacity gate: a galley (capacity 60) cannot carry a sell offer whose
// heaviest leg exceeds that, even though nothing is bound yet.
func TestMessengerTrade_SendCapacityCheckRejectsOverloadedManifest(t *testing.T) {
	f := setupNavalPlayerFixture(t)

	sellerID := f.settlement(t, "Byblos", 0, f.initiatorID, true)
	buyerID := f.settlement(t, "Ugarit", 5, f.counterpartyID, true)
	for q := 1; q <= 4; q++ {
		f.mapTile(t, q, 0, "coastal_sea")
	}
	f.ship(t, sellerID, f.initiatorID, "galley")
	f.good(t, sellerID, "copper", 1000)
	f.good(t, buyerID, "silver", 5000)

	// Silver weighs 2 (migration 025) — 40 silver = 80 weight, already over a
	// galley's capacity of 60, regardless of copper's own weight.
	code, resp := f.post(t, f.initiatorToken,
		"/worlds/"+f.worldID.String()+"/settlements/"+sellerID.String()+"/messengers",
		map[string]any{
			"destination_id": buyerID.String(),
			"message":        "copper for sale",
			"trade_offer": map[string]any{
				"kind": "sell", "offer_good": "copper", "offer_qty": 1.0, "want_silver": 40.0,
			},
		})
	if code != http.StatusUnprocessableEntity {
		t.Fatalf("Send = %d, want 422 (heaviest leg over galley capacity): %v", code, resp)
	}
	errMsg, _ := resp["error"].(string)
	if !strings.Contains(errMsg, "capacity") {
		t.Errorf("error = %q, want it to mention capacity", errMsg)
	}
}
