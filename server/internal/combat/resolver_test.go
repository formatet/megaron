package combat

import (
	"testing"

	"github.com/poleia/server/internal/province"
)

func TestResolve_AttackerWins(t *testing.T) {
	attack := AttackForce{Army: province.ArmyComposition{Infantry: 100, Cavalry: 10}}
	defence := DefenceForce{Army: province.ArmyComposition{Infantry: 20}, WallLevel: 0}

	result := Resolve(attack, defence)
	if result.Outcome != OutcomeAttackerWins {
		t.Errorf("expected attacker wins, got %s (att=%.1f def=%.1f)", result.Outcome, result.AttackStrength, result.DefenceStrength)
	}
}

func TestResolve_DefenderWins(t *testing.T) {
	attack := AttackForce{Army: province.ArmyComposition{Infantry: 10}}
	defence := DefenceForce{Army: province.ArmyComposition{Infantry: 100, Cavalry: 20}, WallLevel: 2}

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

func TestResolve_CatapultsReduceWalls(t *testing.T) {
	// Same attacker with catapults vs strong walls should do better than without.
	defence := DefenceForce{Army: province.ArmyComposition{Infantry: 40}, WallLevel: 3}
	noCatapult := Resolve(AttackForce{Army: province.ArmyComposition{Infantry: 50}}, defence)
	withCatapult := Resolve(AttackForce{Army: province.ArmyComposition{Infantry: 50, Catapult: 4}}, defence)

	if withCatapult.DefenceStrength >= noCatapult.DefenceStrength {
		t.Errorf("catapults should reduce effective wall defence")
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
