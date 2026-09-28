package handlers

// province.HexBoundBuildings and economy.HexBoundBuildingTypes are two
// copies of the SAME set (economy may not import province, G1) — this test
// is their sync check, run from the one package allowed to import both.
// Add a hex-bound building type → add it to BOTH lists, or this goes red.

import (
	"testing"

	"formatet/megaron/server/internal/economy"
	"formatet/megaron/server/internal/province"
)

func TestHexBoundBuildingTypes_MirrorsProvince(t *testing.T) {
	for bt := range province.HexBoundBuildings {
		if !economy.HexBoundBuildingTypes[string(bt)] {
			t.Errorf("province.HexBoundBuildings has %q, economy.HexBoundBuildingTypes does not", bt)
		}
	}
	for bt := range economy.HexBoundBuildingTypes {
		if !province.HexBoundBuildings[province.BuildingType(bt)] {
			t.Errorf("economy.HexBoundBuildingTypes has %q, province.HexBoundBuildings does not", bt)
		}
	}
}
