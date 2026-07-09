package handlers

import "testing"

// Unit tests for the FAS 1 continuous rite-success formula (Timothy 2026-07-09
// kharis omdesign, temenos_kharis.md §"KANONISK OMDESIGN"): success is
// clamp(kharis% + offerMod, riteFloor, riteCeil) — no tiers, the kharis number
// IS the mattare. These are pure-function tests (no DB) matching the plan's
// stated criterion: "monoton i kharis, clampad till [floor,ceil], offerMod
// flyttar rätt håll."

func TestRiteSuccessChance_MonotonicInKharis(t *testing.T) {
	prev := riteSuccessChance(0, 0)
	for k := 1.0; k <= 100; k++ {
		got := riteSuccessChance(k, 0)
		if got < prev {
			t.Fatalf("riteSuccessChance not monotonic: kharis=%v got %v < previous %v", k, got, prev)
		}
		prev = got
	}
}

func TestRiteSuccessChance_ClampedToFloorAndCeil(t *testing.T) {
	if got := riteSuccessChance(0, -1); got != riteFloor {
		t.Errorf("riteSuccessChance(0, -1) = %v, want floor %v", got, riteFloor)
	}
	if got := riteSuccessChance(100, 1); got != riteCeil {
		t.Errorf("riteSuccessChance(100, 1) = %v, want ceil %v", got, riteCeil)
	}
	// Even at kharis=100 with no offer bonus, the ceiling still holds — never
	// literally 100% ("gudarna är inte maskiner").
	if got := riteSuccessChance(100, 0); got > riteCeil {
		t.Errorf("riteSuccessChance(100, 0) = %v, exceeds ceil %v", got, riteCeil)
	}
	// Even at kharis=0 with a punishing offer, the floor still holds — the
	// golv is heligt, gudarna lyssnar alltid *ibland*.
	if got := riteSuccessChance(0, -1); got < riteFloor {
		t.Errorf("riteSuccessChance(0, -1) = %v, below floor %v", got, riteFloor)
	}
}

func TestRiteSuccessChance_TalIsMataren(t *testing.T) {
	// "kharis 95 → ~95%; kharis 40 → ~40%" — the design's own worked examples.
	if got := riteSuccessChance(95, 0); got != 0.95 {
		t.Errorf("riteSuccessChance(95, 0) = %v, want 0.95", got)
	}
	if got := riteSuccessChance(40, 0); got != 0.40 {
		t.Errorf("riteSuccessChance(40, 0) = %v, want 0.40", got)
	}
}

func TestRiteOfferMod_FatOfferIncreasesChance(t *testing.T) {
	fat := riteOfferMod(riteOfferMultiplierMax)
	if fat != riteOfferModFat {
		t.Errorf("riteOfferMod(max=%v) = %v, want exactly %v", riteOfferMultiplierMax, fat, riteOfferModFat)
	}
	if fat <= 0 {
		t.Errorf("riteOfferMod(max) = %v, want > 0 (a fett offer must help)", fat)
	}
}

func TestRiteOfferMod_StingyOfferDecreasesChance(t *testing.T) {
	stingy := riteOfferMod(riteOfferMultiplierMin)
	if stingy != riteOfferModStingy {
		t.Errorf("riteOfferMod(min=%v) = %v, want exactly %v", riteOfferMultiplierMin, stingy, riteOfferModStingy)
	}
	if stingy >= 0 {
		t.Errorf("riteOfferMod(min) = %v, want < 0 (a snålt offer must hurt)", stingy)
	}
}

func TestRiteOfferMod_BaselineIsNeutral(t *testing.T) {
	if got := riteOfferMod(1.0); got != 0 {
		t.Errorf("riteOfferMod(1.0) = %v, want 0 (baseline offer is neutral)", got)
	}
}

func TestRiteOfferMod_MonotonicInMultiplier(t *testing.T) {
	prev := riteOfferMod(riteOfferMultiplierMin)
	for m := riteOfferMultiplierMin; m <= riteOfferMultiplierMax; m += 0.05 {
		got := riteOfferMod(m)
		if got < prev {
			t.Fatalf("riteOfferMod not monotonic: multiplier=%v got %v < previous %v", m, got, prev)
		}
		prev = got
	}
}

func TestRiteOfferMultiplier_DefaultsToBaselineWhenOmitted(t *testing.T) {
	// JSON zero-value (field omitted or explicitly 0) must default to 1.0 —
	// fully backward compatible with callers that never send offer_multiplier.
	if got := riteOfferMultiplier(0); got != 1.0 {
		t.Errorf("riteOfferMultiplier(0) = %v, want 1.0 (baseline default)", got)
	}
	if got := riteOfferMultiplier(-5); got != 1.0 {
		t.Errorf("riteOfferMultiplier(-5) = %v, want 1.0 (invalid negative defaults to baseline)", got)
	}
}

func TestRiteOfferMultiplier_Clamped(t *testing.T) {
	if got := riteOfferMultiplier(10); got != riteOfferMultiplierMax {
		t.Errorf("riteOfferMultiplier(10) = %v, want clamped to max %v", got, riteOfferMultiplierMax)
	}
	if got := riteOfferMultiplier(0.01); got != riteOfferMultiplierMin {
		t.Errorf("riteOfferMultiplier(0.01) = %v, want clamped to min %v", got, riteOfferMultiplierMin)
	}
}
