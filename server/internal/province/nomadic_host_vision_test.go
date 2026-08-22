package province

import "testing"

// Timothy 2026-08-22: "synradie för alla landenheter är två". This reverses the
// 2026-07-15 short-sighted-host rule (base 1, "a people on the move, not a
// scout") — the host now reads ordinary ground at 2 like every other land eye.
// The two departures it always had are unchanged: a mountain is a landmark
// (+2), and a host standing AT the water gets the open horizon (4).
func TestLiveRadius_NomadicHostReadsGroundLikeALandUnit(t *testing.T) {
	cases := []struct {
		name    string
		terrain string
		atWater bool
		want    int
	}{
		{"ordinary ground — the uniform land radius", "plains", false, 2},
		{"hills are ordinary ground too", "hills", false, 2},
		{"open sea hides nothing — for a host ON the coast", "coastal_sea", true, 4},
		{"deep sea likewise", "deep_sea", true, 4},
		{"a mountain is a landmark: base 2 + 2", "mountain_limestone", false, 4},
		{"the red mountains read the same", "mountain_red", false, 4},
	}
	for _, c := range cases {
		if got := LiveRadius(EyeNomadicHost, c.atWater, c.terrain); got != c.want {
			t.Errorf("%s: LiveRadius(host, %v, %q) = %d, want %d", c.name, c.atWater, c.terrain, got, c.want)
		}
	}
}

// The uniform rule stated as an invariant rather than as a table of numbers: on
// land, a host and an army have the same reach. If someone re-splits them, this
// fails regardless of which number they picked.
func TestLiveRadius_NomadicHostMatchesLandUnitOnLand(t *testing.T) {
	for _, terrain := range []string{"plains", "hills", "forest_olive_grove", "mountain_limestone"} {
		host := LiveRadius(EyeNomadicHost, false, terrain)
		army := LiveRadius(EyeLandUnit, false, terrain)
		if host != army {
			t.Errorf("%s: host sees %d, land unit %d — every eye on land reads the same ground", terrain, host, army)
		}
	}
}

// The host's eye is still WIRED even though it now agrees with the default: an
// eye standing at the water must get the open horizon. An unknown kind gets the
// same base 2 on land, so land equality can no longer prove wiring — the sea
// branch can, because it is the branch a missing eye kind would still hit.
func TestLiveRadius_NomadicHostKeepsTheOpenHorizon(t *testing.T) {
	if got := LiveRadius(EyeNomadicHost, true, "coastal_sea"); got != 4 {
		t.Fatalf("host at the water reads sea at %d, want 4 — the open horizon belongs to whoever stands at the water", got)
	}
}
