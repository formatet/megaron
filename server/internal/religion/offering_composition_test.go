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

func TestOfferingShortfall_ReportsTheGap(t *testing.T) {
	if got := OfferingShortfall(60, 100); got < 0.39 || got > 0.41 {
		t.Errorf("60 of 100 is a 40%% shortfall, got %.2f", got)
	}
	if got := OfferingShortfall(150, 100); got != 0 {
		t.Errorf("a generous offering falls short of nothing, got %.2f", got)
	}
}

// Incense counts for more through the gods' taste tables (Timothy 2026-10-19).
func TestOfferingWorth_IncenseCountsForMoreThanGrain(t *testing.T) {
	values := map[string]float64{"grain": 3, "incense": 3}
	for _, id := range []string{PrayerPotniaVision, PrayerPotniaWalls} {
		spec, ok := PrayerSpecs[id]
		if !ok {
			t.Fatalf("no spec %s", id)
		}
		favours := FavoursFor(spec)
		inc := OfferingWorth(map[string]float64{"incense": 10}, values, favours)
		grain := OfferingWorth(map[string]float64{"grain": 10}, values, favours)
		if inc <= grain {
			t.Errorf("%s: incense must count for more than grain at equal value: %.1f vs %.1f", id, inc, grain)
		}
	}
}
