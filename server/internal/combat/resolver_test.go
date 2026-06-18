package combat

import (
	"testing"

	"github.com/poleia/server/internal/province"
)

func TestResolve_AttackerWins(t *testing.T) {
	attack := AttackForce{Army: province.ArmyComposition{Infantry: 100, Chariot: 10}}
	defence := DefenceForce{Army: province.ArmyComposition{Infantry: 20}, WallLevel: 0}

	result := Resolve(attack, defence, 0)
	if result.Outcome != OutcomeAttackerWins {
		t.Errorf("expected attacker wins, got %s (att=%.1f def=%.1f)", result.Outcome, result.AttackStrength, result.DefenceStrength)
	}
}

func TestResolve_DefenderWins(t *testing.T) {
	attack := AttackForce{Army: province.ArmyComposition{Infantry: 10}}
	defence := DefenceForce{Army: province.ArmyComposition{Infantry: 100, Chariot: 20}, WallLevel: 2}

	result := Resolve(attack, defence, 0)
	if result.Outcome != OutcomeDefenderWins {
		t.Errorf("expected defender wins, got %s (att=%.1f def=%.1f)", result.Outcome, result.AttackStrength, result.DefenceStrength)
	}
}

func TestResolve_WallModifierHelps(t *testing.T) {
	army := province.ArmyComposition{Infantry: 50}
	noWall := Resolve(
		AttackForce{Army: province.ArmyComposition{Infantry: 60}},
		DefenceForce{Army: army, WallLevel: 0},
		0,
	)
	withWall := Resolve(
		AttackForce{Army: province.ArmyComposition{Infantry: 60}},
		DefenceForce{Army: army, WallLevel: 3},
		0,
	)
	if withWall.DefenceStrength <= noWall.DefenceStrength {
		t.Errorf("walls should increase defence strength: no wall=%.1f, wall=%.1f", noWall.DefenceStrength, withWall.DefenceStrength)
	}
}

func TestResolve_EliteReducesWallEffectively(t *testing.T) {
	defence := DefenceForce{Army: province.ArmyComposition{Infantry: 40}, WallLevel: 3}
	plainAttack := Resolve(AttackForce{Army: province.ArmyComposition{Infantry: 50}}, defence, 0)
	eliteAttack := Resolve(AttackForce{Army: province.ArmyComposition{EliteInfantry: 50}}, defence, 0)

	if eliteAttack.AttackStrength <= plainAttack.AttackStrength {
		t.Errorf("elite infantry should have higher attack strength than plain infantry")
	}
}

func TestResolve_EqualForcesDefenderWins(t *testing.T) {
	army := province.ArmyComposition{Infantry: 50}
	result := Resolve(
		AttackForce{Army: army},
		DefenceForce{Army: army, WallLevel: 0},
		0,
	)
	if result.Outcome != OutcomeDefenderWins {
		t.Error("equal forces should result in defender victory")
	}
}

func TestWallModifier(t *testing.T) {
	cases := [][2]float64{{0, 1.0}, {1, 1.25}, {2, 1.5}, {3, 1.75}}
	for _, c := range cases {
		if got := WallModifier(int(c[0])); got != c[1] {
			t.Errorf("WallModifier(%d) = %.2f, want %.2f", int(c[0]), got, c[1])
		}
	}
}

func TestStrength(t *testing.T) {
	a := province.ArmyComposition{Infantry: 10, EliteInfantry: 5, Chariot: 4, Priest: 3, Ship: 2}
	want := float64(10*1 + 5*2 + 4*3 + 2*1) // 34
	if got := Strength(a); got != want {
		t.Errorf("Strength = %.0f, want %.0f", got, want)
	}
	if got := Strength(province.ArmyComposition{Priest: 9, Merchantman: 9}); got != 0 {
		t.Errorf("priest/merchantman should give 0 combat strength, got %.0f", got)
	}
}

func TestStrength_NavalOrder(t *testing.T) {
	wg := Strength(province.ArmyComposition{WarGalley: 1})
	g := Strength(province.ArmyComposition{Ship: 1})
	m := Strength(province.ArmyComposition{Merchantman: 1})
	if !(wg > g && g > m) {
		t.Errorf("expected war_galley(%.0f) > galley(%.0f) > merchantman(%.0f)", wg, g, m)
	}
}

func TestWallModifierChariot(t *testing.T) {
	noWall := Resolve(
		AttackForce{Army: province.ArmyComposition{Chariot: 10}},
		DefenceForce{Army: province.ArmyComposition{Infantry: 20}, WallLevel: 0},
		0,
	)
	withWall := Resolve(
		AttackForce{Army: province.ArmyComposition{Chariot: 10}},
		DefenceForce{Army: province.ArmyComposition{Infantry: 20}, WallLevel: 3},
		0,
	)
	if withWall.DefenceStrength <= noWall.DefenceStrength {
		t.Errorf("walls should increase defence strength against chariot attack")
	}
}

// ── W5: fortune + multi-round + rout ─────────────────────────────────────────

func TestResolve_FortuneStored(t *testing.T) {
	army := province.ArmyComposition{Infantry: 50}
	result := Resolve(AttackForce{Army: army}, DefenceForce{Army: army, WallLevel: 0}, 0.15)
	if result.Fortune != 0.15 {
		t.Errorf("Fortune not stored in result: got %.3f, want 0.150", result.Fortune)
	}
}

func TestResolve_PositiveFortuneFavoursAttacker(t *testing.T) {
	// Equal armies: with fortune=0 defender wins; with fortune=+0.2 attacker should win more often.
	army := province.ArmyComposition{Infantry: 50}
	neutral := Resolve(AttackForce{Army: army}, DefenceForce{Army: army}, 0)
	favoured := Resolve(AttackForce{Army: army}, DefenceForce{Army: army}, 0.2)

	if neutral.Outcome != OutcomeDefenderWins {
		t.Error("equal forces, zero fortune should be defender win")
	}
	if favoured.Outcome != OutcomeAttackerWins {
		t.Error("equal forces, max positive fortune should be attacker win")
	}
}

func TestResolve_NegativeFortuneFavoursDefender(t *testing.T) {
	// Slight attacker advantage nullified by max negative fortune.
	attack := AttackForce{Army: province.ArmyComposition{Infantry: 60}}
	defence := DefenceForce{Army: province.ArmyComposition{Infantry: 55}, WallLevel: 0}

	withoutFortune := Resolve(attack, defence, 0)
	withBadFortune := Resolve(attack, defence, -0.2)

	if withoutFortune.Outcome != OutcomeAttackerWins {
		t.Error("slight attacker advantage, no fortune should be attacker win")
	}
	if withBadFortune.Outcome != OutcomeDefenderWins {
		t.Error("slight attacker advantage, max negative fortune should flip to defender win")
	}
}

func TestResolve_RoutedSideHasLossesBelow100(t *testing.T) {
	// Overwhelming attacker: defender should route, not be 100% destroyed.
	result := Resolve(
		AttackForce{Army: province.ArmyComposition{Infantry: 200}},
		DefenceForce{Army: province.ArmyComposition{Infantry: 50}},
		0,
	)
	if result.Outcome != OutcomeAttackerWins {
		t.Fatalf("overwhelming attacker should win, got %s", result.Outcome)
	}
	if !result.DefenderRouted {
		t.Error("weak defender should be routed (not merely outfought after max rounds)")
	}
	if result.DefenderLosses >= 1.0 {
		t.Errorf("routed defender should have <100%% losses, got %.2f", result.DefenderLosses)
	}
}

func TestResolve_AttackerRoutedOnLoss(t *testing.T) {
	// Overwhelming defender: attacker should route.
	result := Resolve(
		AttackForce{Army: province.ArmyComposition{Infantry: 30}},
		DefenceForce{Army: province.ArmyComposition{Infantry: 200}},
		0,
	)
	if result.Outcome != OutcomeDefenderWins {
		t.Fatalf("overwhelming defender should win, got %s", result.Outcome)
	}
	if !result.AttackerRouted {
		t.Error("weak attacker should be routed")
	}
	if result.AttackerLosses >= 1.0 {
		t.Errorf("routed attacker should have <100%% losses, got %.2f", result.AttackerLosses)
	}
}

func TestResolve_RoundsRecorded(t *testing.T) {
	result := Resolve(
		AttackForce{Army: province.ArmyComposition{Infantry: 100}},
		DefenceForce{Army: province.ArmyComposition{Infantry: 100}},
		0,
	)
	if result.Rounds < 1 || result.Rounds > combatMaxRounds {
		t.Errorf("expected rounds in [1, %d], got %d", combatMaxRounds, result.Rounds)
	}
}

func TestRollFortune_InRange(t *testing.T) {
	for i := 0; i < 500; i++ {
		f := rollFortune(1000, 500)
		if f < -0.2 || f > 0.2 {
			t.Errorf("rollFortune out of [-0.2, 0.2]: got %.4f", f)
		}
	}
}
