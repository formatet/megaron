package kharis

import (
	"math"
	"testing"
)

func TestRealmDecay_CitiesAndResidents(t *testing.T) {
	for _, tc := range []struct {
		cities, population int
		want               float64
	}{
		{1, 0, 1.2}, {1, 50000, 1.2}, {3, 2500, 10.8}, {10, 50000, 120}, {0, 0, 0}, {-1, -100, 0},
	} {
		if got := computeDailyDecay(tc.cities, tc.population); math.Abs(got-tc.want) > 1e-10 {
			t.Errorf("%dcities/%dresidents got%g want%g", tc.cities, tc.population, got, tc.want)
		}
	}
}
func TestRealmKharis_OpeningAndTempleCalibration(t *testing.T) {
	if StartingKharis != 40 || EffectiveKharisCap(100, 0) != 40 {
		t.Fatal("start/no-temple cap must both40")
	}
	if got := clampKharis(40-computeDailyDecay(1, 0), EffectiveKharisCap(100, 0)); math.Abs(got-38.8) > 1e-10 {
		t.Fatalf("opening clipped:%g", got)
	}
	if net := 2*(KharisPerTempleWeight/2) - computeDailyDecay(6, 0); math.Abs(net+41.1) > 1e-10 {
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
	}{{1, false, .03}, {14.999, false, .03}, {15, false, 0}, {30, false, 0}, {40, false, 0}, {1, true, .03}, {29.999, true, 0}} {
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
