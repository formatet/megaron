package province

import "time"

// UnitSpec defines the cost and time to train a unit.
type UnitSpec struct {
	CostFood   float64
	CostIron   float64
	CostLumber float64
	CostKharis float64
	Duration   time.Duration // per unit trained
	// RequiresBarracks: unit cannot be trained without a barracks building.
	RequiresBarracks bool
	// RequiresHarbour: unit cannot be trained without a harbour.
	RequiresHarbour bool
}

// UnitSpecs is the canonical catalogue of all trainable unit types.
var UnitSpecs = map[string]UnitSpec{
	"infantry": {CostFood: 10, CostIron: 5, Duration: time.Minute, RequiresBarracks: true},
	"cavalry":  {CostFood: 25, CostIron: 15, Duration: 4 * time.Minute, RequiresBarracks: true},
	"catapult": {CostLumber: 60, CostIron: 40, Duration: 30 * time.Minute, RequiresBarracks: true},
	"priest":   {CostKharis: 50, Duration: 60 * time.Minute},
	"ship":     {CostLumber: 80, CostIron: 30, Duration: 45 * time.Minute, RequiresHarbour: true},
}
