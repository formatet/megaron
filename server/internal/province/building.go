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

// BuildingPurposes is a short human-readable ROLE line for each building —
// what it does, never what it PRODUCES or by how much (megaron_plan_byggnad_pa_hex.md
// §B): a hand-written good name or terrain list here can drift from the real
// production_rules/capacity-table data (farm was claiming "wine from hills
// and plains" while also quietly raising oil — a claim these lines no
// longer make). The numbers live in economy.HexBuildEffects
// (byggnadsregeln), exposed per hex via GET .../placement-options.
var BuildingPurposes = map[BuildingType]string{
	BuildingFarm:        "Stands on a grain hex and works its field",
	BuildingBarracks:    "Enables recruiting spearmen and war chariots",
	BuildingMine:        "Stands on an ore deposit and extracts what the hex holds",
	BuildingLumbermill:  "Stands on a forest hex and works its stands",
	BuildingStonequarry: "Stands on a land hex; its crew quarry the stone there — far more on hills and limestone",
	BuildingMarket:      "Enables trade offers and updates market price snapshots",
	BuildingWall:        "Adds a wall tier (Palisade → Stone Wall → Bronze Wall) for combat defence",
	BuildingHarbour:     "Works the settlement's coastal waters and enables sea trade",
	BuildingShipyard:    "Builds and repairs ships (requires coastal — adjacent sea hex)",
	BuildingFoundry:     "Smelts copper and tin into bronze",
	BuildingStable:      "Enables war chariots",
	BuildingTemple:      "Enables rites, produces cult, and unlocks oracle prayers",
	BuildingOlivePress:  "Refines a press-worker's oil from the settlement's groves",
	BuildingWinery:      "Refines a vintner's wine from the settlement's vines",
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
//
// ── Byggkostnaderna som RECEPT (megaron_ekonomi_underlag_kostnader §9–10,
// Timothy 2026-10-01; megaron_plan_byggkostnader) ──
//
// Materialet är det huset är gjort av: trä för farm, marknad, stall, hamn, varv
// och palissad; sten för stenbrott, gruva, kasern, tempel och stenmur. Golvet är 4
// av varje ingående material, och talen är hela tal. Tid står i §10:s kolumn.
// Talen är i dagsverken (en gubbe på standardterräng ger 1/tick), och en
// nivå 1-byggnad ska betala sig igen på ungefär 8–12 tick — det mäts i soak S5,
// inte här.
var BuildingSpecs = map[BuildingType]BuildingSpec{
	BuildingFarm:        {Costs: map[string]float64{"timber": 6, "stone": 4}, DurationTicks: 4},
	BuildingBarracks:    {Costs: map[string]float64{"timber": 8, "stone": 16}, DurationTicks: 10},
	BuildingMine:        {Costs: map[string]float64{"timber": 6, "stone": 16}, DurationTicks: 10},
	BuildingLumbermill:  {Costs: map[string]float64{"timber": 8, "stone": 4}, DurationTicks: 6},
	BuildingStonequarry: {Costs: map[string]float64{"timber": 6, "stone": 8}, DurationTicks: 6},
	BuildingMarket:      {Costs: map[string]float64{"timber": 10, "stone": 8}, DurationTicks: 8},
	BuildingWall:        {Costs: map[string]float64{"timber": 12, "stone": 4}, DurationTicks: 6, WallsBonus: 1},
	BuildingHarbour:     {Costs: map[string]float64{"timber": 16, "stone": 12}, DurationTicks: 12},
	BuildingShipyard:    {Costs: map[string]float64{"timber": 24, "stone": 12}, DurationTicks: 12},
	BuildingFoundry:     {Costs: map[string]float64{"timber": 8, "stone": 20}, DurationTicks: 16},
	BuildingStable:      {Costs: map[string]float64{"timber": 12, "stone": 8}, DurationTicks: 8},
	BuildingTemple:      {Costs: map[string]float64{"timber": 8, "stone": 24}, DurationTicks: 16},
	BuildingOlivePress:  {Costs: map[string]float64{"timber": 8, "stone": 8}, DurationTicks: 8},
	BuildingWinery:      {Costs: map[string]float64{"timber": 8, "stone": 8}, DurationTicks: 8},
}

// WallLevelSpecs ger kostnad/duration för nästa murnivå (1=Palisade, 2=Stone Wall,
// 3=Bronze Wall). wall byggs upprepat; build-handlern väljer specen för wall_level+1.
// Tre recept, ingen ×2/×4-trappa: palissaden är trä, stenmuren sten, bronsmuren
// sten med brons (§10).
var WallLevelSpecs = map[int]BuildingSpec{
	1: {Costs: map[string]float64{"timber": 12, "stone": 4}, DurationTicks: 6, WallsBonus: 1},
	2: {Costs: map[string]float64{"timber": 4, "stone": 24}, DurationTicks: 12, WallsBonus: 1},
	3: {Costs: map[string]float64{"stone": 48, "bronze": 8}, DurationTicks: 24, WallsBonus: 1},
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
	2: 4,
	3: 8,
}

// LevelBronzeCost är bronspåslaget per nivå för byggnaderna i LevelBronzeBuildings,
// på samma sätt som cedern. ⚠️ Kan inte nås idag: gjuteriet står inte i
// LevelledBuildings, och en nivå på gjuteriet ändrar dess arbetsplatskapacitet —
// en egen slice (megaron_plan_byggkostnader, "Det som INTE ingår"). Raderna finns
// så att §10 är fullständig den dag foundry görs nivåbar.
var LevelBronzeCost = map[int]float64{
	2: 4,
	3: 8,
}

// LevelBronzeBuildings är byggnader vars nivåtrappa kostar brons.
var LevelBronzeBuildings = map[BuildingType]bool{
	BuildingFoundry: true,
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
// Nivå 2+ skalar basmaterialet ×2 (nivå 2) och ×4 (nivå 3, §10) medan tiden
// följer nivån (×1/×2/×3). Byggnaderna i LevelCedarBuildings betalar
// dessutom ädelträ enligt LevelCedarCost.
//
// Materialtrappan infördes 2026-08-27 (S2) när cedern lyftes ur den generella
// progressionen: utan den hade nivå 2 och 3 kostat exakt samma material som nivå 1
// för nio av tolv nivåbyggnader, alltså ingen kostnadstrappa alls. Skalningen är
// läsbar i dagsverken — en farm nivå 3 kostar fyra gånger en farm nivå 1.
// levelMaterialMult är materialtrappan: nivå 2 = ×2, nivå 3 = ×4.
var levelMaterialMult = map[int]float64{1: 1, 2: 2, 3: 4}

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
		costs[k] = v * levelMaterialMult[level]
	}
	if cedar, hasCedar := LevelCedarCost[level]; hasCedar && LevelCedarBuildings[bt] {
		costs["cedar"] += cedar
	}
	if bronze, hasBronze := LevelBronzeCost[level]; hasBronze && LevelBronzeBuildings[bt] {
		costs["bronze"] += bronze
	}
	out := base
	out.Costs = costs
	out.DurationTicks = base.DurationTicks * level
	return out, true
}
