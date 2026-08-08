package combat

// Röd-först-test för belägring S3 (megaron_plan_belagring.md §S3): a
// besieged, starved-out settlement falls to its STRONGEST besieger via the
// same occupied-state transition a battle win uses — no battle fought, no
// ownership transfer (occupy, not annex), stale garrison evicted, the
// starvation clock reset, and the whole thing idempotent against a
// blockade that lifted between scheduling and delivery.
//
// Reuses newSiegeFixture/mkSiegeGarrison/fakeBroadcaster (battle_wall_test.go,
// unit_arrival_notify_test.go — same package, same DATABASE_URL convention).

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"formatet/megaron/server/internal/clock"
	"formatet/megaron/server/internal/events"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/google/uuid"
)

// placeSentryUnit plants an enemy unit in SENTRY stance at (q,r) — mirrors
// economy's placeEnemyUnit (siege_test.go), the only posture that holds a
// chokepoint since the 2026-08-08 revision.
func placeSentryUnit(t *testing.T, pool *pgxpool.Pool, worldID, ownerID uuid.UUID, q, r, size int) uuid.UUID {
	t.Helper()
	var id uuid.UUID
	if err := pool.QueryRow(context.Background(),
		`INSERT INTO units (world_id, owner_id, type, category, size, status, q, r, stance, sentry_q, sentry_r)
		 VALUES ($1, $2, 'spearman', 'land', $3, 'positioned', $4, $5, 'sentry', $4, $5) RETURNING id`,
		worldID, ownerID, size, q, r,
	).Scan(&id); err != nil {
		t.Fatalf("place sentry unit at (%d,%d): %v", q, r, err)
	}
	return id
}

func newSiegeCapitulationHandler(pool *pgxpool.Pool, hub Broadcaster) *SiegeCapitulationHandler {
	sched := events.NewScheduler(pool, clock.NewTestClock(time.Now()))
	store := events.NewStore(pool)
	return NewSiegeCapitulationHandler(pool, store, sched, hub)
}

func runSiegeCapitulation(t *testing.T, h *SiegeCapitulationHandler, worldID, settlementID uuid.UUID) {
	t.Helper()
	payload, err := json.Marshal(SiegeCapitulationPayload{SettlementID: settlementID, WorldID: worldID})
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	if err := h.Handle(context.Background(), events.ScheduledEvent{
		WorldID: worldID, EventType: events.ScheduledSiegeCapitulation, Payload: payload,
	}); err != nil {
		t.Fatalf("handle siege capitulation: %v", err)
	}
}

// TestSiegeCapitulation_CityFallsToStrongestBesieger is S3's core proof: a
// besieged, starving city falls to occupation under its STRONGEST besieger
// (economy.LoadBesiegers orders by size DESC) — not merely the first one
// found, not the attacker of a prior battle. owner_id stays untouched
// (capitulation is occupy, not annex — same PO1 invariant as a battle win),
// the stale defending garrison is evicted, besieged clears, and the
// starvation clock resets to 0.
func TestSiegeCapitulation_CityFallsToStrongestBesieger(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	f := newSiegeFixture(t, pool, 3) // wall level irrelevant — muren rörs aldrig (§S3)

	if _, err := pool.Exec(ctx,
		`UPDATE settlements SET besieged = true, siege_starvation_ticks = 30 WHERE id = $1`, f.defSettlement,
	); err != nil {
		t.Fatalf("mark besieged+starving: %v", err)
	}
	garrisonID := mkSiegeGarrison(t, pool, f, 150)

	var rivalID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO players (username, email, password_hash) VALUES ($1, $2, 'x') RETURNING id`,
		"rival-"+uuid.New().String(), "rival-"+uuid.New().String()+"@test.invalid",
	).Scan(&rivalID); err != nil {
		t.Fatalf("create rival player: %v", err)
	}
	placeSentryUnit(t, pool, f.worldID, rivalID, 1, 0, 200)    // weaker besieger — must NOT win
	placeSentryUnit(t, pool, f.worldID, f.attacker, 1, 0, 900) // stronger — must become occupant

	fb := &fakeBroadcaster{}
	h := newSiegeCapitulationHandler(pool, fb)
	runSiegeCapitulation(t, h, f.worldID, f.defSettlement)

	var state string
	var ownerID, occupantID uuid.UUID
	var besieged bool
	var starvTicks int
	var sinceTick *int
	if err := pool.QueryRow(ctx,
		`SELECT state, owner_id, occupant_id, besieged, siege_starvation_ticks, occupied_since_tick
		 FROM settlements WHERE id = $1`, f.defSettlement,
	).Scan(&state, &ownerID, &occupantID, &besieged, &starvTicks, &sinceTick); err != nil {
		t.Fatalf("read settlement: %v", err)
	}
	if state != "occupied" {
		t.Errorf("state = %q, want \"occupied\"", state)
	}
	if ownerID != f.defender {
		t.Errorf("owner_id = %s, want UNCHANGED %s — capitulation is occupy, not annex", ownerID, f.defender)
	}
	if occupantID != f.attacker {
		t.Errorf("occupant_id = %s, want strongest besieger %s (not the weaker rival)", occupantID, f.attacker)
	}
	if besieged {
		t.Errorf("besieged flag still true, want cleared once occupied")
	}
	if starvTicks != 0 {
		t.Errorf("siege_starvation_ticks = %d, want reset to 0", starvTicks)
	}
	if sinceTick == nil {
		t.Errorf("occupied_since_tick is nil, want set")
	}

	var garrisonStatus string
	if err := pool.QueryRow(ctx, `SELECT status FROM units WHERE id = $1`, garrisonID).Scan(&garrisonStatus); err != nil {
		t.Fatalf("read garrison: %v", err)
	}
	if garrisonStatus != "disbanded" {
		t.Errorf("stale garrison status = %q, want disbanded", garrisonStatus)
	}

	if len(fb.notified) < 2 {
		t.Errorf("expected CityOccupied notifications to both defender and occupant, got %d calls", len(fb.notified))
	}

	// Idempotency (events.Worker requires every handler survive a re-run):
	// the city is already occupied — a second delivery of the same event
	// must be a harmless no-op, not a second eviction/notification cycle.
	runSiegeCapitulation(t, h, f.worldID, f.defSettlement)
	var stateAfter string
	if err := pool.QueryRow(ctx, `SELECT state FROM settlements WHERE id = $1`, f.defSettlement).Scan(&stateAfter); err != nil {
		t.Fatalf("read settlement after re-run: %v", err)
	}
	if stateAfter != "occupied" {
		t.Errorf("state after idempotent re-run = %q, want still \"occupied\"", stateAfter)
	}
}

// TestSiegeCapitulation_BlockadeAlreadyLiftedIsNoOp covers the race the plan
// implicitly requires be safe: the daily tick enqueued this event when the
// city was besieged and starving, but by the time the worker delivers it the
// blockade has already lifted (besieged=false). Must be a pure no-op, not a
// bogus occupation of a currently-fine city.
func TestSiegeCapitulation_BlockadeAlreadyLiftedIsNoOp(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	f := newSiegeFixture(t, pool, 0) // besieged left at its default (false)

	h := newSiegeCapitulationHandler(pool, nil)
	runSiegeCapitulation(t, h, f.worldID, f.defSettlement)

	var state string
	var occupantID *uuid.UUID
	if err := pool.QueryRow(ctx,
		`SELECT state, occupant_id FROM settlements WHERE id = $1`, f.defSettlement,
	).Scan(&state, &occupantID); err != nil {
		t.Fatalf("read settlement: %v", err)
	}
	if state != "active" || occupantID != nil {
		t.Errorf("settlement state = %q occupant = %v, want untouched (\"active\", nil) — blockade already lifted", state, occupantID)
	}
}

// TestSiegeCapitulation_NoBesiegerFoundResetsClock covers the other race:
// besieged was still true when scheduled, but the besieging unit moved off
// its chokepoint (or was destroyed) by delivery time — economy.LoadBesiegers
// finds nobody to hand the city to. Must reset the clock and leave the
// settlement otherwise untouched, not occupy it under nobody.
func TestSiegeCapitulation_NoBesiegerFoundResetsClock(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	f := newSiegeFixture(t, pool, 0)
	if _, err := pool.Exec(ctx,
		`UPDATE settlements SET besieged = true, siege_starvation_ticks = 30 WHERE id = $1`, f.defSettlement,
	); err != nil {
		t.Fatalf("mark besieged+starving: %v", err)
	}

	h := newSiegeCapitulationHandler(pool, nil)
	runSiegeCapitulation(t, h, f.worldID, f.defSettlement)

	var state string
	var starvTicks int
	var occupantID *uuid.UUID
	if err := pool.QueryRow(ctx,
		`SELECT state, siege_starvation_ticks, occupant_id FROM settlements WHERE id = $1`, f.defSettlement,
	).Scan(&state, &starvTicks, &occupantID); err != nil {
		t.Fatalf("read settlement: %v", err)
	}
	if state != "active" || occupantID != nil {
		t.Errorf("settlement state = %q occupant = %v, want untouched — no besieger to hand the city to", state, occupantID)
	}
	if starvTicks != 0 {
		t.Errorf("siege_starvation_ticks = %d, want reset to 0", starvTicks)
	}
}
