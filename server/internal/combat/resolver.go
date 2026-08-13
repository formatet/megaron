// Package combat implements combat resolution for Megaron.
//
// The live field/settlement battle model is battle.go's seeded T12 dice system
// (KR3, megaron_plan_kr3_stridssystem.md), reached through initiateOrJoinBattle.
// The old one-shot strength/fortune/wall resolver (Resolve/ResolveStrengths/
// WallModifier/Strength + fortune.go's rollFortune) was removed once every entry
// point had cut over. Only the loyalty→rout bias below outlived that path: it is
// still consumed by battle.go.
package combat

// combatRoutFraction is the baseline rout threshold: a side at or below this
// fraction of its effective start strength breaks. Loyalty 2 (starting loyalty)
// is the baseline; routFractionForLoyalty biases it. Calibration, not invariant.
const combatRoutFraction = 0.25

// routFractionForLoyalty (L2) biases the rout threshold by the loyalty of the
// settlement supplying an army: a disloyal army breaks sooner (routs at a
// HIGHER remaining-strength fraction), a fanatical one holds longer. Folded in
// deterministically like fortune — the roll still happens once in the handler.
// Calibration, not invariants; loyalty 2 (starting loyalty) is the baseline.
func routFractionForLoyalty(loyalty int) float64 {
	switch loyalty {
	case 1:
		return 0.35 // near-revolt: breaks early
	case 3:
		return 0.20 // devoted: holds
	case 4:
		return 0.15 // fanatical: holds hardest
	default:
		return combatRoutFraction // loyalty 2 (and unknown) = 0.25
	}
}
