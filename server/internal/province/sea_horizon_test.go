package province

import "testing"

// The open horizon over the sea is a SIGHTLINE rule (Timothy 2026-09-25): an eye
// sees a sea hex up to SeaHorizonRadius away only if every hex strictly between
// them on the straight hex line is open sea. The eye's own hex may be land. Before
// this, any eye "at the water" saw a full radius-4 disk of sea — so a spearman on
// the shore at (28,18) of the acceptance world saw an enclosed lake at (24,18)
// across forest, hills and a mountain ridge. And before 2026-08-05, every eye did
// (Timothy 2026-08-04: "där har vi ett designfel idag").

// terrainMap builds a terrainAt func from a default terrain plus overrides.
func terrainMap(def string, over map[MapPosition]string) func(MapPosition) string {
	return func(p MapPosition) string {
		if t, ok := over[p]; ok {
			return t
		}
		return def
	}
}

func classified(terrainAt func(MapPosition) string, eyes ...Eye) []Eye {
	SetSeaHorizons(eyes, terrainAt)
	return eyes
}

// TestSeaHorizon_EnclosedLakeBehindTheShoreIsNotSeen is the reported case from the
// acceptance world: a spearman at (28,18) with open sea to his east; land at
// (27,18), (26,18), (25,18); a small enclosed lake at (24,18)/(24,19). The lake is
// within 4 but the line to it crosses land, so the horizon does not reach it and
// it falls back to the land-unit radius (2) — out of sight.
func TestSeaHorizon_EnclosedLakeBehindTheShoreIsNotSeen(t *testing.T) {
	over := map[MapPosition]string{
		{Q: 24, R: 18}: "coastal_sea",
		{Q: 24, R: 19}: "coastal_sea",
		{Q: 27, R: 18}: "forest_olive_grove",
		{Q: 26, R: 18}: "hills",
		{Q: 25, R: 18}: "mountain_limestone",
	}
	terrain := func(p MapPosition) string {
		if t, ok := over[p]; ok {
			return t
		}
		if p.Q >= 29 {
			return "deep_sea" // the open sea east of the shore
		}
		return "plains"
	}
	eyes := classified(terrain, Eye{Pos: MapPosition{Q: 28, R: 18}, Kind: EyeLandUnit})

	for _, lake := range []MapPosition{{Q: 24, R: 18}, {Q: 24, R: 19}} {
		if AnyEyeSees(eyes, lake, "coastal_sea") {
			t.Errorf("spearman at (28,18) sees the enclosed lake at %v across land — the horizon must follow open water", lake)
		}
	}
	if !AnyEyeSees(eyes, MapPosition{Q: 32, R: 18}, "deep_sea") {
		t.Error("the same spearman must still see 4 out over the open sea to his east")
	}
}

// TestSeaHorizon_OpenCoastSeesFourOverOpenWater: a coastal settlement and a unit on
// the shore both see 4 out over unbroken water, in several directions.
func TestSeaHorizon_OpenCoastSeesFourOverOpenWater(t *testing.T) {
	// Land for q <= 0, open sea for q >= 1.
	terrain := func(p MapPosition) string {
		if p.Q >= 1 {
			return "coastal_sea"
		}
		return "plains"
	}
	for _, kind := range []string{EyeSettlement, EyeLandUnit, EyeNomadicHost} {
		eyes := classified(terrain, Eye{Pos: MapPosition{Q: 0, R: 0}, Kind: kind})
		for _, target := range []MapPosition{{Q: 4, R: 0}, {Q: 4, R: -4}, {Q: 2, R: 2}, {Q: 3, R: -1}} {
			if !AnyEyeSees(eyes, target, "coastal_sea") {
				t.Errorf("%s on the coast must see open sea at %v (distance %d)", kind, target, HexDistance(eyes[0].Pos, target))
			}
		}
		if AnyEyeSees(eyes, MapPosition{Q: 5, R: 0}, "coastal_sea") {
			t.Errorf("%s: the horizon is 4, not 5", kind)
		}
	}
}

// TestSeaHorizon_OneLandHexOnTheLineBlocks: a ship on open water with a single
// island hex between it and the target. Beyond the island is out of the horizon
// (and beyond the ship's own vantage of 1); every other direction is open.
func TestSeaHorizon_OneLandHexOnTheLineBlocks(t *testing.T) {
	terrain := terrainMap("deep_sea", map[MapPosition]string{{Q: 2, R: 0}: "plains"})
	eyes := classified(terrain, Eye{Pos: MapPosition{Q: 0, R: 0}, Kind: EyeShip})

	for _, behind := range []MapPosition{{Q: 3, R: 0}, {Q: 4, R: 0}} {
		if AnyEyeSees(eyes, behind, "deep_sea") {
			t.Errorf("ship sees %v behind the island at (2,0) — one land hex on the line must block", behind)
		}
	}
	if !AnyEyeSees(eyes, MapPosition{Q: 1, R: 0}, "deep_sea") {
		t.Error("the sea hex in front of the island (distance 1) is seen")
	}
	if !AnyEyeSees(eyes, MapPosition{Q: 0, R: 4}, "deep_sea") {
		t.Error("an eye standing on the sea (a ship) must see 4 along open water")
	}
}

// TestSeaHorizon_InlandEyeHasNoHorizon: an eye with no sea neighbour reads the sea
// at its ordinary vantage — its first step off its own hex is land.
func TestSeaHorizon_InlandEyeHasNoHorizon(t *testing.T) {
	terrain := func(p MapPosition) string {
		if p.Q >= 2 {
			return "coastal_sea"
		}
		return "plains"
	}
	eyes := classified(terrain, Eye{Pos: MapPosition{Q: 0, R: 0}, Kind: EyeLandUnit})
	if !AnyEyeSees(eyes, MapPosition{Q: 2, R: 0}, "coastal_sea") {
		t.Error("sea at distance 2 is within the land unit's ordinary vantage")
	}
	if AnyEyeSees(eyes, MapPosition{Q: 3, R: 0}, "coastal_sea") {
		t.Error("an inland army must not see sea beyond its vantage")
	}
}

// TestSeaHorizon_RiverIsNotOpenWater: a river between the eye and the sea blocks
// the line exactly like land (megaron_floden_plan.md §5).
func TestSeaHorizon_RiverIsNotOpenWater(t *testing.T) {
	terrain := func(p MapPosition) string {
		switch {
		case p.Q == 1:
			return "river"
		case p.Q >= 2:
			return "coastal_sea"
		}
		return "plains"
	}
	eyes := classified(terrain, Eye{Pos: MapPosition{Q: 0, R: 0}, Kind: EyeSettlement})
	if AnyEyeSees(eyes, MapPosition{Q: 4, R: 0}, "coastal_sea") {
		t.Error("a river neighbour must not grant the open horizon")
	}
}

// TestSeaHorizon_UnclassifiedEyeFailsClosed: an Eye nobody ran SetSeaHorizons on,
// or one whose lookup found nothing, sees only its ordinary vantage.
func TestSeaHorizon_UnclassifiedEyeFailsClosed(t *testing.T) {
	far := MapPosition{Q: 4, R: 0}
	if AnyEyeSees([]Eye{{Pos: MapPosition{}, Kind: EyeShip}}, far, "deep_sea") {
		t.Error("hand-built ship without a horizon must not see 4 (fail closed)")
	}
	unknown := classified(func(MapPosition) string { return "" }, Eye{Pos: MapPosition{}, Kind: EyeShip})
	if AnyEyeSees(unknown, far, "deep_sea") {
		t.Error("an empty terrain lookup must not grant any horizon (fail closed)")
	}
}

// --- the line itself -------------------------------------------------------

// TestHexLine_ContiguousAndEndpoints: every drawn line starts at a, ends at b, has
// N+1 hexes and steps one neighbour at a time — for every target in a radius-4
// disk, on both tie-break sides.
func TestHexLine_ContiguousAndEndpoints(t *testing.T) {
	a := MapPosition{Q: 3, R: -2}
	for q := -4; q <= 4; q++ {
		for r := -4; r <= 4; r++ {
			b := MapPosition{Q: a.Q + q, R: a.R + r}
			n := HexDistance(a, b)
			if n > 4 {
				continue
			}
			for _, side := range []float64{1, -1} {
				line := hexLine(a, b, side)
				if len(line) != n+1 || line[0] != a || line[n] != b {
					t.Fatalf("hexLine(%v,%v,%v) = %v: want %d hexes from a to b", a, b, side, line, n+1)
				}
				for i := 1; i < len(line); i++ {
					if HexDistance(line[i-1], line[i]) != 1 {
						t.Fatalf("hexLine(%v,%v,%v) = %v: step %d is not to a neighbour", a, b, side, line, i)
					}
				}
			}
		}
	}
}

// TestSeaSightline_EdgeTieIsOpenIfEitherSideIsSea pins the tie rule. The line
// (0,0)→(1,1) runs exactly along the edge between (1,0) and (0,1); the two nudges
// must resolve it to different sides, and the sightline holds if either is sea.
func TestSeaSightline_EdgeTieIsOpenIfEitherSideIsSea(t *testing.T) {
	a, b := MapPosition{Q: 0, R: 0}, MapPosition{Q: 1, R: 1}
	p, m := hexLine(a, b, 1)[1], hexLine(a, b, -1)[1]
	sides := map[MapPosition]bool{{Q: 1, R: 0}: true, {Q: 0, R: 1}: true}
	if p == m || !sides[p] || !sides[m] {
		t.Fatalf("the two nudges must pick the two edge hexes (1,0) and (0,1); got %v and %v", p, m)
	}

	seaAt := func(sea ...MapPosition) func(MapPosition) bool {
		set := map[MapPosition]bool{b: true}
		for _, s := range sea {
			set[s] = true
		}
		return func(x MapPosition) bool { return set[x] }
	}
	if !SeaSightline(a, b, seaAt(MapPosition{Q: 1, R: 0})) || !SeaSightline(a, b, seaAt(MapPosition{Q: 0, R: 1})) {
		t.Error("a line skimming the edge of a sea hex must hold, whichever side the sea is on")
	}
	if SeaSightline(a, b, seaAt()) {
		t.Error("with land on both sides of the edge the line is blocked")
	}
}

// TestSeaSightline_Symmetric: an eye sees a sea hex exactly when that hex sees it —
// from→to equals to→from for every sea pair in a disk, on a mixed map.
func TestSeaSightline_Symmetric(t *testing.T) {
	isSea := func(p MapPosition) bool { return ((p.Q*7+p.R*13)%5+5)%5 != 0 }
	checked, open := 0, 0
	for oq := -2; oq <= 2; oq++ {
		for or := -2; or <= 2; or++ {
			from := MapPosition{Q: oq, R: or}
			for q := -4; q <= 4; q++ {
				for r := -4; r <= 4; r++ {
					to := MapPosition{Q: oq + q, R: or + r}
					if HexDistance(from, to) > 4 || !isSea(from) || !isSea(to) {
						continue
					}
					checked++
					fwd := SeaSightline(from, to, isSea)
					if fwd {
						open++
					}
					if fwd != SeaSightline(to, from, isSea) {
						t.Errorf("sightline %v↔%v is not symmetric", from, to)
					}
				}
			}
		}
	}
	if open == 0 || open == checked {
		t.Fatalf("degenerate fixture: %d of %d lines open — the test must exercise both outcomes", open, checked)
	}
}
