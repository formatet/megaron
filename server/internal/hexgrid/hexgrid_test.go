package hexgrid

import "testing"

func coordSet(cs []Coord) map[Coord]bool {
	m := make(map[Coord]bool, len(cs))
	for _, c := range cs {
		m[c] = true
	}
	return m
}

// TestNeighbors_MatchesEveryDuplicatedSiteExactly pins the SAME six offsets
// every one of the 13 pre-consolidation call sites hardcoded — regression
// insurance for the migration itself, independent of Disk's algorithm.
func TestNeighbors_MatchesEveryDuplicatedSiteExactly(t *testing.T) {
	want := []Coord{{1, 0}, {-1, 0}, {0, 1}, {0, -1}, {1, -1}, {-1, 1}}
	got := Neighbors(Coord{5, -3})
	for i, w := range want {
		wantAbs := Coord{5 + w.Q, -3 + w.R}
		if got[i] != wantAbs {
			t.Errorf("Neighbors[%d] = %v, want %v", i, got[i], wantAbs)
		}
	}
}

// TestDisk_Radius0IsJustCenter.
func TestDisk_Radius0IsJustCenter(t *testing.T) {
	got := Disk(Coord{2, 3}, 0)
	if len(got) != 1 || got[0] != (Coord{2, 3}) {
		t.Errorf("Disk(center, 0) = %v, want [{2 3}]", got)
	}
}

// TestDisk_Radius1MatchesPreP1Catchment: today's catchment (own hex + 6
// neighbors, 7 hexes) must be EXACTLY Disk(center, 1) — this is the
// behavior-preservation contract every migrated call site depends on before
// the radius is flipped to 2.
func TestDisk_Radius1MatchesPreP1Catchment(t *testing.T) {
	center := Coord{10, -4}
	got := Disk(center, 1)
	if len(got) != 7 {
		t.Fatalf("Disk(center, 1) has %d hexes, want 7", len(got))
	}
	gotSet := coordSet(got)
	if !gotSet[center] {
		t.Error("Disk(center, 1) does not include the center hex")
	}
	for _, n := range Neighbors(center) {
		if !gotSet[n] {
			t.Errorf("Disk(center, 1) missing neighbor %v", n)
		}
	}
}

// TestDisk_Radius2Has19Hexes18Productive: megaron_plan_fysisk_gubbemodell.md
// P1 — "Radie 1→2: 19 hexar, 18 produktiva" (center + inner ring of 6 +
// outer ring of 12).
func TestDisk_Radius2Has19Hexes18Productive(t *testing.T) {
	center := Coord{0, 0}
	got := Disk(center, 2)
	if len(got) != 19 {
		t.Fatalf("Disk(center, 2) has %d hexes, want 19", len(got))
	}
	productive := 0
	for _, c := range got {
		if c != center {
			productive++
		}
	}
	if productive != 18 {
		t.Errorf("Disk(center, 2) has %d non-center hexes, want 18", productive)
	}
}

// TestDisk_NoDuplicates covers radius 0-3.
func TestDisk_NoDuplicates(t *testing.T) {
	for radius := 0; radius <= 3; radius++ {
		got := Disk(Coord{1, 1}, radius)
		seen := map[Coord]bool{}
		for _, c := range got {
			if seen[c] {
				t.Errorf("radius %d: duplicate hex %v", radius, c)
			}
			seen[c] = true
		}
	}
}

// TestDisk_EveryHexIsWithinRadius: Disk's own definition, checked against the
// independent Distance function so the two don't share a bug.
func TestDisk_EveryHexIsWithinRadius(t *testing.T) {
	center := Coord{-2, 5}
	for radius := 0; radius <= 3; radius++ {
		for _, c := range Disk(center, radius) {
			if d := Distance(center, c); d > radius {
				t.Errorf("radius %d: hex %v has distance %d from center", radius, c, d)
			}
		}
	}
}

// TestDistance_KnownValues pins the cube-distance formula against
// hand-computed values.
func TestDistance_KnownValues(t *testing.T) {
	cases := []struct {
		a, b Coord
		want int
	}{
		{Coord{0, 0}, Coord{0, 0}, 0},
		{Coord{0, 0}, Coord{1, 0}, 1},
		{Coord{0, 0}, Coord{1, -1}, 1},
		{Coord{0, 0}, Coord{2, 0}, 2},
		{Coord{0, 0}, Coord{2, -1}, 2},
		{Coord{3, -3}, Coord{0, 0}, 3},
	}
	for _, c := range cases {
		if got := Distance(c.a, c.b); got != c.want {
			t.Errorf("Distance(%v, %v) = %d, want %d", c.a, c.b, got, c.want)
		}
	}
}

// TestRing_ExcludesCenterButMatchesDiskOtherwise: Ring(c,r) must equal
// Disk(c,r) with exactly the center hex removed — the productive catchment
// (18 hexes at radius 2) the P1 production queries actually iterate over.
func TestRing_ExcludesCenterButMatchesDiskOtherwise(t *testing.T) {
	center := Coord{4, -1}
	for radius := 0; radius <= 3; radius++ {
		disk := coordSet(Disk(center, radius))
		ring := Ring(center, radius)
		if len(ring) != len(disk)-1 {
			t.Fatalf("radius %d: Ring has %d hexes, want %d (Disk minus center)", radius, len(ring), len(disk)-1)
		}
		for _, c := range ring {
			if c == center {
				t.Errorf("radius %d: Ring includes the center hex", radius)
			}
			if !disk[c] {
				t.Errorf("radius %d: Ring hex %v not in Disk", radius, c)
			}
		}
	}
}

// TestRing_Radius2Has18Hexes pins the productive-catchment count directly.
func TestRing_Radius2Has18Hexes(t *testing.T) {
	if got := len(Ring(Coord{0, 0}, CatchmentRadius)); got != 18 {
		t.Errorf("Ring(center, CatchmentRadius) has %d hexes, want 18", got)
	}
}

// TestQRArrays_PairsSurvive verifies q/r stay paired across the split —
// zipping qs[i]/rs[i] back together must reproduce the original coords.
func TestQRArrays_PairsSurvive(t *testing.T) {
	coords := Disk(Coord{-3, 8}, 2)
	qs, rs := QRArrays(coords)
	if len(qs) != len(coords) || len(rs) != len(coords) {
		t.Fatalf("QRArrays length mismatch: %d coords, %d qs, %d rs", len(coords), len(qs), len(rs))
	}
	for i, c := range coords {
		if int(qs[i]) != c.Q || int(rs[i]) != c.R {
			t.Errorf("index %d: got (%d,%d), want (%d,%d)", i, qs[i], rs[i], c.Q, c.R)
		}
	}
}

// TestDisk_IsSymmetricUnderTranslation: Disk's shape must not depend on
// WHERE the center sits — only its size (radius) — since every call site
// applies it to a different settlement's own coordinates.
func TestDisk_IsSymmetricUnderTranslation(t *testing.T) {
	radius := 2
	origin := Disk(Coord{0, 0}, radius)
	shifted := Disk(Coord{7, -9}, radius)
	if len(origin) != len(shifted) {
		t.Fatalf("origin has %d hexes, shifted has %d", len(origin), len(shifted))
	}
	shiftedSet := coordSet(shifted)
	for _, c := range origin {
		want := Coord{c.Q + 7, c.R - 9}
		if !shiftedSet[want] {
			t.Errorf("shifted disk missing %v (translate of %v)", want, c)
		}
	}
}
