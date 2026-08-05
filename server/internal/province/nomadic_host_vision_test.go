package province

import "testing"

// The host is short-sighted, not blind (Timothy 2026-07-15): only its BASE reach
// is halved. Mountains behave exactly as they do for every other eye, and a host
// ON THE COAST still reads open water — but from 2026-08-05 that requires it to
// actually stand at the water (atWater), like any other eye. See sea_horizon_test.go.
func TestLiveRadius_NomadicHostIsShortSightedOnLandOnly(t *testing.T) {
	cases := []struct {
		name    string
		terrain string
		atWater bool
		want    int
	}{
		{"ordinary ground — half a land unit's reach", "plains", false, 1},
		{"hills are ordinary ground too", "hills", false, 1},
		{"open sea hides nothing — for a host ON the coast", "coastal_sea", true, 4},
		{"deep sea likewise", "deep_sea", true, 4},
		{"a mountain is a landmark: base 1 + 2", "mountain_limestone", false, 3},
		{"the red mountains read the same", "mountain_red", false, 3},
	}
	for _, c := range cases {
		if got := LiveRadius(EyeNomadicHost, c.atWater, c.terrain); got != c.want {
			t.Errorf("%s: LiveRadius(host, %v, %q) = %d, want %d", c.name, c.atWater, c.terrain, got, c.want)
		}
	}
}

// The host must see less than an army over ordinary ground — that is the whole
// point of giving it its own eye rather than reusing EyeLandUnit.
func TestLiveRadius_NomadicHostSeesLessThanALandUnit(t *testing.T) {
	host := LiveRadius(EyeNomadicHost, false, "plains")
	army := LiveRadius(EyeLandUnit, false, "plains")
	if host >= army {
		t.Fatalf("host sees %d hexes of plains, land unit %d — the host must see LESS", host, army)
	}
}

// Guard against the host's eye silently falling back to the land-unit default:
// an unknown kind also yields 2, so equality here would hide a broken wiring.
func TestLiveRadius_NomadicHostIsNotTheUnknownDefault(t *testing.T) {
	if LiveRadius(EyeNomadicHost, false, "plains") == LiveRadius("some-unwired-kind", false, "plains") {
		t.Fatal("host radius equals the unknown-kind default — the eye is not wired")
	}
}
