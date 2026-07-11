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

// L2: passing the baseline fraction for both sides must reproduce ResolveStrengths
// exactly, so existing balance/tests are untouched by the new per-side path.
func TestResolveStrengthsWithRout_BaselineMatchesLegacy(t *testing.T) {
	cases := []struct{ att, def, fortune float64 }{
		{100, 100, 0}, {150, 90, 0.2}, {80, 120, -0.2}, {200, 50, 0},
	}
	for _, c := range cases {
		legacy := ResolveStrengths(c.att, c.def, c.fortune)
		biased := ResolveStrengthsWithRout(c.att, c.def, c.fortune, combatRoutFraction, combatRoutFraction)
		if legacy != biased {
			t.Errorf("baseline rout must match legacy for %+v:\n legacy=%+v\n biased=%+v", c, legacy, biased)
		}
	}
}

// L2: in the same losing matchup a low-loyalty attacker routs sooner (fewer
// rounds, fewer casualties as it retreats earlier) than a fanatical one.
func TestResolveStrengthsWithRout_LowLoyaltyRoutsSooner(t *testing.T) {
	const att, def = 80.0, 120.0 // attacker is the weaker side

	low := ResolveStrengthsWithRout(att, def, 0, routFractionForLoyalty(1), routFractionForLoyalty(2))
	high := ResolveStrengthsWithRout(att, def, 0, routFractionForLoyalty(4), routFractionForLoyalty(2))

	if low.Outcome != OutcomeDefenderWins || high.Outcome != OutcomeDefenderWins {
		t.Fatalf("weaker attacker should lose both: low=%s high=%s", low.Outcome, high.Outcome)
	}
	if !low.AttackerRouted {
		t.Errorf("low-loyalty attacker should rout, got %+v", low)
	}
	if low.Rounds > high.Rounds {
		t.Errorf("low-loyalty attacker should rout no later: low=%d high=%d rounds", low.Rounds, high.Rounds)
	}
	if low.AttackerLosses > high.AttackerLosses {
		t.Errorf("routing earlier should cost no more men: low=%.3f high=%.3f", low.AttackerLosses, high.AttackerLosses)
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
