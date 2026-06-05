package economy

// ProductionRule mirrors a row from the production_rules table.
type ProductionRule struct {
	TerrainType     *string // nil = any terrain
	BuildingType    *string // nil = no building required
	GoodKey         string
	RatePerMin      float64
	RequiresDeposit *string // nil | "copper" | "tin"
}

// LaborRates computes production rates using the labor-allocation formula:
//
//	rate(g) = base_potential(g) × weight(g) × laborPool / REF_LABOR
//
// baseRates is the output of EffectiveRates (base_potential per good).
// weights maps good_key → allocation weight (Σ should = 1.0 over producible goods).
// laborPool is max(0, population − army_pop − transit_pop).
// Goods absent from weights get rate 0.
func LaborRates(baseRates map[string]float64, weights map[string]float64, laborPool float64) map[string]float64 {
	result := make(map[string]float64, len(baseRates))
	for good, base := range baseRates {
		w := weights[good]
		result[good] = base * w * laborPool / REF_LABOR
	}
	return result
}

// EffectiveRates returns the combined production rate per good key for a settlement.
// terrain is the province terrain type; buildings is the list of completed building types;
// hasCopper/hasTin are the province deposit flags.
func EffectiveRates(rules []ProductionRule, terrain string, buildings []string, hasCopper, hasTin bool) map[string]float64 {
	built := make(map[string]bool, len(buildings))
	for _, b := range buildings {
		built[b] = true
	}

	rates := make(map[string]float64)
	for _, r := range rules {
		if r.TerrainType != nil && *r.TerrainType != terrain {
			continue
		}
		if r.BuildingType != nil && !built[*r.BuildingType] {
			continue
		}
		if r.RequiresDeposit != nil {
			switch *r.RequiresDeposit {
			case "copper":
				if !hasCopper {
					continue
				}
			case "tin":
				if !hasTin {
					continue
				}
			}
		}
		rates[r.GoodKey] += r.RatePerMin
	}
	return rates
}
