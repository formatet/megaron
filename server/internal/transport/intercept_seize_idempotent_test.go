package transport

// G2 idempotency regression for InterceptScanHandler's seize step
// (ScheduledInterceptScan, intercept.go). seize's own doc comment: "Guarded so
// a caravan is seized at most once even under concurrent scans" — the flip
// `UPDATE transports SET status='intercepted' ... WHERE status='in_transit'`
// returns RowsAffected==0 on a second call and short-circuits before the loot
// credit. This is a DB integration test (real Postgres, gated by
// DATABASE_URL) proving that guard directly against seize (the mutating half
// of Handle — driving the whole FOW/eyes/sentry-detection sweep through
// Handle itself would require a full tile graph and live-eyes fixture that
// isn't the point of a G2 idempotency check): it calls seize twice for the
// SAME intercepted transport and asserts the interceptor's capital is
// credited the loot exactly once.

import (
	"context"
	"testing"
	"time"

	"formatet/megaron/server/internal/province"
	"github.com/google/uuid"
)

func TestInterceptScanHandler_SeizeReplayIsIdempotent(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()

	if _, err := pool.Exec(ctx, `UPDATE worlds SET status = 'archived' WHERE status = 'active'`); err != nil {
		t.Fatalf("archive leftover active test worlds: %v", err)
	}
	var worldID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO worlds (name, status, current_tick) VALUES ($1, 'active', 500) RETURNING id`,
		"intercept-seize-idem-"+uuid.NewString(),
	).Scan(&worldID); err != nil {
		t.Fatalf("create world: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `UPDATE worlds SET status = 'archived' WHERE id = $1`, worldID)
	})

	var victim, raider uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO players (username, email, password_hash) VALUES ($1, $2, 'x') RETURNING id`,
		"intercept-victim-"+uuid.NewString(), uuid.NewString()+"@test.invalid",
	).Scan(&victim); err != nil {
		t.Fatalf("create victim player: %v", err)
	}
	if err := pool.QueryRow(ctx,
		`INSERT INTO players (username, email, password_hash) VALUES ($1, $2, 'x') RETURNING id`,
		"intercept-raider-"+uuid.NewString(), uuid.NewString()+"@test.invalid",
	).Scan(&raider); err != nil {
		t.Fatalf("create raider player: %v", err)
	}

	var raiderCapProv, raiderCapID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO provinces (world_id, map_q, map_r, terrain_type) VALUES ($1, 0, 0, 'plains') RETURNING id`,
		worldID,
	).Scan(&raiderCapProv); err != nil {
		t.Fatalf("create raider capital province: %v", err)
	}
	if err := pool.QueryRow(ctx,
		`INSERT INTO settlements (world_id, province_id, name, culture_id, owner_id, control_type, is_capital, state)
		 VALUES ($1, $2, 'Raider Capital', 'achaean', $3, 'capital', true, 'active') RETURNING id`,
		worldID, raiderCapProv, raider,
	).Scan(&raiderCapID); err != nil {
		t.Fatalf("create raider capital: %v", err)
	}

	var transportID uuid.UUID
	now := time.Now()
	if err := pool.QueryRow(ctx,
		`INSERT INTO transports (world_id, owner_id, kind, origin_q, origin_r, dest_q, dest_r,
		                          departs_at, arrives_at, due_tick, status, interceptable)
		 VALUES ($1,$2,'trade',5,0,10,0,$3,$4,501,'in_transit',true) RETURNING id`,
		worldID, victim, now.Add(-time.Hour), now.Add(time.Hour),
	).Scan(&transportID); err != nil {
		t.Fatalf("create in-transit transport: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO transport_goods (transport_id, good_key, quantity) VALUES ($1, 'tin', 30)`,
		transportID,
	); err != nil {
		t.Fatalf("seed transport goods: %v", err)
	}

	h := &InterceptScanHandler{pool: pool}
	tr := inFlightTransport{id: transportID, owner: victim, originQ: 5, originR: 0, destQ: 10, destR: 0, category: "land"}
	pos := province.MapPosition{Q: 7, R: 0}
	sentryID := uuid.New()

	if err := h.seize(ctx, worldID, tr, sentryID, raider, pos); err != nil {
		t.Fatalf("seize (first run): %v", err)
	}

	var tinAfterFirst float64
	if err := pool.QueryRow(ctx,
		`SELECT COALESCE(amount, 0) FROM settlement_goods WHERE settlement_id = $1 AND good_key = 'tin'`,
		raiderCapID,
	).Scan(&tinAfterFirst); err != nil {
		t.Fatalf("read raider capital tin after first run: %v", err)
	}
	if tinAfterFirst != 30 {
		t.Fatalf("raider capital tin after first run = %v, want 30 — fixture does not exercise seize", tinAfterFirst)
	}

	var statusAfterFirst string
	if err := pool.QueryRow(ctx, `SELECT status FROM transports WHERE id = $1`, transportID).Scan(&statusAfterFirst); err != nil {
		t.Fatalf("read transport status after first run: %v", err)
	}
	if statusAfterFirst != "intercepted" {
		t.Fatalf("transport status after first run = %q, want intercepted", statusAfterFirst)
	}

	// Replay: a re-run of the same sweep catching the SAME already-seized transport
	// (e.g. a worker retry, or the caravan still sitting in the scan's result set
	// for one more poll before the sweep's own re-enqueue catches up).
	if err := h.seize(ctx, worldID, tr, sentryID, raider, pos); err != nil {
		t.Fatalf("seize (replay): %v", err)
	}

	var tinAfterReplay float64
	if err := pool.QueryRow(ctx,
		`SELECT COALESCE(amount, 0) FROM settlement_goods WHERE settlement_id = $1 AND good_key = 'tin'`,
		raiderCapID,
	).Scan(&tinAfterReplay); err != nil {
		t.Fatalf("read raider capital tin after replay: %v", err)
	}
	if tinAfterReplay != 30 {
		t.Errorf("raider capital tin after replay = %v, want still 30 (a non-idempotent seize would double-credit to 60)", tinAfterReplay)
	}
}
