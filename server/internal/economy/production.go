package economy

// ProductionRule mirrors a row from the production_rules table.
type ProductionRule struct {
	TerrainType     *string // nil = any terrain
	BuildingType    *string // nil = no building required
	GoodKey         string
	RatePerMin      float64
	RequiresDeposit *string // nil | "copper" | "tin"
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
