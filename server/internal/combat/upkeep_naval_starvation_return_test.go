package combat

// Naval starvation auto-return (megaron_plan_svaltretur_till_sjoss.md) — a
// positioned ship whose crew starves down to navalStarvationReturnCrewFraction
// (half) of its full crew turns for home on its own, via the SAME
// dispatchReturnHome mechanics as the explore auto-return and the sentry
// patrol timer (unit_arrival_explore_test.go, unit_arrival_sentry_test.go).
// A ship at sea receives no orders, so this is the only channel a Wanax who
// is away has to ever see the ship again before it starves to nothing.
//
// DB integration tests (real Postgres, gated by DATABASE_URL). Map layout
// mirrors the explore/sentry auto-return tests: a coastal capital at (0,0),
// open sea running east to (3,0) — axialDirs puts (1,0) as (0,0)'s first
// neighbour, so NearestSeaNeighbor resolves the return hex deterministically.

import (
	"context"
	"testing"
	"time"

	"formatet/megaron/server/internal/clock"
	"formatet/megaron/server/internal/economy"
	"formatet/megaron/server/internal/events"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// starvationFixture is a coastal capital at (0,0) with open sea to (3,0),
// grain-empty (every upkeep tick fails to feed a ship from the town too, not
// that it matters — a naval unit's provisions are always 0 in these tests,
// so provisionSourceFor's own-stores draw fails every tick and attrition
// fires immediately, before the settlement grain branch is ever reached).
type starvationFixture struct {
	worldID, ownerID, capitalID uuid.UUID
}

func newStarvationFixture(t *testing.T, pool *pgxpool.Pool, tag string) starvationFixture {
	t.Helper()
	ctx := context.Background()

	if _, err := pool.Exec(ctx, `UPDATE worlds SET status = 'archived' WHERE status = 'active'`); err != nil {
		t.Fatalf("archive leftover active test worlds: %v", err)
	}
	var f starvationFixture
	if err := pool.QueryRow(ctx,
		`INSERT INTO worlds (name, status) VALUES ($1, 'active') RETURNING id`,
		"test-"+tag+"-"+uuid.New().String(),
	).Scan(&f.worldID); err != nil {
		t.Fatalf("create world: %v", err)
	}
	t.Cleanup(func() { _, _ = pool.Exec(ctx, `UPDATE worlds SET status = 'archived' WHERE id = $1`, f.worldID) })

	if err := pool.QueryRow(ctx,
		`INSERT INTO players (username, password_hash) VALUES ($1, 'x') RETURNING id`,
		tag+"-"+uuid.New().String(),
	).Scan(&f.ownerID); err != nil {
		t.Fatalf("create player: %v", err)
	}

	tiles := []struct {
		q, r    int
		terrain string
	}{
		{0, 0, "plains"}, {1, 0, "coastal_sea"}, {2, 0, "coastal_sea"}, {3, 0, "coastal_sea"},
	}
	for _, tl := range tiles {
		if _, err := pool.Exec(ctx,
			`INSERT INTO map_tiles (world_id, q, r, terrain) VALUES ($1,$2,$3,$4)`,
			f.worldID, tl.q, tl.r, tl.terrain); err != nil {
			t.Fatalf("insert map tile (%d,%d): %v", tl.q, tl.r, err)
		}
	}

	var provinceID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO provinces (world_id, map_q, map_r, terrain_type) VALUES ($1,0,0,'plains') RETURNING id`,
		f.worldID).Scan(&provinceID); err != nil {
		t.Fatalf("create capital province: %v", err)
	}
	if err := pool.QueryRow(ctx,
		`INSERT INTO settlements (world_id, province_id, name, culture_id, owner_id, control_type, is_capital, state, population)
		 VALUES ($1,$2,'Capital City','achaean',$3,'capital',true,'active',1000) RETURNING id`,
		f.worldID, provinceID, f.ownerID).Scan(&f.capitalID); err != nil {
		t.Fatalf("create capital settlement: %v", err)
	}
	return f
}

// mkStarvingShip inserts a galley (crew 20, full crew per unit.CrewFor) at
// (3,0), positioned, provisions=0 (so it starves on the very first upkeep
// tick regardless of the town's own granary — provisionSourceFor always
// tries the ship's own stores first for a naval unit). homeSID is the
// unit's home_settlement_id; pass uuid.Nil for "no home settlement".
func mkStarvingShip(t *testing.T, pool *pgxpool.Pool, worldID, ownerID, supportSID, homeSID uuid.UUID) uuid.UUID {
	t.Helper()
	var homeArg any
	if homeSID != uuid.Nil {
		homeArg = homeSID
	}
	var id uuid.UUID
	if err := pool.QueryRow(context.Background(),
		`INSERT INTO units (world_id, owner_id, type, category, size, crew, status,
		                    q, r, support_settlement_id, home_settlement_id, provisions)
		 VALUES ($1, $2, 'galley', 'naval', 1, 20, 'positioned', 3, 0, $3, $4, 0)
		 RETURNING id`,
		worldID, ownerID, supportSID, homeArg,
	).Scan(&id); err != nil {
		t.Fatalf("create starving ship: %v", err)
	}
	return id
}

// newStarvationUpkeepHandler wires an UpkeepHandler with a real
// UnitArrivalHandler behind it — the whole point of this slice is that the
// two share dispatchReturnHome, so the test rig must exercise that, not a
// nil arrivals stub.
func newStarvationUpkeepHandler(pool *pgxpool.Pool, hub Broadcaster) *UpkeepHandler {
	clk := clock.NewTestClock(time.Now())
	arrivals := NewUnitArrivalHandler(pool, events.NewStore(pool), hub, events.NewScheduler(pool, clk), clk, economy.SitosConfig{})
	return NewUpkeepHandler(pool, events.NewScheduler(pool, clk), events.NewStore(pool), hub, arrivals)
}

// 1. Trigger: crew 20 → 18 → 16 → 14 → 12 → 10 over five grain-starved ticks
// (navalAttritionCrewStep=2/tick). At crew=10 — exactly half of the galley's
// full crew (unit.CrewFor(TypeGalley)=20) — the ship must turn for home:
// status flips to 'marching', a ScheduledUnitArrival is queued, and the
// notification/event kind is the NEW type, never UnitExploreReturned.
func TestUpkeep_NavalStarvationReturn_TriggersAtHalfCrew(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	f := newStarvationFixture(t, pool, "starve-trigger")
	seedGoods(t, pool, f.capitalID, 0, 100000, 100000)

	shipID := mkStarvingShip(t, pool, f.worldID, f.ownerID, f.capitalID, f.capitalID)

	broadcaster := &fakeBroadcaster{}
	h := newStarvationUpkeepHandler(pool, broadcaster)

	var status string
	var crew int
	for i := int64(1); i <= 5; i++ {
		if err := h.Handle(ctx, events.ScheduledEvent{ID: 95000 + i, WorldID: f.worldID}); err != nil {
			t.Fatalf("upkeep Handle tick %d: %v", i, err)
		}
		if err := pool.QueryRow(ctx, `SELECT status, crew FROM units WHERE id = $1`, shipID).Scan(&status, &crew); err != nil {
			t.Fatalf("read ship after tick %d: %v", i, err)
		}
		if status == "marching" {
			break
		}
	}

	if crew != 10 {
		t.Errorf("crew at trigger = %d, want 10 (half of 20)", crew)
	}
	if status != "marching" {
		t.Fatalf("ship status = %q after crew reached %d, want marching (turned for home)", status, crew)
	}

	var targetQ, targetR int
	var marchIntent string
	if err := pool.QueryRow(ctx,
		`SELECT COALESCE(march_intent, ''), target_q, target_r FROM units WHERE id = $1`, shipID,
	).Scan(&marchIntent, &targetQ, &targetR); err != nil {
		t.Fatalf("read ship route: %v", err)
	}
	if marchIntent != "explore_return" {
		t.Errorf("march_intent = %q, want explore_return (shared return-leg routing)", marchIntent)
	}
	if targetQ != 1 || targetR != 0 {
		t.Errorf("return target = (%d,%d), want (1,0) — the home harbour hex", targetQ, targetR)
	}

	var scheduledCount int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM scheduled_events
		 WHERE world_id = $1 AND event_type = 'UnitArrival'
		   AND (payload->>'unit_id')::uuid = $2 AND processed_at IS NULL`,
		f.worldID, shipID,
	).Scan(&scheduledCount); err != nil {
		t.Fatalf("count scheduled return arrival: %v", err)
	}
	if scheduledCount != 1 {
		t.Errorf("scheduled return-arrival events = %d, want 1", scheduledCount)
	}

	var starvingEvents, exploreEvents int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM events WHERE stream_id = $1 AND event_type = 'UnitReturnedStarving'`, shipID,
	).Scan(&starvingEvents); err != nil {
		t.Fatalf("count UnitReturnedStarving events: %v", err)
	}
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM events WHERE stream_id = $1 AND event_type = 'UnitExploreReturned'`, shipID,
	).Scan(&exploreEvents); err != nil {
		t.Fatalf("count UnitExploreReturned events: %v", err)
	}
	if starvingEvents != 1 {
		t.Errorf("UnitReturnedStarving events = %d, want 1", starvingEvents)
	}
	if exploreEvents != 0 {
		t.Errorf("UnitExploreReturned events = %d, want 0 — a starving return must never emit the explore event type", exploreEvents)
	}

	foundKind := false
	for _, k := range broadcaster.notified {
		if k == "UnitExploreReturned" {
			t.Errorf("owner was notified with kind UnitExploreReturned for a starvation return — the notification must not lie about why the ship turned")
		}
		if k == "UnitReturnedStarving" {
			foundKind = true
		}
	}
	if !foundKind {
		t.Errorf("owner was never notified with kind UnitReturnedStarving")
	}
}

// 2. Garrison is never touched: a garrisoned ship (docked, not at sea) that
// starves via the same grain-shortage path must never be dispatched home —
// it is already home. Provisions=0 is irrelevant to docked status directly,
// but zero grain in the settlement's own granary forces the fallback
// attrition branch even for a garrisoned ship (atHomePort path).
func TestUpkeep_NavalStarvationReturn_GarrisonNotTouched(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	f := newStarvationFixture(t, pool, "starve-garrison")
	seedGoods(t, pool, f.capitalID, 0, 0, 100000) // no grain at all

	var shipID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO units (world_id, owner_id, type, category, size, crew, status,
		                    settlement_id, support_settlement_id, home_settlement_id, provisions)
		 VALUES ($1, $2, 'galley', 'naval', 1, 20, 'garrison', $3, $3, $3, 0)
		 RETURNING id`,
		f.worldID, f.ownerID, f.capitalID,
	).Scan(&shipID); err != nil {
		t.Fatalf("create garrisoned ship: %v", err)
	}

	h := newStarvationUpkeepHandler(pool, &fakeBroadcaster{})

	var status string
	for i := int64(1); i <= 5; i++ {
		if err := h.Handle(ctx, events.ScheduledEvent{ID: 96000 + i, WorldID: f.worldID}); err != nil {
			t.Fatalf("upkeep Handle tick %d: %v", i, err)
		}
	}
	var crew int
	if err := pool.QueryRow(ctx, `SELECT status, crew FROM units WHERE id = $1`, shipID).Scan(&status, &crew); err != nil {
		t.Fatalf("read ship: %v", err)
	}
	if crew > 10 {
		t.Fatalf("test setup: crew = %d after 5 starved ticks, want <=10 (else the trigger condition was never exercised)", crew)
	}
	if status != "garrison" {
		t.Errorf("garrisoned ship status = %q after crew fell to %d, want garrison unchanged — a docked ship must never be auto-dispatched", status, crew)
	}
}

// 3. Homeless ship: home_settlement_id IS NULL ⇒ no dispatch, no error, ship
// just keeps starving in place until it either dies or (in a real game) an
// owner does something about it another way.
func TestUpkeep_NavalStarvationReturn_NoHomeSettlement(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	f := newStarvationFixture(t, pool, "starve-homeless")
	seedGoods(t, pool, f.capitalID, 0, 100000, 100000)

	shipID := mkStarvingShip(t, pool, f.worldID, f.ownerID, f.capitalID, uuid.Nil)

	h := newStarvationUpkeepHandler(pool, &fakeBroadcaster{})

	var status string
	var crew int
	for i := int64(1); i <= 5; i++ {
		if err := h.Handle(ctx, events.ScheduledEvent{ID: 97000 + i, WorldID: f.worldID}); err != nil {
			t.Fatalf("upkeep Handle tick %d: %v", i, err)
		}
	}
	if err := pool.QueryRow(ctx, `SELECT status, crew FROM units WHERE id = $1`, shipID).Scan(&status, &crew); err != nil {
		t.Fatalf("read ship: %v", err)
	}
	if crew > 10 {
		t.Fatalf("test setup: crew = %d after 5 starved ticks, want <=10", crew)
	}
	if status != "positioned" {
		t.Errorf("homeless ship status = %q after crew fell to %d, want positioned unchanged — nowhere to send it", status, crew)
	}
}

// 4. G2 idempotency (hard grind): calling Handle TWICE with the SAME
// scheduled event on the tick that crosses the threshold must dispatch the
// ship home exactly ONCE — one ScheduledUnitArrival row, one
// UnitReturnedStarving notification. The existing per-unit processed_tick_claims
// claim (same event_id + unit_id) already blocks the whole per-unit body
// including the new starvation-return call on a same-event replay; this test
// proves that holds for this new path too.
func TestUpkeep_NavalStarvationReturn_Idempotent(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	f := newStarvationFixture(t, pool, "starve-idem")
	seedGoods(t, pool, f.capitalID, 0, 100000, 100000)

	shipID := mkStarvingShip(t, pool, f.worldID, f.ownerID, f.capitalID, f.capitalID)

	broadcaster := &fakeBroadcaster{}
	h := newStarvationUpkeepHandler(pool, broadcaster)

	// Four ticks to bring crew to 12 (still above the 10 threshold).
	for i := int64(1); i <= 4; i++ {
		if err := h.Handle(ctx, events.ScheduledEvent{ID: 98000 + i, WorldID: f.worldID}); err != nil {
			t.Fatalf("upkeep Handle warm-up tick %d: %v", i, err)
		}
	}
	var crew int
	if err := pool.QueryRow(ctx, `SELECT crew FROM units WHERE id = $1`, shipID).Scan(&crew); err != nil {
		t.Fatalf("read crew before trigger tick: %v", err)
	}
	if crew != 12 {
		t.Fatalf("test setup: crew = %d before the trigger tick, want 12", crew)
	}

	// The FIFTH tick — same event, run twice — crosses the threshold.
	triggerEvent := events.ScheduledEvent{ID: 98999, WorldID: f.worldID}
	if err := h.Handle(ctx, triggerEvent); err != nil {
		t.Fatalf("upkeep Handle trigger tick (1st): %v", err)
	}
	if err := h.Handle(ctx, triggerEvent); err != nil {
		t.Fatalf("upkeep Handle trigger tick (2nd, replay): %v", err)
	}

	var status string
	if err := pool.QueryRow(ctx, `SELECT status, crew FROM units WHERE id = $1`, shipID).Scan(&status, &crew); err != nil {
		t.Fatalf("read ship after replay: %v", err)
	}
	if crew != 10 {
		t.Errorf("crew after replayed trigger tick = %d, want 10 — a replay must not deduct twice", crew)
	}
	if status != "marching" {
		t.Fatalf("status = %q, want marching", status)
	}

	var scheduledCount int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM scheduled_events
		 WHERE world_id = $1 AND event_type = 'UnitArrival' AND (payload->>'unit_id')::uuid = $2`,
		f.worldID, shipID,
	).Scan(&scheduledCount); err != nil {
		t.Fatalf("count scheduled return arrivals: %v", err)
	}
	if scheduledCount != 1 {
		t.Errorf("scheduled return-arrival events = %d, want exactly 1 across both Handle calls", scheduledCount)
	}

	var starvingEvents int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM events WHERE stream_id = $1 AND event_type = 'UnitReturnedStarving'`, shipID,
	).Scan(&starvingEvents); err != nil {
		t.Fatalf("count UnitReturnedStarving events: %v", err)
	}
	if starvingEvents != 1 {
		t.Errorf("UnitReturnedStarving events = %d, want exactly 1 across both Handle calls", starvingEvents)
	}

	notifiedCount := 0
	for _, k := range broadcaster.notified {
		if k == "UnitReturnedStarving" {
			notifiedCount++
		}
	}
	if notifiedCount != 1 {
		t.Errorf("UnitReturnedStarving notifications = %d, want exactly 1", notifiedCount)
	}
}

// 5. Land unit not touched (plan §5 criterion 4): maybeStarvationReturn is
// only ever called from applyAttrition's NAVAL branch (upkeep.go:805) — a
// land cohort's own starvation path loses SIZE, not crew, and never reaches
// the call at all. That makes this true by construction today, but a call
// site is not a contract: the day someone moves or duplicates that call,
// this stops being true silently. This test exercises the real path — a
// spearman cohort starved for enough ticks to fall under half its starting
// size — and proves no scheduled return-arrival and no UnitReturnedStarving
// event are ever produced for it, and its status is left exactly as it was.
func TestUpkeep_NavalStarvationReturn_LandUnitNotTouched(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	f := newStarvationFixture(t, pool, "starve-land")
	seedGoods(t, pool, f.capitalID, 0, 0, 100000) // no grain at all

	var unitID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO units (world_id, owner_id, type, category, size, crew, status,
		                    q, r, settlement_id, support_settlement_id, home_settlement_id)
		 VALUES ($1, $2, 'spearman', 'land', 100, 0, 'positioned', 0, 0, $3, $3, $3)
		 RETURNING id`,
		f.worldID, f.ownerID, f.capitalID,
	).Scan(&unitID); err != nil {
		t.Fatalf("create starving land unit: %v", err)
	}

	broadcaster := &fakeBroadcaster{}
	h := newStarvationUpkeepHandler(pool, broadcaster)

	// 10%/tick ceil-rounded loss on size 100 crosses below half (50) by tick 7
	// (100→90→81→72→64→57→51→45); nine ticks leaves margin.
	for i := int64(1); i <= 9; i++ {
		if err := h.Handle(ctx, events.ScheduledEvent{ID: 99000 + i, WorldID: f.worldID}); err != nil {
			t.Fatalf("upkeep Handle tick %d: %v", i, err)
		}
	}

	var status string
	var size int
	if err := pool.QueryRow(ctx, `SELECT status, size FROM units WHERE id = $1`, unitID).Scan(&status, &size); err != nil {
		t.Fatalf("read land unit: %v", err)
	}
	if size >= 50 {
		t.Fatalf("test setup: size = %d after 9 starved ticks, want <50 (else the below-half case was never exercised)", size)
	}
	if status != "positioned" {
		t.Errorf("land unit status = %q after starving to size %d, want positioned unchanged — naval starvation return must never touch a land unit", status, size)
	}

	var scheduledCount int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM scheduled_events
		 WHERE world_id = $1 AND event_type = 'UnitArrival' AND (payload->>'unit_id')::uuid = $2`,
		f.worldID, unitID,
	).Scan(&scheduledCount); err != nil {
		t.Fatalf("count scheduled return arrivals: %v", err)
	}
	if scheduledCount != 0 {
		t.Errorf("scheduled return-arrival events for land unit = %d, want 0", scheduledCount)
	}

	var starvingEvents int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM events WHERE stream_id = $1 AND event_type = 'UnitReturnedStarving'`, unitID,
	).Scan(&starvingEvents); err != nil {
		t.Fatalf("count UnitReturnedStarving events: %v", err)
	}
	if starvingEvents != 0 {
		t.Errorf("UnitReturnedStarving events for land unit = %d, want 0", starvingEvents)
	}
}

// 6. Already marching, not redispatched (plan §5 criterion 5): a galley whose
// status is already 'marching' when its starved crew crosses the threshold
// must not be sent through dispatchReturnHome a second time.
// maybeStarvationReturn's own `u.status != "positioned"` guard (upkeep.go:866)
// is exactly the same idempotency guard the cross-tick test above exercises
// for the marching-after-dispatch case — this test covers the OTHER way a
// ship can already be marching: it was already under sail (e.g. given an
// explicit order, or mid an earlier return) before this tick's attrition
// ever ran. Same guard, different starting condition, worth its own proof.
func TestUpkeep_NavalStarvationReturn_AlreadyMarchingNotRedispatched(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	f := newStarvationFixture(t, pool, "starve-already-marching")
	seedGoods(t, pool, f.capitalID, 0, 100000, 100000)

	// Galley (full crew 20 per unit.CrewFor), crew already at exactly half
	// (10), status 'marching' from the very start — never 'positioned' at
	// any point this test observes.
	var shipID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO units (world_id, owner_id, type, category, size, crew, status,
		                    q, r, support_settlement_id, home_settlement_id, provisions)
		 VALUES ($1, $2, 'galley', 'naval', 1, 10, 'marching', 3, 0, $3, $4, 0)
		 RETURNING id`,
		f.worldID, f.ownerID, f.capitalID, f.capitalID,
	).Scan(&shipID); err != nil {
		t.Fatalf("create already-marching starving galley: %v", err)
	}

	broadcaster := &fakeBroadcaster{}
	h := newStarvationUpkeepHandler(pool, broadcaster)

	// Three ticks (crew 10→8→6→4, navalAttritionCrewStep=2) — well clear of
	// disbanding at 0, so the marching status is never overwritten by that
	// unrelated path either.
	for i := int64(1); i <= 3; i++ {
		if err := h.Handle(ctx, events.ScheduledEvent{ID: 99500 + i, WorldID: f.worldID}); err != nil {
			t.Fatalf("upkeep Handle tick %d: %v", i, err)
		}
	}

	var status string
	var crew int
	if err := pool.QueryRow(ctx, `SELECT status, crew FROM units WHERE id = $1`, shipID).Scan(&status, &crew); err != nil {
		t.Fatalf("read ship: %v", err)
	}
	if crew >= 10 {
		t.Fatalf("test setup: crew = %d after 3 starved ticks while marching, want <10 (else the already-marching attrition path was never exercised)", crew)
	}
	if status != "marching" {
		t.Errorf("ship status = %q after starving further while already marching, want marching unchanged", status)
	}

	var scheduledCount int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM scheduled_events
		 WHERE world_id = $1 AND event_type = 'UnitArrival' AND (payload->>'unit_id')::uuid = $2`,
		f.worldID, shipID,
	).Scan(&scheduledCount); err != nil {
		t.Fatalf("count scheduled return arrivals: %v", err)
	}
	if scheduledCount != 0 {
		t.Errorf("scheduled return-arrival events for already-marching ship = %d, want 0 — no second dispatch", scheduledCount)
	}

	var starvingEvents int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM events WHERE stream_id = $1 AND event_type = 'UnitReturnedStarving'`, shipID,
	).Scan(&starvingEvents); err != nil {
		t.Fatalf("count UnitReturnedStarving events: %v", err)
	}
	if starvingEvents != 0 {
		t.Errorf("UnitReturnedStarving events for already-marching ship = %d, want 0", starvingEvents)
	}

	for _, k := range broadcaster.notified {
		if k == "UnitReturnedStarving" {
			t.Errorf("owner was notified with kind UnitReturnedStarving for a ship that was already marching — no second dispatch means no second notification")
		}
	}
}
