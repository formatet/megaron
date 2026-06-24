package province

import "time"

// UnitSpec defines the cost and time to train a unit.
// All material costs are expressed as good_key → amount and deducted from
// settlement_goods. CostKharis is deducted from the settlements.kharis column.
// PopCost is the number of citizens consumed per unit trained (minimum population floor: 50).
type UnitSpec struct {
	Costs            map[string]float64 // good_key → quantity deducted from settlement_goods
	CostKharis       float64
	PopCost          int           // citizens consumed per unit trained
	Duration         time.Duration // per unit trained
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

var UnitSpecs = map[string]UnitSpec{
	"spearman":       {Costs: map[string]float64{"grain": 15, "silver": 2}, PopCost: 5, Duration: time.Minute, RequiresBarracks: true},
	"war_chariot":    {Costs: map[string]float64{"grain": 30, "timber": 5, "bronze": 3, "silver": 5}, PopCost: 8, Duration: 4 * time.Minute, RequiresStable: true},
	// priest borttagen som enhet (mig 060) — präst är ingen enhet längre, kult = tempel-labor.
	"ship":           {Costs: map[string]float64{"timber": 90, "silver": 3}, PopCost: 10, Duration: 45 * time.Minute, RequiresHarbour: true},
	"elite_infantry": {Costs: map[string]float64{"grain": 25, "bronze": 2, "silver": 4}, PopCost: 10, Duration: 5 * time.Minute, RequiresBarracks: true, RequiresFoundry: true},
	"war_galley":     {Costs: map[string]float64{"cedar": 60, "bronze": 4, "silver": 6}, PopCost: 12, Duration: 90 * time.Minute, RequiresHarbour: true, RequiresFoundry: true},
	"merchantman":    {Costs: map[string]float64{"timber": 70, "silver": 2}, PopCost: 8, Duration: 60 * time.Minute, RequiresHarbour: true},
}
