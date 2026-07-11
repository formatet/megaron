package kharis

import "testing"

// Unit tests for the FAS 2 imperie-belastning formula (Timothy 2026-07-09
// kharis omdesign, temenos_kharis.md §"KANONISK OMDESIGN" FAS 2): dailyDecay =
// decayBas + decayPerKoloni × colonies without their own temple beyond
// decayFreeColonies free ones. Criteria per megaron_kharis_plan.md: "decay
// tillämpas på underhållen dag; skalar med tempellösa kolonier > 4; tempel-i-
// koloni nollar dess bidrag."

func TestComputeDailyDecay_BaseCaseNoColonies(t *testing.T) {
	if got := computeDailyDecay(0); got != decayBas {
		t.Errorf("computeDailyDecay(0) = %v, want decayBas %v", got, decayBas)
	}
}

func TestComputeDailyDecay_FreeColoniesDoNotIncreaseDecay(t *testing.T) {
	// Up to decayFreeColonies templeless colonies cost nothing extra.
	for n := 0; n <= decayFreeColonies; n++ {
		if got := computeDailyDecay(n); got != decayBas {
			t.Errorf("computeDailyDecay(%d) = %v, want base-only decayBas %v (within free allowance)", n, got, decayBas)
		}
	}
}

func TestComputeDailyDecay_ScalesAboveFreeColonies(t *testing.T) {
	// One templeless colony beyond the free allowance adds exactly decayPerKoloni.
	over1 := computeDailyDecay(decayFreeColonies + 1)
	if want := decayBas + decayPerKoloni; over1 != want {
		t.Errorf("computeDailyDecay(%d) = %v, want %v", decayFreeColonies+1, over1, want)
	}
	// Three colonies beyond the free allowance adds 3×decayPerKoloni.
	over3 := computeDailyDecay(decayFreeColonies + 3)
	if want := decayBas + 3*decayPerKoloni; over3 != want {
		t.Errorf("computeDailyDecay(%d) = %v, want %v", decayFreeColonies+3, over3, want)
	}
}

func TestComputeDailyDecay_MonotonicNonDecreasing(t *testing.T) {
	prev := computeDailyDecay(0)
	for n := 1; n <= 20; n++ {
		got := computeDailyDecay(n)
		if got < prev {
			t.Fatalf("computeDailyDecay not monotonic: n=%d got %v < previous %v", n, got, prev)
		}
		prev = got
	}
}

// TestComputeDailyDecay_TempleInColonyZeroesItsContribution documents the
// "tempel-i-koloni nollar dess bidrag" criterion at the caller level: a colony
// WITH a temple is never counted in templelessColonies in the first place (the
// SQL in Handle() filters on `NOT EXISTS (... building_type = 'temple')`), so
// building a temple there strictly reduces the count passed into this function
// — verified here by the monotonic/scaling properties above (fewer templeless
// colonies never means MORE decay).
func TestComputeDailyDecay_TempleInColonyZeroesItsContribution(t *testing.T) {
	withTemplelessColony := computeDailyDecay(decayFreeColonies + 1)
	afterBuildingTemple := computeDailyDecay(decayFreeColonies) // one fewer templeless colony
	if afterBuildingTemple >= withTemplelessColony {
		t.Errorf("building a temple should strictly reduce decay: before=%v after=%v", withTemplelessColony, afterBuildingTemple)
	}
}

// TestDecayBas_NetNeutralBand guards the net-neutral recalibration (Timothy
// 2026-07-11, A#4 kharis-rot): decayBas must stay near the passive geographic
// kharis_rate (~0.6/day) so a bare passive Wanax nets ~neutral and an offering-fed
// temple climbs — NOT the old 4.0, which made maintained temples net-negative,
// bled kharis to the floor (bless unreachable), and let a restart's tick catch-up
// nuke every Wanax to 1 in one burst. If a soak proves the climb too fast/slow,
// re-tune within this band; a value ≥ ~2 reintroduces the always-sinks bug.
func TestDecayBas_NetNeutralBand(t *testing.T) {
	if decayBas <= 0 || decayBas > 1.5 {
		t.Errorf("decayBas = %v, want (0, 1.5] — near the passive kharis_rate for net-neutral maintenance", decayBas)
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
