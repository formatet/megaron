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

func TestStrength(t *testing.T) {
	// Elite ×2, cavalry ×3, infantry ×1.
	// Naval: war_galley ×3, galley(ship) ×1; merchantman og catapult ger 0 i strid.
	// Priests ger noll stridsstyrka.
	a := province.ArmyComposition{Infantry: 10, EliteInfantry: 5, Cavalry: 4, Priest: 3, Ship: 2, Catapult: 7}
	want := float64(10*1 + 5*2 + 4*3 + 2*1) // 34 (galley×1 ingår nu)
	if got := Strength(a); got != want {
		t.Errorf("Strength = %.0f, want %.0f", got, want)
	}
	// Catapults, priests, och merchantman ger 0 stridsstyrka.
	if got := Strength(province.ArmyComposition{Priest: 9, Catapult: 9, Merchantman: 9}); got != 0 {
		t.Errorf("präst/katapult/merchantman ska ge 0 stridsstyrka, got %.0f", got)
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

func TestCatapultEffect(t *testing.T) {
	cases := []struct {
		catapults, wall, want int
	}{
		{0, 3, 3},  // no catapults — walls intact
		{1, 3, 3},  // one catapult — needs two per level
		{2, 3, 2},  // two catapults reduce one level
		{4, 3, 1},  // four catapults reduce two levels
		{10, 1, 0}, // never below zero
	}
	for _, c := range cases {
		if got := CatapultEffect(c.catapults, c.wall); got != c.want {
			t.Errorf("CatapultEffect(%d, %d) = %d, want %d", c.catapults, c.wall, got, c.want)
		}
	}
}
