package handlers

// Regression test for the trade-offer good-key validation gap: MessengerHandler.Send
// validated a trade_offer's want_good/offer_good against nothing but emptiness and the
// silver/gold special case (rows ~91-119 in messenger.go before this fix). An unknown
// key ("Tin" instead of "tin") behaved DIFFERENTLY depending on offer kind:
//
//   - sell: the escrow UPDATE (settlement_goods WHERE good_key=$3) matched zero rows for
//     an unknown key and was misreported as insufficientTradeMsg("seller", …) — "you
//     don't have enough Tin" when the real problem is that "Tin" isn't a good at all.
//   - buy: escrow only ever touches settlement_goods good_key='silver' — the unknown
//     want_good was never checked anywhere. The offer was created, the buyer's silver
//     escrowed, and the messenger delivered a trade offer NO ONE could ever accept
//     (TradeAccept's UPDATE on the unknown good_key also matches zero rows → stuck
//     "insufficient" 422 forever). The silver sat locked until OfferExpiryTicks (7
//     in-game days) refunded it.
//
// This file tests the fix: both offer kinds are rejected with 400 before any silver or
// goods are escrowed, naming the bad key and the valid ones. Silver/gold and cult keep
// their own dedicated, unchanged rejection messages (silver = payment currency; cult =
// temple labor, never a tradeable good — see migration 094_cult_is_not_a_good, which
// deleted cult from settlement_goods entirely and left the goods.cult row only as an FK
// anchor for settlement_labor). A valid key ("tin") must still go through exactly as
// before — no regression in escrow or expiry.

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
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func goodValidationTestPool(t *testing.T) *pgxpool.Pool {
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

// goodValidationFixture creates one active world and TWO players — a caller
// (owns origin) and a counterparty (owns destination) — with settlements 5
// hexes apart. Two distinct owners are required: Send's own trade_offer
// precondition (capabilities.CanTradeOffer/CanSell, messenger.go ~139-151)
// gates on visibleForeignSettlements(), which explicitly excludes settlements
// owner_id = caller (internal/capabilities/context.go:261,
// `s.owner_id != $2`) — same-owner settlements would fail that gate before
// ever reaching the good-key check under test here, regardless of this fix.
func goodValidationFixture(t *testing.T, pool *pgxpool.Pool) (worldID, originID, destID uuid.UUID, accessToken string) {
	t.Helper()
	ctx := context.Background()

	// settled()/current_world_tick() needs an active world (one_active_world
	// partial unique index) — see unit_arrival_colonize_test.go.
	if _, err := pool.Exec(ctx,
		`UPDATE worlds SET status = 'archived' WHERE status = 'active' AND name LIKE 'test-world-%'`,
	); err != nil {
		t.Fatalf("archive leftover active test worlds: %v", err)
	}
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
	callerName := "goodval-caller-" + uuid.New().String()
	accessToken, _, err := authSvc.Register(ctx, callerName, callerName+"@test.invalid", "x")
	if err != nil {
		t.Fatalf("register test caller: %v", err)
	}
	claims, err := authSvc.ValidateAccessToken(accessToken)
	if err != nil {
		t.Fatalf("validate minted token: %v", err)
	}
	callerID := claims.PlayerID

	counterName := "goodval-counter-" + uuid.New().String()
	counterToken, _, err := authSvc.Register(ctx, counterName, counterName+"@test.invalid", "x")
	if err != nil {
		t.Fatalf("register test counterparty: %v", err)
	}
	counterClaims, err := authSvc.ValidateAccessToken(counterToken)
	if err != nil {
		t.Fatalf("validate counterparty token: %v", err)
	}
	counterID := counterClaims.PlayerID

	var originProvinceID, destProvinceID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO provinces (world_id, map_q, map_r, terrain_type) VALUES ($1, 0, 0, 'plains') RETURNING id`,
		worldID,
	).Scan(&originProvinceID); err != nil {
		t.Fatalf("create origin province: %v", err)
	}
	if err := pool.QueryRow(ctx,
		`INSERT INTO provinces (world_id, map_q, map_r, terrain_type) VALUES ($1, 5, 0, 'plains') RETURNING id`,
		worldID,
	).Scan(&destProvinceID); err != nil {
		t.Fatalf("create dest province: %v", err)
	}
	if err := pool.QueryRow(ctx,
		`INSERT INTO settlements (world_id, province_id, name, culture_id, owner_id, control_type, is_capital)
		 VALUES ($1, $2, 'GoodvalOrigin', 'akhaier', $3, 'capital', true) RETURNING id`,
		worldID, originProvinceID, callerID,
	).Scan(&originID); err != nil {
		t.Fatalf("create origin settlement: %v", err)
	}
	if err := pool.QueryRow(ctx,
		`INSERT INTO settlements (world_id, province_id, name, culture_id, owner_id, control_type, is_capital)
		 VALUES ($1, $2, 'GoodvalDest', 'akhaier', $3, 'capital', true) RETURNING id`,
		worldID, destProvinceID, counterID,
	).Scan(&destID); err != nil {
		t.Fatalf("create dest settlement: %v", err)
	}
	return worldID, originID, destID, accessToken
}

func goodValidationRouter(pool *pgxpool.Pool, accessToken string) *chi.Mux {
	authSvc := auth.NewService(pool, "test-secret")
	clk := clock.NewTestClock(time.Now())
	mh := NewMessengerHandler(pool, events.NewScheduler(pool, clk), clk, nil)
	r := chi.NewRouter()
	r.Use(auth.Middleware(authSvc))
	r.Post("/worlds/{worldID}/settlements/{settlementID}/messengers", mh.Send)
	return r
}

func settlementSilver(t *testing.T, pool *pgxpool.Pool, settlementID uuid.UUID) float64 {
	t.Helper()
	var amt float64
	err := pool.QueryRow(context.Background(),
		`SELECT COALESCE(settled(amount, rate, calc_tick), 0) FROM settlement_goods
		 WHERE settlement_id = $1 AND good_key = 'silver'`,
		settlementID,
	).Scan(&amt)
	if err != nil {
		t.Fatalf("read settlement silver: %v", err)
	}
	return amt
}

func sendTradeOffer(t *testing.T, r *chi.Mux, accessToken string, worldID, originID, destID uuid.UUID, tradeOfferJSON string) *httptest.ResponseRecorder {
	t.Helper()
	body := `{"destination_id":"` + destID.String() + `","message":"trade?","trade_offer":` + tradeOfferJSON + `}`
	req := httptest.NewRequest(http.MethodPost,
		"/worlds/"+worldID.String()+"/settlements/"+originID.String()+"/messengers",
		strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec
}

// TestSend_TradeOfferBuy_UnknownGoodRejected is the strongest red-before-green
// case: a "buy" offer's want_good was never validated against the goods
// catalog at all (only silver escrow ran). On unfixed code this returns
// 200/201 and deducts the buyer's silver, creating a dead offer nobody can
// ever accept. The fix must reject it with 400 BEFORE any silver moves.
func TestSend_TradeOfferBuy_UnknownGoodRejected(t *testing.T) {
	pool := goodValidationTestPool(t)
	worldID, originID, destID, accessToken := goodValidationFixture(t, pool)

	// Give the origin plenty of silver so a wrongful escrow would visibly
	// succeed if the good-key check were missing — the red run must fail
	// because of AVAILABLE silver being deducted, not because it lacked funds.
	if _, err := pool.Exec(context.Background(),
		`INSERT INTO settlement_goods (settlement_id, good_key, amount, rate, cap, calc_tick)
		 VALUES ($1, 'silver', 500, 0, 10000, 0)`,
		originID,
	); err != nil {
		t.Fatalf("seed origin silver: %v", err)
	}
	silverBefore := settlementSilver(t, pool, originID)

	r := goodValidationRouter(pool, accessToken)
	rec := sendTradeOffer(t, r, accessToken, worldID, originID, destID,
		`{"kind":"buy","want_good":"Tin","want_qty":10,"offer_silver":50}`)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("Send(buy want_good=Tin) = %d, want 400: %s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("parse response: %v", err)
	}
	if !strings.Contains(resp.Error, "Tin") {
		t.Errorf("error message %q does not name the unknown key \"Tin\"", resp.Error)
	}
	if !strings.Contains(resp.Error, "tin") {
		t.Errorf("error message %q does not list the valid good \"tin\"", resp.Error)
	}

	silverAfter := settlementSilver(t, pool, originID)
	if silverAfter != silverBefore {
		t.Errorf("origin silver changed from %.0f to %.0f — escrow ran on a rejected offer", silverBefore, silverAfter)
	}
}

// TestSend_TradeOfferSell_UnknownGoodRejected covers the sell branch: on
// unfixed code the escrow UPDATE against good_key='Tin' matches zero rows and
// is misreported as "seller has insufficient Tin" — the fix must reject with
// 400 before that misleading 422 path is even reached.
func TestSend_TradeOfferSell_UnknownGoodRejected(t *testing.T) {
	pool := goodValidationTestPool(t)
	worldID, originID, destID, accessToken := goodValidationFixture(t, pool)

	// canSell's precondition (internal/capabilities.anySellableGood) only needs
	// SOME sellable good in stock — not specifically the one offered — to let
	// the request past the aggregate gate and down to the good-key check.
	if _, err := pool.Exec(context.Background(),
		`INSERT INTO settlement_goods (settlement_id, good_key, amount, rate, cap, calc_tick)
		 VALUES ($1, 'copper', 100, 0, 1000, 0)`,
		originID,
	); err != nil {
		t.Fatalf("seed origin copper: %v", err)
	}

	r := goodValidationRouter(pool, accessToken)
	rec := sendTradeOffer(t, r, accessToken, worldID, originID, destID,
		`{"kind":"sell","offer_good":"Tin","offer_qty":10,"want_silver":50}`)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("Send(sell offer_good=Tin) = %d, want 400: %s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("parse response: %v", err)
	}
	if strings.Contains(resp.Error, "insufficient") {
		t.Errorf("error message %q is the misleading insufficient-goods message — "+
			"an unknown good must be reported as unknown, not as a shortfall", resp.Error)
	}
	if !strings.Contains(resp.Error, "Tin") {
		t.Errorf("error message %q does not name the unknown key \"Tin\"", resp.Error)
	}
}

// TestSend_TradeOfferSell_KnownGoodStillReportsInsufficient pins the OTHER
// half of the diagnosis: a KNOWN good the seller merely lacks enough of must
// keep going through insufficientTradeMsg — the fix must not swallow real
// shortfalls into the new "unknown good" branch.
func TestSend_TradeOfferSell_KnownGoodStillReportsInsufficient(t *testing.T) {
	pool := goodValidationTestPool(t)
	worldID, originID, destID, accessToken := goodValidationFixture(t, pool)

	// Origin has copper (satisfies canSell's "some sellable good" precondition)
	// but zero tin — a real (known-good) shortfall on the good actually offered.
	if _, err := pool.Exec(context.Background(),
		`INSERT INTO settlement_goods (settlement_id, good_key, amount, rate, cap, calc_tick)
		 VALUES ($1, 'copper', 100, 0, 1000, 0)`,
		originID,
	); err != nil {
		t.Fatalf("seed origin copper: %v", err)
	}

	r := goodValidationRouter(pool, accessToken)
	rec := sendTradeOffer(t, r, accessToken, worldID, originID, destID,
		`{"kind":"sell","offer_good":"tin","offer_qty":10,"want_silver":50}`)

	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("Send(sell offer_good=tin, seller has 0) = %d, want 422: %s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("parse response: %v", err)
	}
	if !strings.Contains(resp.Error, "insufficient") {
		t.Errorf("error message %q should be the insufficient-goods message for a real shortfall on a known good", resp.Error)
	}
}

// TestSend_TradeOfferBuy_SilverAndCultStillRejected pins the two existing
// invariants (silver = payment currency, cult = temple labor) so the new
// catalog check cannot loosen either: silver's rejection is unchanged, and
// cult — which DOES have a row in the `goods` table (mig 094 kept it only as
// an FK anchor for settlement_labor) — must still be refused for trade even
// though it exists in the catalog.
func TestSend_TradeOfferBuy_SilverAndCultStillRejected(t *testing.T) {
	pool := goodValidationTestPool(t)
	worldID, originID, destID, accessToken := goodValidationFixture(t, pool)
	r := goodValidationRouter(pool, accessToken)

	// Seed silver so canTradeOffer's own precondition (silver > 0 to offer)
	// cannot be the reason either rejection fires — isolates the assertion to
	// the silver/cult-specific checks under test, not an unrelated insolvency.
	if _, err := pool.Exec(context.Background(),
		`INSERT INTO settlement_goods (settlement_id, good_key, amount, rate, cap, calc_tick)
		 VALUES ($1, 'silver', 500, 0, 10000, 0)`,
		originID,
	); err != nil {
		t.Fatalf("seed origin silver: %v", err)
	}

	silverRec := sendTradeOffer(t, r, accessToken, worldID, originID, destID,
		`{"kind":"buy","want_good":"silver","want_qty":10,"offer_silver":50}`)
	if silverRec.Code != http.StatusBadRequest {
		t.Fatalf("Send(buy want_good=silver) = %d, want 400: %s", silverRec.Code, silverRec.Body.String())
	}
	var silverResp struct {
		Error string `json:"error"`
	}
	_ = json.Unmarshal(silverRec.Body.Bytes(), &silverResp)
	const wantSilverMsg = "cannot trade for silver — silver is the payment currency, not a tradeable good"
	if silverResp.Error != wantSilverMsg {
		t.Errorf("silver rejection message = %q, want unchanged %q", silverResp.Error, wantSilverMsg)
	}

	cultRec := sendTradeOffer(t, r, accessToken, worldID, originID, destID,
		`{"kind":"buy","want_good":"cult","want_qty":10,"offer_silver":50}`)
	if cultRec.Code != http.StatusBadRequest {
		t.Fatalf("Send(buy want_good=cult) = %d, want 400: %s", cultRec.Code, cultRec.Body.String())
	}
	var cultResp struct {
		Error string `json:"error"`
	}
	_ = json.Unmarshal(cultRec.Body.Bytes(), &cultResp)
	if !strings.Contains(cultResp.Error, "cult") || !strings.Contains(cultResp.Error, "temple labor") {
		t.Errorf("cult rejection message = %q, want it to name cult as temple labor / not tradeable", cultResp.Error)
	}
}

// TestSend_TradeOfferBuy_ValidGoodStillWorks is the no-regression pin: a
// legitimate offer for a real, tradeable good ("tin") must still create the
// messenger, escrow the buyer's silver, and return 201 — exactly as before
// this fix.
func TestSend_TradeOfferBuy_ValidGoodStillWorks(t *testing.T) {
	pool := goodValidationTestPool(t)
	worldID, originID, destID, accessToken := goodValidationFixture(t, pool)

	if _, err := pool.Exec(context.Background(),
		`INSERT INTO settlement_goods (settlement_id, good_key, amount, rate, cap, calc_tick)
		 VALUES ($1, 'silver', 500, 0, 10000, 0)`,
		originID,
	); err != nil {
		t.Fatalf("seed origin silver: %v", err)
	}
	silverBefore := settlementSilver(t, pool, originID)

	r := goodValidationRouter(pool, accessToken)
	rec := sendTradeOffer(t, r, accessToken, worldID, originID, destID,
		`{"kind":"buy","want_good":"tin","want_qty":10,"offer_silver":50}`)

	if rec.Code != http.StatusCreated {
		t.Fatalf("Send(buy want_good=tin) = %d, want 201: %s", rec.Code, rec.Body.String())
	}
	var resp struct {
		ID uuid.UUID `json:"id"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("parse response: %v", err)
	}
	if resp.ID == uuid.Nil {
		t.Fatal("no messenger id in response")
	}

	silverAfter := settlementSilver(t, pool, originID)
	if silverAfter != silverBefore-50 {
		t.Errorf("origin silver = %.0f, want %.0f (50 escrowed for a valid offer)", silverAfter, silverBefore-50)
	}

	var status, tradeOfferStatus string
	var expiresAt *time.Time
	if err := pool.QueryRow(context.Background(),
		`SELECT status, trade_offer->>'status', expires_at FROM messengers WHERE id = $1`,
		resp.ID,
	).Scan(&status, &tradeOfferStatus, &expiresAt); err != nil {
		t.Fatalf("read seeded messenger: %v", err)
	}
	if tradeOfferStatus != "pending" {
		t.Errorf("trade_offer status = %q, want pending", tradeOfferStatus)
	}
	if expiresAt == nil {
		t.Error("expires_at is nil for a trade offer — expiry must still be scheduled")
	}
}
