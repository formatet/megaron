package province

// UnitSpec defines the cost and time to train a unit.
// All material costs are expressed as good_key → amount and deducted from
// settlement_goods. CostKharis is deducted from the settlements.kharis column.
// PopCost is the number of citizens consumed per unit trained (minimum population floor: 50).
type UnitSpec struct {
	Costs            map[string]float64 // good_key → quantity deducted from settlement_goods
	CostKharis       float64
	PopCost          int // citizens consumed per unit trained
	DurationTicks    int // training time per batch-of-10 in world ticks
	RequiresBarracks bool
	RequiresStable   bool
	RequiresHarbour  bool
	RequiresFoundry  bool
}

// UnitSpecs is the canonical catalogue of all trainable unit types.
//
// Skepp-taxonomi (migration 039):
//   - "ship"        = galley — standardskepp, byggs med timber, kräver hamn.
//     DB-kolumn heter `ship` för bakåtkompatibilitet.
//   - "war_galley"  = krigsgalär, elit. Kräver hamn + gjuteri + brons.
//   - "merchantman" = handelsskepp, svag strid, byggs med timber, kräver hamn.
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
var UnitSpecs = map[string]UnitSpec{
	"spearman":       {Costs: map[string]float64{"grain": 3, "silver": 0.2}, PopCost: 5, DurationTicks: 1, RequiresBarracks: true},
	"war_chariot":    {Costs: map[string]float64{"grain": 3.75, "timber": 0.625, "bronze": 0.375, "silver": 0.5}, PopCost: 8, DurationTicks: 3, RequiresStable: true},
	// priest borttagen som enhet (mig 060) — präst är ingen enhet längre, kult = tempel-labor.
	"ship":           {Costs: map[string]float64{"timber": 9, "silver": 0.3}, PopCost: 10, DurationTicks: 3, RequiresHarbour: true},
	"elite_infantry": {Costs: map[string]float64{"grain": 2.5, "bronze": 0.2, "silver": 0.4}, PopCost: 10, DurationTicks: 4, RequiresBarracks: true, RequiresFoundry: true},
	"war_galley":     {Costs: map[string]float64{"cedar": 5, "bronze": 0.33, "silver": 0.6}, PopCost: 12, DurationTicks: 5, RequiresHarbour: true, RequiresFoundry: true},
	"merchantman":    {Costs: map[string]float64{"timber": 8.75, "silver": 0.2}, PopCost: 8, DurationTicks: 4, RequiresHarbour: true},
}
