package combat

// Slice B (megaron_plan_skeppsfart_besattning.md §4): CrewSpeedFactor is the
// term that makes a shorthanded hull sail slower — 1.0 at full crew, rising
// toward crewSpeedMax (2.0) as the benches empty, continuous, no threshold
// and no minimum-crew hard stop (Timothy's decision in the plan: a crew of 1
// still sails, just slowly). Literal expectations, not re-derivations of the
// formula, so a bug in both places at once would still be caught.

import (
	"math"
	"testing"

	"formatet/megaron/server/internal/unit"
)

func TestCrewSpeedFactor_MatchesLiteralValues(t *testing.T) {
	cases := []struct {
		utype unit.Type
		crew  int
		want  float64
	}{
		// War galley, full crew 50.
		{unit.TypeWarGalley, 50, 1.0},
		{unit.TypeWarGalley, 60, 1.0}, // over-full clamps to 1.0, never < 1.0
		{unit.TypeWarGalley, 25, 1.5}, // half crew
		{unit.TypeWarGalley, 0, 2.0},  // empty benches: crewSpeedMax
		{unit.TypeWarGalley, -5, 2.0}, // negative clamps to 0 crew, not a panic

		// Merchantman, full crew 10.
		{unit.TypeMerchantman, 10, 1.0},
		{unit.TypeMerchantman, 5, 1.5},
		{unit.TypeMerchantman, 2, 1.8},

		// Canonical galley, full crew 20 — plan §4's own worked examples:
		// "En halvbemannad galär (10/20) tar 1,5× tiden; en med en enda man
		// ~1,95×."
		{unit.TypeGalley, 20, 1.0},
		{unit.TypeGalley, 10, 1.5},
		{unit.TypeGalley, 1, 1.95},

		// Land units: CrewFor is 0, so the type is exempt regardless of the
		// crew column's value (a spearman must never get double travel time
		// because the column reads 0 by construction — AC4).
		{unit.TypeSpearman, 0, 1.0},
		{unit.TypeEliteInfantry, 0, 1.0},
		{unit.TypeWarChariot, 0, 1.0},
		{unit.TypeNomadicHost, 0, 1.0},
	}

	const epsilon = 1e-9
	for _, c := range cases {
		if got := CrewSpeedFactor(c.utype, c.crew); math.Abs(got-c.want) > epsilon {
			t.Errorf("CrewSpeedFactor(%s, crew=%d) = %v, want %v", c.utype, c.crew, got, c.want)
		}
	}
}
