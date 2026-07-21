package handlers

import (
	"testing"

	"github.com/google/uuid"
)

// A city whose ring has a mountain (specific mine match) and several plains
// hexes (mine also matches everywhere via its generic stone rule). The mine
// must settle on the mountain, not on a generic plains hex.
func TestPlaceRural_MinePrefersSpecificHex(t *testing.T) {
	sid := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	pid := uuid.MustParse("22222222-2222-2222-2222-222222222222")
	cities := map[uuid.UUID]*ruralCity{
		sid: {provinceID: pid, name: "Pylos", cands: map[string][]ruralCandidate{
			"mine": {
				{q: 5, r: 0, specific: false}, // plains — stone rule only
				{q: 4, r: 1, specific: false}, // plains
				{q: 5, r: -1, specific: true}, // mountain — tin/silver deposit rule
			},
		}},
	}
	out := placeRural(cities)
	if len(out) != 1 {
		t.Fatalf("want 1 projection, got %d", len(out))
	}
	if out[0].Q != 5 || out[0].R != -1 {
		t.Fatalf("mine settled at (%d,%d), want the specific mountain hex (5,-1)", out[0].Q, out[0].R)
	}
}

// Two building types whose only compatible hex is the same one must not both
// land there — the second yields (rule 5: shown only in the city drawer).
func TestPlaceRural_NoHexCollision(t *testing.T) {
	sid := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	cities := map[uuid.UUID]*ruralCity{
		sid: {name: "Mykene", cands: map[string][]ruralCandidate{
			"farm": {{q: 1, r: 0, specific: true}},
			"mine": {{q: 1, r: 0, specific: false}}, // same hex, generic
		}},
	}
	out := placeRural(cities)
	if len(out) != 1 {
		t.Fatalf("want 1 projection (collision drops the second), got %d", len(out))
	}
	// farm is placed first (fixed order) and claims the hex.
	if out[0].BuildingType != "farm" {
		t.Fatalf("expected farm to claim the shared hex, got %q", out[0].BuildingType)
	}
}

// Placement must be a pure function of its inputs — same candidates in, same
// hexes out, every time (no wandering across requests).
func TestPlaceRural_Deterministic(t *testing.T) {
	sid := uuid.MustParse("33333333-3333-3333-3333-333333333333")
	mk := func() map[uuid.UUID]*ruralCity {
		return map[uuid.UUID]*ruralCity{
			sid: {name: "Tiryns", cands: map[string][]ruralCandidate{
				"farm":       {{q: 0, r: 1, specific: true}, {q: 1, r: 0, specific: true}},
				"lumbermill": {{q: -1, r: 1, specific: true}, {q: 0, r: -1, specific: true}},
			}},
		}
	}
	first := placeRural(mk())
	for i := 0; i < 20; i++ {
		got := placeRural(mk())
		if len(got) != len(first) {
			t.Fatalf("run %d: length drift %d vs %d", i, len(got), len(first))
		}
		for j := range got {
			if got[j] != first[j] {
				t.Fatalf("run %d: projection %d drift %+v vs %+v", i, j, got[j], first[j])
			}
		}
	}
}

// A building type with no compatible hex is omitted (it stays in the city
// drawer), while its siblings still project.
func TestPlaceRural_OmitsWhenNoCompatibleHex(t *testing.T) {
	sid := uuid.MustParse("44444444-4444-4444-4444-444444444444")
	cities := map[uuid.UUID]*ruralCity{
		sid: {name: "Thebes", cands: map[string][]ruralCandidate{
			"farm": {{q: 2, r: 2, specific: true}},
			// lumbermill has no candidate hexes at all — no forest in the ring.
		}},
	}
	out := placeRural(cities)
	if len(out) != 1 || out[0].BuildingType != "farm" {
		t.Fatalf("want only the farm projection, got %+v", out)
	}
}
