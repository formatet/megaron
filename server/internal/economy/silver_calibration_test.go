package economy

import (
	"context"
	"math"
	"testing"
)

func TestSilverCalibrationMineAnchorsAndMaxLevel(t *testing.T) {
	pool := openScaleTestPool(t)
	defer pool.Close()
	for _, tc := range []struct {
		terrain string
		l1      float64
	}{{"hills", 1}, {"mountain_limestone", 2.5}} {
		var rate float64
		if err := pool.QueryRow(context.Background(), `SELECT rate_per_tick FROM production_rules WHERE good_key='silver' AND building_type='mine' AND terrain_type=$1`, tc.terrain).Scan(&rate); err != nil {
			t.Fatal(err)
		}
		for level := 1; level <= 3; level++ {
			places, mult := hexGoodPlaces(tc.terrain, false, false, true, false, map[string]int{"mine": level}, "silver")
			if places != 5 {
				t.Fatalf("silver places=%d want5 independent of level", places)
			}
			want := 5 * tc.l1 * (1 + .7*float64(level)) / 1.7
			if got := hexYield(rate, places, mult, 5); math.Abs(got-want) > 1e-9 {
				t.Errorf("%s L%d full mine=%g want%g", tc.terrain, level, got, want)
			}
		}
	}
}

func TestSilverCalibrationColonyCapNeverClipsStock(t *testing.T) {
	for _, tc := range []struct{ purse, stock, want float64 }{{0, 0, 960}, {30, 0, 960}, {90, 0, 960}, {30, 1000, 1000}, {700, 0, 1400}} {
		if got := ColonySilverCap(tc.purse, tc.stock); got != tc.want {
			t.Fatalf("purse%g stock%g cap%g want%g", tc.purse, tc.stock, got, tc.want)
		}
	}
}
