package transport

// Physical transport substrate (Del 3-fas-1). A dispatched caravan is a real mover
// with a route and a manifest; on arrival its goods are credited to the destination,
// exactly once, and only if it was not intercepted en route. Its live position is
// interpolated along its FindPath route. DB-gated on DATABASE_URL.

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	"formatet/megaron/server/internal/clock"
	"formatet/megaron/server/internal/events"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func testPool(t *testing.T) *pgxpool.Pool {
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

// fixture builds an active world with a source and destination settlement joined by
// a 4-hex land strip, and returns everything the transport tests need.
type fixture struct {
	worldID  uuid.UUID
	owner    uuid.UUID
	sourceID uuid.UUID
	destID   uuid.UUID
}

func newFixture(t *testing.T, pool *pgxpool.Pool) fixture {
	t.Helper()
	ctx := context.Background()

	if _, err := pool.Exec(ctx,
		`UPDATE worlds SET status = 'archived' WHERE status = 'active' AND name LIKE 'test-world-%'`,
	); err != nil {
		t.Fatalf("archive leftover active test worlds: %v", err)
	}
	var f fixture
	if err := pool.QueryRow(ctx,
		`INSERT INTO worlds (name, status) VALUES ($1, 'active') RETURNING id`,
		"test-world-"+uuid.New().String(),
	).Scan(&f.worldID); err != nil {
		t.Fatalf("create test world: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `UPDATE worlds SET status = 'archived' WHERE id = $1`, f.worldID)
	})

	if err := pool.QueryRow(ctx,
		`INSERT INTO players (username, email, password_hash) VALUES ($1, $2, 'x') RETURNING id`,
		"carter-"+uuid.New().String(), "carter-"+uuid.New().String()+"@test.invalid",
	).Scan(&f.owner); err != nil {
		t.Fatalf("create player: %v", err)
	}

	// A 4-hex land strip (0,0)…(3,0).
	for q := 0; q <= 3; q++ {
		if _, err := pool.Exec(ctx,
			`INSERT INTO map_tiles (world_id, q, r, terrain) VALUES ($1, $2, 0, 'plains')`,
			f.worldID, q,
		); err != nil {
			t.Fatalf("insert map tile (%d,0): %v", q, err)
		}
	}

	mkSettlement := func(name string, q int) uuid.UUID {
		var prov uuid.UUID
		if err := pool.QueryRow(ctx,
			`INSERT INTO provinces (world_id, map_q, map_r, terrain_type) VALUES ($1, $2, 0, 'plains') RETURNING id`,
			f.worldID, q,
		).Scan(&prov); err != nil {
			t.Fatalf("create province %s: %v", name, err)
		}
		var id uuid.UUID
		if err := pool.QueryRow(ctx,
			`INSERT INTO settlements (world_id, province_id, name, culture_id, owner_id, control_type, is_capital, state, population)
			 VALUES ($1, $2, $3, 'achaean', $4, 'capital', $5, 'active', 5000) RETURNING id`,
			f.worldID, prov, name, f.owner, q == 0,
		).Scan(&id); err != nil {
			t.Fatalf("create settlement %s: %v", name, err)
		}
		return id
	}
	f.sourceID = mkSettlement("Source", 0)
	f.destID = mkSettlement("Dest", 3)
	return f
}

// fireArrival loads the TransportArrival event the dispatch enqueued and runs the
// handler, mirroring what the worker does.
func fireArrival(t *testing.T, pool *pgxpool.Pool, worldID uuid.UUID) {
	t.Helper()
	ctx := context.Background()
	var id int64
	var payload json.RawMessage
	if err := pool.QueryRow(ctx,
		`SELECT id, payload FROM scheduled_events
		 WHERE world_id = $1 AND event_type = 'TransportArrival' AND processed_at IS NULL
		 ORDER BY id DESC LIMIT 1`,
		worldID,
	).Scan(&id, &payload); err != nil {
		t.Fatalf("load enqueued TransportArrival: %v", err)
	}
	h := NewArrivalHandler(pool)
	if err := h.Handle(ctx, events.ScheduledEvent{ID: id, WorldID: worldID, Payload: payload}); err != nil {
		t.Fatalf("arrival handle: %v", err)
	}
}

func dispatchGift(t *testing.T, pool *pgxpool.Pool, f fixture, m Manifest) uuid.UUID {
	t.Helper()
	ctx := context.Background()
	sched := events.NewScheduler(pool, clock.NewTestClock(time.Now()))
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	now := time.Now()
	id, err := Dispatch(ctx, tx, sched, DispatchParams{
		WorldID:  f.worldID,
		OwnerID:  f.owner,
		Kind:     "transfer",
		OriginID: f.sourceID,
		DestID:   f.destID,
		Category: "land",
		OriginQ:  0, OriginR: 0,
		DestQ: 3, DestR: 0,
		DepartsAt:     now,
		ArrivesAt:     now.Add(2 * time.Hour),
		DueTick:       1,
		Manifest:      m,
		Interceptable: true,
	})
	if err != nil {
		tx.Rollback(ctx)
		t.Fatalf("dispatch: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit: %v", err)
	}
	return id
}

func goodAmount(t *testing.T, pool *pgxpool.Pool, settlementID uuid.UUID, good string) float64 {
	t.Helper()
	var amt float64
	err := pool.QueryRow(context.Background(),
		`SELECT COALESCE(settled(amount, rate, calc_tick), 0) FROM settlement_goods
		 WHERE settlement_id = $1 AND good_key = $2`, settlementID, good).Scan(&amt)
	if err != nil {
		return 0
	}
	return amt
}

func TestTransport_DeliversManifestOnArrival(t *testing.T) {
	pool := testPool(t)
	f := newFixture(t, pool)

	dispatchGift(t, pool, f, Manifest{"silver": 100, "grain": 50})
	fireArrival(t, pool, f.worldID)

	if got := goodAmount(t, pool, f.destID, "silver"); got != 100 {
		t.Errorf("dest silver = %v, want 100", got)
	}
	if got := goodAmount(t, pool, f.destID, "grain"); got != 50 {
		t.Errorf("dest grain = %v, want 50", got)
	}

	var status string
	if err := pool.QueryRow(context.Background(),
		`SELECT status FROM transports WHERE world_id = $1 ORDER BY created_at DESC LIMIT 1`, f.worldID,
	).Scan(&status); err != nil {
		t.Fatalf("read transport status: %v", err)
	}
	if status != "delivered" {
		t.Errorf("transport status = %q, want delivered", status)
	}
}

func TestTransport_ArrivalIsIdempotent(t *testing.T) {
	pool := testPool(t)
	f := newFixture(t, pool)

	dispatchGift(t, pool, f, Manifest{"silver": 100})
	fireArrival(t, pool, f.worldID)
	fireArrival(t, pool, f.worldID) // second run must not double-credit

	if got := goodAmount(t, pool, f.destID, "silver"); got != 100 {
		t.Errorf("dest silver after double arrival = %v, want 100 (no double-credit)", got)
	}
}

func TestTransport_InterceptedIsNotDelivered(t *testing.T) {
	pool := testPool(t)
	f := newFixture(t, pool)

	id := dispatchGift(t, pool, f, Manifest{"silver": 100})
	// Interception (Del 3-fas-4) flips status before the arrival fires.
	if _, err := pool.Exec(context.Background(),
		`UPDATE transports SET status = 'intercepted' WHERE id = $1`, id); err != nil {
		t.Fatalf("mark intercepted: %v", err)
	}
	fireArrival(t, pool, f.worldID)

	if got := goodAmount(t, pool, f.destID, "silver"); got != 0 {
		t.Errorf("dest silver after intercepted arrival = %v, want 0 (delivery cancelled)", got)
	}
}

func TestTransport_CurrentPositionInterpolatesAlongRoute(t *testing.T) {
	pool := testPool(t)
	f := newFixture(t, pool)
	ctx := context.Background()

	now := time.Unix(1_000_000, 0)
	departs := now.Add(-1 * time.Hour)
	arrives := now.Add(1 * time.Hour) // exactly halfway

	pos, ok, err := CurrentPosition(ctx, pool, f.worldID, 0, 0, 3, 0, "land", departs, arrives, now)
	if err != nil {
		t.Fatalf("current position: %v", err)
	}
	if !ok {
		t.Fatalf("current position ok=false, want a hex on the route")
	}
	// Path (0,0)(1,0)(2,0)(3,0); halfway → index floor(0.5*3)=1 → (1,0).
	if pos.Q != 1 || pos.R != 0 {
		t.Errorf("halfway position = (%d,%d), want (1,0)", pos.Q, pos.R)
	}
}
