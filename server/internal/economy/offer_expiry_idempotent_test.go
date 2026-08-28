package economy

// G2 idempotency regression for OfferExpiryHandler (ScheduledOfferExpiry).
//
// trade.go's own doc comment claims: "Idempotent: does nothing if the offer is
// no longer pending (already accepted, declined, or previously expired)." The
// guard is a status-flip UPDATE ... WHERE trade_offer->>'status'='pending' —
// RowsAffected==0 short-circuits the refund on a replay. This is a DB
// integration test (real Postgres, gated by DATABASE_URL) proving that claim:
// it drives the SAME ScheduledOfferExpiry event through Handle twice and
// asserts the buyer's escrowed silver is refunded exactly once. Without the
// status guard, a worker retry (crash between commit and markDone) would
// double-refund the escrow on every replay.

import (
	"context"
	"encoding/json"
	"os"
	"testing"

	"formatet/megaron/server/internal/events"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func offerExpiryTestPool(t *testing.T) *pgxpool.Pool {
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

func TestOfferExpiryHandler_ReplayIsIdempotent(t *testing.T) {
	pool := offerExpiryTestPool(t)
	ctx := context.Background()

	if _, err := pool.Exec(ctx, `UPDATE worlds SET status = 'archived' WHERE status = 'active'`); err != nil {
		t.Fatalf("archive leftover active test worlds: %v", err)
	}

	var worldID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO worlds (name, status, current_tick) VALUES ($1, 'active', 500) RETURNING id`,
		"offer-expiry-idem-"+uuid.NewString(),
	).Scan(&worldID); err != nil {
		t.Fatalf("create world: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `UPDATE worlds SET status = 'archived' WHERE id = $1`, worldID)
	})

	var buyerID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO players (username, password_hash) VALUES ($1, 'x') RETURNING id`,
		"offer-expiry-buyer-"+uuid.NewString(),
	).Scan(&buyerID); err != nil {
		t.Fatalf("create buyer player: %v", err)
	}

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
		`INSERT INTO settlements (world_id, province_id, name, culture_id, owner_id, control_type, is_capital, state)
		 VALUES ($1, $2, 'OfferOrigin', 'akhaier', $3, 'capital', true, 'active') RETURNING id`,
		worldID, originProvinceID, buyerID,
	).Scan(&originID); err != nil {
		t.Fatalf("create origin settlement: %v", err)
	}
	if err := pool.QueryRow(ctx,
		`INSERT INTO settlements (world_id, province_id, name, culture_id, owner_id, control_type, is_capital, state)
		 VALUES ($1, $2, 'OfferDest', 'akhaier', NULL, 'colony', false, 'active') RETURNING id`,
		worldID, destProvinceID,
	).Scan(&destID); err != nil {
		t.Fatalf("create dest settlement: %v", err)
	}

	// Buyer escrowed 80 silver into a "buy" offer; current on-hand is 20 —
	// after expiry-refund it should read 100, and STILL 100 after a replay.
	if _, err := pool.Exec(ctx,
		`INSERT INTO settlement_goods (settlement_id, good_key, amount, rate, cap, calc_tick)
		 VALUES ($1, 'silver', 20, 0, 1000000, 0)`,
		originID,
	); err != nil {
		t.Fatalf("seed origin silver: %v", err)
	}

	tradeOffer := `{"kind":"buy","want_good":"copper","want_qty":50,"offer_silver":80,"status":"pending"}`
	var messengerID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO messengers (world_id, sender_id, origin_id, destination_id, message_text, trade_offer,
		                         status, hex_q, hex_r, sent_at, arrives_at)
		 VALUES ($1, $2, $3, $4, 'buying copper', $5::jsonb, 'delivered', 0, 0, now(), now())
		 RETURNING id`,
		worldID, buyerID, originID, destID, tradeOffer,
	).Scan(&messengerID); err != nil {
		t.Fatalf("seed offer messenger: %v", err)
	}

	payload, err := json.Marshal(struct {
		MessengerID string `json:"messenger_id"`
	}{MessengerID: messengerID.String()})
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	evt := events.ScheduledEvent{ID: 1, WorldID: worldID, Payload: payload}

	h := NewOfferExpiryHandler(pool, nil, nil)

	if err := h.Handle(ctx, evt); err != nil {
		t.Fatalf("Handle (first run): %v", err)
	}

	var silverAfterFirst float64
	if err := pool.QueryRow(ctx,
		`SELECT amount FROM settlement_goods WHERE settlement_id = $1 AND good_key = 'silver'`,
		originID,
	).Scan(&silverAfterFirst); err != nil {
		t.Fatalf("read silver after first run: %v", err)
	}
	if silverAfterFirst != 100 {
		t.Fatalf("silver after first run = %v, want 100 (20 seed + 80 refunded) — fixture does not exercise the handler", silverAfterFirst)
	}

	var statusAfterFirst string
	if err := pool.QueryRow(ctx,
		`SELECT trade_offer->>'status' FROM messengers WHERE id = $1`, messengerID,
	).Scan(&statusAfterFirst); err != nil {
		t.Fatalf("read offer status after first run: %v", err)
	}
	if statusAfterFirst != "expired" {
		t.Fatalf("offer status after first run = %q, want expired", statusAfterFirst)
	}

	// Replay the SAME event.
	if err := h.Handle(ctx, evt); err != nil {
		t.Fatalf("Handle (replay): %v", err)
	}

	var silverAfterReplay float64
	if err := pool.QueryRow(ctx,
		`SELECT amount FROM settlement_goods WHERE settlement_id = $1 AND good_key = 'silver'`,
		originID,
	).Scan(&silverAfterReplay); err != nil {
		t.Fatalf("read silver after replay: %v", err)
	}
	if silverAfterReplay != 100 {
		t.Errorf("silver after replay = %v, want still 100 (a non-idempotent handler would double-refund to 180)", silverAfterReplay)
	}
}
