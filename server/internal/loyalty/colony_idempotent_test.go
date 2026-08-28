package loyalty

// G2 idempotency regression for ColonyPenaltyHandler (ScheduledColonyPenaltyTick).
//
// Migration 098's comment documents the original bug directly: this handler fans
// ONE scheduled event out across every colony a Wanax owns and mutates loyalty
// directly (no FOR UPDATE, no replay-from-log). A crash or G2 handler timeout
// between the first colony's write and Handle finishing would leave the event
// unprocessed, so events.Worker re-claims and re-runs it from the top — and since
// loyalty is an accumulator (loyalty_points), a duplicate colony_penalty write
// does not just log twice, it MOVES THE VALUE TWICE. Migration 098 added
// processed_tick_claims (event_id, settlement_id) specifically to close this. This
// test proves the guard still holds by running the SAME scheduled event through
// Handle twice and asserting the second run is a no-op.

import (
	"context"
	"os"
	"testing"
	"time"

	"formatet/megaron/server/internal/clock"
	"formatet/megaron/server/internal/events"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func loyaltyTestPool(t *testing.T) *pgxpool.Pool {
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

// colonyFixture creates one active world, one Wanax, a capital and three
// colonies (colonyCount == 3 → settlement.ColonyPenalty returns -1, the
// smallest nonzero penalty band, per internal/settlement/loyalty.go).
type colonyFixture struct {
	worldID   uuid.UUID
	owner     uuid.UUID
	capital   uuid.UUID
	colonies  []uuid.UUID
	tick      int
}

func newColonyFixture(t *testing.T, pool *pgxpool.Pool, tag string) colonyFixture {
	t.Helper()
	ctx := context.Background()
	const worldTick = 500

	if _, err := pool.Exec(ctx,
		`UPDATE worlds SET status = 'archived' WHERE status = 'active'`,
	); err != nil {
		t.Fatalf("archive leftover active test worlds: %v", err)
	}

	f := colonyFixture{tick: worldTick}
	if err := pool.QueryRow(ctx,
		`INSERT INTO worlds (name, status, current_tick) VALUES ($1, 'active', $2) RETURNING id`,
		"test-"+tag+"-"+uuid.New().String(), worldTick,
	).Scan(&f.worldID); err != nil {
		t.Fatalf("create world: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `UPDATE worlds SET status = 'archived' WHERE id = $1`, f.worldID)
	})

	if err := pool.QueryRow(ctx,
		`INSERT INTO players (username, password_hash) VALUES ($1, 'x') RETURNING id`,
		tag+"-"+uuid.New().String(),
	).Scan(&f.owner); err != nil {
		t.Fatalf("create player: %v", err)
	}

	mk := func(q int, name string, capital bool) uuid.UUID {
		var prov, sid uuid.UUID
		if err := pool.QueryRow(ctx,
			`INSERT INTO provinces (world_id, map_q, map_r, terrain_type) VALUES ($1, $2, 0, 'plains') RETURNING id`,
			f.worldID, q,
		).Scan(&prov); err != nil {
			t.Fatalf("create province: %v", err)
		}
		ctype := "colony"
		if capital {
			ctype = "capital"
		}
		if err := pool.QueryRow(ctx,
			`INSERT INTO settlements (world_id, province_id, name, culture_id, owner_id, control_type, is_capital, state, population)
			 VALUES ($1, $2, $3, 'achaean', $4, $5, $6, 'active', 1000) RETURNING id`,
			f.worldID, prov, name, f.owner, ctype, capital,
		).Scan(&sid); err != nil {
			t.Fatalf("create settlement %s: %v", name, err)
		}
		return sid
	}
	f.capital = mk(0, "Mycenae", true)
	f.colonies = []uuid.UUID{
		mk(1, "Colony-1", false),
		mk(2, "Colony-2", false),
		mk(3, "Colony-3", false),
	}
	return f
}

func TestColonyPenaltyHandler_ReplayIsIdempotent(t *testing.T) {
	pool := loyaltyTestPool(t)
	ctx := context.Background()
	f := newColonyFixture(t, pool, "colony-idem")

	h := NewColonyPenaltyHandler(pool, events.NewScheduler(pool, clock.NewTestClock(time.Now())), events.NewStore(pool))
	// Fixed event ID so the second Handle call is a genuine replay of the SAME
	// scheduled event, not a fresh one — that is the exact scenario
	// processed_tick_claims (event_id, settlement_id) exists to guard.
	const fixedEventID int64 = 987001
	evt := events.ScheduledEvent{ID: fixedEventID, WorldID: f.worldID, DueTick: f.tick}

	if err := h.Handle(ctx, evt); err != nil {
		t.Fatalf("Handle (first run): %v", err)
	}

	pointsAfterFirst := make(map[uuid.UUID]float64)
	countAfterFirst := make(map[uuid.UUID]int)
	for _, cid := range f.colonies {
		var pts float64
		if err := pool.QueryRow(ctx, `SELECT loyalty_points FROM settlements WHERE id = $1`, cid).Scan(&pts); err != nil {
			t.Fatalf("read loyalty_points after first run: %v", err)
		}
		pointsAfterFirst[cid] = pts

		var n int
		if err := pool.QueryRow(ctx,
			`SELECT count(*) FROM loyalty_events WHERE settlement_id = $1 AND event_type = 'colony_penalty'`,
			cid,
		).Scan(&n); err != nil {
			t.Fatalf("count colony_penalty events after first run: %v", err)
		}
		countAfterFirst[cid] = n
	}

	// Sanity: the penalty must actually have fired at least once (colonyCount==3
	// → delta -1, from a starting loyalty_points of 37 the band-clamped result
	// still moves), otherwise this test would trivially pass even without the
	// idempotency guard.
	anyPenalized := false
	for _, cid := range f.colonies {
		if countAfterFirst[cid] > 0 {
			anyPenalized = true
		}
	}
	if !anyPenalized {
		t.Fatalf("no colony_penalty event was recorded after the first run — fixture does not exercise the handler")
	}

	// Replay the SAME event.
	if err := h.Handle(ctx, evt); err != nil {
		t.Fatalf("Handle (replay): %v", err)
	}

	for _, cid := range f.colonies {
		var pts float64
		if err := pool.QueryRow(ctx, `SELECT loyalty_points FROM settlements WHERE id = $1`, cid).Scan(&pts); err != nil {
			t.Fatalf("read loyalty_points after replay: %v", err)
		}
		if pts != pointsAfterFirst[cid] {
			t.Errorf("colony %s loyalty_points after replay = %v, want unchanged %v (event_id %d replayed — a non-idempotent handler would move it twice)",
				cid, pts, pointsAfterFirst[cid], fixedEventID)
		}

		var n int
		if err := pool.QueryRow(ctx,
			`SELECT count(*) FROM loyalty_events WHERE settlement_id = $1 AND event_type = 'colony_penalty'`,
			cid,
		).Scan(&n); err != nil {
			t.Fatalf("count colony_penalty events after replay: %v", err)
		}
		if n != countAfterFirst[cid] {
			t.Errorf("colony %s colony_penalty loyalty_events rows after replay = %d, want unchanged %d", cid, n, countAfterFirst[cid])
		}
	}
}
