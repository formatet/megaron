package settlement

// ColonyPenalty returns the loyalty modifier per day for a settlement based on
// how many colonies the owner controls. Penalties stack with other modifiers.
func ColonyPenalty(colonyCount int) int {
	switch {
	case colonyCount <= 2:
		return 0
	case colonyCount == 3:
		return -1
	case colonyCount == 4:
		return -3
	default: // 5+
		return -5
	}
}

// RevoltConditionsMet returns true when a settlement meets all three revolt
// conditions simultaneously: loyalty rock-bottom, garrison dominated by non-owner
// troops, and a trigger event has occurred.
func RevoltConditionsMet(loyalty int, ownerTroopFraction float64, triggerOccurred bool) bool {
	return loyalty == 1 && ownerTroopFraction < 0.5 && triggerOccurred
}

// LoyaltyProjection computes current loyalty by replaying loyalty events.
// baseLevel is the starting loyalty at settlement founding (typically 2).
func LoyaltyProjection(baseLevel int, deltas []int) int {
	v := baseLevel
	for _, d := range deltas {
		v += d
	}
	if v < 1 {
		return 1
	}
	if v > 4 {
		return 4
	}
	return v
}
