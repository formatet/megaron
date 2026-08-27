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
	BuildingFarm:        "Raises grain and oil production from plains; wine from hills and plains",
	BuildingBarracks:    "Enables recruiting spearmen and war chariots",
	BuildingMine:        "Extracts copper or tin from ore deposits in catchment (requires deposit)",
	BuildingSilverMine:  "Extracts silver from silver deposits in catchment (requires deposit)",
	BuildingLumbermill:  "Increases cedar timber production from forest hexes",
	BuildingStonequarry: "Increases stone production from hills and mountain catchment",
	BuildingMarket:      "Enables trade offers and updates market price snapshots",
	BuildingWall:        "Adds a wall tier (Palisade → Stone Wall → Bronze Wall) for combat defence",
	BuildingHarbour:     "Enables fish production and efficient sea trade (requires coastal — adjacent sea hex)",
	BuildingShipyard:    "Builds and repairs ships (requires coastal — adjacent sea hex)",
	BuildingFoundry:     "Enables bronze smelting (copper + tin → bronze)",
	BuildingStable:      "Produces horses and enables war chariots",
	BuildingTemple:      "Enables rites, produces cult, and unlocks oracle prayers",
	BuildingOlivePress:  "Increases oil production from olive groves, plains and hills",
	BuildingWinery:      "Increases wine production from hills, plains and scrub",
}

// BuildingSpecs is the canonical catalogue of all constructable buildings.
// Rate bonuses for goods (grain, cedar, stone, etc.) are registered as
// production_rules rows and applied by BuildCompleteHandler via the UPSERT
// on settlement_goods — they are NOT in BuildingSpec.
// DurationTicks values are ticks — days in the world, since a tick IS a day
// (2026-08-06 canon): a farm takes 2, a barracks/mine/market/temple-tier
// building 3-4, a temple 4, an L3 wall 9 (WallLevelSpecs below). These are
// calibrated as a count of DAYS the build occupies, never against wall-clock
// minutes — the old "≤30 min→2, ≤60 min→3" framing described real-minute
// pacing at the (now-retired) 1-tick=1-hour cadence, which is exactly the
// day/tick conflation this canon change exists to remove.
// Materialkostnaderna omskalade med varje varas divisor
// (megaron_plan_dagsverkesskalan, mig 136, 2026-08-27): timmer ÷216 ·
// sten ÷7,2 · brons orört (divisor 1). Ren division, förhållandena står still.
//
// ⚠️ Läs dessa som DAGSVERKEN från och med nu — det är hela poängen med
// omskalningen. En farm kostar 0,23 dagsverken timmer och 2,78 sten, alltså
// ~3 dagsverken totalt. Ett varv 9,0. En galär 0,83. Timothys riktmärke är
// 30 dagsverken för galären, så hela katalogen ska sättas om — men i S4, inte
// här. Mig 136 byter enhet; den sätter inte priser.
var BuildingSpecs = map[BuildingType]BuildingSpec{
	BuildingFarm:        {Costs: map[string]float64{"timber": 0.2315, "stone": 2.778}, DurationTicks: 2},
	BuildingBarracks:    {Costs: map[string]float64{"timber": 0.3704, "stone": 11.111}, DurationTicks: 3},
	BuildingMine:        {Costs: map[string]float64{"timber": 0.2778, "stone": 5.556}, DurationTicks: 3},
	BuildingSilverMine:  {Costs: map[string]float64{"timber": 0.2778, "stone": 5.556}, DurationTicks: 3},
	BuildingLumbermill:  {Costs: map[string]float64{"timber": 0.1852, "stone": 5.556}, DurationTicks: 2},
	BuildingStonequarry: {Costs: map[string]float64{"timber": 0.2315, "stone": 2.778}, DurationTicks: 2},
	BuildingMarket:      {Costs: map[string]float64{"timber": 0.463, "stone": 8.333}, DurationTicks: 2},
	BuildingWall:        {Costs: map[string]float64{"timber": 0.2315, "stone": 8.333}, DurationTicks: 3, WallsBonus: 1},
	BuildingHarbour:     {Costs: map[string]float64{"timber": 0.6481, "stone": 8.333}, DurationTicks: 3},
	// Strawman, same order of magnitude as the harbour it splits off from —
	// megaron_plan_skeppsreparation.md Slice A step 2 explicitly defers real
	// calibration (temenos_balans_spakar.md) rather than porting the
	// taxonomy's §9.2 gubbetick figures (500/300/25 cedar/180), which use a
	// labor-ticks build model this catalogue doesn't have.
	BuildingShipyard:   {Costs: map[string]float64{"timber": 0.6481, "stone": 8.333}, DurationTicks: 3},
	BuildingFoundry:    {Costs: map[string]float64{"timber": 0.3704, "stone": 13.889}, DurationTicks: 4},
	BuildingStable:     {Costs: map[string]float64{"timber": 0.2778, "stone": 5.556}, DurationTicks: 3},
	BuildingTemple:     {Costs: map[string]float64{"timber": 0.2778, "stone": 8.333}, DurationTicks: 4},
	BuildingOlivePress: {Costs: map[string]float64{"stone": 5.556, "timber": 0.1389}, DurationTicks: 3},
	BuildingWinery:     {Costs: map[string]float64{"stone": 4.167, "timber": 0.1852}, DurationTicks: 3},
}

// WallLevelSpecs ger kostnad/duration för nästa murnivå (1=Palisade, 2=Stone Wall,
// 3=Bronze Wall). wall byggs upprepat; build-handlern väljer specen för wall_level+1.
// Omskalade som BuildingSpecs (mig 136): timmer ÷216, sten ÷7,2, brons orört.
var WallLevelSpecs = map[int]BuildingSpec{
	1: {Costs: map[string]float64{"timber": 0.2315, "stone": 8.333}, DurationTicks: 3, WallsBonus: 1},
	2: {Costs: map[string]float64{"timber": 0.1852, "stone": 22.222}, DurationTicks: 6, WallsBonus: 1},
	3: {Costs: map[string]float64{"stone": 13.889, "bronze": 10}, DurationTicks: 9, WallsBonus: 1},
}

// WallLevelNames är tier-namnen för klient-/hjälptext.
var WallLevelNames = map[int]string{1: "Palisade", 2: "Stone Wall", 3: "Bronze Wall"}

// MaxBuildingLevel är taket för varje nivåbyggnad (murar har sin egen trappa i
// WallLevelSpecs, samma tak). Kapaciteten mättas ändå mot hela stadens befolkning
// långt innan nivå 3 för de flesta varor — taket finns för att nivåtrappan ska ha
// ett slut, inte för att vara bindande.
const MaxBuildingLevel = 3

// LevelledBuildings är de byggnader som går att uppgradera bortom nivå 1. Det är
// varje byggnad som PRODUCERAR något: nivån är hur många medborgare arbetsplatsen
// kan sysselsätta (economy.LaborCapacity), så en nivå är enda sättet att viga mer
// av staden åt en vara. Templet hör hit fastän kult inte är en vara sedan mig 094 —
// dess nivå styr templeDevotionCapacity på exakt samma sätt, och innan detta gick
// det inte att höja (mekaniken Timothy byggde 2026-07-23 var därför inert: alla
// 189 byggnader i drift stod på nivå 1).
//
// DRIFT-GUARD: mängden speglar `SELECT DISTINCT building_type FROM production_rules
// WHERE building_type IS NOT NULL` + temple + shipyard. Lägger du en produktionsregel
// för en ny byggnad — lägg den här också, annars kan dess arbetsplats aldrig växa.
// Shipyard hör hit av samma skäl som temple: ingen production_rules-rad (den
// producerar ingen vara), men nivån styr ändå en arbetsplatskapacitet —
// economy.WorkplaceSlots("shipyard", level), 3/6/10 (Temenos_varutaxonomi_sol.md
// §8.2/§11.2) — för skeppsbygge och -reparation (megaron_plan_skeppsreparation.md).
var LevelledBuildings = map[BuildingType]bool{
	BuildingFarm:        true,
	BuildingHarbour:     true,
	BuildingShipyard:    true,
	BuildingLumbermill:  true,
	BuildingMarket:      true,
	BuildingMine:        true,
	BuildingSilverMine:  true,
	BuildingOlivePress:  true,
	BuildingStable:      true,
	BuildingStonequarry: true,
	BuildingWinery:      true,
	BuildingTemple:      true,
}

// LevelCedarCost är cedar-påslaget för att bygga en arbetsplats till nivå N.
// Cedar är den knappa ädelträvaran (deposit-gatead, 5 hex av 2 240) och bär därmed
// stadens tillväxt bortom det grundläggande: nivå 1 kostar som förut i timber+sten,
// men att bygga ut en arbetsplats kräver handel eller kolonisering efter cedar.
// STRAWMAN-kalibrering — siffrorna hör hemma i temenos_balans_spakar.md §8.
//
// ⚠️ Omskalad ÷72 (mig 136), MEN mätningen 2026-08-27 visade att den här spärren
// är hårdare än någon avsett: ceder finns på 31 av världens hexar och NOLL av
// dem ligger i någon stads upptagningsområde. Det är därför samtliga byggnader
// i drift står på nivå 1 sedan 2026-07-23 — inte spelarslöhet, utan en spärr
// ingen kan passera. Att frikoppla cedern från den generella progressionen och
// flytta den till maritim/monumental/militär specialisering är S2 i
// megaron_plan_dagsverkesskalan; den ändringen görs INTE här.
var LevelCedarCost = map[int]float64{
	2: 0.347,
	3: 0.833,
}

// LevelledSpec returnerar kostnad/duration för att ta en byggnad till nivå `level`.
// Nivå 1 är oförändrad grundkostnad; nivå 2+ lägger på cedar enligt LevelCedarCost
// och tar proportionellt längre tid. Returnerar false om byggnaden inte har någon
// nivåtrappa eller om nivån ligger över taket.
func LevelledSpec(bt BuildingType, level int) (BuildingSpec, bool) {
	base, ok := BuildingSpecs[bt]
	if !ok || level < 1 || level > MaxBuildingLevel {
		return BuildingSpec{}, false
	}
	if level == 1 {
		return base, true
	}
	if !LevelledBuildings[bt] {
		return BuildingSpec{}, false
	}
	// Kopiera kostnadsmappen — BuildingSpecs är en delad katalog och får aldrig muteras.
	costs := make(map[string]float64, len(base.Costs)+1)
	for k, v := range base.Costs {
		costs[k] = v
	}
	if cedar, hasCedar := LevelCedarCost[level]; hasCedar {
		costs["cedar"] += cedar
	}
	out := base
	out.Costs = costs
	out.DurationTicks = base.DurationTicks * level
	return out, true
}
