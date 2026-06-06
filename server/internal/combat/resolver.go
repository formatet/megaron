// Package combat implements deterministic combat resolution for Poleia.
// There is no randomness — outcome is a pure function of force strengths.
package combat

import "github.com/poleia/server/internal/province"

// AttackForce describes the attacking side of a battle.
type AttackForce struct {
	Army           province.ArmyComposition
	SupportStrength float64 // strength contributed by supporting allied armies
}

// DefenceForce describes the defending side of a battle.
type DefenceForce struct {
	Army            province.ArmyComposition
	WallLevel       int
	SupportStrength float64
}

// Outcome is the result of a combat resolution.
type Outcome string

const (
	OutcomeAttackerWins Outcome = "attacker_wins"
	OutcomeDefenderWins Outcome = "defender_wins"
)

// CombatResult holds the full outcome of a resolved battle.
type CombatResult struct {
	Outcome         Outcome
	AttackStrength  float64
	DefenceStrength float64
	// Casualties are a proportion of the losing side's army (0.0–1.0).
	AttackerLosses float64
	DefenderLosses float64
}

// WallModifier returns the defensive multiplier for a given wall level.
// Level 0 = 1.0×, each level adds 0.25×.
func WallModifier(level int) float64 {
	return 1.0 + float64(level)*0.25
}

// Strength computes the raw combat strength of an army composition.
// Elite infantry count at ×2; chariots at ×3.
// Priests provide no field strength — they perform rites, not combat.
// Naval-styrka (sjöstrid): war_galley ×3, galley(ship) ×1, merchantman ×0
// (merchantman transporterar — nästan ingen stridskraft).
func Strength(a province.ArmyComposition) float64 {
	return float64(a.Infantry*1 + a.EliteInfantry*2 + a.Chariot*3 +
		a.WarGalley*3 + a.Ship*1)
}

// Resolve calculates the deterministic outcome of an attack.
func Resolve(attack AttackForce, defence DefenceForce) CombatResult {
	attackStr := Strength(attack.Army) + attack.SupportStrength

	defenceStr := (Strength(defence.Army) + defence.SupportStrength) * WallModifier(defence.WallLevel)

	var result CombatResult
	result.AttackStrength = attackStr
	result.DefenceStrength = defenceStr

	if attackStr > defenceStr {
		result.Outcome = OutcomeAttackerWins
		// Attacker loses a fraction proportional to how close the fight was.
		result.AttackerLosses = 0.1 + 0.3*(defenceStr/max(attackStr, 1))
		result.DefenderLosses = 0.5 + 0.4*(attackStr/max(defenceStr, 1))
		if result.DefenderLosses > 1.0 {
			result.DefenderLosses = 1.0
		}
	} else {
		result.Outcome = OutcomeDefenderWins
		result.DefenderLosses = 0.05 + 0.15*(attackStr/max(defenceStr, 1))
		result.AttackerLosses = 0.4 + 0.5*(defenceStr/max(attackStr, 1))
		if result.AttackerLosses > 1.0 {
			result.AttackerLosses = 1.0
		}
	}

	return result
}

func max(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}
