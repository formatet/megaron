package combat

import (
	"formatet/megaron/server/internal/province"
	"formatet/megaron/server/internal/unit"
	"testing"
)

func TestRO1UnitSpeedFactors(t *testing.T) {
	for _, c := range []struct {
		name, terrain string
		kind          unit.Type
		steps         int
		laden         bool
		want          int
	}{
		{"foot plains", "plains", unit.TypeSpearman, 4, false, 4},
		{"host plains", "plains", unit.TypeNomadicHost, 4, false, 4},
		{"foot hills", "hills", unit.TypeSpearman, 4, false, 8},
		{"foot desert", "semi_desert", unit.TypeSpearman, 4, false, 12},
		{"galley coast", "coastal_sea", unit.TypeGalley, 15, false, 5},
		{"galley deep", "deep_sea", unit.TypeGalley, 25, false, 5},
		{"war galley coast", "coastal_sea", unit.TypeWarGalley, 15, false, 3},
		{"war galley deep", "deep_sea", unit.TypeWarGalley, 25, false, 3},
		{"emporos coast", "coastal_sea", unit.TypeMerchantman, 15, false, 7},
		{"laden galley coast", "coastal_sea", unit.TypeGalley, 12, true, 6},
	} {
		t.Run(c.name, func(t *testing.T) {
			raw := province.TerrainMoveTicks(c.terrain) * float64(c.steps)
			if got := legTravelTicks(c.kind, unit.CrewFor(c.kind), c.laden, raw); got != c.want {
				t.Fatalf("RÖ1 %s duration=%d want=%d", c.name, got, c.want)
			}
		})
	}
}
