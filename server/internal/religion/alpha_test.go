package religion

import "testing"

func TestAlphaCatalogueAndSilverFloor(t *testing.T) {
	if len(CulturePrayers["minoan"]) != 3 {
		t.Fatal("checkpoint A must expose three implemented prayers")
	}
	for _, id := range []string{PrayerPotniaVision, PrayerPotniaWalls, PrayerGreenShoots, PrayerAsterionThread, PrayerStillnessPassage} {
		spec, ok := PrayerSpecs[id]
		if !ok || spec.GodID == "" {
			t.Fatalf("missing alpha spec %s", id)
		}
	}
	if PrayerSpecs[PrayerPotniaVision].GodID != PrayerSpecs[PrayerPotniaWalls].GodID {
		t.Fatal("Potnia cooldown keys differ")
	}
	for _, id := range []string{"minoan_oracle_deposits", "minoan_harvest_blessing", "minoan_battle_frenzy"} {
		if AllowedForCulture("minoan", id) {
			t.Fatal("retired prayer is castable")
		}
	}
	values := map[string]float64{"silver": 1.7, "grain": 220.32}
	if got := OfferingWorth(map[string]float64{"silver": 2}, values, nil); got != 200 {
		t.Fatalf("silver altar floor missing: %v", got)
	}
	if values["silver"] != 1.7 {
		t.Fatal("altar floor mutated market/valuation input")
	}
	if got := OfferingWorth(map[string]float64{"silver": 2}, map[string]float64{"silver": 150}, nil); got != 300 {
		t.Fatal("floor lowered expensive silver")
	}
}
