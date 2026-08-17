package combat

// G2 idempotency regression for UpkeepHandler.Handle (ScheduledUpkeepTick).
//
// UpkeepHandler runs EVERY tick in MVP and fans one scheduled event across
// every active unit in the world, mutating settlement_goods and units with
// plain UPDATEs — no FOR UPDATE, no per-event transaction, no claim. A G2
// handler timeout (events.Worker wraps every handler in a 5s context) or a
// crash between commit and markDone leaves the scheduled event unprocessed,
// so events.Worker re-claims and re-runs Handle from the top. Before the
// migration-098-pattern guard added to upkeep.go, that meant: grain and
// silver deducted twice, unpaid_periods incremented twice (which can push a
// unit past the desertion threshold early), and UpkeepSettled/SilverAudit
// emitted twice. This test drives the SAME scheduled event through Handle
// twice and asserts the second run is a no-op — while also proving the
// FIRST run still does its full, legitimate work (the guard must not skip
// real work, only a replay).

import (
	"context"
	"testing"
	"time"

	"formatet/megaron/server/internal/clock"
	"formatet/megaron/server/internal/events"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// countEvents returns how many rows of event_type exist for worldID in the
// audit log — used to assert UpkeepSettled/SilverAudit fire exactly once
// per scheduled event, not once per Handle() invocation.
func countEvents(t *testing.T, pool *pgxpool.Pool, worldID uuid.UUID, eventType string) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(context.Background(),
		`SELECT count(*) FROM events WHERE world_id = $1 AND event_type = $2`,
		worldID, eventType,
	).Scan(&n); err != nil {
		t.Fatalf("count %s events: %v", eventType, err)
	}
	return n
}

func TestUpkeepHandler_ReplayIsIdempotent(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	f := newSupportFixture(t, pool, "upkeep-idem")

	// Capital: plenty of grain, and EXACTLY 10x the silver a single garrisoned
	// full-size spearman needs (UnitUpkeep: grain 50, silver 1 at size 100,
	// status garrison — no field multiplier). 10, not 1, so a double deduction
	// is observable as a real balance change rather than being masked by the
	// affordability gate rejecting an overdraft.
	seedGoods(t, pool, f.capitalID, f.tick, 100000, 10)
	// Town: plenty of grain but ZERO silver — this unit's silver upkeep is
	// guaranteed to go unpaid, exercising the recordUnpaid/unpaid_periods path.
	seedGoods(t, pool, f.townID, f.tick, 100000, 0)

	var paidUnitID, unpaidUnitID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO units (world_id, owner_id, type, category, size, crew, status,
		                    q, r, settlement_id, support_settlement_id, unpaid_periods)
		 VALUES ($1, $2, 'spearman', 'land', 100, 0, 'garrison', 0, 0, $3, $3, 0)
		 RETURNING id`,
		f.worldID, f.owner, f.capitalID,
	).Scan(&paidUnitID); err != nil {
		t.Fatalf("create paid unit: %v", err)
	}
	if err := pool.QueryRow(ctx,
		`INSERT INTO units (world_id, owner_id, type, category, size, crew, status,
		                    q, r, settlement_id, support_settlement_id, unpaid_periods)
		 VALUES ($1, $2, 'spearman', 'land', 100, 0, 'garrison', 4, 0, $3, $3, 0)
		 RETURNING id`,
		f.worldID, f.owner, f.townID,
	).Scan(&unpaidUnitID); err != nil {
		t.Fatalf("create unpaid unit: %v", err)
	}

	h := NewUpkeepHandler(pool, events.NewScheduler(pool, clock.NewTestClock(time.Now())),
		events.NewStore(pool), &fakeBroadcaster{})
	// soldShare=0 (Del C circulation off) keeps the silver arithmetic in this
	// test a plain debit, decoupled from a feature this test isn't about.
	h.soldShare = 0
	// Fixed event ID so the second Handle call is a genuine replay of the SAME
	// scheduled event, not a fresh one — the exact scenario the per-unit
	// (event_id, unit_id) claim in processed_tick_claims exists to guard.
	const fixedEventID int64 = 555001
	evt := events.ScheduledEvent{ID: fixedEventID, WorldID: f.worldID, DueTick: f.tick}

	if err := h.Handle(ctx, evt); err != nil {
		t.Fatalf("Handle (first run): %v", err)
	}

	grainAfterFirst := settledAmount(t, pool, f.capitalID, "grain")
	silverAfterFirst := settledAmount(t, pool, f.capitalID, "silver")
	var unpaidAfterFirst int
	if err := pool.QueryRow(ctx, `SELECT unpaid_periods FROM units WHERE id = $1`, unpaidUnitID).Scan(&unpaidAfterFirst); err != nil {
		t.Fatalf("read unpaid_periods after first run: %v", err)
	}
	settledEventsAfterFirst := countEvents(t, pool, f.worldID, "UpkeepSettled")
	auditEventsAfterFirst := countEvents(t, pool, f.worldID, "SilverAudit")

	// Sanity: the first run must have done its full, legitimate work, or this
	// test would trivially pass even with a broken (or absent) guard.
	if grainAfterFirst != 100000-50 {
		t.Fatalf("capital grain after first run = %v, want %v — fixture does not exercise the handler", grainAfterFirst, 100000-50)
	}
	if silverAfterFirst != 9 {
		t.Fatalf("capital silver after first run = %v, want 9 (10 seeded - 1 upkeep) — fixture does not exercise the handler", silverAfterFirst)
	}
	if unpaidAfterFirst != 1 {
		t.Fatalf("unpaid_periods after first run = %d, want 1 — fixture does not exercise the handler", unpaidAfterFirst)
	}
	if settledEventsAfterFirst == 0 {
		t.Fatalf("no UpkeepSettled event recorded after first run — fixture does not exercise the handler")
	}
	if auditEventsAfterFirst != 1 {
		t.Fatalf("SilverAudit events after first run = %d, want exactly 1", auditEventsAfterFirst)
	}

	// Replay the SAME event.
	if err := h.Handle(ctx, evt); err != nil {
		t.Fatalf("Handle (replay): %v", err)
	}

	grainAfterReplay := settledAmount(t, pool, f.capitalID, "grain")
	silverAfterReplay := settledAmount(t, pool, f.capitalID, "silver")
	var unpaidAfterReplay int
	if err := pool.QueryRow(ctx, `SELECT unpaid_periods FROM units WHERE id = $1`, unpaidUnitID).Scan(&unpaidAfterReplay); err != nil {
		t.Fatalf("read unpaid_periods after replay: %v", err)
	}
	settledEventsAfterReplay := countEvents(t, pool, f.worldID, "UpkeepSettled")
	auditEventsAfterReplay := countEvents(t, pool, f.worldID, "SilverAudit")

	if grainAfterReplay != grainAfterFirst {
		t.Errorf("capital grain after replay = %v, want unchanged %v (a non-idempotent handler would double-deduct)", grainAfterReplay, grainAfterFirst)
	}
	if silverAfterReplay != silverAfterFirst {
		t.Errorf("capital silver after replay = %v, want unchanged %v (a non-idempotent handler would double-deduct to %v)",
			silverAfterReplay, silverAfterFirst, silverAfterFirst-1)
	}
	if unpaidAfterReplay != unpaidAfterFirst {
		t.Errorf("unpaid_periods after replay = %d, want unchanged %d (a non-idempotent handler would double-increment, pushing the unit toward desertion early)",
			unpaidAfterReplay, unpaidAfterFirst)
	}
	if settledEventsAfterReplay != settledEventsAfterFirst {
		t.Errorf("UpkeepSettled event count after replay = %d, want unchanged %d", settledEventsAfterReplay, settledEventsAfterFirst)
	}
	if auditEventsAfterReplay != auditEventsAfterFirst {
		t.Errorf("SilverAudit event count after replay = %d, want unchanged %d (exactly one audit event for this scheduled event)",
			auditEventsAfterReplay, auditEventsAfterFirst)
	}
}
