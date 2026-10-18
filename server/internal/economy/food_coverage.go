package economy

// FoodNeedPerTick is the population's single grain/fish ration.
func FoodNeedPerTick(population int) float64 {
	if population < 0 {
		population = 0
	}
	return float64(population) * GrainConsumptionPerCitizenPerTick
}

// FoodCoverageTicks is city grain+fish stock divided by that ration.
// Production and army grain are reported separately, not projected into stock.
func FoodCoverageTicks(foodStock float64, population int) float64 {
	need := FoodNeedPerTick(population)
	if need <= 0 {
		return 0
	}
	return foodStock / need
}
