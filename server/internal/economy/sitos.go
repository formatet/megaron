package economy

// DailyFoodNeed is the food a settlement's population eats per day, in food
// units. The population has ONE food need (FoodConsumptionSplit): grain covers
// it first, fish covers whatever grain does not reach. That is why coverage and
// the granary are denominated in food units rather than per-good — a city with
// mountains of fish and little grain is not starving, and a granary that only
// counted grain would think it was.
func DailyFoodNeed(population int) float64 {
	if population < 0 {
		population = 0
	}
	return float64(population) * GrainConsumptionPerCitizenPerTick
}

// CoverageDays is how many days the food on hand would feed the city. This is
// the granary's trigger (B1) — the quantity the old fund never measured. The
// fund compared a price to a moving average OF THAT SAME PRICE, which is a
// momentum detector: silent at every equilibrium, including a stable famine,
// and loud whenever the stock merely moved. Coverage is silent when the city is
// fed and loud when it is not, which is the actual question.
//
// A city with no people has no food need; its coverage is reported as 0 and the
// granary leaves it alone (EvaluateGranaryAction returns noop on need ≤ 0).
func CoverageDays(foodStock float64, population int) float64 {
	need := DailyFoodNeed(population)
	if need <= 0 {
		return 0
	}
	return foodStock / need
}

// GranaryCap is the most food a settlement's granary can hold, in food units.
func GranaryCap(population int, cfg SitosConfig) float64 {
	return DailyFoodNeed(population) * cfg.GranaryCapDays
}

// GranaryAction is the outcome of evaluating one settlement/tick for the
// granary. Quantity is a total in FOOD UNITS and is always ≥ 0; Kind
// disambiguates direction. Splitting the total across goods needs the caps and
// belongs to the caller — see sitos_tick.go.
type GranaryAction struct {
	Kind     string // "store" | "release" | "noop"
	Quantity float64
}

// EvaluateGranaryAction decides what the granary does this tick. Pure (no I/O)
// so the conservation math is unit-testable directly.
//
// FOOD IS STRICTLY CONSERVED. This function never returns a quantity that
// exceeds what the source side actually holds — store is bounded by the surplus
// AND by the granary's remaining capacity, release by the granary's contents —
// so no UPDATE downstream has to clip an amount after the other side has
// already been debited. That was the fund's original sin in reverse: it
// destroyed food on a buy and conjured it on a sell.
//
//   - Coverage above HighDays: the granary takes TithePct of the surplus ABOVE
//     that threshold. Taking only from above the threshold is what makes the
//     tithe self-limiting — a city that is merely comfortable is never taxed
//     into being short, and no extra rule is needed to guarantee it.
//   - Coverage below LowDays: the granary releases up to the shortfall, as far
//     as its contents reach. An empty granary releases nothing. That is the
//     only limit on famine relief (B2 — E1 struck).
//   - Otherwise: noop.
func EvaluateGranaryAction(foodStock, granaryTotal float64, population int, cfg SitosConfig) GranaryAction {
	need := DailyFoodNeed(population)
	if need <= 0 {
		return GranaryAction{Kind: "noop"}
	}
	coverage := foodStock / need

	switch {
	case coverage > cfg.HighDays:
		surplus := foodStock - cfg.HighDays*need
		q := cfg.TithePct * surplus
		if headroom := GranaryCap(population, cfg) - granaryTotal; q > headroom {
			q = headroom
		}
		if q <= 0 {
			return GranaryAction{Kind: "noop"}
		}
		return GranaryAction{Kind: "store", Quantity: q}

	case coverage < cfg.LowDays:
		q := cfg.LowDays*need - foodStock
		if q > granaryTotal {
			q = granaryTotal
		}
		if q <= 0 {
			return GranaryAction{Kind: "noop"}
		}
		return GranaryAction{Kind: "release", Quantity: q}

	default:
		return GranaryAction{Kind: "noop"}
	}
}
