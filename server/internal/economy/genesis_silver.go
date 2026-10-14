package economy

import "math"

// MetropolisSilver moves only the Host's remaining purse into its new city
// (Timothy 2026-10-14). No top-up for inland, coastal or returning founders.
// The same purse-based ceiling as colonies keeps room for growth and never
// clips what was actually carried: max(60, 2*carried, amount).
func MetropolisSilver(carried float64) (amount, cap float64) {
	amount = math.Max(0, carried)
	return amount, ColonySilverCap(amount, amount)
}

// ColonySilverCap never clips a carried purse or a pre-existing settled stock.
// Founding moves silver; it does not mint another endowment.
func ColonySilverCap(purse, stock float64) float64 {
	return math.Max(60, math.Max(2*purse, stock))
}
