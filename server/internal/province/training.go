package province

// UnitSpec defines the cost and time to train a unit.
// All material costs are expressed as good_key → amount and deducted from
// settlement_goods. CostKharis is deducted from the settlements.kharis column.
// PopCost is NOT the number of citizens actually drafted — it is only a coarse
// afford-check threshold used by the can_recruit preview
// (api/handlers/province.go's afford := laborPool >= spec.PopCost, "does this
// city have at least this many citizens"). The real draft size is a fixed
// economy.MaxUnitSize (100) for land units — kohort-rekrytering,
// megaron_plan_rekryteringsmodell.md, was a caller-chosen 10–100-men batch
// before — or the type's fixed crew via unit.CrewFor for naval, debited
// straight from settlements.population at recruit time (province.go's
// Recruit handler).
// There is no hard population floor at recruit time beyond totalMen <
// population; draining a settlement to <=100 population schedules its
// collapse (C-collapse) instead. Corrected 2026-07-30 — this comment
// previously said "citizens consumed per unit trained (minimum population
// floor: 50)", which matches neither the afford-check role of this field nor
// current recruit/collapse behaviour.
//
// DurationTicks is likewise ONE training duration for the whole cohort, not a
// per-10-men batch: recruitBatchTicks (api/handlers/province.go) returns it
// unchanged and Recruit schedules a single ScheduledTrainComplete at
// trainCurrentTick + batchTicks once the unit reaches 100 men — matching
// internal/combat/train.go's "one scheduled TrainComplete (no per-10-men
// batches)". The surviving "per-10-men batch duration (looped)" comment at
// province.go's batchTicks declaration is stale and contradicts both.
type UnitSpec struct {
	Costs            map[string]float64 // good_key → quantity deducted from settlement_goods
	CostKharis       float64
	PopCost          int // can_recruit afford-check threshold — NOT the amount drafted (see doc comment above)
	DurationTicks    int // training time for the WHOLE cohort in world ticks — not a per-10-men batch (see doc comment above)
	RequiresBarracks bool
	RequiresStable   bool
	RequiresHarbour  bool
	RequiresShipyard bool
	RequiresFoundry  bool
}

// UnitSpecs is the canonical catalogue of all trainable unit types.
//
// Skepp-taxonomi (migration 039; units.type-nyckel bytt "ship"→"galley" av
// namn-hygien A, migration 084):
//   - "galley"      = standardskepp, byggs med timber, kräver varv.
//     DB:s legacy integer-armékolumn heter fortf. `ship` (SB7/C8 byter den).
//   - "war_galley"  = krigsgalär, elit. Kräver varv + gjuteri + brons.
//   - "merchantman" = handelsskepp, svag strid, byggs med timber, kräver varv.
//
// Varv, inte hamn (megaron_plan_skeppsreparation.md Slice A, §Beslut B1,
// 2026-08-08): skeppsbygge och -reparation flyttades till en egen `shipyard`-
// byggnad. Hamnen behåller fisket och sin roll som sjöhandelsnav.
//
// Enhetskorrektur (migration 042):
//   - "war_chariot" = stridsvagn, kräver stable + brons (men INTE foundry — en stad
//     som KÖPER brons ska kunna bygga den vid sitt stall). Ersätter "cavalry"/"chariot".
//     Katapulten (catapult) saknar historisk förankring i bronsåldern och tas bort.
// MaxSettlementsPerWanax caps how many active settlements a single Wanax may hold.
// Stops runaway colony-spam from drowning the MVP signal; tune as the metagame settles.
// Lives here (province pkg) so both the dispatch handler and the arrival handler can
// reference it without crossing the G1 dependency order.
const MaxSettlementsPerWanax = 5

// Costs below are PER-MAN (per crew member for naval), matching what
// api/handlers/province.go's Recruit handler actually deducts (Recruit
// multiplies Costs[good] by the number of men drafted). Before Fas 3
// (temenos_capabilities.md) these numbers disagreed with the handler's own
// recruitPerManCosts() — capabilities' recruit checker and the /status
// endpoint's can_recruit both read Costs but nothing enforced the two tables
// staying in sync. recruitPerManCosts now delegates to UnitSpecs[type].Costs
// so there is exactly one source (Fas 3 anti-drift).
// Materialkostnaderna omskalade med VARJE VARAS egen divisor
// (megaron_plan_dagsverkesskalan, mig 136, 2026-08-27): grain ÷43,2 ·
// timmer ÷216 · ceder ÷72. Brons och silver rörs inte (divisor 1 respektive
// valuta). Ren division — de inbördes förhållandena står exakt still.
//
// ── S4, dagsverkeskalibreringen (megaron_plan_dagsverkesskalan, 2026-08-27) ──
//
// Talen är PER MAN; multiplicera med kohortens 100 man eller med unit.CrewFor
// för ett skepp. Måltotaler för HELA enheten, i dagsverken (en gubbe på
// standardterräng producerar 1 enhet/tick sedan mig 136):
//
//	spjutkohort ...... 12      handelsskepp ..... 16
//	elitinfanteri .... 36 (varav 12 brons)      galär ............ 30  ← ankaret
//	stridsvagnar ..... 70 (varav 20 brons)      krigsgalär ....... 80
//
// Galären är Timothys ankare: "en galley kostar 30 timmer, vilket ska motsvara
// 30 arbetstick för en gubbe på optimal produktionsyta" (2026-08-27). 30 / 20
// besättningsmän = 1,5 timmer per man. Ett tick är en väggklockstimme, så
// galären är en natts arbete för tre-fyra huggare — en investering, inte ett
// klick, vilket den var på 0,83 dagsverken före denna kalibrering.
//
// ⚠️ BRONS ÄR EN KEDJA, INTE EN VARA. Receptet är 9 koppar + 1 tenn + ett
// gjuteritick, alltså 11 dagsverken per bronsenhet. Elitinfanteriets 12 brons
// bär därför ~132 dagsverken i verklig arbetsbörda, och stridsvagnarnas 20
// bär ~220. Talen nedan är AVSIKTLIGT inte uppräknade för det: spelarens order
// ska lyda "12 brons", och kedjebördan hör hemma i analysytor. Höj aldrig ett
// synligt bronspris för att kompensera kedjan — flaskhalsen är tennet, och den
// löses där den sitter.
//
// ⚠️ Silver rörs inte (mig 136 skalar inte valutan). Samtliga tal är KANDIDATER
// för soak-testet, inte lås.
var UnitSpecs = map[string]UnitSpec{
	"spearman":       {Costs: map[string]float64{"grain": 0.12, "silver": 0.2}, PopCost: 5, DurationTicks: 1, RequiresBarracks: true},
	"war_chariot":    {Costs: map[string]float64{"grain": 0.40, "timber": 0.05, "cedar": 0.05, "bronze": 0.20, "silver": 0.5}, PopCost: 8, DurationTicks: 3, RequiresStable: true},
	"galley":         {Costs: map[string]float64{"timber": 1.5, "silver": 0.3}, PopCost: 10, DurationTicks: 3, RequiresShipyard: true},
	"elite_infantry": {Costs: map[string]float64{"grain": 0.24, "bronze": 0.12, "silver": 0.4}, PopCost: 10, DurationTicks: 4, RequiresBarracks: true, RequiresFoundry: true},
	"war_galley":     {Costs: map[string]float64{"cedar": 1.6, "silver": 0.6}, PopCost: 12, DurationTicks: 5, RequiresShipyard: true, RequiresFoundry: true},
	"merchantman":    {Costs: map[string]float64{"timber": 1.6, "silver": 0.2}, PopCost: 8, DurationTicks: 4, RequiresShipyard: true},
}
