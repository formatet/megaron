package loyalty

// G2 idempotency regression for DecayHandler (ScheduledLoyaltyDecayTick).
//
// Unlike its sibling ColonyPenaltyHandler (colony.go, guarded by migration
// 098's processed_tick_claims), applyDecay had NO per-event claim: its
// "neglected colony" query only excludes settlements with a recent
// gift-type loyalty_events row — it never checked whether THIS scheduled
// event already applied a long_silence decay. A worker retry (G2 handler
// timeout, or a crash between commit and events.Worker's markDone) would
// re-select the same neglected settlements and move loyalty_points a
// second time for the same event. This test proves the fix (a claim in the
// SAME shared processed_tick_claims table, keyed (event_id, settlement_id))
// by running one scheduled event through Handle twice and asserting the
// second run is a no-op, while also asserting the first (only) run still
// applies decay — the guard must not swallow legitimate first-run work.

import (
	"context"
	"testing"
	"time"

	"formatet/megaron/server/internal/clock"
	"formatet/megaron/server/internal/events"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// decayFixture creates one active world and a single non-capital, owned
// settlement with default loyalty (2, loyalty_points 37 — migration 083) and
// no loyalty_events rows at all, so it is neglected by construction: the "no
// gift/governor_visit/victory_nearby in the grace window" query holds
// vacuously (uses loyaltyTestPool from colony_idempotent_test.go, same
// package).
type decayFixture struct {
	worldID    uuid.UUID
	settlement uuid.UUID
	tick       int
}

func newDecayFixture(t *testing.T, pool *pgxpool.Pool, tag string) decayFixture {
	t.Helper()
	ctx := context.Background()
	const worldTick = 500

	if _, err := pool.Exec(ctx,
		`UPDATE worlds SET status = 'archived' WHERE status = 'active'`,
	); err != nil {
		t.Fatalf("archive leftover active test worlds: %v", err)
	}

	f := decayFixture{tick: worldTick}
	if err := pool.QueryRow(ctx,
		`INSERT INTO worlds (name, status, current_tick) VALUES ($1, 'active', $2) RETURNING id`,
		"test-"+tag+"-"+uuid.New().String(), worldTick,
	).Scan(&f.worldID); err != nil {
		t.Fatalf("create world: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `UPDATE worlds SET status = 'archived' WHERE id = $1`, f.worldID)
	})

	var owner uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO players (username, password_hash) VALUES ($1, 'x') RETURNING id`,
		tag+"-"+uuid.New().String(),
	).Scan(&owner); err != nil {
		t.Fatalf("create player: %v", err)
	}

	var prov uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO provinces (world_id, map_q, map_r, terrain_type) VALUES ($1, 0, 0, 'plains') RETURNING id`,
		f.worldID,
	).Scan(&prov); err != nil {
		t.Fatalf("create province: %v", err)
	}

	if err := pool.QueryRow(ctx,
		`INSERT INTO settlements (world_id, province_id, name, culture_id, owner_id, control_type, is_capital, state, population)
		 VALUES ($1, $2, 'Neglected-Colony', 'achaean', $3, 'colony', false, 'active', 1000) RETURNING id`,
		f.worldID, prov, owner,
	).Scan(&f.settlement); err != nil {
		t.Fatalf("create settlement: %v", err)
	}

	return f
}

func TestDecayHandler_ReplayIsIdempotent(t *testing.T) {
	pool := loyaltyTestPool(t)
	ctx := context.Background()
	f := newDecayFixture(t, pool, "decay-idem")

	h := NewDecayHandler(pool, events.NewScheduler(pool, clock.NewTestClock(time.Now())), events.NewStore(pool))
	// Fixed event ID so the second Handle call is a genuine replay of the SAME
	// scheduled event, not a fresh one — exactly the scenario the claim guards.
	const fixedEventID int64 = 987101
	evt := events.ScheduledEvent{ID: fixedEventID, WorldID: f.worldID, DueTick: f.tick}

	var pointsBefore float64
	if err := pool.QueryRow(ctx, `SELECT loyalty_points FROM settlements WHERE id = $1`, f.settlement).Scan(&pointsBefore); err != nil {
		t.Fatalf("read loyalty_points before run: %v", err)
	}

	if err := h.Handle(ctx, evt); err != nil {
		t.Fatalf("Handle (first run): %v", err)
	}

	var pointsAfterFirst float64
	if err := pool.QueryRow(ctx, `SELECT loyalty_points FROM settlements WHERE id = $1`, f.settlement).Scan(&pointsAfterFirst); err != nil {
		t.Fatalf("read loyalty_points after first run: %v", err)
	}
	var countAfterFirst int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM events WHERE stream_id = $1 AND event_type = 'LoyaltyDecay'`,
		f.settlement,
	).Scan(&countAfterFirst); err != nil {
		t.Fatalf("count LoyaltyDecay events after first run: %v", err)
	}

	// Sanity: the guard must not have swallowed legitimate first-run work —
	// decay must actually have moved loyalty_points down and logged one event,
	// otherwise this test would trivially pass even without any idempotency
	// guard at all.
	if pointsAfterFirst >= pointsBefore {
		t.Fatalf("loyalty_points after first run = %v, want < starting %v — fixture does not exercise the handler (first-run decay did not fire)",
			pointsAfterFirst, pointsBefore)
	}
	if countAfterFirst != 1 {
		t.Fatalf("LoyaltyDecay audit events after first run = %d, want exactly 1", countAfterFirst)
	}

	// Replay the SAME event.
	if err := h.Handle(ctx, evt); err != nil {
		t.Fatalf("Handle (replay): %v", err)
	}

	var pointsAfterReplay float64
	if err := pool.QueryRow(ctx, `SELECT loyalty_points FROM settlements WHERE id = $1`, f.settlement).Scan(&pointsAfterReplay); err != nil {
		t.Fatalf("read loyalty_points after replay: %v", err)
	}
	if pointsAfterReplay != pointsAfterFirst {
		t.Errorf("loyalty_points after replay = %v, want unchanged %v (event_id %d replayed — a non-idempotent handler would move it twice)",
			pointsAfterReplay, pointsAfterFirst, fixedEventID)
	}

	var countAfterReplay int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM events WHERE stream_id = $1 AND event_type = 'LoyaltyDecay'`,
		f.settlement,
	).Scan(&countAfterReplay); err != nil {
		t.Fatalf("count LoyaltyDecay events after replay: %v", err)
	}
	if countAfterReplay != countAfterFirst {
		t.Errorf("LoyaltyDecay audit events after replay = %d, want unchanged %d", countAfterReplay, countAfterFirst)
	}
}
