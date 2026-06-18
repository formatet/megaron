// Package combat implements combat resolution for Poleia.
// Resolve() is a pure deterministic function; callers roll fortune once (in the
// event handler) and pass it in. Randomness never lives inside Resolve itself.
package combat

import (
	"math"

	"github.com/poleia/server/internal/province"
)

// AttackForce describes the attacking side of a battle.
type AttackForce struct {
	Army            province.ArmyComposition
	SupportStrength float64
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

// Combat tuning constants (retuned at W8 reseed; change only these, not the formulas).
const (
	combatMaxRounds    = 3    // rounds before a forced decision
	combatRoutFraction = 0.25 // side at ≤25% of effective start strength routes
	roundAttritionBase = 0.30 // base damage fraction per round
	roundAttritionMax  = 0.70 // per-round loss cap (prevents single-round annihilation)
)

// CombatResult holds the full outcome of a resolved battle.
type CombatResult struct {
	Outcome         Outcome
	AttackStrength  float64 // effective attack strength (fortune-adjusted)
	DefenceStrength float64 // effective defence strength (wall-adjusted)
	AttackerLosses  float64 // proportion of effective start strength lost (0.0–1.0)
	DefenderLosses  float64
	AttackerRouted  bool    // attacker dropped to rout threshold; retreats, not destroyed
	DefenderRouted  bool    // defender dropped to rout threshold
	Fortune         float64 // fortune value used (logged in event payload)
	Rounds          int     // rounds the battle lasted
}

// WallModifier returns the defensive multiplier for a given wall level.
// Level 0 = 1.0×, each level adds 0.25×.
func WallModifier(level int) float64 {
	return 1.0 + float64(level)*0.25
}

// Strength computes the raw combat strength of an army composition.
// Multipliers: infantry ×1, elite_infantry ×2, chariot ×3, galley(ship) ×1, war_galley ×3.
// Priest and merchantman contribute 0 field strength.
func Strength(a province.ArmyComposition) float64 {
	return float64(a.Infantry*1 + a.EliteInfantry*2 + a.Chariot*3 +
		a.WarGalley*3 + a.Ship*1)
}

// ResolveStrengths runs multi-round attrition combat given pre-computed effective strengths.
// attStr and defStr must already include fortune (for attacker) and wall modifier (for defender).
// fortune is stored in the result for event logging only; it has no effect on the computation here.
//
// Each round both sides take proportional losses. A side that drops to ≤combatRoutFraction
// of its effective start strength routes: it survives with remaining men and retreats
// rather than being annihilated. If neither routes after combatMaxRounds, higher remaining
// strength wins; ties go to the defender (home advantage).
func ResolveStrengths(attStr, defStr, fortune float64) CombatResult {
	startAtt, startDef := attStr, defStr
	curAtt, curDef := attStr, defStr

	var attRouted, defRouted bool
	rounds := 0

	for rounds < combatMaxRounds && !attRouted && !defRouted {
		rounds++
		ratio := curAtt / maxF(curDef, 0.001)
		defLoss := math.Min(roundAttritionBase*ratio, roundAttritionMax)
		attLoss := math.Min(roundAttritionBase/ratio, roundAttritionMax)
		curDef *= 1 - defLoss
		curAtt *= 1 - attLoss
		attRouted = curAtt <= startAtt*combatRoutFraction
		defRouted = curDef <= startDef*combatRoutFraction
	}

	// Both route simultaneously or neither → defender holds (home advantage).
	outcome := OutcomeDefenderWins
	if defRouted || (!attRouted && curAtt > curDef) {
		outcome = OutcomeAttackerWins
	}

	return CombatResult{
		Outcome:         outcome,
		AttackStrength:  startAtt,
		DefenceStrength: startDef,
		AttackerLosses:  1 - curAtt/maxF(startAtt, 0.001),
		DefenderLosses:  1 - curDef/maxF(startDef, 0.001),
		AttackerRouted:  attRouted && outcome == OutcomeDefenderWins,
		DefenderRouted:  defRouted && outcome == OutcomeAttackerWins,
		Fortune:         fortune,
		Rounds:          rounds,
	}
}

// Resolve is a convenience wrapper that computes effective strengths from force
// compositions and wall level before calling ResolveStrengths.
//
// fortune ∈ [-0.2, +0.2]: positive favours attacker.
// Roll fortune in the event handler (via rollFortune) and pass it here;
// Resolve itself contains no randomness.
func Resolve(attack AttackForce, defence DefenceForce, fortune float64) CombatResult {
	effectiveAtt := (Strength(attack.Army) + attack.SupportStrength) * (1 + fortune)
	effectiveDef := (Strength(defence.Army) + defence.SupportStrength) * WallModifier(defence.WallLevel)
	return ResolveStrengths(effectiveAtt, effectiveDef, fortune)
}

func maxF(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}
