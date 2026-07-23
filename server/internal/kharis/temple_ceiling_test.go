package kharis

import "testing"

// TestTempleCeiling_Level1StaysBelowBless — kärnan i hela åtgärden. Ett blygsamt
// tempel ska ge en Wanax kharis och riter, men ALDRIG gudarnas uppmärksamhet.
// Efter att kharisPerTempleDay höjdes 1.2→8.0 och feedTempleBySubstitution gjorde
// varje tempel matbart klättrade alla obehindrat till 100 — 13 av 17 Wanaxer satt
// på exakt taket och riten hade slutat vara ett vad (92 % lyckade kast).
func TestTempleCeiling_Level1StaysBelowBless(t *testing.T) {
	l1 := TempleKharisCeiling(1)
	if l1 >= blessThreshold {
		t.Errorf("a level-1 temple must not reach the bless threshold %.0f, ceiling is %.0f",
			blessThreshold, l1)
	}
	if l1 <= punishThreshold {
		t.Errorf("a level-1 temple must still keep a Wanax clear of divine punishment (<%.0f), ceiling is %.0f",
			punishThreshold, l1)
	}
}

// TestTempleCeiling_HigherTemplesReachFurther — nivåerna ska vara en klättring mot
// NÅGOT: varje nivå måste flytta taket, och den högsta ska nå hela vägen.
func TestTempleCeiling_HigherTemplesReachFurther(t *testing.T) {
	prev := TempleKharisCeiling(0)
	for level := 1; level <= 3; level++ {
		got := TempleKharisCeiling(level)
		if got <= prev {
			t.Errorf("level %d ceiling %.0f must exceed level %d's %.0f", level, got, level-1, prev)
		}
		prev = got
	}
	if top := TempleKharisCeiling(3); top < 100 {
		t.Errorf("the grandest temple must reach the full pool (100), got %.0f", top)
	}
	if TempleKharisCeiling(2) < blessThreshold {
		t.Errorf("a level-2 temple must be able to attract divine favour (≥%.0f), got %.0f",
			blessThreshold, TempleKharisCeiling(2))
	}
}

// TestEffectiveKharisCap_TakesTheLower — templet får höja ambitionen, aldrig
// spränga Wanaxens egen kharis_cap.
func TestEffectiveKharisCap_TakesTheLower(t *testing.T) {
	if got := EffectiveKharisCap(100, 1); got != TempleKharisCeiling(1) {
		t.Errorf("a modest temple must bind below the record cap, got %.0f", got)
	}
	if got := EffectiveKharisCap(100, 3); got != 100 {
		t.Errorf("a level-3 temple must not exceed the record cap of 100, got %.0f", got)
	}
	if got := EffectiveKharisCap(40, 3); got != 40 {
		t.Errorf("the record cap must still bind when it is the lower of the two, got %.0f", got)
	}
}

// TestClampKharis_RespectsTheTempleCeiling — vakten mot återfall: en Wanax som
// redan står på 100 måste dras ned till sitt tempels tak vid nästa underhåll,
// annars biter fixen bara nya spelare.
func TestClampKharis_RespectsTheTempleCeiling(t *testing.T) {
	capForL1 := EffectiveKharisCap(100, 1)
	if got := clampKharis(100, capForL1); got != capForL1 {
		t.Errorf("a Wanax sitting at 100 with a level-1 temple must be pulled down to %.0f, got %.0f",
			capForL1, got)
	}
}

// TestTempleCeiling_NoTempleStillHasAFloor — ingen panik utan tempel: taket är
// lågt men golvet är heligt, och en tempellös Wanax tynar dit ändå via decay.
func TestTempleCeiling_NoTempleStillHasAFloor(t *testing.T) {
	if got := TempleKharisCeiling(0); got <= 0 {
		t.Errorf("even a templeless Wanax needs a positive ceiling, got %.0f", got)
	}
	if TempleKharisCeiling(0) >= TempleKharisCeiling(1) {
		t.Error("having a temple must be better than having none")
	}
}
