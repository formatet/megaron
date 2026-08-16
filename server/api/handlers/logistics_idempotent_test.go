package handlers

// G2 idempotency regression for LogisticsArrivalHandler (ScheduledLogisticsArrival).
//
// logistics.go's own doc comment claims: "Idempotent: claims the event in
// processed_deliveries before crediting." This is a DB integration test (real
// Postgres, gated by DATABASE_URL) proving that claim: it drives the SAME
// ScheduledLogisticsArrival event through Handle twice and asserts the
// destination settlement's silver is credited exactly once. Without the
// processed_deliveries claim, a worker retry (crash between commit and
// markDone — see migration 034's comment) would double-credit the caravan's
// silver on every replay.

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"os"
	"testing"

	"formatet/megaron/server/internal/events"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func logisticsTestPool(t *testing.T) *pgxpool.Pool {
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

func TestLogisticsArrivalHandler_ReplayIsIdempotent(t *testing.T) {
	pool := logisticsTestPool(t)
	ctx := context.Background()

	// logistics.go credits settlement_goods.calc_tick via current_world_tick(),
	// which only resolves against the single active world (one_active_world
	// partial unique index) — so unlike the train.go/build.go fixtures this one
	// needs an ACTIVE world, and must archive any leftover active test worlds
	// first (mirrors internal/combat's newSupportFixture).
	if _, err := pool.Exec(ctx, `UPDATE worlds SET status = 'archived' WHERE status = 'active'`); err != nil {
		t.Fatalf("archive leftover active test worlds: %v", err)
	}

	var worldID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO worlds (name, status, current_tick) VALUES ($1, 'active', 500) RETURNING id`,
		"logistics-idem-"+uuid.NewString(),
	).Scan(&worldID); err != nil {
		t.Fatalf("create world: %v", err)
	}
	t.Cleanup(func() { _, _ = pool.Exec(context.Background(), `UPDATE worlds SET status = 'archived' WHERE id = $1`, worldID) })

	var ownerID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO players (username, email, password_hash) VALUES ($1, $2, 'x') RETURNING id`,
		"logistics-idem-owner-"+uuid.NewString(), uuid.NewString()+"@test.invalid",
	).Scan(&ownerID); err != nil {
		t.Fatalf("create player: %v", err)
	}

	var provinceID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO provinces (world_id, map_q, map_r, terrain_type) VALUES ($1, 0, 0, 'plains') RETURNING id`,
		worldID,
	).Scan(&provinceID); err != nil {
		t.Fatalf("create province: %v", err)
	}

	var settlementID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO settlements (world_id, province_id, name, culture_id, owner_id, is_capital, state, population)
		 VALUES ($1, $2, 'Caravantown', 'akhaier', $3, true, 'active', 500) RETURNING id`,
		worldID, provinceID, ownerID,
	).Scan(&settlementID); err != nil {
		t.Fatalf("create settlement: %v", err)
	}

	if _, err := pool.Exec(ctx,
		`INSERT INTO settlement_goods (settlement_id, good_key, amount, rate, cap, calc_tick)
		 VALUES ($1, 'silver', 50, 0, 1000000, 0)`,
		settlementID,
	); err != nil {
		t.Fatalf("seed silver: %v", err)
	}

	payload, err := json.Marshal(struct {
		Kind        string    `json:"kind"`
		Destination uuid.UUID `json:"destination"`
		GoodKey     string    `json:"good_key"`
		Quantity    float64   `json:"quantity"`
	}{
		Kind:        "settlement_good",
		Destination: settlementID,
		GoodKey:     "silver",
		Quantity:    100,
	})
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}

	// processed_deliveries keys ONLY on event_id (no settlement/world scoping,
	// unlike processed_tick_claims), so a literal constant here would collide
	// with a previous run's claim row on a DB that persists across `go test`
	// invocations (this table has no per-test cleanup). Derive the ID from the
	// test's own random world, which is unique per run, so the second Handle
	// call is still a genuine replay of the SAME scheduled event within this
	// run — the exact scenario processed_deliveries exists to guard — without
	// colliding across runs.
	fixedEventID := int64(binary.BigEndian.Uint64(worldID[:8]) &^ (1 << 63)) // clear sign bit: BIGINT positive
	evt := events.ScheduledEvent{ID: fixedEventID, WorldID: worldID, Payload: payload}

	h := NewLogisticsArrivalHandler(pool)

	if err := h.Handle(ctx, evt); err != nil {
		t.Fatalf("Handle (first run): %v", err)
	}

	var silverAfterFirst float64
	if err := pool.QueryRow(ctx,
		`SELECT amount FROM settlement_goods WHERE settlement_id = $1 AND good_key = 'silver'`,
		settlementID,
	).Scan(&silverAfterFirst); err != nil {
		t.Fatalf("read silver after first run: %v", err)
	}
	if silverAfterFirst != 150 {
		t.Fatalf("silver after first run = %v, want 150 (50 seed + 100 delivered)", silverAfterFirst)
	}

	// Replay the SAME event.
	if err := h.Handle(ctx, evt); err != nil {
		t.Fatalf("Handle (replay): %v", err)
	}

	var silverAfterReplay float64
	if err := pool.QueryRow(ctx,
		`SELECT amount FROM settlement_goods WHERE settlement_id = $1 AND good_key = 'silver'`,
		settlementID,
	).Scan(&silverAfterReplay); err != nil {
		t.Fatalf("read silver after replay: %v", err)
	}
	if silverAfterReplay != 150 {
		t.Errorf("silver after replay = %v, want still 150 (a non-idempotent handler would double-credit to 250)", silverAfterReplay)
	}

	var claimCount int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM processed_deliveries WHERE event_id = $1`, fixedEventID).Scan(&claimCount); err != nil {
		t.Fatalf("count processed_deliveries claim rows: %v", err)
	}
	if claimCount != 1 {
		t.Errorf("processed_deliveries rows for event %d = %d, want exactly 1", fixedEventID, claimCount)
	}
}
