package kharis

import (
	"math"
	"testing"
)

func TestRealmDecay_EveryCityCostsMore(t *testing.T) {
	prev := 0.0
	for n := 1; n <= 20; n++ {
		got := computeDailyDecay(n)
		want := .1 + .1*float64(n*(n-1)/2)
		if math.Abs(got-want) > 1e-10 {
			t.Fatalf("N%d got%g want%g", n, got, want)
		}
		if n > 1 {
			delta := got - computeDailyDecay(n-1)
			if delta <= prev {
				t.Fatalf("N%d marginal%g <=%g", n, delta, prev)
			}
			prev = delta
		}
	}
	if math.Abs(computeDailyDecay(5)-1.1) > 1e-10 {
		t.Fatal("five cities must cost1 plus base.1")
	}
}
func TestRealmKharis_OpeningAndTempleCalibration(t *testing.T) {
	if StartingKharis != 40 || EffectiveKharisCap(100, 0) != 40 {
		t.Fatal("start/no-temple cap must both40")
	}
	if got := clampKharis(40-computeDailyDecay(1), EffectiveKharisCap(100, 0)); math.Abs(got-39.9) > 1e-10 {
		t.Fatalf("opening clipped:%g", got)
	}
	if net := .15*kharisPerTempleDay - computeDailyDecay(6); math.Abs(net-.5) > 1e-10 {
		t.Fatalf("six-city L1 net%g", net)
	}
	for level, want := range []float64{40, 50, 75, 100} {
		if TempleKharisCeiling(level) != want {
			t.Fatalf("cap%d", level)
		}
	}
}
func TestRealmPunishment_BandBoundaries(t *testing.T) {
	for _, tc := range []struct {
		k      float64
		temple bool
		p      float64
	}{{1, false, .03}, {29.999, false, .03}, {30, false, 0}, {40, false, 0}, {1, true, 0}, {29.999, true, 0}} {
		if got := PunishmentProbability(tc.k, tc.temple); got != tc.p {
			t.Fatalf("k%g temple%v probability%g want%g", tc.k, tc.temple, got, tc.p)
		}
	}
}
func TestClampKharis_FloorAndCap(t *testing.T) {
	if got := clampKharis(-50, 100); got != kharisFloor {
		t.Errorf("clampKharis(-50, 100) = %v, want floor %v", got, kharisFloor)
	}
	if got := clampKharis(500, 100); got != 100 {
		t.Errorf("clampKharis(500, 100) = %v, want cap 100", got)
	}
	if got := clampKharis(50, 100); got != 50 {
		t.Errorf("clampKharis(50, 100) = %v, want unchanged 50", got)
	}
}

func TestClampKharis_NeverExactlyZero(t *testing.T) {
	// "golvet är heligt" — a settlement that decays to nothing must still sit
	// above 0, never AT 0.
	if got := clampKharis(0, 100); got <= 0 {
		t.Errorf("clampKharis(0, 100) = %v, want > 0 (heligt golv)", got)
	}
	if got := clampKharis(-1000, 100); got <= 0 {
		t.Errorf("clampKharis(-1000, 100) = %v, want > 0 (heligt golv)", got)
	}
}
