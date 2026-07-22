package religion

import "testing"

var testValues = map[string]float64{
	"grain": 3, "oil": 4, "wine": 5, "tin": 34, "stone": 2,
}

// Taste is what makes composition a decision instead of arithmetic: the same
// quantity of the same total value must be worth more to the god who wants it.
func TestOfferingWorth_TasteDecides(t *testing.T) {
	dionysos := map[string]float64{"wine": 2.0, "grain": 0.5}
	ares := map[string]float64{"grain": 2.0, "wine": 0.5}

	wineGift := map[string]float64{"wine": 20}
	if w, a := OfferingWorth(wineGift, testValues, dionysos), OfferingWorth(wineGift, testValues, ares); w <= a {
		t.Fatalf("wine must please the wine-god more: %.0f vs %.0f", w, a)
	}

	// A good the god has no opinion about is a lesser gift, never an insult.
	neutral := OfferingWorth(map[string]float64{"stone": 10}, testValues, dionysos)
	if neutral != 10*testValues["stone"]*affinityDefault {
		t.Errorf("unfavoured goods must weigh the default, got %.1f", neutral)
	}
}

// Scarcity feeds straight through: the same bushel is worth more when the world
// is short of it. This is the whole Timothy-2026-07-22 revision in one assertion.
func TestOfferingWorth_ScarcityRaisesTheGift(t *testing.T) {
	plentiful := map[string]float64{"tin": 12}
	scarce := map[string]float64{"tin": 34}
	gift := map[string]float64{"tin": 10}
	if OfferingWorth(gift, scarce, nil) <= OfferingWorth(gift, plentiful, nil) {
		t.Fatal("a scarcer good must make the same gift worth more")
	}
}

func TestOfferingWorth_IgnoresUnknownAndEmpty(t *testing.T) {
	if got := OfferingWorth(map[string]float64{"ambrosia": 100}, testValues, nil); got != 0 {
		t.Errorf("a good outside the world's economy is worth nothing, got %.1f", got)
	}
	if got := OfferingWorth(map[string]float64{"wine": 0, "oil": -5}, testValues, nil); got != 0 {
		t.Errorf("empty and negative amounts must contribute nothing, got %.1f", got)
	}
}

// An offering can never buy what standing must earn — so generosity saturates
// while the bounds stay exactly where the fixed-recipe era left them.
func TestOfferMod_SaturatesAndRespectsOldBounds(t *testing.T) {
	if got := OfferMod(100, 100, 1); got != 0 {
		t.Errorf("an offering at the baseline is neutral, got %+.3f", got)
	}
	tenfold := OfferMod(1000, 100, 1)
	twofold := OfferMod(200, 100, 1)
	if tenfold <= twofold {
		t.Fatal("more must still be better")
	}
	if tenfold > offerModMax {
		t.Errorf("generosity must not exceed the old ceiling: %+.3f > %+.3f", tenfold, offerModMax)
	}
	if (tenfold - twofold) > (twofold - 0) {
		t.Error("the curve must saturate — the second doubling cannot beat the first")
	}
	if got := OfferMod(0, 100, 0); got != offerModMin {
		t.Errorf("an empty offering must sit at the floor, got %+.3f", got)
	}
}

// Stinginess is noticed more keenly than generosity is rewarded — the asymmetry
// is inherited deliberately from the old riteOfferMod (+0.10 / −0.15).
func TestOfferMod_StingierThanGenerous(t *testing.T) {
	half := -OfferMod(50, 100, 1)
	double := OfferMod(200, 100, 1)
	if half <= double {
		t.Errorf("halving should cost more than doubling gains: %.3f vs %.3f", half, double)
	}
}

// Many kinds please more than a heap of one — and many kinds can only be
// assembled by trading. The bonus must tilt, never dictate.
func TestOfferMod_VarietyIsSmallButReal(t *testing.T) {
	one := OfferMod(100, 100, 1)
	four := OfferMod(100, 100, 4)
	if four <= one {
		t.Fatal("a varied offering of equal worth must be worth more")
	}
	if four-one > varietyBonusMax+1e-9 {
		t.Errorf("variety bonus exceeded its cap: %+.3f", four-one)
	}
	many := OfferMod(100, 100, 12)
	if many != four+0 && many-one > varietyBonusMax+1e-9 {
		t.Errorf("variety must stop counting past the cap, got %+.3f", many-one)
	}
}

func TestOfferingShortfall_ReportsTheGap(t *testing.T) {
	if got := OfferingShortfall(60, 100); got < 0.39 || got > 0.41 {
		t.Errorf("60 of 100 is a 40%% shortfall, got %.2f", got)
	}
	if got := OfferingShortfall(150, 100); got != 0 {
		t.Errorf("a generous offering falls short of nothing, got %.2f", got)
	}
}
