package province

import "time"

// BuildingSpec defines the cost and effect of constructing a building.
type BuildingSpec struct {
	CostLumber float64
	CostStone  float64
	CostIron   float64
	CostKharis float64
	CostBronze float64
	Duration   time.Duration
	// Rate bonuses applied to the province when the building is complete (per level).
	FoodRate   float64
	LumberRate float64
	StoneRate  float64
	IronRate   float64
	GoldRate   float64
	KharisRate float64
	// WallsBonus is added to province.walls (only for wall/tower type buildings).
	WallsBonus int
}

// BuildingSpecs is the canonical catalogue of all constructable buildings.
var BuildingSpecs = map[BuildingType]BuildingSpec{
	BuildingFarm:        {CostLumber: 50, CostStone: 20, Duration: 30 * time.Minute, FoodRate: 2.0},
	BuildingBarracks:    {CostLumber: 80, CostStone: 50, CostIron: 30, Duration: 60 * time.Minute},
	BuildingMine:        {CostLumber: 60, CostStone: 40, Duration: 45 * time.Minute, IronRate: 1.5},
	BuildingLumbermill:  {CostLumber: 40, CostStone: 20, CostIron: 20, Duration: 30 * time.Minute, LumberRate: 2.0},
	BuildingStonequarry: {CostLumber: 50, CostIron: 20, Duration: 30 * time.Minute, StoneRate: 2.0},
	BuildingMarket:      {CostLumber: 100, CostStone: 60, Duration: 90 * time.Minute, GoldRate: 0.5},
	BuildingWall:        {CostLumber: 40, CostStone: 120, CostIron: 60, Duration: 120 * time.Minute, WallsBonus: 1},
	BuildingTower:       {CostLumber: 60, CostStone: 80, CostIron: 40, Duration: 90 * time.Minute, WallsBonus: 1},
	BuildingHarbour:     {CostLumber: 120, CostStone: 60, CostIron: 40, Duration: 90 * time.Minute, GoldRate: 0.3},
	BuildingFoundry:    {CostLumber: 80, CostStone: 60, CostIron: 50, Duration: 90 * time.Minute},
	BuildingStable:     {CostLumber: 60, CostStone: 40, Duration: 60 * time.Minute},
	BuildingBronzeWall: {CostStone: 80, CostIron: 40, CostBronze: 10, Duration: 180 * time.Minute, WallsBonus: 1},
}
