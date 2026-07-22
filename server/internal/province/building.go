package province

// BuildingSpec defines the cost and effect of constructing a building.
// All material costs are expressed as good_key → amount and deducted from
// settlement_goods. CostSilver is deducted from the settlement_goods silver row.
type BuildingSpec struct {
	Costs         map[string]float64 // good_key → quantity deducted from settlement_goods
	CostSilver    float64            // silver deducted from settlement_goods (good_key='silver')
	DurationTicks int                // build time in world ticks (1 tick = TICK_MINUTES real minutes)
	KharisRate    float64            // added to settlements.kharis_rate when complete
	WallsBonus    int                // added to settlements.wall_level (capped at 3)
}

// BuildingPurposes is a short human-readable description of each building's role,
// exposed via GET /api/v1/buildings and the CLI `build --list`.
var BuildingPurposes = map[BuildingType]string{
	BuildingFarm:        "Raises grain and oil production from plains; wine from hills",
	BuildingBarracks:    "Enables recruiting spearmen and war chariots",
	BuildingMine:        "Extracts copper or tin from ore deposits in catchment (requires deposit)",
	BuildingSilverMine:  "Extracts silver from silver deposits in catchment (requires deposit)",
	BuildingLumbermill:  "Increases cedar timber production from forest hexes",
	BuildingStonequarry: "Increases stone production from hills and mountain catchment",
	BuildingMarket:      "Enables trade offers and updates market price snapshots",
	BuildingWall:        "Adds a wall tier (Palisade → Stone Wall → Bronze Wall) for combat defence",
	BuildingHarbour:     "Enables ships and fish production (requires coastal — adjacent sea hex)",
	BuildingFoundry:     "Enables bronze smelting (copper + tin → bronze)",
	BuildingStable:      "Produces horses and enables war chariots",
	BuildingTemple:      "Enables rites, produces cult, and unlocks oracle prayers",
	BuildingOlivePress:  "Increases oil production from olive groves, plains and hills",
	BuildingWinery:      "Increases wine production from hills",
}

// BuildingSpecs is the canonical catalogue of all constructable buildings.
// Rate bonuses for goods (grain, cedar, stone, etc.) are registered as
// production_rules rows and applied by BuildCompleteHandler via the UPSERT
// on settlement_goods — they are NOT in BuildingSpec.
// DurationTicks values: ≤30 min→2, ≤60 min→3, ≤90 min→4, larger→5-6 (calibrated against 720-tick world).
var BuildingSpecs = map[BuildingType]BuildingSpec{
	BuildingFarm:        {Costs: map[string]float64{"timber": 50, "stone": 20}, DurationTicks: 2},
	BuildingBarracks:    {Costs: map[string]float64{"timber": 80, "stone": 80}, DurationTicks: 3},
	BuildingMine:        {Costs: map[string]float64{"timber": 60, "stone": 40}, DurationTicks: 3},
	BuildingSilverMine:  {Costs: map[string]float64{"timber": 60, "stone": 40}, DurationTicks: 3},
	BuildingLumbermill:  {Costs: map[string]float64{"timber": 40, "stone": 40}, DurationTicks: 2},
	BuildingStonequarry: {Costs: map[string]float64{"timber": 50, "stone": 20}, DurationTicks: 2},
	BuildingMarket:      {Costs: map[string]float64{"timber": 100, "stone": 60}, DurationTicks: 2},
	BuildingWall:        {Costs: map[string]float64{"timber": 50, "stone": 60}, DurationTicks: 3, WallsBonus: 1},
	BuildingHarbour:     {Costs: map[string]float64{"timber": 140, "stone": 60}, DurationTicks: 3},
	BuildingFoundry:     {Costs: map[string]float64{"timber": 80, "stone": 100}, DurationTicks: 4},
	BuildingStable:      {Costs: map[string]float64{"timber": 60, "stone": 40}, DurationTicks: 3},
	BuildingTemple:      {Costs: map[string]float64{"timber": 60, "stone": 60}, DurationTicks: 4},
	BuildingOlivePress:  {Costs: map[string]float64{"stone": 40, "timber": 30}, DurationTicks: 3},
	BuildingWinery:      {Costs: map[string]float64{"stone": 30, "timber": 40}, DurationTicks: 3},
}

// WallLevelSpecs ger kostnad/duration för nästa murnivå (1=Palisade, 2=Stone Wall,
// 3=Bronze Wall). wall byggs upprepat; build-handlern väljer specen för wall_level+1.
var WallLevelSpecs = map[int]BuildingSpec{
	1: {Costs: map[string]float64{"timber": 50, "stone": 60}, DurationTicks: 3, WallsBonus: 1},
	2: {Costs: map[string]float64{"timber": 40, "stone": 160}, DurationTicks: 6, WallsBonus: 1},
	3: {Costs: map[string]float64{"stone": 100, "bronze": 10}, DurationTicks: 9, WallsBonus: 1},
}

// WallLevelNames är tier-namnen för klient-/hjälptext.
var WallLevelNames = map[int]string{1: "Palisade", 2: "Stone Wall", 3: "Bronze Wall"}
