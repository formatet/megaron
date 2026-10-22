package economy

const FieldGrainFactor = 2.0

// UpkeepSpec — grain + silver per upkeep-period för en full-size enhet.
type UpkeepSpec struct {
	Grain  float64
	Silver float64
}

// UpkeepSpecs stores food only. UnitUpkeep derives pay from the shared wage owner.
// Naval food is derived from the spearman ration and actual crew in UnitUpkeep.
// Präst = ingen upkeep (kult kostar inget löpande).
//
// Landenheternas grain (SLICE A, Timothy 2026-08-05): kalibrerad så att en
// garnisonerad full-size (100) enhet äter EXAKT vad en civil äter —
// pop×0.5/dag (economy/recompute.go:359) — dvs 50 korn/dygn för 100 man.
// Typvariationen är avsiktligt bevarad (elit äter mer, stridsvagnen har
// hästar). UnitUpkeep dubblar grain ovanpå detta i fält (marching/positioned).
//
// Rowers eat like soldiers (Timothy 2026-10-22): actual crew,
// with field rations while marching, positioned or freighting.
//
// Ordinary spearmen live on grain, not silver (Timothy 2026-10-13).
// Professional troops and rowers earn the shared wage below. Starting
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
	"spearman":       {Grain: 0.50},
	"elite_infantry": {Grain: 0.60},
	"war_chariot":    {Grain: 0.80},
	"galley":         {},
	"war_galley":     {},
	"merchantman":    {},
}

// FieldPayPerHundred is one worker's wage for a full hundred people in the
// field. Type factors change the wage, not the food ration.
const FieldPayPerHundred = 1.0

var payFactors = map[string]float64{
	"spearman": 0, "elite_infantry": 1.5, "war_chariot": 2,
	"galley": 1, "war_galley": 1, "merchantman": 1,
}

// fieldUpkeep keeps food and wages on the same operational status map.
func fieldUpkeep(category, status string) bool {
	return status == "marching" || status == "positioned" || category == "naval" && status == "freighting" || category == "land" && status == "embarked"
}

func unitPay(unitType, category string, size, crew int, status string) float64 {
	if status == "laid_up" {
		return 0
	}
	people := max(0, size)
	if category == "naval" {
		people = max(0, crew)
	}
	pay := FieldPayPerHundred * payFactors[unitType] * float64(people) / 100
	if !fieldUpkeep(category, status) {
		pay *= .5
	}
	return pay
}

// UnitUpkeep returns the grain + silver one unit costs per upkeep-period (the
// daily upkeep tick). Land units scale with size/100; naval food scales with
// actual crew, including naval pay. Unknown types cost nothing. This is the single
// source of truth for the scaling — both the charging loop (Handle) and the army
// read surface (api/handlers) call it, so shown upkeep can never drift from what
// is actually debited.
//
// Kanon 2026-08-05: a soldier is a person. Garrisoned, he eats like a civilian
// (UpkeepSpecs' land grain figures are calibrated to that anchor — see
// economy/recompute.go:359, pop×0.5/day). In the field — status "marching" or
// "positioned" — he eats double: mobilising costs, and standing out costs more
// than standing home. Silver follows the wage owner: the
// wage is half at home and full in the field. Naval food uses the same soldier ration per
// rower; garrison/repairing/fitting_out are home, freighting is in the field.
// A laid-up ship has returned its crew to civilian population and is exempt
// from both ship rations and pay until Fit out drafts them again.
func UnitUpkeep(unitType, category string, size int, status string, crew int) UpkeepSpec {
	if status == "laid_up" {
		return UpkeepSpec{}
	}
	spec, ok := UpkeepSpecs[unitType]
	if !ok {
		return UpkeepSpec{}
	}
	if category == "naval" {
		grain := UpkeepSpecs["spearman"].Grain * float64(max(0, crew)) / 100
		if fieldUpkeep(category, status) {
			grain *= FieldGrainFactor
		}
		return UpkeepSpec{Grain: grain, Silver: unitPay(unitType, category, size, crew, status)}
	}
	if category != "land" {
		return spec
	}
	f := float64(size) / 100.0
	grain := spec.Grain * f
	// "embarked" (SLICE, 2026-08-05): a cohort aboard a ship is maximally away
	// from the city's stores — field ration applies exactly like marching/
	// positioned. It is grouped WITH them here so this stays the single trigger
	// set for the doubling; Handle's own status filter below must list the same
	// three-plus-embarked set or a billed status stops being billed.
	if fieldUpkeep(category, status) {
		grain *= FieldGrainFactor
	}
	return UpkeepSpec{Grain: grain, Silver: unitPay(unitType, category, size, crew, status)}
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
func VoyageRation(shipType string, shipSize int, cargoType string, cargoSize int, crew int) float64 {
	ration := UnitUpkeep(shipType, "naval", shipSize, "positioned", crew).Grain
	if cargoType != "" && cargoSize > 0 {
		ration += UnitUpkeep(cargoType, "land", cargoSize, "embarked", 0).Grain
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
