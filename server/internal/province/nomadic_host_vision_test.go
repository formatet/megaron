package province

import "testing"

// Timothy 2026-08-22: "synradie för alla landenheter är två". This reverses the
// 2026-07-15 short-sighted-host rule (base 1, "a people on the move, not a
// scout") — the host now reads ordinary ground at 2 like every other land eye.
// The two departures it always had are unchanged: a mountain is a landmark
// (+2), and a host standing AT the water gets the open horizon (4) — along open
// water only (Eye.Sees / SeaSightline, tested below and in sea_horizon_test.go).
func TestLiveRadius_NomadicHostReadsGroundLikeALandUnit(t *testing.T) {
	cases := []struct {
		name    string
		terrain string
		want    int
	}{
		{"ordinary ground — the uniform land radius", "plains", 2},
		{"hills are ordinary ground too", "hills", 2},
		{"sea off the horizon is read at the same vantage", "coastal_sea", 2},
		{"a mountain is a landmark: base 2 + 2", "mountain_limestone", 4},
		{"the red mountains read the same", "mountain_red", 4},
	}
	for _, c := range cases {
		if got := LiveRadius(EyeNomadicHost, c.terrain); got != c.want {
			t.Errorf("%s: LiveRadius(host, %q) = %d, want %d", c.name, c.terrain, got, c.want)
		}
	}
}

// The uniform rule stated as an invariant rather than as a table of numbers: on
// land, a host and an army have the same reach. If someone re-splits them, this
// fails regardless of which number they picked.
func TestLiveRadius_NomadicHostMatchesLandUnitOnLand(t *testing.T) {
	for _, terrain := range []string{"plains", "hills", "forest_olive_grove", "mountain_limestone"} {
		host := LiveRadius(EyeNomadicHost, terrain)
		army := LiveRadius(EyeLandUnit, terrain)
		if host != army {
			t.Errorf("%s: host sees %d, land unit %d — every eye on land reads the same ground", terrain, host, army)
		}
	}
}

// The host's eye is still WIRED even though it now agrees with the default: a
// host on the shore must get the open horizon over open water.
func TestAnyEyeSees_NomadicHostKeepsTheOpenHorizon(t *testing.T) {
	// Host on land at (0,0); everything with q >= 1 is open sea.
	eyes := []Eye{{Pos: MapPosition{Q: 0, R: 0}, Kind: EyeNomadicHost}}
	SetSeaHorizons(eyes, func(p MapPosition) string {
		if p.Q >= 1 {
			return "coastal_sea"
		}
		return "plains"
	})
	if !AnyEyeSees(eyes, MapPosition{Q: 4, R: 0}, "coastal_sea") {
		t.Fatal("host on the shore must see 4 out over open water — the open horizon belongs to whoever stands at the water")
	}
}
