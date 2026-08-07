package religion

import "testing"

// TestPrayerCatalog_AllCulturesHaveThreePrayers verifies each culture has exactly
// oracle, harvest_blessing, and battle_frenzy entries.
func TestPrayerCatalog_AllCulturesHaveThreePrayers(t *testing.T) {
	wantEffects := map[string]bool{
		EffectOracleRevealDeposits: true,
		EffectHarvestBlessing:      true,
		EffectBattleFrenzy:         true,
	}
	for culture, ids := range CulturePrayers {
		if len(ids) != 3 {
			t.Errorf("culture %q: want 3 prayers, got %d", culture, len(ids))
		}
		seen := map[string]bool{}
		for _, id := range ids {
			spec, ok := PrayerSpecs[id]
			if !ok {
				t.Errorf("culture %q: prayer %q not in PrayerSpecs", culture, id)
				continue
			}
			if !wantEffects[spec.EffectType] {
				t.Errorf("culture %q: prayer %q has unknown effect type %q", culture, id, spec.EffectType)
			}
			seen[spec.EffectType] = true
		}
		for effect := range wantEffects {
			if !seen[effect] {
				t.Errorf("culture %q: missing prayer with effect %q", culture, effect)
			}
		}
	}
}

// TestAllCulturesPresent verifies all six Bronze Age cultures have a prayer list.
func TestAllCulturesPresent(t *testing.T) {
	wantCultures := []string{"akhaier", "khemetiu", "knaani", "thrakes", "minoan", "hatti"}
	for _, c := range wantCultures {
		if _, ok := CulturePrayers[c]; !ok {
			t.Errorf("missing prayer list for culture %q", c)
		}
	}
}

// TestCultureGate_AllowsOwnPrayer verifies that each prayer is allowed for its own culture.
func TestCultureGate_AllowsOwnPrayer(t *testing.T) {
	for culture, ids := range CulturePrayers {
		for _, id := range ids {
			if !AllowedForCulture(culture, id) {
				t.Errorf("AllowedForCulture(%q, %q) = false, want true", culture, id)
			}
		}
	}
}

// TestCultureGate_RejectsOtherCulturePrayer verifies that a prayer from one culture
// is rejected for a different culture — the 403 gate.
func TestCultureGate_RejectsOtherCulturePrayer(t *testing.T) {
	// Hatti should not be able to cast a Kna'ani prayer.
	hattiAllowed := AllowedForCulture("hatti", "knaani_oracle_deposits")
	if hattiAllowed {
		t.Error("AllowedForCulture(hatti, knaani_oracle_deposits) = true, want false")
	}

	// Akhaier should not be able to cast a Khemetiu prayer.
	akhaierAllowed := AllowedForCulture("akhaier", "khemetiu_battle_frenzy")
	if akhaierAllowed {
		t.Error("AllowedForCulture(akhaier, khemetiu_battle_frenzy) = true, want false")
	}
}

// TestDefaultBattleFrenzyFor verifies the backward-compat lookup.
func TestDefaultBattleFrenzyFor(t *testing.T) {
	for culture := range CulturePrayers {
		id := DefaultBattleFrenzyFor(culture)
		spec, ok := PrayerSpecs[id]
		if !ok {
			t.Errorf("DefaultBattleFrenzyFor(%q) = %q, not in PrayerSpecs", culture, id)
			continue
		}
		if spec.EffectType != EffectBattleFrenzy {
			t.Errorf("DefaultBattleFrenzyFor(%q): spec.EffectType = %q, want %q", culture, spec.EffectType, EffectBattleFrenzy)
		}
	}
}

// TestKharisNeverSpent verifies that no PrayerSpec has a non-zero "cost" field —
// kharis is a tier-gate, not a resource to deduct.
// (There is no CostKharis field in PrayerSpec — this test documents the invariant.)
func TestKharisNeverSpent(t *testing.T) {
	for id, spec := range PrayerSpecs {
		// MinKharis is a minimum threshold (gate), not a deduction.
		// Verify it maps to one of the three supported tier thresholds
		// (0-100 scale, Timothy 2026-07-09 kharis omdesign — was 100/400/800).
		validThresholds := map[float64]bool{5: true, 30: true, 60: true}
		if !validThresholds[spec.MinKharis] {
			t.Errorf("prayer %q: MinKharis = %.0f is not a recognised tier (5/30/60)", id, spec.MinKharis)
		}
	}
}

// TestSeaGods_FavourFishAboveEverything verifies the S5 tabellutvidgning
// (megaron_plan_varukatalogen.md: "fish → favoritoffer hos vissa gudar
// (Havsdaimonerna/Potnia Theron)") — Potnia's oracle and Poseidon's battle
// frenzy must weigh fish HIGHER than any other good in their own taste table,
// and higher than the shared archetype's fish weight other cultures still use.
func TestSeaGods_FavourFishAboveEverything(t *testing.T) {
	cases := []struct {
		prayerID string
		effect   string
	}{
		{"minoan_oracle_deposits", EffectOracleRevealDeposits},
		{"minoan_battle_frenzy", EffectBattleFrenzy},
	}
	for _, c := range cases {
		spec, ok := PrayerSpecs[c.prayerID]
		if !ok {
			t.Fatalf("prayer %q not in PrayerSpecs", c.prayerID)
		}
		favours := FavoursFor(spec)
		fishWeight, hasFish := favours["fish"]
		if !hasFish {
			t.Fatalf("%s: FavoursFor has no fish weight", c.prayerID)
		}
		for good, w := range favours {
			if good != "fish" && w >= fishWeight {
				t.Errorf("%s: fish (%.2f) is not the favourite offering — %s weighs %.2f, want strictly less",
					c.prayerID, fishWeight, good, w)
			}
		}
		archetypeFish := favoursByEffect[c.effect]["fish"]
		if fishWeight <= archetypeFish {
			t.Errorf("%s: fish weight %.2f must exceed the shared archetype's %.2f — a favourite offering, not the default",
				c.prayerID, fishWeight, archetypeFish)
		}
	}
}

// TestSeaGods_OnlyFishChanged verifies the per-prayer Favours override for
// Potnia/Poseidon restates every OTHER weight from its archetype unchanged —
// FavoursFor never merges a per-prayer override with the archetype, so a
// careless override would silently reset every other good to affinityDefault.
func TestSeaGods_OnlyFishChanged(t *testing.T) {
	cases := []struct {
		prayerID string
		effect   string
	}{
		{"minoan_oracle_deposits", EffectOracleRevealDeposits},
		{"minoan_battle_frenzy", EffectBattleFrenzy},
	}
	for _, c := range cases {
		favours := FavoursFor(PrayerSpecs[c.prayerID])
		archetype := favoursByEffect[c.effect]
		for good, want := range archetype {
			if good == "fish" {
				continue
			}
			if got := favours[good]; got != want {
				t.Errorf("%s: %s weight = %.2f, want %.2f (unchanged from the archetype)", c.prayerID, good, got, want)
			}
		}
	}
}

// TestOtherCultures_FishStaysAtArchetypeWeight verifies the fish favour is
// scoped to the two sea prayers only — every other culture's equivalent
// prayer must keep reading the shared (unfavoured) archetype weight for fish.
func TestOtherCultures_FishStaysAtArchetypeWeight(t *testing.T) {
	for id, spec := range PrayerSpecs {
		if id == "minoan_oracle_deposits" || id == "minoan_battle_frenzy" {
			continue
		}
		favours := FavoursFor(spec)
		if got, want := favours["fish"], favoursByEffect[spec.EffectType]["fish"]; got != want {
			t.Errorf("%s: fish weight = %.2f, want %.2f (unfavoured archetype default)", id, got, want)
		}
	}
}

// TestPrayerSpecsHaveRequiredFields verifies every entry has non-empty ID, EffectType, God, Name.
func TestPrayerSpecsHaveRequiredFields(t *testing.T) {
	for key, spec := range PrayerSpecs {
		if spec.ID == "" {
			t.Errorf("PrayerSpecs[%q].ID is empty", key)
		}
		if spec.ID != key {
			t.Errorf("PrayerSpecs[%q].ID = %q, want key to match ID", key, spec.ID)
		}
		if spec.EffectType == "" {
			t.Errorf("PrayerSpecs[%q].EffectType is empty", key)
		}
		if spec.God == "" {
			t.Errorf("PrayerSpecs[%q].God is empty", key)
		}
		if spec.Name == "" {
			t.Errorf("PrayerSpecs[%q].Name is empty", key)
		}
		if spec.Description == "" {
			t.Errorf("PrayerSpecs[%q].Description is empty — rite --list needs an effect line (A7)", key)
		}
		if spec.CooldownTicks <= 0 {
			t.Errorf("PrayerSpecs[%q].CooldownTicks is zero or negative", key)
		}
		// Every prayer must demand a material offering — no free prayers.
		// Religion is an economic sink that drives trade for wine/oil/silver.
		if len(spec.Offering) == 0 {
			t.Errorf("PrayerSpecs[%q].Offering is empty — prayers must cost goods", key)
		}
		for good, amt := range spec.Offering {
			if amt <= 0 {
				t.Errorf("PrayerSpecs[%q].Offering[%q] = %v, want > 0", key, good, amt)
			}
		}
	}
}
