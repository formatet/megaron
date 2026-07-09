package kharis

import "testing"

// Unit tests for the FAS 3 offer-underhåll gain-scaling formula (Timothy
// 2026-07-09 kharis omdesign, temenos_kharis.md §"KANONISK OMDESIGN" §4).
// Criteria per megaron_kharis_plan.md: "med offer i lager -> gain; utan ->
// ingen/svag gain + netto-drift nedåt."

func TestComputeOfferFraction_FullOfferGivesFullFraction(t *testing.T) {
	if got := computeOfferFraction(3, 3); got != 1.0 {
		t.Errorf("computeOfferFraction(3, 3) = %v, want 1.0 (full offer everywhere -> full gain)", got)
	}
}

func TestComputeOfferFraction_NoOfferGivesZero(t *testing.T) {
	if got := computeOfferFraction(0, 3); got != 0.0 {
		t.Errorf("computeOfferFraction(0, 3) = %v, want 0.0 (no offer anywhere -> no gain, decay wins)", got)
	}
}

func TestComputeOfferFraction_PartialOfferScalesLinearly(t *testing.T) {
	if got := computeOfferFraction(1, 2); got != 0.5 {
		t.Errorf("computeOfferFraction(1, 2) = %v, want 0.5", got)
	}
}

func TestComputeOfferFraction_NoTemplesIsZeroNotNaN(t *testing.T) {
	if got := computeOfferFraction(0, 0); got != 0.0 {
		t.Errorf("computeOfferFraction(0, 0) = %v, want 0.0 (defensive: no divide-by-zero)", got)
	}
}

// TestGainScaling_NetDriftDownWithoutOffer documents the composed effect (FAS 2
// + FAS 3 together, as processMaintenance applies them): with cult produced but
// zero offer, gain is fully suppressed and only dailyDecay acts — net kharis
// change for the day is strictly negative, i.e. depreciation wins.
func TestGainScaling_NetDriftDownWithoutOffer(t *testing.T) {
	const cultGain = 10.0 // whatever cult labor would have produced
	offerFraction := computeOfferFraction(0, 2)
	gain := cultGain * offerFraction
	dailyDecay := computeDailyDecay(0)
	net := gain - dailyDecay
	if net >= 0 {
		t.Errorf("net = %v, want < 0 (unfed temples: depreciation must win)", net)
	}
	if gain != 0 {
		t.Errorf("gain = %v, want 0 (zero offer -> zero gain)", gain)
	}
}

// TestGainScaling_FullOfferPreservesCultGain documents the positive case: a
// fully-fed temple set passes the cult gain through unmodified.
func TestGainScaling_FullOfferPreservesCultGain(t *testing.T) {
	const cultGain = 10.0
	offerFraction := computeOfferFraction(2, 2)
	gain := cultGain * offerFraction
	if gain != cultGain {
		t.Errorf("gain = %v, want unmodified cultGain %v (full offer -> full gain)", gain, cultGain)
	}
}
