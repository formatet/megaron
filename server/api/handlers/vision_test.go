package handlers

// interpolatedEyePos tests — live fog-of-war vision must track a marching unit's
// current position along its route, not just its departure hex (loadLiveEyes calls
// this to place a marching unit's eye; see world.go). Pure/DB-free: fixed time.Time
// values stand in for the clock.Clock the real caller threads through.

import (
	"testing"
	"time"

	"github.com/poleia/server/internal/province"
)

func testPath() []province.MapPosition {
	return []province.MapPosition{
		{Q: 0, R: 0},
		{Q: 1, R: 0},
		{Q: 2, R: 0},
		{Q: 3, R: 0},
		{Q: 4, R: 0},
	}
}

// TestInterpolatedEyePos_AtDeparture verifies 0% elapsed places the eye at the
// first path hex (the origin), matching pre-fix behaviour exactly at departure.
func TestInterpolatedEyePos_AtDeparture(t *testing.T) {
	departs := time.Date(2026, 7, 9, 12, 0, 0, 0, time.UTC)
	arrives := departs.Add(4 * time.Hour)
	path := testPath()

	got := interpolatedEyePos(departs, departs, arrives, path)
	want := path[0]
	if got != want {
		t.Errorf("interpolatedEyePos at departure = %+v, want %+v", got, want)
	}
}

// TestInterpolatedEyePos_Midpoint verifies 50% elapsed places the eye at the
// middle path hex — this is the exact bug: the ship's fog bubble should have
// moved to the route's midpoint, not still sit at the harbour.
func TestInterpolatedEyePos_Midpoint(t *testing.T) {
	departs := time.Date(2026, 7, 9, 12, 0, 0, 0, time.UTC)
	arrives := departs.Add(4 * time.Hour)
	now := departs.Add(2 * time.Hour) // 50% elapsed
	path := testPath()

	got := interpolatedEyePos(now, departs, arrives, path)
	want := path[2] // middle of 5 hexes
	if got != want {
		t.Errorf("interpolatedEyePos at 50%% = %+v, want %+v (midpoint)", got, want)
	}
}

// TestInterpolatedEyePos_AtArrival verifies 100% elapsed places the eye at the
// last path hex (the target).
func TestInterpolatedEyePos_AtArrival(t *testing.T) {
	departs := time.Date(2026, 7, 9, 12, 0, 0, 0, time.UTC)
	arrives := departs.Add(4 * time.Hour)
	path := testPath()

	got := interpolatedEyePos(arrives, departs, arrives, path)
	want := path[len(path)-1]
	if got != want {
		t.Errorf("interpolatedEyePos at arrival = %+v, want %+v", got, want)
	}
}

// TestInterpolatedEyePos_ClampsBeforeDeparture verifies a read before departs_at
// (clock skew or a stale row) clamps to the first hex instead of extrapolating
// backwards off the path.
func TestInterpolatedEyePos_ClampsBeforeDeparture(t *testing.T) {
	departs := time.Date(2026, 7, 9, 12, 0, 0, 0, time.UTC)
	arrives := departs.Add(4 * time.Hour)
	now := departs.Add(-1 * time.Hour)
	path := testPath()

	got := interpolatedEyePos(now, departs, arrives, path)
	want := path[0]
	if got != want {
		t.Errorf("interpolatedEyePos before departure = %+v, want %+v (clamped)", got, want)
	}
}

// TestInterpolatedEyePos_ClampsAfterArrival verifies a read past arrives_at (e.g.
// the arrival event hasn't processed yet) clamps to the last hex.
func TestInterpolatedEyePos_ClampsAfterArrival(t *testing.T) {
	departs := time.Date(2026, 7, 9, 12, 0, 0, 0, time.UTC)
	arrives := departs.Add(4 * time.Hour)
	now := arrives.Add(1 * time.Hour)
	path := testPath()

	got := interpolatedEyePos(now, departs, arrives, path)
	want := path[len(path)-1]
	if got != want {
		t.Errorf("interpolatedEyePos after arrival = %+v, want %+v (clamped)", got, want)
	}
}

// TestInterpolatedEyePos_EmptyPathFallsBackSanely verifies an empty path (FindPath
// failed or returned nothing) never panics — it returns the zero MapPosition.
// Callers (loadLiveEyes) must check len(path) > 0 before calling and fall back to
// the unit's stored (q,r) themselves; this only guards the pure function itself.
func TestInterpolatedEyePos_EmptyPathFallsBackSanely(t *testing.T) {
	departs := time.Date(2026, 7, 9, 12, 0, 0, 0, time.UTC)
	arrives := departs.Add(4 * time.Hour)
	now := departs.Add(2 * time.Hour)

	got := interpolatedEyePos(now, departs, arrives, nil)
	want := province.MapPosition{}
	if got != want {
		t.Errorf("interpolatedEyePos with empty path = %+v, want zero value %+v", got, want)
	}
}

// TestInterpolatedEyePos_ZeroDurationMarch verifies a march with departs_at ==
// arrives_at (degenerate/instant, shouldn't happen but must not divide by zero)
// clamps to progress 0 and returns the first hex rather than NaN-ing the index.
func TestInterpolatedEyePos_ZeroDurationMarch(t *testing.T) {
	at := time.Date(2026, 7, 9, 12, 0, 0, 0, time.UTC)
	path := testPath()

	got := interpolatedEyePos(at, at, at, path)
	want := path[0]
	if got != want {
		t.Errorf("interpolatedEyePos zero-duration march = %+v, want %+v", got, want)
	}
}
