package messenger

// Pure-math regression tests for the moving-target interception fix
// (temenos_orderlopare_plan.md, 2026-07-30): a recall/redirect Runner used to
// be aimed at the marching unit's position AT THE DISPATCH INSTANT (a
// snapshot). That snapshot is always stale — the runner takes real time to
// travel even to that point, and the unit keeps marching while it does — so
// whenever the courier's ETA to the snapshot exceeded the unit's own
// remaining march time, the runner arrived (ScheduledOrderDelivery fired)
// AFTER the unit had already completed its march and stopped "marching".
// combat.ExecuteRecall only checks unit.status == "marching" at delivery —
// never the runner's aim point — so that case silently no-opped (just a
// slog.Info, no OrderFailed) even though the API had already promised the
// Wanax a 202 "order_dispatched".
//
// These tests exercise InterceptAlongPath directly (no DB — a hand-built
// TileGraph + straight-line path), proving:
//  1. a scenario where the naive dispatch-time snapshot is provably
//     undeliverable (its own courier ETA exceeds the unit's remaining march
//     time) but a LATER hex on the same path is still catchable — the fix
//     must find it;
//  2. a scenario where no hex on the remaining path is catchable at all —
//     the fix must report ok=false so the caller can fail visibly, never
//     silently queue a doomed courier.

import (
	"testing"
	"time"

	"formatet/megaron/server/internal/province"
)

// straightLineGrid returns a fully-connected plains TileGraph covering
// q in [0,maxQ], r in [0,maxR] — enough for the courier's own A* to route
// between the march's line (r=0) and an origin placed off to the side.
func straightLineGrid(maxQ, maxR int) province.TileGraph {
	g := make(province.TileGraph)
	for q := 0; q <= maxQ; q++ {
		for r := 0; r <= maxR; r++ {
			g[[2]int{q, r}] = "plains"
		}
	}
	return g
}

// straightLinePath returns the n hexes (0,0)..(n-1,0).
func straightLinePath(n int) []province.MapPosition {
	path := make([]province.MapPosition, n)
	for i := 0; i < n; i++ {
		path[i] = province.MapPosition{Q: i, R: 0}
	}
	return path
}

// RÖ1: the eight-tick plains march has two ticks left at 75% progress.
// The snapshot costs the runner three ticks; the destination costs two.
func TestInterceptAlongPath_FindsCatchableHexPastNaiveSnapshot(t *testing.T) {
	withTickSeconds(t, 3600) // 1 tick = 1 real hour, matching the hand-worked math above
	g := straightLineGrid(8, 5)
	path := straightLinePath(9) // eight plains steps

	now := time.Now()
	departsAt := now.Add(-6 * time.Hour) // six ticks elapsed of an eight-tick march
	arrivesAt := departsAt.Add(8 * time.Hour)
	courierOrigin := province.MapPosition{Q: 8, R: 3}

	// Sanity-check the fixture's own premise: the naive snapshot really is
	// undeliverable, so this test would be vacuous if it weren't.
	snapshot := province.MapPosition{Q: 6, R: 0}
	_, naiveDur, naiveOK := CourierTravelOnGraph(g, courierOrigin, snapshot)
	if !naiveOK {
		t.Fatal("test fixture broken: no route to the naive snapshot at all (grid is all plains)")
	}
	remaining := arrivesAt.Sub(now)
	if naiveDur <= remaining {
		t.Fatalf("test fixture broken: naive snapshot ETA %v should EXCEED the unit's remaining march time %v "+
			"(otherwise this scenario doesn't reproduce the bug)", naiveDur, remaining)
	}

	target, ok := InterceptAlongPath(g, courierOrigin, path, departsAt, arrivesAt, now)
	if !ok {
		t.Fatalf("InterceptAlongPath = ok=false, want an intercept at (8,0)")
	}
	if want := (province.MapPosition{Q: 8, R: 0}); target != want {
		t.Errorf("InterceptAlongPath = %+v, want %+v — the march's own destination is the only hex "+
			"a courier from (8,3) can reach before the unit does", target, want)
	}
}

// TestInterceptAlongPath_NoInterceptPossible_ReportsFalse: courierOrigin
// (6,5) sits equidistant (5 hexes) from every remaining hex on the path,
// including the destination — 1.875h ≈ 2 ticks everywhere, always more than
// the 1.5h left on the march. No point on the path is catchable. Callers
// (api/handlers/unit.go's Recall) must treat this as a genuine, visible
// dispatch failure, never a silently-doomed courier.
func TestInterceptAlongPath_NoInterceptPossible_ReportsFalse(t *testing.T) {
	withTickSeconds(t, 3600)
	g := straightLineGrid(8, 5)
	path := straightLinePath(9)

	now := time.Now()
	departsAt := now.Add(-4*time.Hour - 30*time.Minute)
	arrivesAt := departsAt.Add(6 * time.Hour)
	courierOrigin := province.MapPosition{Q: 6, R: 5}

	if target, ok := InterceptAlongPath(g, courierOrigin, path, departsAt, arrivesAt, now); ok {
		t.Fatalf("InterceptAlongPath = %+v, ok=true — want ok=false: courierOrigin (6,5) is too far "+
			"from every remaining hex for any courier to catch this unit before it arrives", target)
	}
}

// TestInterceptAlongPath_AlreadyArrived_ReportsFalse: now is past arrivesAt —
// there is nothing left to intercept (the unit has already stopped
// marching). Defensive: Recall's own status==marching check should already
// prevent dispatch in this state, but the intercept search must not pretend
// a stale unit is still catchable.
func TestInterceptAlongPath_AlreadyArrived_ReportsFalse(t *testing.T) {
	withTickSeconds(t, 3600)
	g := straightLineGrid(8, 5)
	path := straightLinePath(9)

	departsAt := time.Now().Add(-7 * time.Hour)
	arrivesAt := departsAt.Add(6 * time.Hour) // arrived 1h ago
	now := departsAt.Add(6*time.Hour + time.Hour)

	if target, ok := InterceptAlongPath(g, province.MapPosition{Q: 0, R: 3}, path, departsAt, arrivesAt, now); ok {
		t.Fatalf("InterceptAlongPath = %+v, ok=true — want ok=false: the unit already arrived", target)
	}
}
