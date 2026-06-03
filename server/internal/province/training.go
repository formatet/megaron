package province

import "time"

// UnitSpec defines the cost and time to train a unit.
// All material costs are expressed as good_key → amount and deducted from
// settlement_goods. CostKharis is deducted from the settlements.kharis column.
// PopCost is the number of citizens consumed per unit trained (minimum population floor: 50).
type UnitSpec struct {
	Costs            map[string]float64 // good_key → quantity deducted from settlement_goods
	CostKharis       float64
	PopCost          int           // citizens consumed per unit trained
	Duration         time.Duration // per unit trained
	RequiresBarracks bool
	RequiresHarbour  bool
	RequiresFoundry  bool
}

// UnitSpecs is the canonical catalogue of all trainable unit types.
var UnitSpecs = map[string]UnitSpec{
	"infantry":       {Costs: map[string]float64{"grain": 15}, PopCost: 5, Duration: time.Minute, RequiresBarracks: true},
	"cavalry":        {Costs: map[string]float64{"grain": 30, "timber": 5}, PopCost: 8, Duration: 4 * time.Minute, RequiresBarracks: true},
	"catapult":       {Costs: map[string]float64{"cedar": 100}, PopCost: 2, Duration: 30 * time.Minute, RequiresBarracks: true},
	"priest":         {Costs: map[string]float64{"grain": 15}, PopCost: 3, Duration: 60 * time.Minute},
	"ship":           {Costs: map[string]float64{"cedar": 110}, PopCost: 10, Duration: 45 * time.Minute, RequiresHarbour: true},
	"elite_infantry": {Costs: map[string]float64{"grain": 25, "bronze": 2}, PopCost: 10, Duration: 5 * time.Minute, RequiresBarracks: true, RequiresFoundry: true},
}
