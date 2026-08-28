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
// ── S4, dagsverkeskalibreringen (megaron_plan_dagsverkesskalan, 2026-08-27) ──
//
// Talen är nu ANTAL DAGSVERKEN: en gubbe på varans standardterräng producerar
// 1 enhet per tick (mig 136), så summan av en byggnads kostnader är hur många
// gubbtick den kräver. Det är hela poängen med omskalningen — priserna går att
// resonera om utan att slå upp en produktionstabell.
//
// Måltotaler satta mot Timothys måttstock ("ett skepp eller en byggnad ska vara
// en investering; inte orimligt att behöva spara några verkliga dygn") med
// galären som ankare på 30 dagsverken. Ett tick är en väggklockstimme, så 30
// dagsverken är en natt för tre-fyra gubbar och drygt ett dygn för en ensam.
//
//	farm, stenbrott ......... 10      hamn, varv, tempel ..... 40
//	sågverk, press, vineri .. 24      gjuteri ................ 48
//	kasern, stall, gruva,             mur 1/2/3 ......... 20/48/90+brons
//	  silvergruva, marknad .. 30
//
// ⭐ Fördelningen MELLAN varor är oförändrad — varje byggnads gamla kvot mellan
// timmer och sten är bevarad och bara skalad till sin nya total. Det är ett
// medvetet minimalt designval: S4 sätter vad en sak KOSTAR, inte vad den byggs
// AV. Att sten dominerar nästan varje byggnad är alltså ärvt, inte nytt, och
// är en egen fråga (sten är också den vara en gubbe producerar minst av — 7,2
// per tick före omskalningen mot timrets 216).
//
// ⚠️ Samtliga tal är KANDIDATER för soak-testet (planens S5), inte lås. Mät dem
// mot en färsk värld — en befintlig värld bär 65 000 sten i arv och skulle inte
// känna av någon prisändring alls.
var BuildingSpecs = map[BuildingType]BuildingSpec{
	BuildingFarm:        {Costs: map[string]float64{"timber": 0.769, "stone": 9.231}, DurationTicks: 6},
	BuildingBarracks:    {Costs: map[string]float64{"timber": 0.968, "stone": 29.032}, DurationTicks: 16},
	BuildingMine:        {Costs: map[string]float64{"timber": 1.429, "stone": 28.571}, DurationTicks: 16},
	BuildingSilverMine:  {Costs: map[string]float64{"timber": 1.429, "stone": 28.571}, DurationTicks: 16},
	BuildingLumbermill:  {Costs: map[string]float64{"timber": 0.774, "stone": 23.226}, DurationTicks: 12},
	BuildingStonequarry: {Costs: map[string]float64{"timber": 0.769, "stone": 9.231}, DurationTicks: 6},
	BuildingMarket:      {Costs: map[string]float64{"timber": 1.579, "stone": 28.421}, DurationTicks: 16},
	BuildingWall:        {Costs: map[string]float64{"timber": 0.541, "stone": 19.459}, DurationTicks: 12, WallsBonus: 1},
	BuildingHarbour:     {Costs: map[string]float64{"timber": 2.887, "stone": 37.113}, DurationTicks: 24},
	// Strawman, same order of magnitude as the harbour it splits off from —
	// megaron_plan_skeppsreparation.md Slice A step 2 explicitly defers real
	// calibration (temenos_balans_spakar.md) rather than porting the
	// taxonomy's §9.2 gubbetick figures (500/300/25 cedar/180), which use a
	// labor-ticks build model this catalogue doesn't have.
	BuildingShipyard:   {Costs: map[string]float64{"timber": 2.887, "stone": 37.113}, DurationTicks: 24},
	BuildingFoundry:    {Costs: map[string]float64{"timber": 1.247, "stone": 46.753}, DurationTicks: 30},
	BuildingStable:     {Costs: map[string]float64{"timber": 1.429, "stone": 28.571}, DurationTicks: 16},
	BuildingTemple:     {Costs: map[string]float64{"timber": 1.290, "stone": 38.710}, DurationTicks: 24},
	BuildingOlivePress: {Costs: map[string]float64{"stone": 23.415, "timber": 0.585}, DurationTicks: 12},
	BuildingWinery:     {Costs: map[string]float64{"stone": 22.979, "timber": 1.021}, DurationTicks: 12},
}

// WallLevelSpecs ger kostnad/duration för nästa murnivå (1=Palisade, 2=Stone Wall,
// 3=Bronze Wall). wall byggs upprepat; build-handlern väljer specen för wall_level+1.
// Kalibrerade i dagsverken som BuildingSpecs (S4): 20 · 48 · 90 sten + 10 brons.
// Murtrappan är avsiktligt brantare än någon arbetsplats — palissaden är billig
// nog att resa tidigt, bronsmuren är ett projekt. De 10 bronsen bär dessutom sin
// egen kedja (9 koppar + 1 tenn + smältning per enhet), så bronsmuren kostar
// långt mer i verklig arbetsbörda än de 100 dagsverkena i sten antyder.
var WallLevelSpecs = map[int]BuildingSpec{
	1: {Costs: map[string]float64{"timber": 0.541, "stone": 19.459}, DurationTicks: 12, WallsBonus: 1},
	2: {Costs: map[string]float64{"timber": 0.397, "stone": 47.603}, DurationTicks: 24, WallsBonus: 1},
	3: {Costs: map[string]float64{"stone": 90, "bronze": 10}, DurationTicks: 48, WallsBonus: 1},
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
// ⚠️ Omskalad ÷72 (mig 136). Gäller sedan S2 (megaron_plan_dagsverkesskalan,
// 2026-08-27) ENDAST byggnaderna i LevelCedarBuildings — se den för varför.
var LevelCedarCost = map[int]float64{
	2: 0.347,
	3: 0.833,
}

// LevelCedarBuildings är de byggnader vars nivåtrappa kostar ädelträ.
//
// Före 2026-08-27 gällde LevelCedarCost varje nivåbyggnad, och det gjorde
// cedern till ett krav för all tillväxt. Mätningen samma dag visade vad det
// betydde i drift: ceder finns på 31 av världens 2 240 hexar och **noll av dem
// ligger i någon stads upptagningsområde**. Samtliga 70 byggnader i drift stod
// därför på nivå 1 sedan 2026-07-23 — inte spelarslöhet, utan en spärr ingen
// kunde passera. En jordbruksstad kunde aldrig få sin andra produktivitetsnivå.
//
// Knappheten flyttas, den upphävs inte: cedern är kvar lika sällsynt, men bär
// nu det maritima och monumentala i stället för den vardagliga
// jordbruksintensifieringen. Hamn och varv är fönstret mot havet; templet är
// palatsbygget. Krigsgalären kräver ceder redan i sin egen rekryteringspost
// (UnitSpecs), oberoende av den här mappen.
//
// Basbyggnaderna — farm, gruva, sågverk, stenbrott, marknad, stall, olivpress,
// vineri, silvergruva — betalar i stället sin nivåtrappa i timmer och sten,
// skalat med nivån (se LevelledSpec). Lokala material, nåbara för varje stad
// som har en hex att bruka.
var LevelCedarBuildings = map[BuildingType]bool{
	BuildingHarbour:  true,
	BuildingShipyard: true,
	BuildingTemple:   true,
}

// LevelledSpec returnerar kostnad/duration för att ta en byggnad till nivå `level`.
//
// Nivå 1 är oförändrad grundkostnad — nivåtrappan får aldrig fördyra grundbygget.
// Nivå 2+ skalar basmaterialet med nivån (nivå 2 kostar dubbelt, nivå 3 tredubbelt)
// och tar proportionellt längre tid. Byggnaderna i LevelCedarBuildings betalar
// dessutom ädelträ enligt LevelCedarCost.
//
// Materialtrappan infördes 2026-08-27 (S2) när cedern lyftes ur den generella
// progressionen: utan den hade nivå 2 och 3 kostat exakt samma material som nivå 1
// för nio av tolv nivåbyggnader, alltså ingen kostnadstrappa alls. Skalningen är
// läsbar i dagsverken — en farm nivå 3 kostar tre gånger en farm nivå 1.
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
		costs[k] = v * float64(level)
	}
	if cedar, hasCedar := LevelCedarCost[level]; hasCedar && LevelCedarBuildings[bt] {
		costs["cedar"] += cedar
	}
	out := base
	out.Costs = costs
	out.DurationTicks = base.DurationTicks * level
	return out, true
}
