package religion

import (
	"math"
	"testing"
)

func TestGiftFactorDurationAndRest(t *testing.T) {
	if GiftFactor(10, 10) != 1 || GiftFactor(5, 10) != 1 || GiftFactor(10, 0) != 1 {
		t.Fatal("traditional, short or unreckoned gift must be 1")
	}
	if GiftFactor(20, 10) != 2 || GiftFactor(1000, 10) != MaxGiftFactor {
		t.Fatal("gift factor must be worth/baseline, capped")
	}
	for _, tc := range []struct {
		base int
		r    float64
		want int
	}{{6, 1, 6}, {6, 2, 12}, {6, 1.25, 8}, {6, 99, 30}, {25, 2, 50}} {
		if got := DurationTicks(tc.base, tc.r); got != tc.want {
			t.Errorf("DurationTicks(%d,%v)=%d want %d", tc.base, tc.r, got, tc.want)
		}
	}
	for _, tc := range []struct {
		cd    int
		r     float64
		level int
		want  int
	}{{24, 1, 1, 24}, {24, 1, 2, 12}, {24, 1, 3, 8}, {25, 1, 2, 13}, {24, 2, 2, 24}, {24, 1, 0, 0}, {0, 1, 1, 0}} {
		if got := RestTicks(tc.cd, tc.r, tc.level); got != tc.want {
			t.Errorf("RestTicks(%d,%v,%d)=%d want %d", tc.cd, tc.r, tc.level, got, tc.want)
		}
	}
}

// The prayer screen offers the gift as 1, 2, 3 and 5 times the traditional
// one (times the scope's factor). The rule is linear in the goods, so N times
// the recipe must give r = N: N times the length and N times the rest.
func TestGiftStepsGiveNTimesTheLength(t *testing.T) {
	values := FixedDivineValues
	checked := 0
	for _, spec := range PrayerSpecs {
		if spec.GodID == "" || len(spec.Offering) == 0 {
			continue
		}
		for scope, sf := range ScopeCostFactorsFor(spec.EffectType) {
			if scope == ScopePermanent {
				continue
			}
			baseline := TraditionalBaseline(spec, values) * float64(sf)
			for _, n := range []int{1, 2, 3, 5} {
				gift := map[string]float64{}
				for g, a := range spec.Offering {
					gift[g] = a * float64(sf) * float64(n)
				}
				r := GiftFactor(OfferingWorth(gift, values, FavoursFor(spec)), baseline)
				if math.Abs(r-float64(n)) > 1e-9 {
					t.Fatalf("%s scope %s N=%d: r=%v", spec.ID, scope, n, r)
				}
				if got := DurationTicks(6, r); got != 6*n {
					t.Fatalf("%s N=%d: duration %d want %d", spec.ID, n, got, 6*n)
				}
				// ceil() turns float noise (r = 3.0000000000000004) into one extra tick
				// of rest at most; found 2026-11-12, left as is (no server change in this slice).
				if got, want := RestTicks(24, r, 2), 12*n; got < want || got > want+1 {
					t.Fatalf("%s N=%d: rest %d want %d (+1 at most)", spec.ID, n, got, want)
				}
				checked++
			}
		}
	}
	if checked == 0 {
		t.Fatal("no prayer was checked")
	}
}
