package handlers

import (
	"fmt"
	"math"
	"sort"

	"formatet/megaron/server/internal/economy"
	"formatet/megaron/server/internal/province"
	"formatet/megaron/server/internal/unit"
)

func recruitBatchMen(kind string) int {
	if unit.CategoryOf(unit.Type(kind)) == unit.CategoryNaval {
		return unit.CrewFor(unit.Type(kind))
	}
	return economy.MaxUnitSize
}

// Read-only explanation of the same one-action gates Recruit applies. Population
// must leave one resident; PopCost is legacy catalogue metadata, not that gate.
func recruitReasons(kind string, population int, buildings map[string]bool, stock map[string]float64, pendingShips int) []string {
	spec := province.UnitSpecs[kind]
	batch := recruitBatchMen(kind)
	reasons := []string{}
	if population <= batch {
		reasons = append(reasons, fmt.Sprintf("needs %d citizens (have %d; leave 1 in the city)", batch, population))
	}
	for _, b := range []struct {
		name   string
		needed bool
	}{{"barracks", spec.RequiresBarracks}, {"stable", spec.RequiresStable}, {"harbour", spec.RequiresHarbour}, {"shipyard", spec.RequiresShipyard}, {"foundry", spec.RequiresFoundry}} {
		if b.needed && !buildings[b.name] {
			reasons = append(reasons, "needs a "+b.name)
		}
	}
	keys := make([]string, 0, len(spec.Costs))
	for g := range spec.Costs {
		keys = append(keys, g)
	}
	sort.Strings(keys)
	for _, g := range keys {
		need := spec.Costs[g] * float64(batch)
		have := stock[g]
		label := g
		if g == "timber" {
			have += math.Max(0, stock["cedar"]-spec.Costs["cedar"]*float64(batch))
			label = "timber (or cedar)"
		}
		if !province.GoodsAffordable(have, need) {
			reasons = append(reasons, fmt.Sprintf("needs %g %s (have %g)", need, label, have))
		}
	}
	if unit.CategoryOf(unit.Type(kind)) == unit.CategoryNaval && pendingShips >= 10 {
		reasons = append(reasons, "shipyard queue full (10 vessels); wait for a ship to finish or cancel a build")
	}
	return reasons
}
