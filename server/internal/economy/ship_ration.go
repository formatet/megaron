package economy

const FieldGrainFactor = 2.0

// UpkeepSpec — grain + silver per upkeep-period för en full-size enhet.
type UpkeepSpec struct {
	Grain  float64
	Silver float64
}

// UpkeepSpecs: landenheter skalas med size/100; navala är flat (size=1).
// Präst = ingen upkeep (kult kostar inget löpande).
//
// Landenheternas grain (SLICE A, Timothy 2026-08-05): kalibrerad så att en
// garnisonerad full-size (100) enhet äter EXAKT vad en civil äter —
// pop×0.5/dag (economy/recompute.go:359) — dvs 50 korn/dygn för 100 man.
// Typvariationen är avsiktligt bevarad (elit äter mer, stridsvagnen har
// hästar). UnitUpkeep dubblar grain ovanpå detta i fält (marching/positioned).
//
// Navala talen är ORÖRDA av samma beslut: naval upkeep är per skrov, skalar
// inte med size alls, och "dubbelt så mycket som en medborgare" har därför
// ingen innebörd för en besättning på det sättet landenheternas har för en
// soldat. UnitUpkeep dubblar därför aldrig navalt grain, oavsett status.
//
// Ordinary spearmen live on grain, not silver (Timothy 2026-10-13).
// Professional troops and vessels keep their existing silver pay. Starting
// reserves and colonist purses are fixed independently of this table.
//
// Grain-kolumnen ÷100 (megaron_plan_dagsverkesskalan, mig 136, 2026-08-27) —
// SAMMA faktor som GrainConsumptionPerCitizenPerTick (0,5 → 0,005), inte
// grain-varans divisor 43,2. Skälet är en likhet som måste överleva
// omskalningen: en landkohort är 100 man och åt före exakt vad en gubbe
// (100 invånare) åt, 50 mot 50. Efter migrationen äter båda 0,50. Hade upkeep
// följt varudivisorn i stället hade soldaten plötsligt ätit dubbelt mot
// civilbefolkningen utan att någon beslutat det.
//
// Silver rörs INTE — det är en valuta och skalas inte i mig 136 (se
// migrationens huvudkommentar: kollapsmekaniken är en egen systemfråga).
var UpkeepSpecs = map[string]UpkeepSpec{
	"spearman":       {Grain: 0.50, Silver: 0},
	"elite_infantry": {Grain: 0.60, Silver: 2},
	"war_chariot":    {Grain: 0.80, Silver: 3},
	"galley":         {Grain: 0.04, Silver: 1.5},
	"war_galley":     {Grain: 0.06, Silver: 2.5},
	"merchantman":    {Grain: 0.03, Silver: 1},
}

// UnitUpkeep returns the grain + silver one unit costs per upkeep-period (the
// daily upkeep tick). Land units scale with size/100; naval and everything else
// are flat (per vessel); unknown types cost nothing. This is the single
// source of truth for the scaling — both the charging loop (Handle) and the army
// read surface (api/handlers) call it, so shown upkeep can never drift from what
// is actually debited.
//
// Kanon 2026-08-05: a soldier is a person. Garrisoned, he eats like a civilian
// (UpkeepSpecs' land grain figures are calibrated to that anchor — see
// economy/recompute.go:359, pop×0.5/day). In the field — status "marching" or
// "positioned" — he eats double: mobilising costs, and standing out costs more
// than standing home. Silver (where paid) never changes with status — the pay is the
// same wherever the man stands. Naval upkeep is per hull, not per person, and
// is untouched by status entirely (see UpkeepSpecs' comment).
func UnitUpkeep(unitType, category string, size int, status string) UpkeepSpec {
	spec, ok := UpkeepSpecs[unitType]
	if !ok {
		return UpkeepSpec{}
	}
	if category != "land" {
		return spec // naval/other: flat, status never changes it
	}
	f := float64(size) / 100.0
	grain := spec.Grain * f
	// "embarked" (SLICE, 2026-08-05): a cohort aboard a ship is maximally away
	// from the city's stores — field ration applies exactly like marching/
	// positioned. It is grouped WITH them here so this stays the single trigger
	// set for the doubling; Handle's own status filter below must list the same
	// three-plus-embarked set or a billed status stops being billed.
	if status == "marching" || status == "positioned" || status == "embarked" {
		grain *= FieldGrainFactor
	}
	return UpkeepSpec{Grain: grain, Silver: spec.Silver * f}
}

// VoyageRation är vad ett skepp äter per dygn till sjöss, inklusive en eventuell
// embarkerad kohort — alltså exakt det tal provianteringen ska täcka.
//
// Den bor HÄR, bredvid UnitUpkeep, med flit: provianten och dragningen måste
// räknas ur samma källa, annars provianterar man för ett tal och debiterar ett
// annat, och skillnaden syns först som ett skepp som svälter mitt i en resa det
// betalade fullt för.
//
// Kohorten ombord äter FÄLTRANSON (FieldGrainFactor): en embarkerad
// soldat är maximalt borta från stadens förråd, vilket är precis den motivering
// 'embarked' lades till i fältmängden för (2026-08-05, embarkerad ranson).
// cargoType/cargoSize är nollvärden när skeppet går tomt.
func VoyageRation(shipType string, shipSize int, cargoType string, cargoSize int) float64 {
	ration := UnitUpkeep(shipType, "naval", shipSize, "positioned").Grain
	if cargoType != "" && cargoSize > 0 {
		ration += UnitUpkeep(cargoType, "land", cargoSize, "embarked").Grain
	}
	return ration
}

// VoyageProvisions är vad som ska dras ur hemstaden vid avfärd:
// dygnsranson × (ut + station + hem).
//
// Hemresan approximeras som SYMMETRISK med utresan. Den exakta hemvägen är inte
// känd vid utskick (skeppet kan bli omdirigerat, och A* körs mot ett annat
// startläge), och de två felen är inte lika dyra: överproviantering kommer hem
// igen och lastas av, underproviantering strandar ett skepp. Approximationen
// ska därför alltid luta uppåt.
//
// stationTicks är 0 för en vanlig marsch och SentryPatrolTicks för en
// sjösentry, som per definition ligger stilla hela sin patrull.
func VoyageProvisions(ration float64, travelTicks, stationTicks int) float64 {
	if travelTicks < 1 {
		travelTicks = 1
	}
	if stationTicks < 0 {
		stationTicks = 0
	}
	return ration * float64(2*travelTicks+stationTicks)
}
