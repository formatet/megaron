package economy

// ProductionRule mirrors a row from the production_rules table.
type ProductionRule struct {
	TerrainType     *string // nil = any terrain
	BuildingType    *string // nil = no building required
	GoodKey         string
	RatePerMin      float64
	RequiresDeposit *string // nil | "copper" | "tin"
	RequiresCoastal bool    // true = only produced at coastal settlements
}

// effectiveRates applies a rule set to a given terrain/buildings/deposit context and
// returns base_potential per good key. Used only in tests (bronze-chain geography checks);
// production code goes through RecomputeProduction which queries the DB directly.
func effectiveRates(rules []ProductionRule, terrain string, buildings []string, hasCopper, hasTin bool) map[string]float64 {
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

// LaborRates computes production rates using the citizen-allocation formula:
//
//	yield_per_worker(g) = base_potential(g) / REF_LABOR
//	rate(g)             = yield_per_worker(g) × citizens(g)
//
// baseRates maps good_key → base_potential (SUM of matching production_rules).
// citizens maps good_key → number of citizens allocated to that good.
// Goods absent from citizens get rate 0.
// laborPool is accepted but unused (kept for backward compatibility with tests).
func LaborRates(baseRates map[string]float64, citizens map[string]float64, _ float64) map[string]float64 {
	result := make(map[string]float64, len(baseRates))
	for good, base := range baseRates {
		c := citizens[good]
		result[good] = (base / REF_LABOR) * c
	}
	return result
}

