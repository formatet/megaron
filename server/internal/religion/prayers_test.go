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
		// Verify it maps to one of the three supported tier thresholds.
		validThresholds := map[float64]bool{100: true, 400: true, 800: true}
		if !validThresholds[spec.MinKharis] {
			t.Errorf("prayer %q: MinKharis = %.0f is not a recognised tier (100/400/800)", id, spec.MinKharis)
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
		if spec.Cooldown <= 0 {
			t.Errorf("PrayerSpecs[%q].Cooldown is zero or negative", key)
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
