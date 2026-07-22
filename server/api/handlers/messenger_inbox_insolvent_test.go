package handlers

// Regression test for Fas 2a: an insolvent pending trade offer used to
// disappear from the inbox ENTIRELY (the solvency check was part of the
// WHERE clause), even though canTradeAccept (internal/capabilities) reports
// such an offer as "pending, but you can't afford it — decline it or wait"
// (HintTradeAcceptInsolvent). With the offer invisible in inbox, there was no
// way to ever discover its id to actually decline it — capabilities and
// Inbox disagreed about what a Wanax could see and act on, exactly the
// divergence `poleia actions trade` referencing an unfindable offer pointed
// at. Fix: affordability is now a DATA column (affordable), not a visibility
// filter — the offer (and its id) always shows up while pending+unexpired.

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"formatet/megaron/server/internal/auth"
	"formatet/megaron/server/internal/clock"
	"formatet/megaron/server/internal/events"
)

func inboxInsolventTestPool(t *testing.T) *pgxpool.Pool {
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

func TestInbox_InsolventPendingOfferStillVisible(t *testing.T) {
	pool := inboxInsolventTestPool(t)
	ctx := context.Background()

	// settled()/solvency EXISTS needs an active world (one_active_world) —
	// see unit_arrival_colonize_test.go for why leftovers must be archived.
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
	username := "insolvent-" + uuid.New().String()
	accessToken, _, err := authSvc.Register(ctx, username, username+"@test.invalid", "x")
	if err != nil {
		t.Fatalf("register test player: %v", err)
	}
	claims, err := authSvc.ValidateAccessToken(accessToken)
	if err != nil {
		t.Fatalf("validate minted token: %v", err)
	}
	playerID := claims.PlayerID

	var originProvinceID, destProvinceID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO provinces (world_id, map_q, map_r, terrain_type) VALUES ($1, 0, 0, 'plains') RETURNING id`,
		worldID,
	).Scan(&originProvinceID); err != nil {
		t.Fatalf("create origin province: %v", err)
	}
	if err := pool.QueryRow(ctx,
		`INSERT INTO provinces (world_id, map_q, map_r, terrain_type) VALUES ($1, 6, 0, 'plains') RETURNING id`,
		worldID,
	).Scan(&destProvinceID); err != nil {
		t.Fatalf("create dest province: %v", err)
	}
	var originID, destID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO settlements (world_id, province_id, name, culture_id, owner_id, control_type, is_capital)
		 VALUES ($1, $2, 'InsolventOrigin', 'akhaier', $3, 'capital', true) RETURNING id`,
		worldID, originProvinceID, playerID,
	).Scan(&originID); err != nil {
		t.Fatalf("create origin settlement: %v", err)
	}
	if err := pool.QueryRow(ctx,
		`INSERT INTO settlements (world_id, province_id, name, culture_id, owner_id, control_type, is_capital)
		 VALUES ($1, $2, 'InsolventDest', 'akhaier', $3, 'colony', false) RETURNING id`,
		worldID, destProvinceID, playerID,
	).Scan(&destID); err != nil {
		t.Fatalf("create dest settlement: %v", err)
	}

	// Destination (buyer, for a "buy" offer it's the SELLER of the good) has
	// ZERO of the wanted good — deliberately insolvent for this offer.
	if _, err := pool.Exec(ctx,
		`INSERT INTO settlement_goods (settlement_id, good_key, amount, rate, cap, calc_tick)
		 VALUES ($1, 'copper', 0, 0, 1000, 0)`,
		destID,
	); err != nil {
		t.Fatalf("seed dest copper (zero): %v", err)
	}

	expiresAt := time.Now().Add(7 * 24 * time.Hour).Truncate(time.Second).UTC()
	// "buy" offer: sender wants 50 copper, offers 80 silver — destination
	// (seller) must hold >= 50 copper to be solvent. It holds 0.
	tradeOffer := `{"kind":"buy","want_good":"copper","want_qty":50,"offer_silver":80,"status":"pending"}`
	var messengerID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO messengers (world_id, sender_id, origin_id, destination_id, message_text, trade_offer,
		                         status, hex_q, hex_r, sent_at, arrives_at, expires_at)
		 VALUES ($1, $2, $3, $4, 'buying copper', $5::jsonb, 'delivered', 0, 0, now(), now(), $6)
		 RETURNING id`,
		worldID, playerID, originID, destID, tradeOffer, expiresAt,
	).Scan(&messengerID); err != nil {
		t.Fatalf("seed messenger: %v", err)
	}

	clk := clock.NewTestClock(time.Now())
	mh := NewMessengerHandler(pool, events.NewScheduler(pool, clk), clk, nil)

	r := chi.NewRouter()
	r.Use(auth.Middleware(authSvc))
	r.Get("/worlds/{worldID}/messengers/inbox", mh.Inbox)

	req := httptest.NewRequest(http.MethodGet, "/worlds/"+worldID.String()+"/messengers/inbox", nil)
	req.Header.Set("Authorization", "Bearer "+accessToken)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("Inbox = %d: %s", rec.Code, rec.Body.String())
	}
	var resp []struct {
		ID         uuid.UUID `json:"id"`
		Affordable *bool     `json:"affordable"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("parse Inbox response: %v", err)
	}
	for _, m := range resp {
		if m.ID == messengerID {
			if m.Affordable == nil {
				t.Fatal("affordable field missing for the trade-offer row")
			}
			if *m.Affordable {
				t.Error("affordable = true, want false (destination holds 0 of the wanted good)")
			}
			return
		}
	}
	t.Fatalf("insolvent pending offer %s is NOT in the inbox — this is the exact phantom-offer bug (Fas 2a): "+
		"present in response = %+v", messengerID, resp)
}
