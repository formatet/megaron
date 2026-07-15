package province

import "testing"

// The host is short-sighted, not blind (Timothy 2026-07-15): only its BASE reach
// is halved. Sea and mountains behave exactly as they do for every other eye, so
// a host on the coast still reads open water, and a mountain is still a landmark.
func TestLiveRadius_NomadicHostIsShortSightedOnLandOnly(t *testing.T) {
	cases := []struct {
		name    string
		terrain string
		want    int
	}{
		{"ordinary ground — half a land unit's reach", "plains", 1},
		{"hills are ordinary ground too", "hills", 1},
		{"open sea hides nothing, for anyone", "coastal_sea", 4},
		{"deep sea likewise", "deep_sea", 4},
		{"a mountain is a landmark: base 1 + 2", "mountain_limestone", 3},
		{"the red mountains read the same", "mountain_red", 3},
	}
	for _, c := range cases {
		if got := LiveRadius(EyeNomadicHost, c.terrain); got != c.want {
			t.Errorf("%s: LiveRadius(host, %q) = %d, want %d", c.name, c.terrain, got, c.want)
		}
	}
}

// The host must see less than an army over ordinary ground — that is the whole
// point of giving it its own eye rather than reusing EyeLandUnit.
func TestLiveRadius_NomadicHostSeesLessThanALandUnit(t *testing.T) {
	host := LiveRadius(EyeNomadicHost, "plains")
	army := LiveRadius(EyeLandUnit, "plains")
	if host >= army {
		t.Fatalf("host sees %d hexes of plains, land unit %d — the host must see LESS", host, army)
	}
}

// Guard against the host's eye silently falling back to the land-unit default:
// an unknown kind also yields 2, so equality here would hide a broken wiring.
func TestLiveRadius_NomadicHostIsNotTheUnknownDefault(t *testing.T) {
	if LiveRadius(EyeNomadicHost, "plains") == LiveRadius("some-unwired-kind", "plains") {
		t.Fatal("host radius equals the unknown-kind default — the eye is not wired")
	}
}
