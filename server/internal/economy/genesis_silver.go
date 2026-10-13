package economy

import "math"

// Fixed starting reserves (Timothy 2026-10-13), independent of current payroll,
// population or prices. Returning hosts only move their remaining purse.
const (
	MetropolisSilverInland  = 730.0
	MetropolisSilverCoastal = 1278.0
)

func MetropolisSilver(coastal bool, carried float64, refugee bool) (amount, cap float64) {
	endowment := MetropolisSilverInland
	if coastal {
		endowment = MetropolisSilverCoastal
	}
	amount = math.Max(0, carried)
	if !refugee {
		amount = math.Max(amount, endowment)
	}
	return amount, math.Max(2*endowment, amount)
}

// ColonySilverCap never clips a carried purse or a pre-existing settled stock.
// Founding moves silver; it does not mint another endowment.
func ColonySilverCap(purse, stock float64) float64 {
	return math.Max(60, math.Max(2*purse, stock))
}
