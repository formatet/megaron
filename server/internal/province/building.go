package province

import "time"

// BuildingSpec defines the cost and effect of constructing a building.
// All material costs are expressed as good_key → amount and deducted from
// settlement_goods. CostSilver is deducted from the settlement_goods silver row.
type BuildingSpec struct {
	Costs      map[string]float64 // good_key → quantity deducted from settlement_goods
	CostSilver float64            // silver deducted from settlement_goods (good_key='silver')
	Duration   time.Duration
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
	BuildingSilverMine:  {Costs: map[string]float64{"timber": 60, "stone": 40}, Duration: 45 * time.Minute},
	BuildingLumbermill:  {Costs: map[string]float64{"timber": 40, "stone": 40}, Duration: 30 * time.Minute},
	BuildingStonequarry: {Costs: map[string]float64{"timber": 50, "stone": 20}, Duration: 30 * time.Minute},
	BuildingMarket:      {Costs: map[string]float64{"timber": 100, "stone": 60}, Duration: 90 * time.Minute},
	BuildingWall:        {Costs: map[string]float64{"timber": 50, "stone": 60}, Duration: 60 * time.Minute, WallsBonus: 1},
	BuildingHarbour:     {Costs: map[string]float64{"timber": 140, "stone": 60}, Duration: 90 * time.Minute},
	BuildingFoundry:     {Costs: map[string]float64{"timber": 80, "stone": 100}, Duration: 90 * time.Minute},
	BuildingStable:      {Costs: map[string]float64{"timber": 60, "stone": 40}, Duration: 60 * time.Minute},
	BuildingTemple:      {Costs: map[string]float64{"timber": 60, "stone": 60}, Duration: 60 * time.Minute},
	BuildingOlivePress:  {Costs: map[string]float64{"stone": 40, "timber": 30}, Duration: 45 * time.Minute},
	BuildingWinery:      {Costs: map[string]float64{"stone": 30, "timber": 40}, Duration: 45 * time.Minute},
}

// WallLevelSpecs ger kostnad/duration för nästa murnivå (1=Palisade, 2=Stone Wall,
// 3=Bronze Wall). wall byggs upprepat; build-handlern väljer specen för wall_level+1.
var WallLevelSpecs = map[int]BuildingSpec{
	1: {Costs: map[string]float64{"timber": 50, "stone": 60}, Duration: 60 * time.Minute, WallsBonus: 1},
	2: {Costs: map[string]float64{"timber": 40, "stone": 160}, Duration: 120 * time.Minute, WallsBonus: 1},
	3: {Costs: map[string]float64{"stone": 100, "bronze": 10}, Duration: 180 * time.Minute, WallsBonus: 1},
}

// WallLevelNames är tier-namnen för klient-/hjälptext.
var WallLevelNames = map[int]string{1: "Palisade", 2: "Stone Wall", 3: "Bronze Wall"}
