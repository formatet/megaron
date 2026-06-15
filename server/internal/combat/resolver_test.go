package combat

import (
	"testing"

	"github.com/poleia/server/internal/province"
)

func TestResolve_AttackerWins(t *testing.T) {
	attack := AttackForce{Army: province.ArmyComposition{Infantry: 100, Chariot: 10}}
	defence := DefenceForce{Army: province.ArmyComposition{Infantry: 20}, WallLevel: 0}

	result := Resolve(attack, defence)
	if result.Outcome != OutcomeAttackerWins {
		t.Errorf("expected attacker wins, got %s (att=%.1f def=%.1f)", result.Outcome, result.AttackStrength, result.DefenceStrength)
	}
}

func TestResolve_DefenderWins(t *testing.T) {
	attack := AttackForce{Army: province.ArmyComposition{Infantry: 10}}
	defence := DefenceForce{Army: province.ArmyComposition{Infantry: 100, Chariot: 20}, WallLevel: 2}

	result := Resolve(attack, defence)
	if result.Outcome != OutcomeDefenderWins {
		t.Errorf("expected defender wins, got %s (att=%.1f def=%.1f)", result.Outcome, result.AttackStrength, result.DefenceStrength)
	}
}

func TestResolve_WallModifierHelps(t *testing.T) {
	army := province.ArmyComposition{Infantry: 50}
	noWall := Resolve(
		AttackForce{Army: province.ArmyComposition{Infantry: 60}},
		DefenceForce{Army: army, WallLevel: 0},
	)
	withWall := Resolve(
		AttackForce{Army: province.ArmyComposition{Infantry: 60}},
		DefenceForce{Army: army, WallLevel: 3},
	)
	if withWall.DefenceStrength <= noWall.DefenceStrength {
		t.Errorf("walls should increase defence strength: no wall=%.1f, wall=%.1f", noWall.DefenceStrength, withWall.DefenceStrength)
	}
}

func TestResolve_EliteReducesWallEffectively(t *testing.T) {
	// Elite infantry (×2) vs strong walls: attacker with elites should do better than plain infantry.
	defence := DefenceForce{Army: province.ArmyComposition{Infantry: 40}, WallLevel: 3}
	plainAttack := Resolve(AttackForce{Army: province.ArmyComposition{Infantry: 50}}, defence)
	eliteAttack := Resolve(AttackForce{Army: province.ArmyComposition{EliteInfantry: 50}}, defence)

	if eliteAttack.AttackStrength <= plainAttack.AttackStrength {
		t.Errorf("elite infantry should have higher attack strength than plain infantry")
	}
}

func TestResolve_EqualForcesDefenderWins(t *testing.T) {
	// Tie goes to defender (attackStr > defenceStr required for attacker win)
	army := province.ArmyComposition{Infantry: 50}
	result := Resolve(
		AttackForce{Army: army},
		DefenceForce{Army: army, WallLevel: 0},
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
	// Elite ×2, chariot ×3, infantry ×1.
	// Naval: war_galley ×3, galley(ship) ×1; merchantman gives 0 in combat.
	// Priests give zero field strength.
	a := province.ArmyComposition{Infantry: 10, EliteInfantry: 5, Chariot: 4, Priest: 3, Ship: 2}
	want := float64(10*1 + 5*2 + 4*3 + 2*1) // 34 (galley×1 included)
	if got := Strength(a); got != want {
		t.Errorf("Strength = %.0f, want %.0f", got, want)
	}
	// Priests and merchantman give zero combat strength.
	if got := Strength(province.ArmyComposition{Priest: 9, Merchantman: 9}); got != 0 {
		t.Errorf("priest/merchantman should give 0 combat strength, got %.0f", got)
	}
}

func TestStrength_NavalOrder(t *testing.T) {
	// Sjöstridsstyrka: war_galley > galley > merchantman(0)
	wg := Strength(province.ArmyComposition{WarGalley: 1})   // 3
	g := Strength(province.ArmyComposition{Ship: 1})          // 1
	m := Strength(province.ArmyComposition{Merchantman: 1})   // 0
	if !(wg > g && g > m) {
		t.Errorf("förväntad ordning war_galley(%.0f) > galley(%.0f) > merchantman(%.0f)", wg, g, m)
	}
}

func TestWallModifierChariot(t *testing.T) {
	// Chariot (×3) attacker vs walled defender: verify wall modifier is applied.
	noWall := Resolve(
		AttackForce{Army: province.ArmyComposition{Chariot: 10}},
		DefenceForce{Army: province.ArmyComposition{Infantry: 20}, WallLevel: 0},
	)
	withWall := Resolve(
		AttackForce{Army: province.ArmyComposition{Chariot: 10}},
		DefenceForce{Army: province.ArmyComposition{Infantry: 20}, WallLevel: 3},
	)
	if withWall.DefenceStrength <= noWall.DefenceStrength {
		t.Errorf("walls should increase defence strength against chariot attack")
	}
}
