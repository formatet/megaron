package combat

import (
	"math"
	"testing"

	"formatet/megaron/server/internal/hexgrid"
)

// Live terrain around storm 6311a034 (world 1b062500, q14-21, r17-25). Land hexes listed; everything else in the box and west is sea.
var liveLand = map[[2]int]bool{}

func init() {
	for _, c := range [][2]int{{19, 17}, {20, 17}, {21, 17}, {18, 18}, {19, 18}, {20, 18}, {21, 18}, {18, 19}, {19, 19}, {20, 19}, {21, 19}, {18, 20}, {19, 20}, {20, 20}, {21, 20}, {19, 21}, {20, 21}, {21, 21}, {19, 22}, {20, 22}, {21, 22}, {19, 23}, {20, 23}, {21, 23}, {19, 24}, {20, 24}, {21, 24}, {19, 25}, {20, 25}} {
		liveLand[c] = true
	}
}
func liveSea(c hexgrid.Coord) bool {
	if c.Q > 21 || c.R < 10 || c.R > 32 || c.Q < 5 {
		return false
	}
	return !liveLand[[2]int{c.Q, c.R}]
}

func TestStepStorm_TurnsAwayFromTheCoast(t *testing.T) {
	worst, worstSeed := 0, 0
	hist := map[int]int{}
	for seed := uint64(1); seed <= 500; seed++ {
		d := newStormDice(seed)
		s := StormShape{{Q: 17, R: 21}, {Q: 17, R: 20}, {Q: 17, R: 22}}
		h := 1
		run, best := 0, 0
		px, py := centroid(s)
		for tick := 0; tick < 60; tick++ {
			for k := 0; k < StormStepsPerTick; k++ {
				s, h, _ = StepStorm(s, h, liveSea, d)
			}
			x, y := centroid(s)
			if x == px && y == py {
				run++
			} else {
				run = 0
			}
			px, py = x, y
			if run > best {
				best = run
			}
		}
		hist[best/5*5]++
		if best > worst {
			worst, worstSeed = best, int(seed)
		}
	}
	t.Logf("worst stationary run %d ticks (seed %d); histogram by 5: %v", worst, worstSeed, hist)
	if worst > 8 {
		t.Fatalf("a storm pressed against the coast stood still %d ticks (seed %d); want <= 8", worst, worstSeed)
	}
}

// The coast rule must not slow open-sea drift: with steady dice (never random, never turns)
// every heading and shape still travels >= 15 hexes in 30 ticks.
func TestStepStorm_OpenSeaDriftNotSlowed(t *testing.T) {
	shapes := map[string]StormShape{
		"line":     {{Q: 0, R: 0}, {Q: 1, R: 0}, {Q: -1, R: 0}},
		"bent":     {{Q: 0, R: 0}, {Q: 1, R: 0}, {Q: 1, R: -1}},
		"triangle": {{Q: 0, R: 0}, {Q: 1, R: 0}, {Q: 0, R: 1}},
	}
	for name, start := range shapes {
		for h := 0; h < 6; h++ {
			s := start.clone()
			hd := h
			x0, y0 := centroid(s)
			for tick := 0; tick < 30; tick++ {
				for k := 0; k < StormStepsPerTick; k++ {
					s, hd, _ = StepStorm(s, hd, openSea, steadyStormDice{})
				}
			}
			x, y := centroid(s)
			dx, dy := x-x0, y-y0
			if dist := math.Sqrt(dx*dx + dy*dy); dist < 15 {
				t.Errorf("%s heading %d: drifted only %.1f in 30 ticks, want >= 15", name, h, dist)
			}
		}
	}
}
