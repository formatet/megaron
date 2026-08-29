package loyalty

// G2 idempotency regression for WelfareHandler (ScheduledLoyaltyWelfareTick).
//
// welfare.go's own doc comment on welfareEventTypes: "Used both to pick the
// emitted row's type and to guard idempotency (no welfare event of any of
// these types already written this game-day)" — the query's NOT EXISTS
// against loyalty_events within welfareWindowSeconds() (one tick) excludes a
// settlement that was already given a welfare verdict this tick, so an
// immediate replay (worker retry — crash between commit and markDone) finds
// zero due settlements the second time. This is a DB integration test (real
// Postgres, gated by DATABASE_URL), reusing colony_idempotent_test.go's
// fixture shape: it drives the SAME ScheduledLoyaltyWelfareTick event through
// Handle twice and asserts the settlement's loyalty_points only move once.

import (
	"context"
	"testing"
	"time"

	"formatet/megaron/server/internal/clock"
	"formatet/megaron/server/internal/events"
	"github.com/google/uuid"
)

func TestWelfareHandler_ReplayIsIdempotent(t *testing.T) {
	pool := loyaltyTestPool(t)
	ctx := context.Background()

	if _, err := pool.Exec(ctx, `UPDATE worlds SET status = 'archived' WHERE status = 'active'`); err != nil {
		t.Fatalf("archive leftover active test worlds: %v", err)
	}
	var worldID uuid.UUID
	const worldTick = 500
	if err := pool.QueryRow(ctx,
		`INSERT INTO worlds (name, status, current_tick) VALUES ($1, 'active', $2) RETURNING id`,
		"welfare-idem-"+uuid.NewString(), worldTick,
	).Scan(&worldID); err != nil {
		t.Fatalf("create world: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `UPDATE worlds SET status = 'archived' WHERE id = $1`, worldID)
	})

	var owner uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO players (username, password_hash) VALUES ($1, 'x') RETURNING id`,
		"welfare-idem-"+uuid.NewString(),
	).Scan(&owner); err != nil {
		t.Fatalf("create player: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO player_world_records (player_id, world_id, kharis_amount, kharis_rate, kharis_cap, kharis_calc_tick)
		 VALUES ($1, $2, 0, 0, 100, 0)`,
		owner, worldID,
	); err != nil {
		t.Fatalf("create player_world_record: %v", err)
	}

	var prov, settlementID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO provinces (world_id, map_q, map_r, terrain_type) VALUES ($1, 0, 0, 'plains') RETURNING id`,
		worldID,
	).Scan(&prov); err != nil {
		t.Fatalf("create province: %v", err)
	}
	if err := pool.QueryRow(ctx,
		`INSERT INTO settlements (world_id, province_id, name, culture_id, owner_id, control_type, is_capital, state, population, loyalty, loyalty_points)
		 VALUES ($1, $2, 'WelfareTown', 'achaean', $3, 'capital', true, 'active', 1000, 3, 37) RETURNING id`,
		worldID, prov, owner,
	).Scan(&settlementID); err != nil {
		t.Fatalf("create settlement: %v", err)
	}
	// Fed (netto >= 0, stock > 0), no kharis favour, no diet variety —
	// welfareDelta = +1 ("well_fed"), the smallest nonzero case, matching
	// colony_idempotent_test's choice of the smallest nonzero band.
	//
	// rate=510 is GROSS production (Utfodringsordningen D1, 2026-08-26 —
	// settlement_goods.rate is no longer netted against consumption).
	// population=1000 -> demand 500 (economy.GrainConsumptionPerCitizenPerTick),
	// so netto via economy.GrainBalance(510, 1000) = +10 — the same small
	// positive surplus this fixture always intended, now expressed as gross.
	if _, err := pool.Exec(ctx,
		`INSERT INTO settlement_goods (settlement_id, good_key, amount, rate, cap, calc_tick)
		 VALUES ($1, 'grain', 500, 510, 1000000, $2)`,
		settlementID, worldTick,
	); err != nil {
		t.Fatalf("seed grain: %v", err)
	}

	h := NewWelfareHandler(pool, events.NewScheduler(pool, clock.NewTestClock(time.Now())), events.NewStore(pool))
	const fixedEventID int64 = 987002
	evt := events.ScheduledEvent{ID: fixedEventID, WorldID: worldID, DueTick: worldTick}

	if err := h.Handle(ctx, evt); err != nil {
		t.Fatalf("Handle (first run): %v", err)
	}

	var pointsAfterFirst float64
	if err := pool.QueryRow(ctx, `SELECT loyalty_points FROM settlements WHERE id = $1`, settlementID).Scan(&pointsAfterFirst); err != nil {
		t.Fatalf("read loyalty_points after first run: %v", err)
	}
	var countAfterFirst int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM loyalty_events WHERE settlement_id = $1 AND event_type = 'well_fed'`,
		settlementID,
	).Scan(&countAfterFirst); err != nil {
		t.Fatalf("count well_fed events after first run: %v", err)
	}
	if countAfterFirst == 0 {
		t.Fatalf("no well_fed event was recorded after the first run — fixture does not exercise the handler")
	}

	// Replay the SAME event.
	if err := h.Handle(ctx, evt); err != nil {
		t.Fatalf("Handle (replay): %v", err)
	}

	var pointsAfterReplay float64
	if err := pool.QueryRow(ctx, `SELECT loyalty_points FROM settlements WHERE id = $1`, settlementID).Scan(&pointsAfterReplay); err != nil {
		t.Fatalf("read loyalty_points after replay: %v", err)
	}
	if pointsAfterReplay != pointsAfterFirst {
		t.Errorf("loyalty_points after replay = %v, want unchanged %v (event %d replayed — a non-idempotent handler would move it twice)",
			pointsAfterReplay, pointsAfterFirst, fixedEventID)
	}

	var countAfterReplay int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM loyalty_events WHERE settlement_id = $1 AND event_type = 'well_fed'`,
		settlementID,
	).Scan(&countAfterReplay); err != nil {
		t.Fatalf("count well_fed events after replay: %v", err)
	}
	if countAfterReplay != countAfterFirst {
		t.Errorf("well_fed loyalty_events rows after replay = %d, want unchanged %d", countAfterReplay, countAfterFirst)
	}
}
