package combat

import (
	"encoding/json"
	"testing"

	"formatet/megaron/server/internal/province"
	"formatet/megaron/server/internal/unit"
)

// TestBuildRoute_OkAndShape: a real three-hex path with its step hours builds
// a StoredRoute whose Move() reproduces the same costs (R2's thousandths-of-
// an-hour scale, rounded, floored at 1).
func TestBuildRoute_OkAndShape(t *testing.T) {
	path := []province.MapPosition{{Q: 0, R: 0}, {Q: 1, R: 0}, {Q: 2, R: 0}}
	steps := []float64{2.5, 0.75} // ford, plains — same numbers as movement's T1
	r, ok := BuildRoute(path, steps, 0, 10)
	if !ok {
		t.Fatalf("BuildRoute rejected a valid path")
	}
	if r.StartTick != 0 || r.EndTick != 10 {
		t.Fatalf("StartTick/EndTick = %d/%d, want 0/10", r.StartTick, r.EndTick)
	}
	wantHexes := [][2]int{{0, 0}, {1, 0}, {2, 0}}
	for i, h := range wantHexes {
		if r.Hexes[i] != h {
			t.Errorf("Hexes[%d] = %v, want %v", i, r.Hexes[i], h)
		}
	}
	wantCosts := []int64{2500, 750}
	for i, c := range wantCosts {
		if r.Costs[i] != c {
			t.Errorf("Costs[%d] = %d, want %d", i, r.Costs[i], c)
		}
	}
	if err := r.Move().Validate(); err != nil {
		t.Errorf("Move() from a BuildRoute result failed Validate: %v", err)
	}
}

// TestBuildRoute_TooShort: colonize-in-place (origin==target) and any other
// sub-two-hex path save nothing — callers persist NULL.
func TestBuildRoute_TooShort(t *testing.T) {
	if _, ok := BuildRoute([]province.MapPosition{{Q: 0, R: 0}}, nil, 0, 1); ok {
		t.Error("BuildRoute should reject a single-hex path (colonize-in-place)")
	}
	if _, ok := BuildRoute(nil, nil, 0, 1); ok {
		t.Error("BuildRoute should reject an empty path")
	}
}

// TestBuildRoute_MismatchedSteps: a caller bug (steps not matching path) must
// not silently save a corrupt route.
func TestBuildRoute_MismatchedSteps(t *testing.T) {
	path := []province.MapPosition{{Q: 0, R: 0}, {Q: 1, R: 0}, {Q: 2, R: 0}}
	if _, ok := BuildRoute(path, []float64{1.0}, 0, 10); ok {
		t.Error("BuildRoute should reject len(stepHours) != len(path)-1")
	}
}

// TestBuildRoute_ZeroCostFloorsAtOne: movement.Move forbids a zero cost
// (Validate rejects it), so a genuinely free step (e.g. a terrain bug) must
// floor to 1, not slip through as 0 and fail Validate downstream.
func TestBuildRoute_ZeroCostFloorsAtOne(t *testing.T) {
	path := []province.MapPosition{{Q: 0, R: 0}, {Q: 1, R: 0}}
	r, ok := BuildRoute(path, []float64{0}, 0, 1)
	if !ok {
		t.Fatalf("BuildRoute rejected a zero-cost step instead of flooring it")
	}
	if r.Costs[0] != 1 {
		t.Errorf("Costs[0] = %d, want 1 (floored)", r.Costs[0])
	}
}

// validRouteRaw is a small helper StoredRoute JSON matching depart/arrive 0/10.
func validRouteRaw(t *testing.T) []byte {
	t.Helper()
	r := StoredRoute{
		StartTick: 0, EndTick: 10,
		Hexes: [][2]int{{0, 0}, {1, 0}, {2, 0}},
		Costs: []int64{2500, 750},
	}
	raw, err := json.Marshal(r)
	if err != nil {
		t.Fatalf("marshal fixture route: %v", err)
	}
	return raw
}

// TestLoadActiveRoute_Matches: status marching + matching tick pair loads ok.
func TestLoadActiveRoute_Matches(t *testing.T) {
	depart, arrive := 0, 10
	r, ok := LoadActiveRoute(validRouteRaw(t), string(unit.StatusMarching), &depart, &arrive)
	if !ok {
		t.Fatal("LoadActiveRoute rejected a matching, well-formed route")
	}
	if r.StartTick != 0 || r.EndTick != 10 {
		t.Errorf("loaded route StartTick/EndTick = %d/%d, want 0/10", r.StartTick, r.EndTick)
	}
}

// TestLoadActiveRoute_Mismatches covers every ok=false path: invariant 2 says
// none of these are bugs — they just mean "use the old code".
func TestLoadActiveRoute_Mismatches(t *testing.T) {
	depart, arrive := 0, 10
	otherDepart, otherArrive := 0, 20

	cases := []struct {
		name       string
		raw        []byte
		status     string
		departTick *int
		arriveTick *int
	}{
		{"not marching", validRouteRaw(t), string(unit.StatusGarrison), &depart, &arrive},
		{"nil raw", nil, string(unit.StatusMarching), &depart, &arrive},
		{"empty raw", []byte{}, string(unit.StatusMarching), &depart, &arrive},
		{"nil depart tick", validRouteRaw(t), string(unit.StatusMarching), nil, &arrive},
		{"nil arrive tick", validRouteRaw(t), string(unit.StatusMarching), &depart, nil},
		{"stale route from an earlier march", validRouteRaw(t), string(unit.StatusMarching), &otherDepart, &otherArrive},
		{"unparseable JSON", []byte("not json"), string(unit.StatusMarching), &depart, &arrive},
		{"structurally invalid route", func() []byte {
			b, _ := json.Marshal(StoredRoute{StartTick: 0, EndTick: 10, Hexes: [][2]int{{0, 0}}, Costs: nil})
			return b
		}(), string(unit.StatusMarching), &depart, &arrive},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if _, ok := LoadActiveRoute(c.raw, c.status, c.departTick, c.arriveTick); ok {
				t.Errorf("LoadActiveRoute should have rejected: %s", c.name)
			}
		})
	}
}

// TestRoutePositionAt_MatchesMovement: the rounding-trap scenario from
// movement_test.go's T7, wrapped in a StoredRoute — proves combat's wrapper
// does not change movement's own answer.
func TestRoutePositionAt_MatchesMovement(t *testing.T) {
	r := StoredRoute{
		StartTick: 0, EndTick: 10,
		Hexes: [][2]int{{0, 0}, {1, 0}, {2, 0}},
		Costs: []int64{9000, 1000}, // Hexes[1]'s entry cost dominates
	}
	pos, err := RoutePositionAt(r, 8000) // 80% of the journey
	if err != nil {
		t.Fatalf("RoutePositionAt: %v", err)
	}
	want := province.MapPosition{Q: 1, R: 0}
	if pos != want {
		t.Errorf("RoutePositionAt(80%%) = %+v, want %+v (T7: the dominant-cost hex)", pos, want)
	}
}

// TestRouteEnterMilli: Hexes[0] enters at StartTick*1000 (val A — instant
// departure — which is also, always, exactly b_0: Boundaries' own doc says
// bounds[0] IS StartTick*1000). Hexes[1] (the ford, T1's fixture) therefore
// ALSO enters at b_0 — the unit leaves the start hex and enters the ford in
// the same instant, since the start hex is never actually dwelt in. Hexes[2]
// (the target) enters at b_1 = 7692, matching movement's own T1/Presence
// result ([0,7692) for the ford, [7692,10000) for the target).
func TestRouteEnterMilli(t *testing.T) {
	r := StoredRoute{
		StartTick: 0, EndTick: 10,
		Hexes: [][2]int{{0, 0}, {1, 0}, {2, 0}},
		Costs: []int64{2500, 750}, // same fixture as movement's T1: bounds = [0, 7692, 10000]
	}
	enter, err := RouteEnterMilli(r)
	if err != nil {
		t.Fatalf("RouteEnterMilli: %v", err)
	}
	if len(enter) != 3 {
		t.Fatalf("len(enter) = %d, want 3", len(enter))
	}
	if enter[0] != 0 {
		t.Errorf("enter[0] = %d, want 0 (StartTick*1000, instant departure)", enter[0])
	}
	if enter[1] != 0 {
		t.Errorf("enter[1] = %d, want 0 (b_0 — entered the instant the start hex is left)", enter[1])
	}
	if enter[2] != 7692 {
		t.Errorf("enter[2] = %d, want 7692 (b_1)", enter[2])
	}
}
