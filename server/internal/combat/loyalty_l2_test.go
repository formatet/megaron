package combat

import "testing"

// L2: rout threshold is biased by loyalty — a disloyal army breaks sooner.
func TestRoutFractionForLoyalty_Monotonic(t *testing.T) {
	f1 := routFractionForLoyalty(1)
	f2 := routFractionForLoyalty(2)
	f3 := routFractionForLoyalty(3)
	f4 := routFractionForLoyalty(4)

	if !(f1 > f2 && f2 > f3 && f3 > f4) {
		t.Fatalf("rout fraction must fall as loyalty rises: 1=%.2f 2=%.2f 3=%.2f 4=%.2f", f1, f2, f3, f4)
	}
	if f2 != combatRoutFraction {
		t.Errorf("loyalty 2 must be the baseline %.2f, got %.2f", combatRoutFraction, f2)
	}
	// Unknown loyalties fall back to baseline, never to near-revolt.
	if routFractionForLoyalty(0) != combatRoutFraction || routFractionForLoyalty(5) != combatRoutFraction {
		t.Errorf("unknown loyalty must fall back to baseline %.2f", combatRoutFraction)
	}
}

// L2: silver-shortage desertion severity scales with loyalty.
func TestDesertionStepForLoyalty(t *testing.T) {
	if got := desertionStepForLoyalty(1); got != upkeepDesertionStep*2 {
		t.Errorf("loyalty 1: want %d, got %d", upkeepDesertionStep*2, got)
	}
	if got := desertionStepForLoyalty(2); got != upkeepDesertionStep {
		t.Errorf("loyalty 2 baseline: want %d, got %d", upkeepDesertionStep, got)
	}
	if got := desertionStepForLoyalty(3); got != upkeepDesertionStep {
		t.Errorf("loyalty 3: want %d, got %d", upkeepDesertionStep, got)
	}
	if got := desertionStepForLoyalty(4); got != upkeepDesertionStep/2 {
		t.Errorf("loyalty 4: want %d, got %d", upkeepDesertionStep/2, got)
	}
	// Unknown loyalty must not escalate desertion.
	if got := desertionStepForLoyalty(0); got != upkeepDesertionStep {
		t.Errorf("unknown loyalty must be baseline %d, got %d", upkeepDesertionStep, got)
	}
}
