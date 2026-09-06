package economy

// Röd-först coverage for megaron_plan_belagringsdispatch.md's own contract:
//   - SyncSiegeState fires SiegeStarted on false→true, SiegeLifted on
//     true→false, and nothing when besieged is unchanged
//   - calling it twice in a row with no state change fires nothing the
//     second time (idempotent via siege_notified, mig 143)
//   - the payload carries the settlement's own q,r (for "⌖ Take me there")
//     and, for SiegeStarted, the largest besieger's name+unit

import (
	"context"
	"testing"

	"formatet/megaron/server/internal/events"
	"github.com/google/uuid"
)

type fakeSiegeNotifier struct {
	calls []fakeSiegeCall
}

type fakeSiegeCall struct {
	worldID  uuid.UUID
	playerID uuid.UUID
	kind     string
	level    int
	payload  SiegeDispatchPayload
}

func (f *fakeSiegeNotifier) NotifyPlayer(ctx context.Context, worldID, playerID uuid.UUID, kind string, level int, payload any) error {
	f.calls = append(f.calls, fakeSiegeCall{worldID, playerID, kind, level, payload.(SiegeDispatchPayload)})
	return nil
}

// TestSyncSiegeState_FiresDispatchOnStartAndLift drives a real siege via
// RecomputeProduction (the corridor-chokepoint fixture from siege_test.go)
// so besieged is written the exact way production does it in the real tick
// loop, then checks SyncSiegeState's transition detection sits correctly
// downstream of that write.
func TestSyncSiegeState_FiresDispatchOnStartAndLift(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	const tick = 100

	settlementID, worldID, ownerID := seedSiegeFixture(t, tick, 100)
	seedTile(t, worldID, 1, 0, "hills")
	seedTile(t, worldID, 2, 0, "hills")
	seedTile(t, worldID, 1, 1, "mountain_limestone")
	seedTile(t, worldID, 2, -1, "mountain_limestone")
	placeGubbe(t, settlementID, 1, 2, 0, GoodGrain)

	store := events.NewStore(pool)
	notifier := &fakeSiegeNotifier{}

	// ── No enemy: not besieged, sync fires nothing. ────────────────────────
	if err := RecomputeProduction(ctx, pool, settlementID); err != nil {
		t.Fatalf("RecomputeProduction (no enemy): %v", err)
	}
	if err := SyncSiegeState(ctx, pool, store, notifier, worldID, settlementID); err != nil {
		t.Fatalf("SyncSiegeState (no enemy): %v", err)
	}
	if len(notifier.calls) != 0 {
		t.Fatalf("dispatches fired with no siege present: %v", notifier.calls)
	}

	// ── Enemy sentried on the sole corridor hex: besieged flips true. ──────
	enemyOwner := uuid.New()
	if _, err := pool.Exec(ctx,
		`INSERT INTO players (id, username, password_hash) VALUES ($1, $2, 'x')`,
		enemyOwner, "siege-dispatch-enemy-"+enemyOwner.String(),
	); err != nil {
		t.Fatalf("create enemy player: %v", err)
	}
	enemyUnitID := placeEnemyUnit(t, worldID, enemyOwner, 1, 0)

	if err := RecomputeProduction(ctx, pool, settlementID); err != nil {
		t.Fatalf("RecomputeProduction (besieged): %v", err)
	}
	if !readBesieged(t, settlementID) {
		t.Fatalf("besieged = false after placing a sentried enemy on the corridor, want true")
	}
	if err := SyncSiegeState(ctx, pool, store, notifier, worldID, settlementID); err != nil {
		t.Fatalf("SyncSiegeState (besieged): %v", err)
	}
	if len(notifier.calls) != 1 {
		t.Fatalf("dispatch calls after the siege starts = %d, want 1", len(notifier.calls))
	}
	started := notifier.calls[0]
	if started.kind != EventSiegeStarted {
		t.Errorf("kind = %q, want %q", started.kind, EventSiegeStarted)
	}
	if started.level != 2 {
		t.Errorf("level = %d, want 2 (start/warning, same as HexBlockaded)", started.level)
	}
	if started.playerID != ownerID {
		t.Errorf("notified player = %v, want the settlement owner %v", started.playerID, ownerID)
	}
	if started.payload.Q != 0 || started.payload.R != 0 {
		t.Errorf("payload q,r = (%d,%d), want the SETTLEMENT'S OWN hex (0,0), not the besieger's — take-me-there points at the city under siege", started.payload.Q, started.payload.R)
	}
	if started.payload.Name == "" {
		t.Errorf("payload name is empty — every dispatch must name its subject")
	}
	if started.payload.BesiegerUnit != "spearman" {
		t.Errorf("besieger unit = %q, want %q", started.payload.BesiegerUnit, "spearman")
	}

	// Idempotency: calling again with NOTHING changed must fire nothing more.
	notifier.calls = nil
	if err := RecomputeProduction(ctx, pool, settlementID); err != nil {
		t.Fatalf("RecomputeProduction (steady state): %v", err)
	}
	if err := SyncSiegeState(ctx, pool, store, notifier, worldID, settlementID); err != nil {
		t.Fatalf("SyncSiegeState (steady state): %v", err)
	}
	if len(notifier.calls) != 0 {
		t.Fatalf("dispatches fired on an unchanged besieged steady state: %v", notifier.calls)
	}

	// ── Enemy withdraws: besieged flips false, SiegeLifted fires. ──────────
	removeUnit(t, enemyUnitID)
	if err := RecomputeProduction(ctx, pool, settlementID); err != nil {
		t.Fatalf("RecomputeProduction (lifted): %v", err)
	}
	if readBesieged(t, settlementID) {
		t.Fatalf("besieged = true after the enemy withdrew, want false")
	}
	if err := SyncSiegeState(ctx, pool, store, notifier, worldID, settlementID); err != nil {
		t.Fatalf("SyncSiegeState (lifted): %v", err)
	}
	if len(notifier.calls) != 1 {
		t.Fatalf("dispatch calls after the siege lifts = %d, want 1", len(notifier.calls))
	}
	lifted := notifier.calls[0]
	if lifted.kind != EventSiegeLifted {
		t.Errorf("kind = %q, want %q", lifted.kind, EventSiegeLifted)
	}
	if lifted.level != 3 {
		t.Errorf("level = %d, want 3 (lifted/good news, same as HexUnblockaded)", lifted.level)
	}
	if lifted.payload.BesiegerName != "" || lifted.payload.BesiegerUnit != "" {
		t.Errorf("SiegeLifted payload carries a besieger (%q, %q), want both empty", lifted.payload.BesiegerName, lifted.payload.BesiegerUnit)
	}

	// A real event was appended for both transitions (events store outcomes,
	// per CLAUDE.md's event-sourcing rule).
	var eventCount int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM events WHERE stream_id = $1 AND event_type IN ($2, $3)`,
		settlementID, EventSiegeStarted, EventSiegeLifted,
	).Scan(&eventCount); err != nil {
		t.Fatalf("count events: %v", err)
	}
	if eventCount != 2 {
		t.Errorf("events appended = %d, want 2 (one per transition)", eventCount)
	}

	// siege_notified persisted false again, matching besieged.
	var siegeNotified bool
	if err := pool.QueryRow(ctx,
		`SELECT siege_notified FROM settlements WHERE id = $1`, settlementID,
	).Scan(&siegeNotified); err != nil {
		t.Fatalf("read siege_notified: %v", err)
	}
	if siegeNotified {
		t.Errorf("siege_notified = true after the siege lifted, want false")
	}
}

// TestSyncSiegeState_Backfill_NoSpuriousStartOnAlreadyBesiegedSettlement is
// the plan's acceptance criterion 5: a settlement besieged BEFORE this
// migration's backfill ran (siege_notified = besieged) must not fire a
// SiegeStarted the first time SyncSiegeState runs after deploy.
func TestSyncSiegeState_Backfill_NoSpuriousStartOnAlreadyBesiegedSettlement(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	const tick = 100

	settlementID, worldID, _ := seedSiegeFixture(t, tick, 100)
	// Simulate the migration's backfill: besieged was already true, and
	// siege_notified was backfilled to match it (mig 143's own UPDATE).
	if _, err := pool.Exec(ctx,
		`UPDATE settlements SET besieged = true, siege_notified = true WHERE id = $1`,
		settlementID,
	); err != nil {
		t.Fatalf("simulate pre-existing siege: %v", err)
	}

	store := events.NewStore(pool)
	notifier := &fakeSiegeNotifier{}
	if err := SyncSiegeState(ctx, pool, store, notifier, worldID, settlementID); err != nil {
		t.Fatalf("SyncSiegeState: %v", err)
	}
	if len(notifier.calls) != 0 {
		t.Fatalf("dispatches fired on a backfilled already-besieged settlement: %v", notifier.calls)
	}
}

// TestSyncSiegeState_NoOwner_DispatchesNothing mirrors SyncHexBlockade's own
// guard: a not-yet-claimed settlement has no wanax to notify.
func TestSyncSiegeState_NoOwner_DispatchesNothing(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	const tick = 100

	settlementID, worldID, _ := seedSiegeFixture(t, tick, 100)
	if _, err := pool.Exec(ctx,
		`UPDATE settlements SET owner_id = NULL, besieged = true WHERE id = $1`,
		settlementID,
	); err != nil {
		t.Fatalf("clear owner: %v", err)
	}

	store := events.NewStore(pool)
	notifier := &fakeSiegeNotifier{}
	if err := SyncSiegeState(ctx, pool, store, notifier, worldID, settlementID); err != nil {
		t.Fatalf("SyncSiegeState: %v", err)
	}
	if len(notifier.calls) != 0 {
		t.Fatalf("dispatches fired for an unowned settlement: %v", notifier.calls)
	}
}
