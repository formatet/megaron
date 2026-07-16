package handlers

// Regression test for Fas 2b: a pending trade offer's escrow expires_at was
// never exposed anywhere in the CLI (inbox/outbox), so a Wanax had no visible
// deadline for when a stuck offer's silver/goods would be refunded. The
// lock+refund machinery (ScheduledOfferExpiry) already existed — this was a
// pure visibility gap: ListSent/Inbox simply didn't SELECT the column.

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
	"github.com/poleia/server/internal/auth"
	"github.com/poleia/server/internal/clock"
	"github.com/poleia/server/internal/events"
)

func messengerExpiryTestPool(t *testing.T) *pgxpool.Pool {
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

func TestMessengerExpiresAt_VisibleInInboxAndOutbox(t *testing.T) {
	pool := messengerExpiryTestPool(t)
	ctx := context.Background()

	// Inbox's per-offer solvency check (EXISTS ... settled(...) >= want_silver)
	// calls current_world_tick(), which needs an active world (one_active_world
	// partial unique index) — see unit_arrival_colonize_test.go for why
	// leftovers must be archived first.
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
	username := "trader-" + uuid.New().String()
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
		`INSERT INTO provinces (world_id, map_q, map_r, terrain_type) VALUES ($1, 5, 0, 'plains') RETURNING id`,
		worldID,
	).Scan(&destProvinceID); err != nil {
		t.Fatalf("create dest province: %v", err)
	}
	var originID, destID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO settlements (world_id, province_id, name, culture_id, owner_id, control_type, is_capital)
		 VALUES ($1, $2, 'Origintown', 'akhaier', $3, 'capital', true) RETURNING id`,
		worldID, originProvinceID, playerID,
	).Scan(&originID); err != nil {
		t.Fatalf("create origin settlement: %v", err)
	}
	// Same player owns both settlements — irrelevant to what's under test
	// (the expires_at column reaching the JSON response), simpler fixture.
	if err := pool.QueryRow(ctx,
		`INSERT INTO settlements (world_id, province_id, name, culture_id, owner_id, control_type, is_capital)
		 VALUES ($1, $2, 'Desttown', 'akhaier', $3, 'colony', false) RETURNING id`,
		worldID, destProvinceID, playerID,
	).Scan(&destID); err != nil {
		t.Fatalf("create dest settlement: %v", err)
	}

	// Inbox's solvency check for a "sell" offer requires the destination
	// (buyer) to hold >= want_silver — otherwise the row is excluded entirely.
	if _, err := pool.Exec(ctx,
		`INSERT INTO settlement_goods (settlement_id, good_key, amount, rate, cap, calc_tick)
		 VALUES ($1, 'silver', 100, 0, 1000, 0)`,
		destID,
	); err != nil {
		t.Fatalf("seed dest silver: %v", err)
	}

	expiresAt := time.Now().Add(7 * 24 * time.Hour).Truncate(time.Second).UTC()
	tradeOffer := `{"kind":"sell","offer_good":"copper","offer_qty":10,"want_silver":50,"status":"pending"}`
	var messengerID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO messengers (world_id, sender_id, origin_id, destination_id, message_text, trade_offer,
		                         status, hex_q, hex_r, sent_at, arrives_at, expires_at)
		 VALUES ($1, $2, $3, $4, 'selling copper', $5::jsonb, 'delivered', 0, 0, now(), now(), $6)
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
	r.Get("/worlds/{worldID}/settlements/{settlementID}/messengers", mh.ListSent)

	// Inbox (destination side).
	inboxReq := httptest.NewRequest(http.MethodGet, "/worlds/"+worldID.String()+"/messengers/inbox", nil)
	inboxReq.Header.Set("Authorization", "Bearer "+accessToken)
	inboxRec := httptest.NewRecorder()
	r.ServeHTTP(inboxRec, inboxReq)
	if inboxRec.Code != http.StatusOK {
		t.Fatalf("Inbox = %d: %s", inboxRec.Code, inboxRec.Body.String())
	}
	var inboxResp []struct {
		ID        uuid.UUID  `json:"id"`
		ExpiresAt *time.Time `json:"expires_at"`
	}
	if err := json.Unmarshal(inboxRec.Body.Bytes(), &inboxResp); err != nil {
		t.Fatalf("parse Inbox response: %v", err)
	}
	found := false
	for _, m := range inboxResp {
		if m.ID == messengerID {
			found = true
			if m.ExpiresAt == nil {
				t.Error("Inbox: expires_at missing for pending trade offer")
			} else if !m.ExpiresAt.Equal(expiresAt) {
				t.Errorf("Inbox: expires_at = %v, want %v", m.ExpiresAt, expiresAt)
			}
		}
	}
	if !found {
		t.Fatalf("Inbox response does not include seeded messenger %s: %+v", messengerID, inboxResp)
	}

	// Outbox (origin side).
	outboxReq := httptest.NewRequest(http.MethodGet,
		"/worlds/"+worldID.String()+"/settlements/"+originID.String()+"/messengers", nil)
	outboxReq.Header.Set("Authorization", "Bearer "+accessToken)
	outboxRec := httptest.NewRecorder()
	r.ServeHTTP(outboxRec, outboxReq)
	if outboxRec.Code != http.StatusOK {
		t.Fatalf("ListSent = %d: %s", outboxRec.Code, outboxRec.Body.String())
	}
	var outboxResp []struct {
		ID        uuid.UUID  `json:"id"`
		ExpiresAt *time.Time `json:"expires_at"`
	}
	if err := json.Unmarshal(outboxRec.Body.Bytes(), &outboxResp); err != nil {
		t.Fatalf("parse ListSent response: %v", err)
	}
	found = false
	for _, m := range outboxResp {
		if m.ID == messengerID {
			found = true
			if m.ExpiresAt == nil {
				t.Error("ListSent (outbox): expires_at missing for pending trade offer")
			} else if !m.ExpiresAt.Equal(expiresAt) {
				t.Errorf("ListSent: expires_at = %v, want %v", m.ExpiresAt, expiresAt)
			}
		}
	}
	if !found {
		t.Fatalf("ListSent response does not include seeded messenger %s: %+v", messengerID, outboxResp)
	}
}
