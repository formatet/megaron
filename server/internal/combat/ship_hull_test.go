package combat

// megaron_plan_skeppsreparation.md Slice B — pins the two calibration
// details the plan explicitly delegates to the implementer (strawman, not
// canon): the casualty-fraction → hull-point mapping, and the embarked-
// cohort loss fraction. Pure, no DB — ship_hull_db_test.go covers the
// DB-backed end-to-end wiring (battle → hull draw → home march → cargo loss).

import "testing"

func TestHullLossForCasualtyFraction(t *testing.T) {
	cases := []struct {
		name     string
		fraction float64
		want     int
	}{
		{"no losses draws no hull", 0, 0},
		{"negative fraction (defensive) draws no hull", -0.1, 0},
		{"a fifth of the fleet lost draws one point", 0.2, 1},
		{"a third rounds to two points", 0.35, 2},
		{"half the fleet lost draws the midpoint, rounds to 3", 0.5, 3},
		{"most of the fleet lost draws heavily", 0.8, 4},
		{"total naval wipe caps at hullMax, never exceeds it", 1.0, hullMax},
		{"defensive: a fraction > 1 still caps at hullMax", 1.4, hullMax},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := hullLossForCasualtyFraction(c.fraction)
			if got != c.want {
				t.Errorf("hullLossForCasualtyFraction(%v) = %d, want %d", c.fraction, got, c.want)
			}
			if got < 0 || got > hullMax {
				t.Errorf("hullLossForCasualtyFraction(%v) = %d, out of [0,%d] bounds", c.fraction, got, hullMax)
			}
		})
	}
}

// TestCargoSizeAfterHullLoss pins §Beslut B3's cargo rule against the OTHER
// reading the plan flags as equally plausible from Timothy's own "3/5"
// example ("bestäm om förlusten = hull-förlusten eller = kvarvarande hull").
// This implementation reads B3's actual locked sentence — "förlorar manskap
// PROPORTIONELLT MOT skeppets hull-FÖRLUST" — literally: the cohort loses the
// hull-LOSS fraction of its manpower, not the hull-REMAINING fraction.
func TestCargoSizeAfterHullLoss(t *testing.T) {
	cases := []struct {
		name     string
		size     int
		hullLoss int
		want     int
	}{
		{"no hull loss, no cargo loss", 60, 0, 60},
		{"one of five hull points lost — cohort loses a fifth", 60, 1, 48},
		// Pins the exact reading over the ambiguous alternative: a ship that
		// loses 2 of 5 hull points (down to hull 3, matching Timothy's "till
		// 3/5" framing) costs the cohort 2/5 = 40% of its manpower — NOT the
		// 3/5 = 60% a "loses what's left" misreading would give.
		{"ship down to 3/5 hull (lost 2) — cohort loses 2/5, not 3/5", 60, 2, 36},
		{"full 5-point loss (sinks) — cohort wiped", 60, 5, 0},
		{"hullLoss beyond hullMax (defensive) still wipes, never negative", 60, 9, 0},
		{"empty cohort stays empty", 0, 3, 0},
		{"fractional loss floors (7×1/5=1.4→1 lost)", 7, 1, 6},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := cargoSizeAfterHullLoss(c.size, c.hullLoss)
			if got != c.want {
				t.Errorf("cargoSizeAfterHullLoss(%d, %d) = %d, want %d", c.size, c.hullLoss, got, c.want)
			}
			if got < 0 || got > c.size {
				t.Errorf("cargoSizeAfterHullLoss(%d, %d) = %d, out of [0,%d] bounds", c.size, c.hullLoss, got, c.size)
			}
		})
	}
}
