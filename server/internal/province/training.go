package province

import "time"

// UnitSpec defines the cost and time to train a unit.
type UnitSpec struct {
	CostFood   float64
	CostLumber float64
	CostKharis float64
	CostBronze float64
	Duration   time.Duration // per unit trained
	// RequiresBarracks: unit cannot be trained without a barracks building.
	RequiresBarracks bool
	// RequiresHarbour: unit cannot be trained without a harbour.
	RequiresHarbour bool
	// RequiresFoundry: unit cannot be trained without a foundry.
	RequiresFoundry bool
}

// UnitSpecs is the canonical catalogue of all trainable unit types.
var UnitSpecs = map[string]UnitSpec{
	"infantry":       {CostFood: 15, Duration: time.Minute, RequiresBarracks: true},
	"cavalry":        {CostFood: 30, CostLumber: 5, Duration: 4 * time.Minute, RequiresBarracks: true},
	"catapult":       {CostLumber: 100, Duration: 30 * time.Minute, RequiresBarracks: true},
	"priest":         {CostFood: 15, Duration: 60 * time.Minute},
	"ship":           {CostLumber: 110, Duration: 45 * time.Minute, RequiresHarbour: true},
	"elite_infantry": {CostFood: 25, CostBronze: 2, Duration: 5 * time.Minute, RequiresBarracks: true, RequiresFoundry: true},
}
