package loyalty

import "testing"

func TestLoyaltyFromPoints_Bands(t *testing.T) {
	cases := []struct {
		points float64
		want   int
	}{
		{1, 1}, {24.9, 1},
		{25, 2}, {49.9, 2},
		{50, 3}, {74.9, 3},
		{75, 4}, {100, 4},
	}
	for _, c := range cases {
		if got := LoyaltyFromPoints(c.points); got != c.want {
			t.Errorf("LoyaltyFromPoints(%.1f) = %d, want %d", c.points, got, c.want)
		}
	}
}

func TestScaleDeltaToPoints_Asymmetric(t *testing.T) {
	// Gains ×1.0, losses ×1.5 — collapse outruns recovery.
	cases := []struct {
		delta int
		want  float64
	}{
		{2, 2.0}, {1, 1.0}, {0, 0}, {-1, -1.5}, {-2, -3.0},
	}
	for _, c := range cases {
		if got := scaleDeltaToPoints(c.delta); got != c.want {
			t.Errorf("scaleDeltaToPoints(%d) = %.2f, want %.2f", c.delta, got, c.want)
		}
	}
}

func TestClampPoints(t *testing.T) {
	if got := clampPoints(-5); got != LoyaltyPointsFloor {
		t.Errorf("clampPoints(-5) = %.1f, want floor %.1f", got, LoyaltyPointsFloor)
	}
	if got := clampPoints(500); got != LoyaltyPointsCap {
		t.Errorf("clampPoints(500) = %.1f, want cap %.1f", got, LoyaltyPointsCap)
	}
	if got := clampPoints(42); got != 42 {
		t.Errorf("clampPoints(42) = %.1f, want unchanged 42", got)
	}
}

// The whole point of the redesign: loyalty is built over WEEKS. A colony that
// starts at loyalty 2 (37 points) and enjoys a +1 net welfare gain every game-day
// must take many days — not one tick — to climb a band. This mirrors the SQL
// band-derivation (covered end-to-end by combat's applyBattleLoyalty DB test).
func TestLoyaltyCrawlsOverManyDays(t *testing.T) {
	points := LoyaltyStartColony // 37 → loyalty 2
	if LoyaltyFromPoints(points) != 2 {
		t.Fatalf("colony must start at loyalty 2, got %d", LoyaltyFromPoints(points))
	}
	// 12 daily +1 gains: 37 → 49, still loyalty 2.
	for i := 0; i < 12; i++ {
		points = clampPoints(points + scaleDeltaToPoints(1))
	}
	if got := LoyaltyFromPoints(points); got != 2 {
		t.Errorf("after 12 good days (points %.1f) loyalty should still be 2, got %d", points, got)
	}
	// The 13th good day crosses Band2Ceil (50) → loyalty 3.
	points = clampPoints(points + scaleDeltaToPoints(1))
	if got := LoyaltyFromPoints(points); got != 3 {
		t.Errorf("after 13 good days (points %.1f) loyalty should be 3, got %d", points, got)
	}
}
