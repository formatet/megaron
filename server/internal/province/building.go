package province

import "time"

// BuildingSpec defines the cost and effect of constructing a building.
// All material costs are expressed as good_key → amount and deducted from
// settlement_goods. Gold is the only currency that comes from the settlements
// column directly (CostGold).
type BuildingSpec struct {
	Costs      map[string]float64 // good_key → quantity deducted from settlement_goods
	CostGold   float64            // gold deducted from settlements.gold_amount
	Duration   time.Duration
	GoldRate   float64 // added to settlements.gold_rate when complete
	KharisRate float64 // added to settlements.kharis_rate when complete
	WallsBonus int     // added to settlements.wall_level (capped at 3)
}

// BuildingSpecs is the canonical catalogue of all constructable buildings.
// Rate bonuses for goods (grain, cedar, stone, etc.) are registered as
// production_rules rows and applied by BuildCompleteHandler via the UPSERT
// on settlement_goods — they are NOT in BuildingSpec.
var BuildingSpecs = map[BuildingType]BuildingSpec{
	BuildingFarm:        {Costs: map[string]float64{"timber": 50, "stone": 20}, Duration: 30 * time.Minute},
	BuildingBarracks:    {Costs: map[string]float64{"timber": 80, "stone": 80}, Duration: 60 * time.Minute},
	BuildingMine:        {Costs: map[string]float64{"timber": 60, "stone": 40}, Duration: 45 * time.Minute},
	BuildingLumbermill:  {Costs: map[string]float64{"timber": 40, "stone": 40}, Duration: 30 * time.Minute},
	BuildingStonequarry: {Costs: map[string]float64{"timber": 50, "stone": 20}, Duration: 30 * time.Minute},
	BuildingMarket:      {Costs: map[string]float64{"timber": 100, "stone": 60}, Duration: 90 * time.Minute, GoldRate: 0.5},
	BuildingWall:        {Costs: map[string]float64{"timber": 40, "stone": 160}, Duration: 120 * time.Minute, WallsBonus: 1},
	BuildingTower:       {Costs: map[string]float64{"timber": 60, "stone": 110}, Duration: 90 * time.Minute, WallsBonus: 1},
	BuildingHarbour:     {Costs: map[string]float64{"timber": 140, "stone": 60}, Duration: 90 * time.Minute, GoldRate: 0.3},
	BuildingFoundry:     {Costs: map[string]float64{"timber": 80, "stone": 100}, Duration: 90 * time.Minute},
	BuildingStable:      {Costs: map[string]float64{"timber": 60, "stone": 40}, Duration: 60 * time.Minute},
	BuildingBronzeWall:  {Costs: map[string]float64{"stone": 100, "bronze": 10}, Duration: 180 * time.Minute, WallsBonus: 1},
	BuildingOlivePress:  {Costs: map[string]float64{"stone": 40, "timber": 30}, Duration: 45 * time.Minute},
	BuildingWinery:      {Costs: map[string]float64{"stone": 30, "timber": 40}, Duration: 45 * time.Minute},
}
