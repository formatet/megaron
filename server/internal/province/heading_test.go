package province

import (
	"testing"
	"time"
)

// The compass must be the map's (FuzzyBearing shares it since 2026-09-26): a +r step is due south on
// the flat-top map (hexPx: y = √3·(r + q/2)), a +q step is south-east.
func TestScreenCompass_MatchesTheDrawnMap(t *testing.T) {
	o := MapPosition{Q: 0, R: 0}
	cases := []struct {
		to   MapPosition
		want string
	}{
		{MapPosition{Q: 0, R: 1}, "south"},
		{MapPosition{Q: 0, R: -1}, "north"},
		{MapPosition{Q: 1, R: 0}, "south-east"},
		{MapPosition{Q: 1, R: -1}, "north-east"},
		{MapPosition{Q: -1, R: 1}, "south-west"},
		{MapPosition{Q: -1, R: 0}, "north-west"},
		{MapPosition{Q: 2, R: -1}, "east"},
		{o, ""},
	}
	for _, c := range cases {
		if got := ScreenCompass(o, c.to); got != c.want {
			t.Errorf("ScreenCompass(0,0 → %d,%d) = %q, want %q", c.to.Q, c.to.R, got, c.want)
		}
	}
}

func straightSouth(n int) []MapPosition {
	p := make([]MapPosition, n+1)
	for i := range p {
		p[i] = MapPosition{Q: 0, R: i}
	}
	return p
}

func TestReadMarch_HeadingAndArrivalIf(t *testing.T) {
	departs := time.Date(2026, 9, 26, 0, 0, 0, 0, time.UTC)
	arrives := departs.Add(10 * time.Hour) // 10 steps, 1 h each
	now := departs.Add(4 * time.Hour)

	m, ok := ReadMarch(straightSouth(10), departs, arrives, now)
	if !ok {
		t.Fatal("ReadMarch: ok = false")
	}
	if m.Pos != (MapPosition{Q: 0, R: 4}) || m.Heading != "south" {
		t.Fatalf("march = %+v, want at (0,4) heading south", m)
	}
	if got := m.ArrivalIf(3); !got.Equal(departs.Add(7 * time.Hour)) {
		t.Errorf("ArrivalIf(3) = %v, want departs+7h", got)
	}
	if got := m.ArrivalTickIf(3, 100, 110, 10); got != 107 {
		t.Errorf("ArrivalTickIf(3) = %d, want 107", got)
	}
}

// A city ahead in the march's heading seems to be its goal; one off to the side
// does not — even if the unit's real destination is that side city.
func TestSeemsBoundFor_OnlyWhatLiesAhead(t *testing.T) {
	departs := time.Now()
	m, _ := ReadMarch(straightSouth(10), departs, departs.Add(10*time.Hour), departs)

	ahead := disk(MapPosition{Q: 0, R: 8}, 2)
	if d, ok := m.SeemsBoundFor(ahead); !ok || d != 6 {
		t.Errorf("catchment ahead: (%d, %v), want (6, true)", d, ok)
	}
	aside := disk(MapPosition{Q: -8, R: 4}, 2) // due west-ish, not south
	if _, ok := m.SeemsBoundFor(aside); ok {
		t.Error("catchment off to the side seems to be the goal")
	}
	if d, ok := m.SeemsBoundFor(disk(MapPosition{Q: 0, R: 1}, 2)); !ok || d != 0 {
		t.Errorf("already inside catchment: (%d, %v), want (0, true)", d, ok)
	}
}

// disk: every hex within radius of c (test-local; hexgrid.Disk is not
// importable from province under G1).
func disk(c MapPosition, radius int) []MapPosition {
	var out []MapPosition
	for dq := -radius; dq <= radius; dq++ {
		for dr := -radius; dr <= radius; dr++ {
			p := MapPosition{Q: c.Q + dq, R: c.R + dr}
			if HexDistance(c, p) <= radius {
				out = append(out, p)
			}
		}
	}
	return out
}
